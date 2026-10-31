package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/storage"
)

type Service struct {
	store  *storage.Store
	secret func(id string) ([]byte, error)
	mu     sync.Mutex
	key    string
}

func New(store *storage.Store, secret func(id string) ([]byte, error), apiKey string) *Service {
	return &Service{store: store, secret: secret, key: apiKey}
}

func (s *Service) APIKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key
}

func (s *Service) RotateKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := hex.EncodeToString(b)
	if err := s.store.SaveSecret("proxy", []byte(key), encryptFn); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.key = key
	s.mu.Unlock()
	return key, nil
}

func (s *Service) Authorize(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return auth == "Bearer "+s.APIKey()
}

func (s *Service) Models() ([]map[string]any, error) {
	p := s.store.Proxy()
	if p.DefaultAccountID == "" {
		return []map[string]any{}, nil
	}
	acct, ok := s.store.Account(p.DefaultAccountID)
	if !ok || !acct.Enabled {
		return []map[string]any{}, nil
	}
	out := []map[string]any{}
	traeProvider := trae.Provider{}
	for _, m := range traeProvider.ListModels(acct.Region) {
		out = append(out, map[string]any{
			"id": "trae/" + m.ID, "object": "model", "created": 0, "owned_by": "xiaotao",
		})
	}
	return out, nil
}

