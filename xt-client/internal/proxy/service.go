package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

type Service struct {
	store  *storage.Store
	secret func(id string) ([]byte, error)
	usage  *usage.Service
	mu     sync.Mutex
	key    string
}

func New(store *storage.Store, secret func(id string) ([]byte, error), apiKey string) *Service {
	return &Service{store: store, secret: secret, key: apiKey}
}

func (s *Service) SetUsage(u *usage.Service) {
	s.usage = u
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
	var msgs []map[string]any
	if err := json.Unmarshal(body.Messages, &msgs); err != nil || len(msgs) == 0 {
		writeError(w, 400, "invalid_request_error", "messages 无效")
		return
	}
	var tools []map[string]any
	if len(body.Tools) > 0 {
		if err := json.Unmarshal(body.Tools, &tools); err != nil {
			writeError(w, 400, "invalid_request_error", "tools 无效")
			return
		}
	}

	p := s.store.Proxy()
	acct, ok := s.store.Account(p.DefaultAccountID)
	id := "chatcmpl-" + randHex()
	created := time.Now().Unix()
	ctx := r.Context()

	var reqTracker *usage.ActiveRequest
	if s.usage != nil {
		acctID := ""
		channel := "trae"
		if ok {
			acctID = acct.ID
			if acct.Provider != "" {
				channel = acct.Provider
			}
		}
		tag, title := detectAction(msgs, tools)
		reqTracker = s.usage.StartRequest(id, channel, body.Model, acctID, tag, title)
	}

	if !ok || !acct.Enabled {
		if reqTracker != nil {
			reqTracker.OnError("缺少可用账号，请在账号页启用并设为默认")
		}
		writeError(w, 409, "no_ready_account", "缺少可用账号，请在账号页启用并设为默认")
		return
	}
	credBytes, err := s.secret(acct.ID)
	if err != nil {
		if reqTracker != nil {
			reqTracker.OnError("账号凭据不可用")
		}
		writeError(w, 409, "account_unavailable", "账号凭据不可用")
		return
	}
	var cred providers.Credential
	if err := json.Unmarshal(credBytes, &cred); err != nil {
		if reqTracker != nil {
			reqTracker.OnError("账号凭据损坏")
		}
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
			if reqTracker != nil {
				reqTracker.OnError("账号登录已失效，请重新登录")
			}
			writeError(w, 401, "account_unauthorized", "账号登录已失效，请重新登录")
			return
		}
	}
	upstream := map[string]any{
		"function": "chat_v3", "request_id": randHex(), "session_id": randHex(),
	}
	upstream["messages"] = trae.NativeMessages(msgs)
	if len(body.Tools) > 0 {
		upstream["tools"] = convertTools(tools)
	}
	if body.Stream {
		s.streamLive(ctx, reqTracker, w, id, created, body.Model, &cred, acct, acct.MachineID, acct.DeviceID, model, upstream)
		return
	}
	result, err := trae.CollectChat(ctx, &cred, acct.MachineID, acct.DeviceID, model, upstream)
	if err != nil {
		if reqTracker != nil {
			if ctx.Err() != nil {
				reqTracker.OnCanceled()
			} else {
				reqTracker.OnError(err.Error())
			}
		}
		writeError(w, 502, "upstream_error", redact(err.Error()))
		return
	}
	if reqTracker != nil {
		if result.FirstTokenAt != nil {
			reqTracker.OnFirstToken()
		}
		if result.Usage != nil {
			var pPtr, cPtr, caPtr *int64
			if p, ok := result.Usage["prompt_tokens"].(int64); ok {
				pPtr = &p
			}
			if c, ok := result.Usage["completion_tokens"].(int64); ok {
				cPtr = &c
			}
			if ca, ok := result.Usage["cached_tokens"].(int64); ok {
				caPtr = &ca
			}
			reqTracker.OnUsage(pPtr, cPtr, caPtr)
		}
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
		if reqTracker != nil {
			reqTracker.OnError(result.ErrMsg)
		}
		writeError(w, upstreamStatus(result.ErrCode), "upstream_error", result.ErrMsg)
		return
	}
	if reqTracker != nil {
		reqTracker.OnCompleted()
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
func (s *Service) streamLive(ctx context.Context, reqTracker *usage.ActiveRequest, w http.ResponseWriter, id string, created int64, model string, cred *providers.Credential, acct storage.Account, machineID, deviceID, modelID string, upstream map[string]any) {
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
		OnFirstToken: func(t time.Time) {
			if reqTracker != nil {
				reqTracker.OnFirstToken()
			}
		},
		OnDelta: func(delta map[string]any) {
			chunk(delta, nil)
			if reqTracker != nil {
				if content, ok := delta["content"].(string); ok && content != "" {
					reqTracker.OnStreamDelta(content)
				}
				if reason, ok := delta["reasoning_content"].(string); ok && reason != "" {
					reqTracker.OnStreamDelta(reason)
				}
				if tcList, ok := delta["tool_calls"].([]any); ok && len(tcList) > 0 {
					if tc, ok := tcList[0].(map[string]any); ok {
						if fn, ok := tc["function"].(map[string]any); ok {
							if name, _ := fn["name"].(string); name != "" {
								tag, title := classifyTool(name, "")
								reqTracker.SetAction(tag, title)
							}
						}
					}
				}
			}
		},
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
			if reqTracker != nil {
				var pPtr, cPtr, caPtr *int64
				if p, ok := next["prompt_tokens"].(int64); ok {
					pPtr = &p
				}
				if c, ok := next["completion_tokens"].(int64); ok {
					cPtr = &c
				}
				if ca, ok := next["cached_tokens"].(int64); ok {
					caPtr = &ca
				}
				reqTracker.OnUsage(pPtr, cPtr, caPtr)
			}
		},
	}
	err := trae.StreamChat(ctx, cred, machineID, deviceID, modelID, upstream, ev)
	if err != nil {
		if reqTracker != nil {
			if ctx.Err() != nil {
				reqTracker.OnCanceled()
			} else {
				reqTracker.OnInterrupted(err.Error())
			}
		}
		// Headers are already sent; surface the failure as a terminal
		// error chunk so clients do not hang.
		chunk(map[string]any{"content": "\n\n[upstream error: " + redact(err.Error()) + "]"}, nil)
	} else if v, ok := upstreamErr.Load().(struct {
		code any
		msg  string
	}); ok && v.msg != "" {
		if reqTracker != nil {
			reqTracker.OnError(v.msg)
		}
	} else {
		if reqTracker != nil {
			reqTracker.OnCompleted()
		}
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

func detectAction(msgs []map[string]any, tools []map[string]any) (string, string) {
	if len(msgs) == 0 {
		return "用户", "对话交互"
	}

	// 1. Check if the latest message is a tool execution result
	last := msgs[len(msgs)-1]
	role, _ := last["role"].(string)

	if role == "tool" {
		name, _ := last["name"].(string)
		args := ""
		if name == "" {
			callID, _ := last["tool_call_id"].(string)
			name, args = findToolByCallID(msgs, callID)
		}
		if name != "" {
			return classifyTool(name, args)
		}
	}

	// Check for tool_result block in user message
	if callID := extractToolResultCallID(last["content"]); callID != "" {
		name, args := findToolByCallID(msgs, callID)
		if name != "" {
			return classifyTool(name, args)
		}
	}

	// 2. If the last message is a user prompt, extract the specific user request
	if role == "user" {
		txt := extractContentText(last["content"])
		sum := cleanSummary(txt)
		if sum != "" {
			return "用户", sum
		}
	}

	// 3. Scan backwards for the latest meaningful action
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		r, _ := m["role"].(string)
		if r == "assistant" {
			if tcList, ok := m["tool_calls"].([]any); ok && len(tcList) > 0 {
				if tc, ok := tcList[0].(map[string]any); ok {
					if fn, ok := tc["function"].(map[string]any); ok {
						name, _ := fn["name"].(string)
						args, _ := fn["arguments"].(string)
						return classifyTool(name, args)
					}
				}
			}
		}
		if r == "tool" {
			name, _ := m["name"].(string)
			args := ""
			if name == "" {
				callID, _ := m["tool_call_id"].(string)
				name, args = findToolByCallID(msgs, callID)
			}
			if name != "" {
				return classifyTool(name, args)
			}
		}
		if r == "user" {
			txt := extractContentText(m["content"])
			sum := cleanSummary(txt)
			if sum != "" {
				return "用户", sum
			}
		}
	}

	txt := extractContentText(last["content"])
	sum := cleanSummary(txt)
	if sum != "" {
		return "用户", sum
	}
	return "用户", "对话交互"
}

