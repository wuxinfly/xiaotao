package trae

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

type ChatResult struct {
	Content string
	Reason  string
	Tools   []map[string]any
	Usage   map[string]any
	ErrCode any
	ErrMsg  string
	EOF     bool
	FirstTokenAt *time.Time
}

func streamChat(ctx context.Context, cred *providers.Credential, machineID, deviceID, model string, body map[string]any, onEvent func(event string, data map[string]any)) error {
	r := RegionOf(cred.Region)
	body["model"] = model
	body["config_name"] = model
	body["stream"] = true
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ChatBase+chatEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	for k, v := range headers(cred, machineID, deviceID) {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("trae chat http %d: %s", resp.StatusCode, redact(string(b)))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var event string
	var data strings.Builder
	var seenDone bool
	var seenError bool
	flush := func() {
		if data.Len() == 0 {
			event = ""
			return
		}
		raw := data.String()
		data.Reset()
		ev := event
		event = ""
		var obj map[string]any
		if json.Unmarshal([]byte(raw), &obj) != nil || obj == nil {
			obj = map[string]any{"raw": raw}
		}
		onEvent(ev, obj)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
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
			if value == "done" {
				seenDone = true
			} else if value == "error" {
				seenError = true
			}
		case "data":
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(value)
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}
	if !seenDone && !seenError {
		return fmt.Errorf("stream disconnected unexpectedly: missing completion event")
	}
	return nil
}

func collectChat(ctx context.Context, cred *providers.Credential, machineID, deviceID, model string, body map[string]any) (*ChatResult, error) {
	result := &ChatResult{Tools: []map[string]any{}}
	calls := map[int]map[string]string{}
	err := streamChat(ctx, cred, machineID, deviceID, model, body, func(ev string, data map[string]any) {
		switch ev {
		case "error":
			result.ErrCode = data["code"]
			result.ErrMsg = errText(data)
		case "output":
			hasOutput := false
			if v, ok := data["reasoning_content"].(string); ok {
				result.Reason += v
				if v != "" {
					hasOutput = true
				}
			}
			if v, ok := data["response"].(string); ok {
				result.Content += v
				if v != "" {
					hasOutput = true
				}
			}
			if items, ok := data["tool_calls"].([]any); ok {
				mergeToolCalls(calls, items)
				if len(items) > 0 {
					hasOutput = true
				}
			}
			if hasOutput && result.FirstTokenAt == nil {
				now := time.Now()
				result.FirstTokenAt = &now
			}
		case "token_usage":
			usage := map[string]any{}
			for _, k := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "cached_tokens", "prompt_cache_hit_tokens", "cache_tokens"} {
				if v, ok := data[k].(float64); ok {
					usage[k] = int64(v)
				}
			}
			if details, ok := data["prompt_tokens_details"].(map[string]any); ok {
				if v, ok := details["cached_tokens"].(float64); ok {
					usage["cached_tokens"] = int64(v)
				}
			}
			if p, ok := usage["prompt_cache_hit_tokens"]; ok && usage["cached_tokens"] == nil {
				usage["cached_tokens"] = p
			}
			if c, ok := usage["cache_tokens"]; ok && usage["cached_tokens"] == nil {
				usage["cached_tokens"] = c
			}
			result.Usage = usage
		case "done":
			result.EOF = true
		case "notify_usage":
			if remain, ok := data["remain_usage"]; ok {
				if result.Usage == nil {
					result.Usage = map[string]any{}
				}
				result.Usage["remain_usage"] = remain
			}
		}
	})
	if err != nil {
		return nil, err
	}
	for _, idx := range sortedKeys(calls) {
		c := calls[idx]
		result.Tools = append(result.Tools, map[string]any{
			"index": idx, "id": c["id"], "type": "function",
			"function": map[string]any{"name": c["name"], "arguments": c["arguments"]},
		})
	}
	return result, nil
}

