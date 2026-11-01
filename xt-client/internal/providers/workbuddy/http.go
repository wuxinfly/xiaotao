package workbuddy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

func readBody(r io.Reader) []byte {
	raw, _ := io.ReadAll(io.LimitReader(r, 1<<20))
	return raw
}

func readBodyString(r io.Reader) string {
	return string(readBody(r))
}

func readAllBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, 1<<20))
}

type chatEvent struct {
	Choices []struct {
		Delta struct {
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			ToolCalls        json.RawMessage `json:"tool_calls"`
		} `json:"delta"`
		FinishReason any `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]any `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
		Details string `json:"details"`
	} `json:"error"`
}

func readSSE(ctx context.Context, r io.Reader, onEvent func(event string, data []byte) error) error {
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 16*1024)
	event := ""
	var data strings.Builder
	flush := func() error {
		if data.Len() == 0 {
			event = ""
			return nil
		}
		payload := data.String()
		data.Reset()
		ev := event
		event = ""
		return onEvent(ev, []byte(payload))
	}
	for {
		select {
		case <-ctx.Done():
			_ = flush()
			return ctx.Err()
		default:
		}
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				idx := bytes.IndexByte(buf, '\n')
				if idx < 0 {
					break
				}
				line := string(buf[:idx])
				buf = append(buf[:0], buf[idx+1:]...)
				if line == "" {
					if err := flush(); err != nil {
						return err
					}
					continue
				}
				if strings.HasPrefix(line, ":") {
					continue
				}
				name, value, found := strings.Cut(line, ":")
				if found && strings.HasPrefix(value, " ") {
					value = value[1:]
				}
				switch name {
				case "event":
					event = value
				case "data":
					if data.Len() > 0 {
						data.WriteString("\n")
					}
					data.WriteString(value)
				}
			}
		}
		if err != nil {
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func dispatchChatEvent(model string, event string, data []byte, ev *providers.ChatEvents) error {
	dsml := getDSMLBuffer(model)
	if event == "done" || strings.TrimSpace(string(data)) == "[DONE]" {
		if leftover := dsml.flush(); leftover != "" && ev.OnDelta != nil {
			ev.OnDelta(map[string]any{"content": leftover})
		}
		return nil
	}
	var chunk chatEvent
	if err := json.Unmarshal(data, &chunk); err != nil {
		return fmt.Errorf("invalid chat stream payload")
	}
	if chunk.Error != nil {
		status := 502
		if code, ok := chunk.Error.Code.(float64); ok && code >= 400 && code < 600 {
			status = int(code)
		}
		return &providers.UpstreamError{Status: status, Message: chunk.Error.Message}
	}
	hasOutput := false
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			cleaned, detected := dsml.addChunk(choice.Delta.Content)
			if cleaned != "" {
				if ev.OnDelta != nil {
					ev.OnDelta(map[string]any{"content": cleaned})
				}
				hasOutput = true
			}
			if len(detected) > 0 && !dsml.nativeToolCalls {
				if ev.OnDelta != nil {
					ev.OnDelta(map[string]any{"tool_calls": detected})
				}
			}
		}
		if choice.Delta.ReasoningContent != "" {
			if ev.OnDelta != nil {
				ev.OnDelta(map[string]any{"reasoning_content": choice.Delta.ReasoningContent})
			}
			hasOutput = true
		}
		if len(choice.Delta.ToolCalls) > 0 && string(choice.Delta.ToolCalls) != "null" {
			var toolCalls []map[string]any
			if err := json.Unmarshal(choice.Delta.ToolCalls, &toolCalls); err == nil && len(toolCalls) > 0 {
				dsml.nativeToolCalls = true
				if ev.OnDelta != nil {
					ev.OnDelta(map[string]any{"tool_calls": toolCalls})
				}
				hasOutput = true
			}
		}
	}
	if hasOutput && ev.OnFirstToken != nil {
		ev.OnFirstToken()
	}
	if chunk.Usage != nil {
		normalizeUsage(chunk.Usage)
		if ev.OnUsage != nil {
			ev.OnUsage(chunk.Usage)
		}
	}
	return nil
}

func normalizeUsage(usage map[string]any) {
	for key, value := range usage {
		if n, ok := value.(float64); ok {
			usage[key] = int64(n)
		}
	}
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if cached, ok := details["cached_tokens"].(float64); ok {
			usage["cached_tokens"] = int64(cached)
		}
	}
}

