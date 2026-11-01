package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
)

type Collector struct{}

type Streamer struct{}

func (Collector) Collect(ctx context.Context, cred *providers.Credential, model string, body map[string]any) (map[string]any, error) {
	upstreamModel, err := requestModel(cred, model, stringFromAny(body["reasoning_effort"]))
	if err != nil {
		return nil, &providers.UpstreamError{Status: 400, Message: err.Error()}
	}
	cache := NewSignatureCache()
	envelope := buildRequest(body, cred.ProjectID, upstreamModel, cache)
	payload, err := upstreamRequest(ctx, cred.AccessToken, "generateContent", envelope, false)
	if err != nil {
		return nil, err
	}
	return responseToChat(payload, model)
}

func (Streamer) Stream(ctx context.Context, cred *providers.Credential, model string, body map[string]any, ev *providers.ChatEvents) error {
	upstreamModel, err := requestModel(cred, model, stringFromAny(body["reasoning_effort"]))
	if err != nil {
		return &providers.UpstreamError{Status: 400, Message: err.Error()}
	}
	cache := NewSignatureCache()
	envelope := buildRequest(body, cred.ProjectID, upstreamModel, cache)
	resp, err := upstreamStream(ctx, cred.AccessToken, "streamGenerateContent", envelope)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(newIdleTimeoutReader(resp.Body, streamIdleTimeout))
	firstToken := false
	var usageMu sync.Mutex
	var usage map[string]any
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(data), &payload); err == nil {
				if errText, code, ok := inBandError(payload); ok {
					return &providers.UpstreamError{Status: code, Message: errText}
				}
				if chunk := emitChatChunk(payload, model, &firstToken, &usageMu, &usage, ev); chunk != nil {
					_ = chunk
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
	if usageMu.TryLock() {
		defer usageMu.Unlock()
		if usage != nil && ev.OnUsage != nil {
			ev.OnUsage(usage)
		}
	}
	return nil
}

type idleTimeoutReader struct {
	r        io.Reader
	timeout  time.Duration
	lastData time.Time
}

func newIdleTimeoutReader(r io.Reader, timeout time.Duration) *idleTimeoutReader {
	return &idleTimeoutReader{r: r, timeout: timeout, lastData: time.Now()}
}

func (i *idleTimeoutReader) Read(p []byte) (int, error) {
	if time.Since(i.lastData) > i.timeout {
		return 0, context.DeadlineExceeded
	}
	n, err := i.r.Read(p)
	if n > 0 {
		i.lastData = time.Now()
	}
	return n, err
}

func upstreamRequest(ctx context.Context, token, method string, body map[string]any, _ bool) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, base := range endpoints {
		url := base + "/v1internal:" + method
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		for k, v := range metadataHeaders(token) {
			req.Header.Set(k, v)
		}
		client := newHTTPClient(connectTimeout + nonStreamTimeout)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := readAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = &providers.UpstreamError{Status: resp.StatusCode, Message: truncate(string(data), 300)}
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			lastErr = fmt.Errorf("invalid upstream JSON")
			continue
		}
		return payload, nil
	}
	return nil, lastErr
}

