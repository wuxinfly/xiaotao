package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/sourceplugin"
)

func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func operate(ctx context.Context, method string, op sourceplugin.Operation, emit func(map[string]any)) (sourceplugin.Result, error) {
	var c providers.Credential
	if len(op.Credential) > 0 && json.Unmarshal(op.Credential, &c) != nil {
		return sourceplugin.Result{}, errors.New("invalid credential")
	}
	result := sourceplugin.Result{}
	if method == "credentials/import" {
		if json.Unmarshal(op.Input, &c) != nil || c.RefreshToken == "" || (c.Region != "" && c.Region != "cn") {
			return result, errors.New("invalid legacy credential")
		}
		if c.MachineID == "" {
			c.MachineID = randomID()
		}
		if c.DeviceID == "" {
			c.DeviceID = randomID()
		}
		c.Region = "cn"
		result.Credential, _ = json.Marshal(c)
		result.Models = []sourceplugin.Model{}
		return result, nil
	}
	if method == "login/import" {
		var input struct {
			RefreshToken string `json:"refreshToken"`
		}
		if json.Unmarshal(op.Input, &input) != nil || input.RefreshToken == "" {
			return result, errors.New("refresh token required")
		}
		c = providers.Credential{RefreshToken: input.RefreshToken, Region: "cn", MachineID: randomID(), DeviceID: randomID()}
	}
	if c.RefreshToken == "" {
		return result, errors.New("login required")
	}
	if method == "login/import" || c.AccessToken == "" || c.ExpiresAt < time.Now().Unix()+120 {
		next, err := trae.Exchange("cn", c.RefreshToken)
		if err != nil {
			return result, err
		}
		next.MachineID = c.MachineID
		next.DeviceID = c.DeviceID
		c = *next
	}
	// Persist refreshed credentials even when the subsequent action fails: host
	// performs refresh as a separate operation before model requests.
	result.Credential, _ = json.Marshal(c)
	switch method {
	case "login/import", "credentials/refresh":
		if method == "login/import" {
			result.Models = []sourceplugin.Model{}
		}
		return result, nil
	case "models/refresh":
		models, err := trae.FetchUpstreamModels(&c, c.MachineID, c.DeviceID)
		if err != nil {
			return result, err
		}
		result.Models = []sourceplugin.Model{}
		for _, m := range models {
			upstream := m.UpstreamID
			if upstream == "" {
				upstream = trae.UpstreamModel(m.ID)
			}
			result.Models = append(result.Models, sourceplugin.Model{Key: m.ID, UpstreamID: upstream, Label: m.DisplayName, ContextWindow: m.MaxInputTokens, SupportsImages: m.Multimodal, SupportsReasoning: m.Reasoning, ReasoningEfforts: m.Efforts})
		}
	case "quota/get":
		value, err := trae.FetchQuota(&c, c.MachineID, c.DeviceID)
		if err != nil {
			return result, err
		}
		result.Value, _ = json.Marshal(value)
	case "checkin/status":
		value, err := trae.FetchCheckinStatus(&c, "xiaotao.trae")
		if err != nil {
			return result, err
		}
		result.Value, _ = json.Marshal(value)
	case "checkin/claim":
		value, err := trae.ClaimCheckin(&c, "xiaotao.trae")
		if err != nil {
			return result, err
		}
		result.Value, _ = json.Marshal(value)
	case "chat/start":
		body, err := nativeBody(op.Body)
		if err != nil {
			return result, err
		}
		if strings.HasSuffix(op.Model, "__max") {
			window, _ := op.Body["_plugin_context_window"].(float64)
			output := float64(0)
			switch v := body["max_tokens"].(type) {
			case float64:
				output = v
			}
			if window > output && window > 0 {
				body["user_message_context"] = map[string]any{"model_info": map[string]any{"prompt_max_tokens": window - output}}
			}
		}
		stream, _ := op.Body["stream"].(bool)
		if !stream {
			collected, err := trae.CollectChat(ctx, &c, c.MachineID, c.DeviceID, op.Model, body)
			if err != nil {
				return result, err
			}
			if collected.ErrMsg != "" {
				return result, errors.New("upstream rejected completion")
			}
			finish := "stop"
			message := map[string]any{"role": "assistant", "content": collected.Content}
			if collected.Reason != "" {
				message["reasoning_content"] = collected.Reason
			}
			if len(collected.Tools) > 0 {
				finish = "tool_calls"
				message["tool_calls"] = collected.Tools
			}
			value := map[string]any{"object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": collected.Usage}
			result.Value, _ = json.Marshal(value)
			break
		}
		tools := false
		err = (trae.StreamProvider{}).Stream(ctx, &c, op.Model, body, &providers.ChatEvents{OnDelta: func(delta map[string]any) {
			if delta["tool_calls"] != nil {
				tools = true
			}
			emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
		}, OnUsage: func(usage map[string]any) { emit(map[string]any{"choices": []any{}, "usage": usage}) }})
		if err != nil {
			return result, err
		}
		finish := "stop"
		if tools {
			finish = "tool_calls"
		}
		emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}})
	default:
		return result, errors.New("unsupported method")
	}
	// Chat never returns a redundant credential update; refresh ran before chat.
	if method == "chat/start" {
		result.Credential = nil
	}
	return result, nil
}
func nativeBody(body map[string]any) (map[string]any, error) {
	if body == nil {
		return nil, errors.New("missing body")
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		return nil, errors.New("missing messages")
	}
	msgs := make([]map[string]any, 0, len(messages))
	for _, v := range messages {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("invalid message")
		}
		msgs = append(msgs, m)
	}
	id := randomID()
	out := map[string]any{"function": "chat_v3", "request_id": id, "session_id": id, "messages": trae.NativeMessages(msgs)}
	if v := body["max_tokens"]; v != nil {
		out["max_tokens"] = v
	} else if v = body["max_completion_tokens"]; v != nil {
		out["max_tokens"] = v
	}
	if effort, ok := body["reasoning_effort"].(string); ok {
		out["reasoning_effort"] = trae.NativeEffort(effort)
	}
	if tools, ok := body["tools"].([]any); ok {
		converted := []any{}
		for _, v := range tools {
			t, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if fn, ok := t["function"].(map[string]any); ok {
				params := fn["parameters"]
				if _, ok := params.(string); !ok {
					raw, _ := json.Marshal(params)
					params = string(raw)
				}
				converted = append(converted, map[string]any{"type": "function", "function": map[string]any{"name": fn["name"], "description": fn["description"], "parameters": params}})
			}
		}
		out["tools"] = converted
	}
	return out, nil
}