func collectUpstream(model string, payload []byte) (map[string]any, error) {
	content := strings.Builder{}
	reason := strings.Builder{}
	tools := map[int]map[string]string{}
	finish := ""
	var usage map[string]any
	dsml := &dsmlBuffer{}
	err := readSSE(context.Background(), bytes.NewReader(payload), func(event string, data []byte) error {
		if event == "done" || strings.TrimSpace(string(data)) == "[DONE]" {
			return nil
		}
		var chunk chatEvent
		if err := json.Unmarshal(data, &chunk); err != nil {
			return fmt.Errorf("invalid chat stream payload")
		}
		if chunk.Error != nil {
			status := 502
			if code, ok := chunk.Error.Code.(float64); ok && code >= 400 && code < 600 {
				status = int(code)
			}
			return &providers.UpstreamError{Status: status, Message: chunk.Error.Message}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				cleaned, detected := dsml.addChunk(choice.Delta.Content)
				content.WriteString(cleaned)
				if len(detected) > 0 && !dsml.nativeToolCalls {
					for _, call := range detected {
						fn, _ := call["function"].(map[string]any)
						name, _ := fn["name"].(string)
						args, _ := fn["arguments"].(string)
						id, _ := call["id"].(string)
						mergeToolCalls(tools, []map[string]any{{
							"index": float64(len(tools)), "id": id,
							"function": map[string]any{"name": name, "arguments": args},
						}})
					}
				}
			}
			if choice.Delta.ReasoningContent != "" {
				reason.WriteString(choice.Delta.ReasoningContent)
			}
			if len(choice.Delta.ToolCalls) > 0 && string(choice.Delta.ToolCalls) != "null" {
				var toolCalls []map[string]any
				if err := json.Unmarshal(choice.Delta.ToolCalls, &toolCalls); err == nil {
					mergeToolCalls(tools, toolCalls)
				}
			}
			if text, ok := choice.FinishReason.(string); ok && text != "" {
				finish = text
			}
		}
		if chunk.Usage != nil {
			normalizeUsage(chunk.Usage)
			usage = chunk.Usage
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if leftover := dsml.flush(); leftover != "" {
		content.WriteString(leftover)
	}
	toolCalls := []map[string]any{}
	for _, idx := range sortedToolKeys(tools) {
		call := tools[idx]
		toolCalls = append(toolCalls, map[string]any{
			"index": idx, "id": call["id"], "type": "function",
			"function": map[string]any{"name": call["name"], "arguments": call["arguments"]},
		})
	}
	if len(toolCalls) > 0 {
		finish = "tool_calls"
	}
	message := map[string]any{"role": "assistant", "content": content.String()}
	if reason.Len() > 0 {
		message["reasoning_content"] = reason.String()
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	return map[string]any{
		"id":      "chatcmpl-" + randHex(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": orStop(finish)}},
		"usage":   usage,
	}, nil
}

func mergeToolCalls(calls map[int]map[string]string, items []map[string]any) {
	for _, item := range items {
		idx := 0
		if value, ok := item["index"].(float64); ok {
			idx = int(value)
		}
		call, ok := calls[idx]
		if !ok {
			call = map[string]string{"id": "", "name": "", "arguments": ""}
			calls[idx] = call
		}
		if id, ok := item["id"].(string); ok && id != "" {
			call["id"] = id
		}
		if fn, ok := item["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				call["name"] = name
			}
			if args, ok := fn["arguments"].(string); ok {
				call["arguments"] += args
			}
		}
	}
}

func sortedToolKeys(calls map[int]map[string]string) []int {
	keys := make([]int, 0, len(calls))
	for key := range calls {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func orStop(value string) string {
	if value == "" {
		return "stop"
	}
	return value
}

func randHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "wb"
	}
	return hex.EncodeToString(b)
}

func postChat(ctx context.Context, baseURL string, cred *providers.Credential, body map[string]any) ([]byte, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+chatPath, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for k, v := range headersFor(cred) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, statusError(resp.StatusCode, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(readBodyString(resp.Body), 200)))
	}
	return readAllBody(resp.Body)
}

func streamChat(ctx context.Context, baseURL string, cred *providers.Credential, body map[string]any) (*http.Response, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+chatPath, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for k, v := range headersFor(cred) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, statusError(resp.StatusCode, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(readBodyString(resp.Body), 200)))
	}
	return resp, nil
}
