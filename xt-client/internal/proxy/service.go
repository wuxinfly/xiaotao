package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/antigravity"
	"xiaotao/xt-client/internal/providers/codex"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

type Service struct {
	store     *storage.Store
	secret    func(id string) ([]byte, error)
	usage     *usage.Service
	mu        sync.Mutex
	key       string
	catalogs  map[string]catalogSnapshot
	cooldowns map[string]time.Time
	collect   func(context.Context, *providers.Credential, string, map[string]any) (map[string]any, error)
	stream    func(context.Context, *providers.Credential, string, map[string]any, *providers.ChatEvents) error
	traeUsage func(context.Context, *providers.Credential, string) (map[string]any, error)
}

type catalogSnapshot struct {
	region   string
	revision string
	models   []providers.ModelInfo
	at       time.Time
	source   string
}

func New(store *storage.Store, secret func(id string) ([]byte, error), apiKey string) *Service {
	return &Service{
		store: store, secret: secret, key: apiKey,
		catalogs: map[string]catalogSnapshot{}, cooldowns: map[string]time.Time{},
		traeUsage: trae.SessionUsage,
	}
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
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, acct := range s.store.Accounts() {
		if !acct.Enabled {
			continue
		}
		models, source := s.AccountModelCatalog(*acct)
		if acct.Provider == "qoder" && source == "error" {
			return nil, fmt.Errorf("Qoder model directory unavailable")
		}
		for _, m := range models {
			id := acct.Provider + "/" + m.ID
			if seen[id] {
				continue
			}
			seen[id] = true
			// Keep metadata from the first catalog in account routing order.
			// A union could advertise capabilities the selected account cannot serve.
			entry := map[string]any{
				"id": id, "object": "model", "created": 0, "owned_by": "xiaotao",
				"supports_images": m.Multimodal, "supports_reasoning": m.Reasoning,
			}
			if m.PriceFactor != nil {
				entry["price_factor"] = *m.PriceFactor
			}
			if m.IsFree != nil {
				entry["is_free"] = *m.IsFree
			}
			if m.MaxInputTokens > 0 {
				entry["max_input_tokens"] = m.MaxInputTokens
			}
			out = append(out, entry)
		}
	}
	return out, nil
}

func listProviderModels(acct storage.Account) []providers.ModelInfo {
	p, err := providers.Get(acct.Provider)
	if err != nil {
		return []providers.ModelInfo{}
	}
	return p.ListModels(acct.Region)
}

// AccountModels uses the account catalog when available, otherwise the static fallback.
func (s *Service) AccountModels(acct storage.Account) []providers.ModelInfo {
	models, _ := s.AccountModelCatalog(acct)
	return models
}

// AccountModelCatalog reads the shared catalog, fetching only when absent.
func (s *Service) AccountModelCatalog(acct storage.Account) ([]providers.ModelInfo, string) {
	models, source, _ := s.accountModelCatalog(acct, false)
	return models, source
}

func (s *Service) chatModelRoutes(acct storage.Account) map[string]string {
	models := s.AccountModels(acct)
	routes := map[string]string{}
	for _, m := range models {
		if m.UpstreamID != "" {
			routes[m.ID] = m.UpstreamID
		}
		for effort, route := range m.Routes {
			routes[m.ID+"@"+effort] = route
		}
	}
	return routes
}

func providerCollector(provider string) (providers.ChatCollector, bool) {
	switch provider {
	case "antigravity":
		return antigravity.Collector{}, true
	case "codex":
		return codex.Collector{}, true
	case "workbuddy":
		return workbuddy.Collector{}, true
	}
	return nil, false
}