// StreamEvents carries incremental upstream output. Tool call deltas are
// pre-merged per index so consumers receive stable argument fragments.
type StreamEvents struct {
	OnFirstToken func(t time.Time)
	OnDelta func(delta map[string]any)
	OnError func(code any, msg string)
	OnUsage func(usage map[string]any)
	OnDone  func()
}

// StreamChat runs the native chat channel and pushes each upstream
// increment through the StreamEvents callbacks as it arrives.
func StreamChat(ctx context.Context, cred *providers.Credential, machineID, deviceID, model string, body map[string]any, ev *StreamEvents) error {
	calls := map[int]map[string]string{}
	toolStarted := map[int]bool{}
	toolEmitted := map[int]int{}
	var firstTokenFired bool
	err := streamChat(ctx, cred, machineID, deviceID, model, body, func(event string, data map[string]any) {
		switch event {
		case "error":
			if ev.OnError != nil {
				ev.OnError(data["code"], errText(data))
			}
		case "output":
			hasOutput := false
			if v, ok := data["reasoning_content"].(string); ok && v != "" {
				hasOutput = true
			}
			if v, ok := data["response"].(string); ok && v != "" {
				hasOutput = true
			}
			if items, ok := data["tool_calls"].([]any); ok && len(items) > 0 {
				hasOutput = true
			}
			if hasOutput && !firstTokenFired {
				firstTokenFired = true
				if ev.OnFirstToken != nil {
					ev.OnFirstToken(time.Now())
				}
			}
			if ev.OnDelta == nil {
				return
			}
			if v, ok := data["reasoning_content"].(string); ok && v != "" {
				ev.OnDelta(map[string]any{"reasoning_content": v})
			}
			if v, ok := data["response"].(string); ok && v != "" {
				ev.OnDelta(map[string]any{"content": v})
			}
			if items, ok := data["tool_calls"].([]any); ok {
				mergeToolCalls(calls, items)
				for _, idx := range sortedKeys(calls) {
					c := calls[idx]
					args := c["arguments"]
					if !toolStarted[idx] {
						toolStarted[idx] = true
						toolEmitted[idx] = len(args)
						ev.OnDelta(map[string]any{"tool_calls": []any{map[string]any{
							"index": idx, "id": c["id"], "type": "function",
							"function": map[string]any{"name": c["name"], "arguments": args},
						}}})
					} else if len(args) > toolEmitted[idx] {
						fragment := args[toolEmitted[idx]:]
						toolEmitted[idx] = len(args)
						ev.OnDelta(map[string]any{"tool_calls": []any{map[string]any{
							"index":    idx,
							"function": map[string]any{"arguments": fragment},
						}}})
					}
				}
			}
		case "token_usage":
			usage := map[string]any{}
			for _, k := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "cached_tokens", "prompt_cache_hit_tokens", "cache_tokens"} {
				if v, ok := data[k].(float64); ok {
					usage[k] = int64(v)
				}
			}
			if details, ok := data["prompt_tokens_details"].(map[string]any); ok {
				if v, ok := details["cached_tokens"].(float64); ok {
					usage["cached_tokens"] = int64(v)
				}
			}
			if p, ok := usage["prompt_cache_hit_tokens"]; ok && usage["cached_tokens"] == nil {
				usage["cached_tokens"] = p
			}
			if c, ok := usage["cache_tokens"]; ok && usage["cached_tokens"] == nil {
				usage["cached_tokens"] = c
			}
			if ev.OnUsage != nil && len(usage) > 0 {
				ev.OnUsage(usage)
			}
		case "done":
			if ev.OnDone != nil {
				ev.OnDone()
			}
		case "notify_usage":
			if remain, ok := data["remain_usage"]; ok {
				if ev.OnUsage != nil {
					ev.OnUsage(map[string]any{"remain_usage": remain})
				}
			}
		}
	})
	return err
}