func (s *Service) HandleChat(w http.ResponseWriter, r *http.Request) {
	if !s.Authorize(r) {
		writeError(w, 401, "invalid_api_key", "代理访问密钥无效")
		return
	}
	var body struct {
		Model    string          `json:"model"`
		Stream   bool            `json:"stream"`
		Messages json.RawMessage `json:"messages"`
		Tools    json.RawMessage `json:"tools"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&body); err != nil {
		writeError(w, 400, "invalid_request_error", "请求体无法解析")
		return
	}
	model := strings.TrimPrefix(body.Model, "trae/")
	if model == "" {
		writeError(w, 400, "invalid_request_error", "缺少 model")
		return
	}
	p := s.store.Proxy()
	acct, ok := s.store.Account(p.DefaultAccountID)
	if !ok || !acct.Enabled {
		writeError(w, 409, "no_ready_account", "缺少可用账号，请在账号页启用并设为默认")
		return
	}
	credBytes, err := s.secret(acct.ID)
	if err != nil {
		writeError(w, 409, "account_unavailable", "账号凭据不可用")
		return
	}
	var cred providers.Credential
	if err := json.Unmarshal(credBytes, &cred); err != nil {
		writeError(w, 500, "account_unavailable", "账号凭据损坏")
		return
	}
	cred.UID = acct.UID
	cred.Region = acct.Region
	cred.MachineID = acct.MachineID
	cred.DeviceID = acct.DeviceID
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err := s.refreshCredential(&acct, &cred); err != nil {
			_ = s.store.UpdateAccount(acct.ID, func(a *storage.Account) { a.AuthState = "expired" })
			writeError(w, 401, "account_unauthorized", "账号登录已失效，请重新登录")
			return
		}
	}
	upstream := map[string]any{
		"function": "chat_v3", "request_id": randHex(), "session_id": randHex(),
	}
	var msgs []map[string]any
	if err := json.Unmarshal(body.Messages, &msgs); err != nil || len(msgs) == 0 {
		writeError(w, 400, "invalid_request_error", "messages 无效")
		return
	}
	upstream["messages"] = trae.NativeMessages(msgs)
	if len(body.Tools) > 0 {
		var tools []map[string]any
		if err := json.Unmarshal(body.Tools, &tools); err != nil {
			writeError(w, 400, "invalid_request_error", "tools 无效")
			return
		}
		upstream["tools"] = convertTools(tools)
	}
	id := "chatcmpl-" + randHex()
	created := time.Now().Unix()
	ctx := r.Context()
	if body.Stream {
		s.streamLive(w, id, created, body.Model, &cred, acct, acct.MachineID, acct.DeviceID, model, upstream)
		return
	}
	result, err := trae.CollectChat(ctx, &cred, acct.MachineID, acct.DeviceID, model, upstream)
	if err != nil {
		writeError(w, 502, "upstream_error", redact(err.Error()))
		return
	}
	if result.Usage != nil {
		if prompt, ok := result.Usage["prompt_tokens"].(int64); ok {
			var completion int64
			if v, ok := result.Usage["completion_tokens"].(int64); ok {
				completion = v
			}
			_ = s.store.UpdateAccount(acct.ID, func(a *storage.Account) {
				a.UsageTokens += prompt + completion
				a.UsageCalls++
			})
		}
	}
	if result.ErrMsg != "" {
		writeError(w, upstreamStatus(result.ErrCode), "upstream_error", result.ErrMsg)
		return
	}
	if body.Stream {
		streamResult(w, id, created, body.Model, result)
		return
	}
	message := map[string]any{"role": "assistant", "content": result.Content}
	finish := "stop"
	if len(result.Tools) > 0 {
		message["tool_calls"] = result.Tools
		finish = "tool_calls"
	}
	writeJSON(w, map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": body.Model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   result.Usage,
	})
}

// streamLive proxies the upstream SSE stream incrementally: each upstream
// delta is converted to an OpenAI chunk and flushed immediately, so
// downstream clients see tokens as they are generated.
func (s *Service) streamLive(w http.ResponseWriter, id string, created int64, model string, cred *providers.Credential, acct storage.Account, machineID, deviceID, modelID string, upstream map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	writeChunk := func(payload map[string]any) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk := func(delta map[string]any, finish any) {
		writeChunk(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		})
	}
	chunk(map[string]any{"role": "assistant", "content": ""}, nil)
	var usage atomic.Value
	var upstreamErr atomic.Value
	ev := &trae.StreamEvents{
		OnDelta: func(delta map[string]any) { chunk(delta, nil) },
		OnError: func(code any, msg string) {
			upstreamErr.Store(struct {
				code any
				msg  string
			}{code, msg})
		},
		OnUsage: func(u map[string]any) {
			prev, _ := usage.Load().(map[string]any)
			next := map[string]any{}
			for k, v := range prev {
				next[k] = v
			}
			for k, v := range u {
				next[k] = v
			}
			usage.Store(next)
		},
	}
	err := trae.StreamChat(context.Background(), cred, machineID, deviceID, modelID, upstream, ev)
	if err != nil {
		// Headers are already sent; surface the failure as a terminal
		// error chunk so clients do not hang.
		chunk(map[string]any{"content": "\n\n[upstream error: " + redact(err.Error()) + "]"}, nil)
	}
	if v, ok := upstreamErr.Load().(struct {
		code any
		msg  string
	}); ok && v.msg != "" {
		chunk(map[string]any{"content": "\n\n[" + redact(v.msg) + "]"}, nil)
	}
	final, _ := usage.Load().(map[string]any)
	chunk(map[string]any{}, "stop")
	if len(final) > 0 {
		writeChunk(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{}, "usage": final,
		})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if prompt, ok := final["prompt_tokens"].(int64); ok {
		var completion int64
		if v, ok := final["completion_tokens"].(int64); ok {
			completion = v
		}
		_ = s.store.UpdateAccount(acct.ID, func(a *storage.Account) {
			a.UsageTokens += prompt + completion
			a.UsageCalls++
		})
	}
}

func (s *Service) refreshCredential(acct *storage.Account, cred *providers.Credential) error {
	next, err := trae.Exchange(cred.Region, cred.RefreshToken)
	if err != nil {
		return err
	}
	next.UID = cred.UID
	next.MachineID = cred.MachineID
	next.DeviceID = cred.DeviceID
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := s.store.SaveSecret(acct.ID, b, encryptFn); err != nil {
		return err
	}
	*cred = *next
	return nil
}

func streamResult(w http.ResponseWriter, id string, created int64, model string, result *trae.ChatResult) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	chunk := func(delta map[string]any, finish any) {
		payload := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk(map[string]any{"role": "assistant", "content": ""}, nil)
	if result.Reason != "" {
		chunk(map[string]any{"reasoning_content": result.Reason}, nil)
	}
	if result.Content != "" {
		chunk(map[string]any{"content": result.Content}, nil)
	}
	if len(result.Tools) > 0 {
		chunk(map[string]any{"tool_calls": result.Tools}, nil)
	}
	chunk(map[string]any{}, "stop")
	if result.Usage != nil {
		b, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{}, "usage": result.Usage,
		})
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func convertTools(tools []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		if fn == nil {
			continue
		}
		params := fn["parameters"]
		switch params.(type) {
		case string:
		default:
			b, _ := json.Marshal(params)
			params = string(b)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": fn["name"], "description": fn["description"], "parameters": params,
			},
		})
	}
	return out
}

func upstreamStatus(code any) int {
	s := strings.TrimSpace(fmt.Sprintf("%v", code))
	switch s {
	case "1001", "4010":
		return 401
	case "4008", "4011", "4021", "4031":
		return 429
	}
	return 502
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message, "type": code}})
}

func writeJSON(w http.ResponseWriter, args ...any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	status := 200
	var v any
	if len(args) == 2 {
		status, _ = args[0].(int)
		v = args[1]
	} else if len(args) == 1 {
		v = args[0]
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func redact(s string) string {
	low := strings.ToLower(s)
	for _, key := range []string{"token", "credential", "userjwt", "authorization"} {
		if i := strings.Index(low, key); i >= 0 {
			return s[:i] + key + "=<redacted>"
		}
	}
	return s
}
