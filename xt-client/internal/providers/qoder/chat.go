package qoder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"xiaotao/xt-client/internal/providers"
)

type ChatClient struct {
	Region                    Region
	HTTP                      *http.Client
	IdleTimeout, TotalTimeout time.Duration
}
type ChatEvent struct {
	Delta  map[string]any
	Usage  map[string]any
	Finish string
}

var chatTransport = &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 300 * time.Second}

func buildChatBody(model providers.ModelInfo, body map[string]any) map[string]any {
	out := map[string]any{"model": model.UpstreamID, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	for _, key := range []string{"messages", "tools", "tool_choice", "temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "reasoning_effort", "presence_penalty", "frequency_penalty", "response_format", "seed", "user", "parallel_tool_calls", "system"} {
		if value, ok := body[key]; ok {
			out[key] = value
		}
	}
	raw, _ := json.Marshal(body["messages"])
	var messages []map[string]any
	json.Unmarshal(raw, &messages)
	prompt := ""
	for _, m := range messages {
		if m["tool_calls"] != nil && m["content"] == nil {
			m["content"] = ""
		}
		if m["role"] == "developer" {
			m["role"] = "system"
			delete(m, "tool_calls")
		}
		if m["role"] == "user" {
			if text, ok := m["content"].(string); ok {
				prompt = text
			} else if parts, ok := m["content"].([]any); ok {
				for _, part := range parts {
					if p, ok := part.(map[string]any); ok && p["type"] == "text" {
						if text, ok := p["text"].(string); ok {
							prompt = text
						}
					}
				}
			}
		}
	}
	out["messages"] = messages
	request, set, session := newUUIDString(), newUUIDString(), newUUIDString()
	out["request_id"], out["request_set_id"], out["chat_record_id"], out["session_id"] = request, set, request, session
	out["chat_task"], out["is_reply"], out["is_retry"], out["source"], out["version"] = "FREE_INPUT", true, false, 1, "3"
	out["agent_id"], out["task_id"], out["session_type"], out["aliyun_user_type"] = "agent_common", "common", "qoderclicn", ""
	out["chat_context"] = map[string]any{"text": prompt, "features": []any{}, "extra": map[string]any{"context": []any{}, "modelConfig": map[string]any{"key": model.UpstreamID, "is_reasoning": model.Reasoning}, "originalContent": prompt}, "chatPrompt": "", "imageUrls": nil}
	maxInput := model.MaxInputTokens
	if maxInput == 0 {
		maxInput = 128000
	}
	out["model_config"] = map[string]any{"key": model.UpstreamID, "display_name": model.DisplayName, "model": "", "format": "openai", "is_vl": model.Multimodal, "is_reasoning": model.Reasoning, "api_key": "", "url": "", "source": "system", "max_input_tokens": maxInput}
	name := []rune(prompt)
	if len(name) > 10 {
		name = name[:10]
	}
	out["business"] = map[string]any{"product": "cli", "version": COSYVersion, "type": "agent", "id": set, "name": string(name), "begin_at": time.Now().UnixMilli(), "stage": "processing"}
	return out
}

func (c ChatClient) Stream(ctx context.Context, cred *providers.Credential, model providers.ModelInfo, body map[string]any, emit func(ChatEvent) error) error {
	total := c.TotalTimeout
	if total <= 0 {
		total = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, total)
	defer cancel()
	idle := c.IdleTimeout
	if idle <= 0 {
		idle = 300 * time.Second
	}
	timer := time.AfterFunc(idle, cancel)
	defer timer.Stop()
	raw, err := json.Marshal(buildChatBody(model, body))
	if err != nil {
		return fmt.Errorf("invalid Qoder request")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Transport: chatTransport}
	}
	resp, err := (HTTPClient{Do: client.Do}).SignedPost(ctx, c.Region, strings.TrimPrefix(c.Region.ChatURL(), c.Region.InferBase), string(raw), cred.UID, cred.AccessToken, cred.MachineID, SignOptions{ModelKey: model.UpstreamID})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Qoder chat request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &providers.UpstreamError{Status: resp.StatusCode, Message: "Qoder chat request rejected"}
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	data := []string{}
	done := false
	process := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		if payload == "[DONE]" {
			done = true
			return nil
		}
		var frame map[string]any
		if json.Unmarshal([]byte(payload), &frame) != nil {
			return fmt.Errorf("invalid Qoder SSE frame")
		}
		if inner, ok := frame["body"].(string); ok {
			if inner == "[DONE]" {
				done = true
				return nil
			}
			if json.Unmarshal([]byte(inner), &frame) != nil {
				return fmt.Errorf("invalid Qoder inner frame")
			}
		}
		if frame["error"] != nil || (frame["choices"] == nil && frame["usage"] == nil && (frame["code"] != nil || frame["message"] != nil)) {
			return chatError(frame)
		}
		if u, ok := frame["usage"].(map[string]any); ok {
			if err := emit(ChatEvent{Usage: normalizeUsage(u)}); err != nil {
				return err
			}
		}
		choices, _ := frame["choices"].([]any)
		for _, choice := range choices {
			m, _ := choice.(map[string]any)
			delta, _ := m["delta"].(map[string]any)
			finish, _ := m["finish_reason"].(string)
			if err := emit(ChatEvent{Delta: delta, Finish: finish}); err != nil {
				return err
			}
		}
		return nil
	}
	for scanner.Scan() {
		timer.Reset(idle)
		line := scanner.Text()
		if line == "" {
			if err := process(); err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scanner.Err() != nil {
		return fmt.Errorf("Qoder stream interrupted")
	}
	if err := process(); err != nil {
		return err
	}
	if done {
		return nil
	}
	return fmt.Errorf("Qoder stream ended without completion marker")
}

func chatError(frame map[string]any) error {
	status := 502
	code := fmt.Sprint(frame["code"])
	switch code {
	case "401", "TOKEN_EXPIRE", "TOKEN_EXPIRED":
		status = 401
	case "112", "403":
		status = 403
	case "429", "10605":
		status = 429
	}
	message := stringField(frame, "message")
	if details, ok := frame["details"].(map[string]any); ok {
		if e, ok := details["error"].(map[string]any); ok {
			if v := stringField(e, "message"); v != "" {
				message = v
			}
		}
	}
	if e, ok := frame["error"].(map[string]any); ok {
		if v := stringField(e, "message"); v != "" {
			message = v
		}
	}
	if message == "" {
		message = "Qoder upstream error"
	}
	for _, key := range []string{"token", "authorization", "verifier", "credential", "cosy."} {
		if i := strings.Index(strings.ToLower(message), key); i >= 0 {
			message = message[:i] + "[redacted]"
		}
	}
	if len(message) > 500 {
		message = message[:500]
	}
	return &providers.UpstreamError{Status: status, Message: message}
}

func normalizeUsage(input map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range input {
		out[k] = v
	}
	for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "cached_tokens"} {
		if n, ok := input[key].(float64); ok {
			out[key] = int64(n)
		}
	}
	if detail, ok := input["prompt_tokens_details"].(map[string]any); ok {
		if n, ok := detail["cached_tokens"].(float64); ok {
			out["cached_tokens"] = int64(n)
		}
	}
	return out
}

