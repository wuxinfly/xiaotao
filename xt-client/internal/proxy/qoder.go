package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/qoder"
	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

func (s *Service) handleQoder(ctx context.Context, w http.ResponseWriter, tracker *usage.ActiveRequest, acct storage.Account, cred *providers.Credential, model string, stream bool, raw json.RawMessage) {
	var entry providers.ModelInfo
	for _, m := range s.AccountModels(acct) {
		if m.ID == model {
			entry = m
			break
		}
	}
	if entry.UpstreamID == "" {
		writeError(w, 400, "invalid_model", "Qoder 模型目录不可用或模型已停用")
		if tracker != nil {
			tracker.OnError("模型不可用")
		}
		return
	}
	var body map[string]any
	json.Unmarshal(raw, &body)
	reg, ok := qoder.Regions[acct.Region]
	if !ok {
		if tracker != nil {
			tracker.OnError("Qoder 区域无效")
		}
		writeError(w, 400, "invalid_region", "Qoder 区域无效")
		return
	}
	client := qoder.ChatClient{Region: reg}
	if !stream {
		queueCtx, queueCancel := context.WithTimeout(ctx, 3*time.Minute)
		result, err := collectQoderWithQueue(queueCtx, client, cred, model, entry, body)
		queueCancel()
		if err != nil {
			if tracker != nil {
				if ctx.Err() != nil {
					tracker.OnCanceled()
				} else {
					tracker.OnError(err.Error())
				}
			}
			writeError(w, upstreamHTTPStatus(err), "upstream_error", redact(err.Error()))
			return
		}
		if u, ok := result["usage"].(map[string]any); ok {
			s.recordUsage(tracker, acct, u)
		}
		if tracker != nil {
			tracker.OnCompleted()
		}
		writeJSON(w, result)
		return
	}
	queueCtx, queueCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer queueCancel()
	started := false
	first := false
	confirmed := map[string]any{}
	finish := "stop"
	flusher, _ := w.(http.Flusher)
	responseID, created := "chatcmpl-"+randHex(), time.Now().Unix()
	send := func(payload map[string]any) error {
		payload["id"], payload["created"] = responseID, created
		if !started {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			started = true
		}
		data, _ := json.Marshal(payload)
		_, err := fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
		return err
	}
	err := qoderWithQueue(queueCtx, client, cred, entry, body, func(event qoder.ChatEvent) error {
		for k, v := range event.Usage {
			confirmed[k] = v
		}
		if event.Finish != "" {
			finish = event.Finish
		}
		if len(event.Delta) > 0 {
			if !first {
				first = true
				if tracker != nil {
					tracker.OnFirstToken()
				}
			}
			if tracker != nil {
				for _, key := range []string{"content", "reasoning_content"} {
					if text, ok := event.Delta[key].(string); ok {
						tracker.OnStreamDelta(text)
					}
				}
			}
			return send(map[string]any{"object": "chat.completion.chunk", "model": "qoder/" + model, "choices": []any{map[string]any{"index": 0, "delta": event.Delta, "finish_reason": nil}}})
		}
		return nil
	})
	if len(confirmed) > 0 {
		s.recordUsage(tracker, acct, confirmed)
	}
	if err != nil {
		if tracker != nil {
			if ctx.Err() != nil {
				tracker.OnCanceled()
			} else if started {
				tracker.OnInterrupted(err.Error())
			} else {
				tracker.OnError(err.Error())
			}
		}
		if !started {
			writeError(w, upstreamHTTPStatus(err), "upstream_error", redact(err.Error()))
		} else {
			_ = send(map[string]any{"error": map[string]any{"message": redact(err.Error()), "type": "upstream_error"}})
		}
		return
	}
	_ = send(map[string]any{"object": "chat.completion.chunk", "model": "qoder/" + model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}})
	if len(confirmed) > 0 {
		_ = send(map[string]any{"object": "chat.completion.chunk", "model": "qoder/" + model, "choices": []any{}, "usage": confirmed})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	if tracker != nil {
		tracker.OnCompleted()
	}
}

var qoderQueueWait = qoder.QueueWait
var qoderQueueSleep = func(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func qoderWithQueue(ctx context.Context, client qoder.ChatClient, cred *providers.Credential, model providers.ModelInfo, body map[string]any, emit func(qoder.ChatEvent) error) error {
	deadline, _ := ctx.Deadline()
	for round := 0; ; round++ {
		emitted := false
		err := client.Stream(ctx, cred, model, body, func(event qoder.ChatEvent) error {
			if len(event.Delta) > 0 || len(event.Usage) > 0 || event.Finish != "" {
				emitted = true
			}
			return emit(event)
		})
		if err == nil {
			return nil
		}
		wait := qoderQueueWait(err)
		if wait <= 0 || emitted || round >= 2 {
			return err
		}
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return ctx.Err()
			}
			if wait >= remaining {
				return err
			}
		}
		if err := qoderQueueSleep(ctx, wait); err != nil {
			return err
		}
	}
}

func collectQoderWithQueue(ctx context.Context, client qoder.ChatClient, cred *providers.Credential, public string, model providers.ModelInfo, body map[string]any) (map[string]any, error) {
	content, reason, finish := "", "", "stop"
	usage := map[string]any{}
	tools := map[int]map[string]any{}
	err := qoderWithQueue(ctx, client, cred, model, body, func(event qoder.ChatEvent) error {
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
	result := map[string]any{"id": "chatcmpl-" + randHex(), "object": "chat.completion", "created": time.Now().Unix(), "model": "qoder/" + public, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}}
	if len(usage) > 0 {
		result["usage"] = usage
	}
	return result, nil
}