func classifyTool(name, args string) (string, string) {
	cleanName := name
	if idx := strings.LastIndex(name, "__"); idx >= 0 && idx < len(name)-2 {
		cleanName = name[idx+2:]
	}
	cleanLow := strings.ToLower(cleanName)
	target := extractPathOrTarget(args)

	if strings.Contains(cleanLow, "read") || strings.Contains(cleanLow, "view") ||
		strings.Contains(cleanLow, "cat") || strings.Contains(cleanLow, "grep") ||
		strings.Contains(cleanLow, "search") || strings.Contains(cleanLow, "list") ||
		strings.Contains(cleanLow, "get") {
		if target != "" {
			return "读", "分析: " + target
		}
		return "读", "分析项目文件"
	}
	if strings.Contains(cleanLow, "write") || strings.Contains(cleanLow, "edit") ||
		strings.Contains(cleanLow, "patch") || strings.Contains(cleanLow, "apply") ||
		strings.Contains(cleanLow, "save") || strings.Contains(cleanLow, "create") {
		if target != "" {
			return "写", "更新: " + target
		}
		return "写", "更新代码文件"
	}
	if strings.Contains(cleanLow, "exec") || strings.Contains(cleanLow, "cmd") ||
		strings.Contains(cleanLow, "bash") || strings.Contains(cleanLow, "sh") ||
		strings.Contains(cleanLow, "terminal") || strings.Contains(cleanLow, "run") {
		if target != "" {
			return "执行", target
		}
		return "执行", "执行命令"
	}
	if cleanName != "" {
		return "工具", cleanName
	}
	return "工具", "工具调用"
}