func upstreamStream(ctx context.Context, token, method string, body map[string]any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, base := range endpoints {
		url := base + "/v1internal:" + method + "?alt=sse"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		for k, v := range metadataHeaders(token) {
			req.Header.Set(k, v)
		}
		req.Header.Set("Accept", "text/event-stream")
		client := newHTTPClient(connectTimeout + streamIdleTimeout)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 400 {
			data, _ := readAll(resp.Body)
			resp.Body.Close()
			lastErr = &providers.UpstreamError{Status: resp.StatusCode, Message: truncate(string(data), 300)}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// emitChatChunk converts a single Gemini SSE payload into OpenAI deltas.
func emitChatChunk(payload map[string]any, model string, firstToken *bool, usageMu *sync.Mutex, usage *map[string]any, ev *providers.ChatEvents) map[string]any {
	inner := payload
	if value, ok := payload["response"].(map[string]any); ok {
		inner = value
	}
	if candidates, ok := inner["candidates"].([]any); ok {
		for _, item := range candidates {
			cand, ok := item.(map[string]any)
			if !ok {
				continue
			}
			content, _ := cand["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, p := range parts {
				part, ok := p.(map[string]any)
				if !ok {
					continue
				}
				if isThought, _ := part["thought"].(bool); isThought {
					if text, ok := part["text"].(string); ok && text != "" && ev.OnDelta != nil {
						ev.OnDelta(map[string]any{"reasoning_content": text})
						markFirstToken(firstToken, ev)
					}
					continue
				}
				if call, ok := part["functionCall"].(map[string]any); ok {
					name, _ := call["name"].(string)
					callID := "call_" + randomHex(12)
					signature, _ := part["thoughtSignature"].(string)
					NewSignatureCache().Remember(callID, signature, name)
					args, _ := call["args"].(map[string]any)
					raw, _ := json.Marshal(args)
					delta := map[string]any{
						"tool_calls": []any{map[string]any{
							"id": callID, "type": "function", "function": map[string]any{
								"name": name, "arguments": string(raw),
							},
							"gemini_thought_signature": signature,
						}},
					}
					if ev.OnDelta != nil {
						ev.OnDelta(delta)
						markFirstToken(firstToken, ev)
					}
					continue
				}
				if text, ok := part["text"].(string); ok && text != "" && ev.OnDelta != nil {
					ev.OnDelta(map[string]any{"content": text})
					markFirstToken(firstToken, ev)
				}
			}
		}
	}
	if meta, ok := inner["usageMetadata"].(map[string]any); ok {
		usageMu.Lock()
		defer usageMu.Unlock()
		*usage = usageFromMetadata(meta)
	}
	return nil
}

func markFirstToken(first *bool, ev *providers.ChatEvents) {
	if *first {
		return
	}
	*first = true
	if ev.OnFirstToken != nil {
		ev.OnFirstToken()
	}
}

func inBandError(payload map[string]any) (string, int, bool) {
	inner := payload
	if value, ok := payload["response"].(map[string]any); ok {
		inner = value
	}
	if err, ok := inner["error"].(map[string]any); ok {
		code := 502
		if n, ok := err["code"].(float64); ok && n >= 400 && n < 600 {
			code = int(n)
		}
		msg, _ := err["message"].(string)
		return msg, code, true
	}
	return "", 0, false
}

func responseToChat(payload map[string]any, model string) (map[string]any, error) {
	inner := payload
	if value, ok := payload["response"].(map[string]any); ok {
		inner = value
	}
	candidates, _ := inner["candidates"].([]any)
	message := map[string]any{"role": "assistant"}
	finish := "stop"
	content := strings.Builder{}
	reasoning := strings.Builder{}
	var toolCalls []map[string]any
	cache := NewSignatureCache()
	if len(candidates) > 0 {
		cand, _ := candidates[0].(map[string]any)
		contentMap, _ := cand["content"].(map[string]any)
		parts, _ := contentMap["parts"].([]any)
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if isThought, _ := part["thought"].(bool); isThought {
				if text, ok := part["text"].(string); ok {
					reasoning.WriteString(text)
				}
				continue
			}
			if call, ok := part["functionCall"].(map[string]any); ok {
				name, _ := call["name"].(string)
				callID := "call_" + randomHex(12)
				signature, _ := part["thoughtSignature"].(string)
				cache.Remember(callID, signature, name)
				args, _ := call["args"].(map[string]any)
				raw, _ := json.Marshal(args)
				toolCalls = append(toolCalls, map[string]any{
					"id": callID, "type": "function",
					"function":                 map[string]any{"name": name, "arguments": string(raw)},
					"gemini_thought_signature": signature,
				})
				finish = "tool_calls"
				continue
			}
			if text, ok := part["text"].(string); ok {
				content.WriteString(text)
			}
		}
		if raw, ok := cand["finishReason"].(string); ok {
			switch raw {
			case "MAX_TOKENS":
				finish = "length"
			case "SAFETY", "RECITATION":
				finish = "content_filter"
			case "STOP":
				if len(toolCalls) > 0 {
					finish = "tool_calls"
				} else {
					finish = "stop"
				}
			}
		}
	}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	message["content"] = content.String()
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	var usage map[string]any
	if meta, ok := inner["usageMetadata"].(map[string]any); ok {
		usage = usageFromMetadata(meta)
	}
	return map[string]any{
		"id":      "chatcmpl-" + randomHex(12),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   usage,
	}, nil
}

func usageFromMetadata(meta map[string]any) map[string]any {
	prompt := intFromAny(meta["promptTokenCount"])
	completion := intFromAny(meta["candidatesTokenCount"])
	total := intFromAny(meta["totalTokenCount"])
	cached := intFromAny(meta["cachedContentTokenCount"])
	out := map[string]any{
		"prompt_tokens":     prompt,
		"completion_tokens": completion,
		"total_tokens":      total,
		"prompt_tokens_details": map[string]any{
			"cached_tokens": cached,
		},
	}
	return out
}

func intFromAny(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case json.Number:
		if n, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func stringFromAny(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