func (c ChatClient) Collect(ctx context.Context, cred *providers.Credential, public string, model providers.ModelInfo, body map[string]any) (map[string]any, error) {
	content, reason, finish := "", "", "stop"
	usage := map[string]any{}
	tools := map[int]map[string]any{}
	err := c.Stream(ctx, cred, model, body, func(event ChatEvent) error {
		if event.Finish != "" {
			finish = event.Finish
		}
		for k, v := range event.Usage {
			usage[k] = v
		}
		if text, ok := event.Delta["content"].(string); ok {
			content += text
		}
		if text, ok := event.Delta["reasoning_content"].(string); ok {
			reason += text
		}
		calls, _ := event.Delta["tool_calls"].([]any)
		for _, call := range calls {
			tc, _ := call.(map[string]any)
			index, _ := tc["index"].(float64)
			i := int(index)
			out := tools[i]
			if out == nil {
				out = map[string]any{"type": "function", "function": map[string]any{"name": "", "arguments": ""}}
				tools[i] = out
			}
			if id, ok := tc["id"].(string); ok {
				out["id"] = id
			}
			fn, _ := tc["function"].(map[string]any)
			target := out["function"].(map[string]any)
			for _, key := range []string{"name", "arguments"} {
				if s, ok := fn[key].(string); ok {
					target[key] = target[key].(string) + s
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	message := map[string]any{"role": "assistant", "content": content}
	if reason != "" {
		message["reasoning_content"] = reason
	}
	if len(tools) > 0 {
		calls := []any{}
		for i := 0; i < len(tools); i++ {
			if call := tools[i]; call != nil {
				calls = append(calls, call)
			}
		}
		message["tool_calls"] = calls
	}
	result := map[string]any{"id": "chatcmpl-" + newUUIDString(), "object": "chat.completion", "created": time.Now().Unix(), "model": "qoder/" + public, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}}
	if len(usage) > 0 {
		result["usage"] = usage
	}
	return result, nil
}