func extractPathOrTarget(args string) string {
	if args == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) == nil {
		// Support apply_patch patch content
		if patchInput, ok := m["input"].(string); ok {
			for _, prefix := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
				if idx := strings.Index(patchInput, prefix); idx >= 0 {
					sub := patchInput[idx+len(prefix):]
					if endIdx := strings.Index(sub, "\n"); endIdx > 0 {
						fn := strings.TrimSpace(sub[:endIdx])
						return filepath.Base(fn)
					}
				}
			}
		}
		for _, k := range []string{"path", "file", "filename", "cmd", "command", "query", "url"} {
			if v, ok := m[k].(string); ok && v != "" {
				if strings.Contains(v, "/") || strings.Contains(v, "\\") {
					v = filepath.Base(v)
				}
				runes := []rune(v)
				if len(runes) > 28 {
					v = string(runes[:28]) + "..."
				}
				return v
			}
		}
	}
	return ""
}

func findToolByCallID(msgs []map[string]any, callID string) (string, string) {
	if callID == "" {
		return "", ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if tcList, ok := m["tool_calls"].([]any); ok {
			for _, item := range tcList {
				if tc, ok := item.(map[string]any); ok {
					if id, _ := tc["id"].(string); id == callID {
						if fn, ok := tc["function"].(map[string]any); ok {
							name, _ := fn["name"].(string)
							args, _ := fn["arguments"].(string)
							return name, args
						}
					}
				}
			}
		}
	}
	return "", ""
}

func extractToolResultCallID(content any) string {
	if list, ok := content.([]any); ok {
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if t, _ := m["type"].(string); t == "tool_result" {
					if id, ok := m["tool_use_id"].(string); ok && id != "" {
						return id
					}
				}
			}
		}
	}
	return ""
}

func extractContentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if txt, ok := m["text"].(string); ok && txt != "" {
					return txt
				}
			}
		}
	}
	return ""
}

func cleanSummary(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Extract real user prompt from common chat template wrappers
	if idx := strings.Index(s, "## My request:"); idx >= 0 {
		s = strings.TrimSpace(s[idx+len("## My request:"):])
	} else if idx := strings.Index(s, "My request:"); idx >= 0 {
		s = strings.TrimSpace(s[idx+len("My request:"):])
	}

	lines := strings.Split(s, "\n")
	found := ""
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		cleaned := strings.TrimLeft(trimmed, "#*-`> \t")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}
		if strings.HasPrefix(cleaned, "Files mentioned") ||
			strings.HasPrefix(cleaned, "Distinguish instructions") ||
			strings.HasPrefix(cleaned, "Image attachment") ||
			strings.HasPrefix(cleaned, "Response annotations") {
			continue
		}
		found = cleaned
		break
	}

	if found == "" {
		found = s
	}

	found = strings.TrimSpace(found)
	runes := []rune(found)
	if len(runes) > 30 {
		return string(runes[:30]) + "..."
	}
	return found
}
