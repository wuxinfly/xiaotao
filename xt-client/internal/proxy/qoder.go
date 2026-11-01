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
		result, err := client.Collect(ctx, cred, model, entry, body)
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
	err := client.Stream(ctx, cred, entry, body, func(event qoder.ChatEvent) error {
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
