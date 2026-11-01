package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/antigravity"
	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

func (s *Service) handleAntigravityWithFailover(
	ctx context.Context,
	w http.ResponseWriter,
	tracker *usage.ActiveRequest,
	id string,
	created int64,
	model string,
	stream bool,
	msgs []map[string]any,
	tools []map[string]any,
	toolChoice any,
	reasoningEffort string,
) {
	accounts, modelID := s.accountCandidates(model)
	if len(accounts) == 0 {
		if s.hasCoolingAccount(model) {
			message := "Antigravity 账号均处于限流冷却，请稍后再试"
			if tracker != nil {
				tracker.OnError(message)
			}
			writeError(w, http.StatusTooManyRequests, "rate_limit_error", message)
			return
		}
		if tracker != nil {
			tracker.OnError("该模型没有可用的已启用账号")
		}
		writeError(w, 409, "no_ready_account", "该模型没有可用的已启用账号")
		return
	}
	var lastErr error
	for i := range accounts {
		acct := accounts[i]
		cred, err := s.antigravityCredential(&acct)
		if err != nil {
			lastErr = err
			if i+1 < len(accounts) {
				continue
			}
			break
		}
		if tracker != nil && i > 0 {
			tracker.SetAccount(acct.ID)
		}
		upstream := antigravityBody(msgs, tools, toolChoice, reasoningEffort)
		lastErr = s.runAntigravityAttempt(ctx, w, tracker, id, created, model, modelID, stream, &acct, cred, upstream)
		if lastErr == nil || ctx.Err() != nil || !retryableAccountError(lastErr) {
			break
		}
		s.markAccountCooling(acct.ID, false)
	}
	if lastErr == nil {
		return
	}
	if ctx.Err() != nil {
		if tracker != nil {
			tracker.OnCanceled()
		}
		return
	}
	if tracker != nil {
		tracker.OnError(lastErr.Error())
	}
	writeError(w, upstreamHTTPStatus(lastErr), "upstream_error", redact(lastErr.Error()))
}

func (s *Service) antigravityCredential(acct *storage.Account) (*providers.Credential, error) {
	raw, err := s.secret(acct.ID)
	if err != nil {
		return nil, fmt.Errorf("账号凭据不可用: %w", err)
	}
	var cred providers.Credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return nil, fmt.Errorf("账号凭据损坏: %w", err)
	}
	cred.UID = acct.UID
	cred.Region = acct.Region
	cred.MachineID = acct.MachineID
	cred.DeviceID = acct.DeviceID
	cred.ModelRoutes = s.chatModelRoutes(*acct)
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err := s.refreshCredential(acct, &cred); err != nil {
			_ = s.store.UpdateAccount(acct.ID, func(a *storage.Account) { a.AuthState = "expired" })
			return nil, &providers.UpstreamError{Status: http.StatusUnauthorized, Message: "账号登录已失效，请重新登录"}
		}
	}
	return &cred, nil
}

func antigravityBody(msgs []map[string]any, tools []map[string]any, toolChoice any, reasoningEffort string) map[string]any {
	body := map[string]any{"messages": msgs}
	if choice := normalizeToolChoice(toolChoice); choice != "" {
		body["tool_choice"] = choice
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if reasoningEffort != "" {
		body["reasoning_effort"] = reasoningEffort
	}
	return body
}

func retryableAccountError(err error) bool {
	var upstream *providers.UpstreamError
	if !errors.As(err, &upstream) {
		return false
	}
	return RetryAccountError(upstream.Status)
}

func (s *Service) runAntigravityAttempt(
	ctx context.Context,
	w http.ResponseWriter,
	tracker *usage.ActiveRequest,
	id string,
	created int64,
	model string,
	modelID string,
	stream bool,
	acct *storage.Account,
	cred *providers.Credential,
	body map[string]any,
) error {
	collector := s.collect
	if collector == nil {
		collector = antigravity.Collector{}.Collect
	}
	streamer := s.stream
	if streamer == nil {
		streamer = antigravity.Streamer{}.Stream
	}
	if !stream {
		result, err := collector(ctx, cred, modelID, body)
		if err != nil {
			return err
		}
		if usageMap, ok := result["usage"].(map[string]any); ok {
			s.recordUsage(tracker, *acct, usageMap)
		}
		if tracker != nil {
			tracker.OnCompleted()
		}
		writeJSON(w, result)
		return nil
	}

	var gate antigravityStreamGate
	var usageMu sync.Mutex
	var finalUsage map[string]any
	events := &providers.ChatEvents{
		OnFirstToken: func() {
			gate.commit(w)
			if tracker != nil {
				tracker.OnFirstToken()
			}
		},
		OnDelta: func(delta map[string]any) {
			gate.commit(w)
			if gate.err != nil {
				return
			}
			writeAntigravityChunk(w, id, created, model, delta, nil)
			if tracker != nil {
				for _, key := range []string{"content", "reasoning_content"} {
					if text, ok := delta[key].(string); ok && text != "" {
						tracker.OnStreamDelta(text)
					}
				}
			}
		},
		OnUsage: func(value map[string]any) {
			usageMu.Lock()
			finalUsage = value
			usageMu.Unlock()
		},
	}
	err := streamer(ctx, cred, modelID, body, events)
	committed, gateErr := gate.result()
	if !committed {
		if err != nil {
			return err
		}
		return &providers.UpstreamError{Status: http.StatusBadGateway, Message: "Antigravity 上游在首个事件前结束"}
	}
	if gateErr != nil {
		return gateErr
	}
	if tracker != nil {
		if err == nil {
			tracker.OnCompleted()
		} else if ctx.Err() != nil {
			tracker.OnCanceled()
		} else {
			tracker.OnInterrupted(err.Error())
		}
	}
	usageMu.Lock()
	usageMap := finalUsage
	usageMu.Unlock()
	writeAntigravityChunk(w, id, created, model, map[string]any{}, "stop")
	if len(usageMap) > 0 {
		writeAntigravityUsage(w, id, created, model, usageMap)
		s.recordUsage(nil, *acct, usageMap)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

type antigravityStreamGate struct {
	mu        sync.Mutex
	committed bool
	err       error
}

func (g *antigravityStreamGate) commit(w http.ResponseWriter) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.committed {
		return
	}
	g.committed = true
	g.err = startAntigravityStream(w)
}

func (g *antigravityStreamGate) result() (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.committed, g.err
}

func startAntigravityStream(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeAntigravityChunk(w http.ResponseWriter, id string, created int64, model string, delta map[string]any, finish any) {
	payload := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
	}
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeAntigravityUsage(w http.ResponseWriter, id string, created int64, model string, usageMap map[string]any) {
	payload := map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []any{}, "usage": usageMap,
	}
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
}