func (s *Service) HandleChat(w http.ResponseWriter, r *http.Request) {
	if !s.Authorize(r) {
		logChatAuthFail()
		writeError(w, 401, "invalid_api_key", "代理访问密钥无效")
		return
	}
	var requestBody json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&requestBody); err != nil {
		logChatParseFail("request_body_decode_failed")
		writeError(w, 400, "invalid_request_error", "请求体无法解析")
		return
	}
	var body struct {
		ReasoningEffort     string          `json:"reasoning_effort"`
		Model               string          `json:"model"`
		Stream              bool            `json:"stream"`
		MaxTokens           int64           `json:"max_tokens"`
		MaxCompletionTokens int64           `json:"max_completion_tokens"`
		Messages            json.RawMessage `json:"messages"`
		Tools               json.RawMessage `json:"tools"`
		ToolChoice          any             `json:"tool_choice"`
	}
	if err := json.Unmarshal(requestBody, &body); err != nil {
		logChatParseFail("json_unmarshal_failed")
		writeError(w, 400, "invalid_request_error", "请求体无法解析")
		return
	}
	var msgs []map[string]any
	if err := json.Unmarshal(body.Messages, &msgs); err != nil || len(msgs) == 0 {
		logChatParseFail("messages_invalid")
		writeError(w, 400, "invalid_request_error", "messages 无效")
		return
	}
	var tools []map[string]any
	if len(body.Tools) > 0 {
		if err := json.Unmarshal(body.Tools, &tools); err != nil {
			logChatParseFail("tools_invalid")
			writeError(w, 400, "invalid_request_error", "tools 无效")
			return
		}
	}

	id := "chatcmpl-" + randHex()
	sessionH := r.Header.Get("X-Xiaotao-Session")
	runH := r.Header.Get("X-Xiaotao-Run")
	ctx := r.Context()
	logChatEntry(id, logging.ReqID(ctx), body.Model, body.Stream, len(msgs), len(tools), sessionH, runH)

	acct, model, ok := s.SelectAccount(body.Model)
	if body.Model == "" {
		logChatParseFail("missing_model")
		writeError(w, 400, "invalid_request_error", "缺少 model")
		return
	}
	cands, _ := s.accountCandidates(body.Model)
	logAccountSelected(id, acct.Provider, acct.ID, len(cands))

	maxTokens := body.MaxTokens
	if maxTokens <= 0 {
		maxTokens = body.MaxCompletionTokens
	}
	providerID, _ := splitModelID(body.Model)
	created := time.Now().Unix()

	var reqTracker *usage.ActiveRequest
	if s.usage != nil {
		acctID := ""
		channel := "trae"
		if acct.Provider != "" {
			channel = acct.Provider
		}
		acctID = acct.ID
		tag, title := detectAction(msgs, tools)
		reqTracker = s.usage.StartRequest(id, channel, body.Model, acctID, tag, title)
	}

	if providerID == "antigravity" {
		s.handleAntigravityWithFailover(ctx, w, reqTracker, id, created, body.Model, body.Stream, msgs, tools, body.ToolChoice, body.ReasoningEffort, maxTokens)
		return
	}
	if !ok {
		if reqTracker != nil {
			reqTracker.OnError("该模型没有可用的已启用账号")
		}
		writeError(w, 409, "no_ready_account", "该模型没有可用的已启用账号")
		return
	}
	credBytes, err := s.secret(acct.ID)
	if err != nil {
		logCredError(acct.ID, "read_secret_failed", err)
		if reqTracker != nil {
			reqTracker.OnError("账号凭据不可用")
		}
		writeError(w, 409, "account_unavailable", "账号凭据不可用")
		return
	}
	var cred providers.Credential
	if err := json.Unmarshal(credBytes, &cred); err != nil {
		logCredError(acct.ID, "json_unmarshal_failed", err)
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
		err := s.refreshCredential(&acct, &cred)
		logCredRefresh(acct.ID, err == nil)
		if err != nil {
			_ = s.store.UpdateAccount(acct.ID, func(a *storage.Account) { a.AuthState = "expired" })
			if reqTracker != nil {
				reqTracker.OnError("账号登录已失效，请重新登录")
			}
			writeError(w, 401, "account_unauthorized", "账号登录已失效，请重新登录")
			return
		}
	}
	if acct.Provider == "qoder" {
		s.handleQoder(ctx, w, reqTracker, acct, &cred, model, body.Stream, requestBody)
		return
	}
	var upstream map[string]any
	sessionID := ""
	if acct.Provider == "trae" || acct.Provider == "antigravity" {
		cred.ModelRoutes = s.chatModelRoutes(acct)
		if acct.Provider == "trae" {
			if route := cred.ModelRoutes[model]; route != "" {
				model = route
			} else {
				model = trae.UpstreamModel(model)
			}
		}
	}
	if acct.Provider == "workbuddy" || acct.Provider == "antigravity" || acct.Provider == "codex" {
		upstream = map[string]any{"messages": msgs}
		if reqChoice := normalizeToolChoice(body.ToolChoice); reqChoice != "" {
			upstream["tool_choice"] = reqChoice
		}
		if len(tools) > 0 {
			upstream["tools"] = tools
		}
	} else {
		sessionID = randHex()
		upstream = map[string]any{
			"function": "chat_v3", "request_id": sessionID, "session_id": sessionID,
		}
		upstream["messages"] = trae.NativeMessages(msgs)
		if len(body.Tools) > 0 {
			upstream["tools"] = convertTools(tools)
		}
	}
	if body.ReasoningEffort != "" {
		upstream["reasoning_effort"] = providerEffort(acct.Provider, body.ReasoningEffort)
	}
	if acct.Provider == "trae" && trae.IsMaxModel(body.Model) {
		if window := trae.MaxWindow(trae.BaseModel(body.Model), s.AccountModels(acct)); window > 0 {
			upstream["user_message_context"] = map[string]any{"model_info": map[string]any{"prompt_max_tokens": window - maxTokens}}
		}
	}
	if maxTokens > 0 {
		upstream["max_tokens"] = maxTokens
	}
	if body.Stream {
		s.streamLive(ctx, reqTracker, w, id, created, body.Model, &cred, acct, acct.MachineID, acct.DeviceID, model, upstream, sessionID)
		return
	}
	if collector, ok := providerCollector(acct.Provider); ok {
		result, err := collector.Collect(ctx, &cred, model, upstream)
		if err != nil {
			if reqTracker != nil {
				if ctx.Err() != nil {
					reqTracker.OnCanceled()
				} else {
					reqTracker.OnError(err.Error())
				}
			}
			writeError(w, upstreamHTTPStatus(err), "upstream_error", redact(err.Error()))
			return
		}
		if usageMap, ok := result["usage"].(map[string]any); ok {
			s.recordUsage(reqTracker, acct, usageMap)
		}
		if reqTracker != nil {
			reqTracker.OnCompleted()
		}
		writeJSON(w, result)
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
	if acct.Provider == "trae" && result.Usage != nil {
		result.Usage = s.enrichTraeUsage(ctx, &cred, traeUsageSessionID(result.SessionID, sessionID), result.Usage)
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

func (s *Service) recordUsage(reqTracker *usage.ActiveRequest, acct storage.Account, usageMap map[string]any) {
	if reqTracker != nil {
		var pPtr, cPtr, caPtr *int64
		if p, ok := usageMap["prompt_tokens"].(int64); ok {
			pPtr = &p
		}
		if c, ok := usageMap["completion_tokens"].(int64); ok {
			cPtr = &c
		}
		if ca, ok := usageMap["cached_tokens"].(int64); ok {
			caPtr = &ca
		}
		reqTracker.OnUsage(pPtr, cPtr, caPtr)
	}
	if prompt, ok := usageMap["prompt_tokens"].(int64); ok {
		var completion int64
		if v, ok := usageMap["completion_tokens"].(int64); ok {
			completion = v
		}
		_ = s.store.UpdateAccount(acct.ID, func(a *storage.Account) {
			a.UsageTokens += prompt + completion
			a.UsageCalls++
		})
	}
}

// streamLive proxies the upstream SSE stream incrementally: each upstream
// delta is converted to an OpenAI chunk and flushed immediately, so
// downstream clients see tokens as they are generated.
func (s *Service) streamLive(ctx context.Context, reqTracker *usage.ActiveRequest, w http.ResponseWriter, id string, created int64, model string, cred *providers.Credential, acct storage.Account, machineID, deviceID, modelID string, upstream map[string]any, upstreamSessionID string) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)
	writeChunk := func(payload map[string]any) { writeSSEChunk(w, flusher, payload) }
	chunk := func(delta map[string]any, finish any) {
		writeChunk(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		})
	}
	chunk(map[string]any{"role": "assistant", "content": ""}, nil)
	var usage atomic.Value
	var upstreamErr atomic.Value
	var events *providers.ChatEvents
	if acct.Provider == "workbuddy" || acct.Provider == "antigravity" || acct.Provider == "codex" {
		events = &providers.ChatEvents{
			OnFirstToken: func() {
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
				}
			},
			OnError: func(status int, msg string) {
				upstreamErr.Store(struct {
					code any
					msg  string
				}{status, msg})
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
		var err error
		if acct.Provider == "antigravity" {
			err = antigravity.Streamer{}.Stream(ctx, cred, modelID, upstream, events)
		} else if acct.Provider == "codex" {
			err = codex.Streamer{}.Stream(ctx, cred, modelID, upstream, events)
		} else {
			err = workbuddy.Streamer{}.Stream(ctx, cred, modelID, upstream, events)
		}
		if err != nil {
			if reqTracker != nil {
				if ctx.Err() != nil {
					reqTracker.OnCanceled()
				} else {
					reqTracker.OnInterrupted(err.Error())
				}
			}
			chunk(map[string]any{"content": "\n\n[upstream error: " + redact(err.Error()) + "]"}, nil)
		} else if v, ok := upstreamErr.Load().(struct {
			code any
			msg  string
		}); ok && v.msg != "" {
			if reqTracker != nil {
				reqTracker.OnError(v.msg)
			}
			chunk(map[string]any{"content": "\n\n[" + redact(v.msg) + "]"}, nil)
		} else if reqTracker != nil {
			reqTracker.OnCompleted()
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
		return
	}
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
		OnSessionID: func(id string) {
			upstreamSessionID = id
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
	}
	if v, ok := upstreamErr.Load().(struct {
		code any
		msg  string
	}); ok && v.msg != "" {
		chunk(map[string]any{"content": "\n\n[" + redact(v.msg) + "]"}, nil)
	}
	final, _ := usage.Load().(map[string]any)
	if acct.Provider == "trae" && len(final) > 0 {
		final = s.enrichTraeUsage(ctx, cred, upstreamSessionID, final)
		usage.Store(final)
		if reqTracker != nil {
			var pPtr, cPtr, caPtr *int64
			if p, ok := final["prompt_tokens"].(int64); ok {
				pPtr = &p
			}
			if c, ok := final["completion_tokens"].(int64); ok {
				cPtr = &c
			}
			if ca, ok := final["cached_tokens"].(int64); ok {
				caPtr = &ca
			}
			reqTracker.OnUsage(pPtr, cPtr, caPtr)
		}
	}
	if err == nil {
		if v, ok := upstreamErr.Load().(struct {
			code any
			msg  string
		}); !ok || v.msg == "" {
			if reqTracker != nil {
				reqTracker.OnCompleted()
			}
		}
	}
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
	return refreshStoredCredential(context.Background(), s.store, s.secret, *acct, cred)
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

func upstreamHTTPStatus(err error) int {
	var upstream *providers.UpstreamError
	if errors.As(err, &upstream) {
		return upstream.Status
	}
	var httpStatus interface{ UpstreamStatus() int }
	if errors.As(err, &httpStatus) {
		return httpStatus.UpstreamStatus()
	}
	return 502
}

// normalizeToolChoice collapses the structured OpenAI tool_choice form
// ({"type":"function","function":{"name":"x"}}) into the plain string that
// OpenAI-compatible upstreams expect ("x"). Valid string values pass through.
func normalizeToolChoice(choice any) string {
	switch value := choice.(type) {
	case string:
		return value
	case map[string]any:
		if value["type"] != "function" {
			return ""
		}
		fn, _ := value["function"].(map[string]any)
		name, _ := fn["name"].(string)
		return name
	}
	return ""
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
