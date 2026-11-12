package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/sourceplugin"
	"xiaotao/xt-client/internal/storage"
)

func rpcErrorFor(err error) *sourceplugin.RPCError {
	var rpc *sourceplugin.RPCError
	if errors.As(err, &rpc) {
		return rpc
	}
	var httpErr *trae.HTTPError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &sourceplugin.RPCError{Code: sourceplugin.RPCTimeout, Message: "upstream timeout"}
	case errors.As(err, &httpErr) && (httpErr.Code == http.StatusUnauthorized || httpErr.Code == http.StatusForbidden):
		return &sourceplugin.RPCError{Code: sourceplugin.RPCAuthExpired, Message: "login expired"}
	case errors.As(err, &httpErr) && httpErr.Code == http.StatusTooManyRequests:
		return &sourceplugin.RPCError{Code: sourceplugin.RPCRateLimited, Message: "upstream rate limited"}
	case errors.As(err, &httpErr):
		return &sourceplugin.RPCError{Code: sourceplugin.RPCUpstream, Message: "upstream request failed"}
	default:
		return &sourceplugin.RPCError{Code: sourceplugin.RPCPluginError, Message: "operation failed"}
	}
}

func checkinAccount(op sourceplugin.Operation) string {
	if account := strings.TrimSpace(op.Settings["checkinAccount"]); account != "" {
		return account
	}
	return "xiaotao.trae"
}

func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }

var supportedMethods = map[string]bool{
	"login/prepare": true, "login/complete": true, "login/browser": true, "login/import": true,
	"credentials/import": true, "credentials/refresh": true, "models/refresh": true,
	"quota/get": true, "checkin/status": true, "checkin/claim": true, "chat/start": true,
}

func operate(ctx context.Context, method string, op sourceplugin.Operation, emit func(map[string]any)) (sourceplugin.Result, error) {
	if !supportedMethods[method] {
		return sourceplugin.Result{}, &sourceplugin.RPCError{Code: sourceplugin.RPCUnsupported, Message: "unsupported method"}
	}
	var c providers.Credential
	if len(op.Credential) > 0 && json.Unmarshal(op.Credential, &c) != nil {
		return sourceplugin.Result{}, errors.New("invalid credential")
	}
	result := sourceplugin.Result{}
	if method == "login/prepare" {
		value, _ := json.Marshal(sourceplugin.LoginDescriptor{
			URL:         "https://" + trae.RegionOf("cn").AuthHost + "/authorization?login_version=1&auth_from=solo&login_channel=native_ide&plugin_version=trae-handoff-1.0&auth_type=local&client_id=" + trae.ClientID() + "&redirect=0&login_trace_id={state}&auth_callback_url={callback}",
			StateParam:  "login_trace_id",
			ResultParam: "refreshToken",
			Callback:    "/authorize",
		})
		result.Value = value
		return result, nil
	}
	if method == "login/complete" {
		var input struct {
			Callback map[string]string `json:"callback"`
		}
		if json.Unmarshal(op.Input, &input) != nil {
			return result, errors.New("invalid login callback")
		}
		query := url.Values{}
		for key, value := range input.Callback {
			query.Set(key, value)
		}
		token := trae.ExtractRefreshToken("?" + query.Encode())
		if token == "" {
			return result, errors.New("login result missing")
		}
		method = "login/import"
		op.Input, _ = json.Marshal(map[string]string{"refreshToken": token})
	}
	if method == "login/browser" {
		token, err := browserLogin(ctx)
		if err != nil {
			return result, err
		}
		method = "login/import"
		op.Input, _ = json.Marshal(map[string]string{"refreshToken": token})
	}
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
		result.Account = accountInfo(c)
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
		next.UID = c.UID
		next.Nickname = c.Nickname
		next.MachineID = c.MachineID
		next.DeviceID = c.DeviceID
		c = *next
	}
	if method == "login/import" {
		if uid, name, err := trae.UserInfo(&c); err == nil {
			c.UID, c.Nickname = uid, name
		}
	}
	if c.UID == "" {
		c.UID = trae.JWTUserID(c.AccessToken)
		if c.UID == "" {
			uid, name, err := trae.UserInfo(&c)
			if err == nil {
				c.UID, c.Nickname = uid, name
			}
		}
	}
	// Persist refreshed credentials even when the subsequent action fails: host
	// performs refresh as a separate operation before model requests.
	if method == "login/import" && c.UID == "" {
		return result, errors.New("Trae account identity unavailable")
	}
	result.Credential, _ = json.Marshal(c)
	result.Account = accountInfo(c)
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
			result.Models = append(result.Models, sourceplugin.Model{Key: m.ID, UpstreamID: upstream, Label: m.DisplayName, ContextWindow: m.MaxInputTokens, SupportsImages: m.Multimodal, SupportsReasoning: m.Reasoning, ReasoningEfforts: m.Efforts, PriceFactor: m.PriceFactor})
		}
	case "quota/get":
		value, err := trae.FetchQuota(&c, c.MachineID, c.DeviceID)
		if err != nil {
			return result, err
		}
		result.Value, _ = json.Marshal(value)
		result.Display = quotaDisplay(value)
	case "checkin/status":
		value, err := trae.FetchCheckinStatus(&c, checkinAccount(op))
		if err != nil {
			return result, err
		}
		result.Value, _ = json.Marshal(value)
		result.Display = checkinDisplay(value)
	case "checkin/claim":
		value, err := trae.ClaimCheckin(&c, checkinAccount(op))
		if err != nil {
			return result, err
		}
		result.Value, _ = json.Marshal(value)
		result.Display = claimDisplay(value)
	case "chat/start":
		if c.UID == "" {
			return result, errors.New("cannot determine Trae account identity")
		}
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
		return result, &sourceplugin.RPCError{Code: sourceplugin.RPCUnsupported, Message: "unsupported method"}
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

func accountInfo(c providers.Credential) *sourceplugin.AccountInfo {
	if c.UID == "" {
		return nil
	}
	name := c.Nickname
	if name == "" {
		name = "用户 " + c.UID
	}
	return &sourceplugin.AccountInfo{ID: c.UID, Name: name}
}

// quotaDisplay turns platform quota data into host-renderable text. The host
// never interprets platform-specific meanings.
func quotaDisplay(snapshot *storage.QuotaSnapshot) *sourceplugin.Display {
	if snapshot == nil || len(snapshot.Items) == 0 {
		return nil
	}
	display := &sourceplugin.Display{}
	for _, item := range snapshot.Items {
		entry := sourceplugin.DisplayItem{Label: item.Label, Value: quotaValue(item)}
		if item.Used != nil && item.Total != nil && *item.Total > 0 {
			percent := *item.Used / *item.Total * 100
			if percent < 0 {
				percent = 0
			}
			if percent > 100 {
				percent = 100
			}
			display.Items = append(display.Items, sourceplugin.DisplayItem{Label: entry.Label, Value: entry.Value, Percent: &percent})
			continue
		}
		display.Items = append(display.Items, entry)
	}
	if len(snapshot.Items) > 0 {
		display.Summary = quotaValue(snapshot.Items[0])
	}
	return display
}

func quotaValue(item storage.QuotaItem) string {
	unit := "积分"
	if item.Unit == "dollar" {
		unit = "美元"
	}
	if item.Used != nil && item.Total != nil {
		return fmt.Sprintf("%s / %s %s", trimFloat(*item.Used), trimFloat(*item.Total), unit)
	}
	if item.Total != nil {
		return fmt.Sprintf("%s %s", trimFloat(*item.Total), unit)
	}
	return "额度未知"
}

func trimFloat(value float64) string {
	if value == math.Trunc(value) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func checkinDisplay(status *trae.CheckinStatus) *sourceplugin.Display {
	if status == nil {
		return nil
	}
	text := "今日尚未签到"
	switch {
	case !status.Supported:
		text = "当前账号不支持签到"
	case !status.Active:
		text = "签到活动未开启"
	case status.CheckedIn:
		text = "今日已签到"
	}
	display := &sourceplugin.Display{Summary: text}
	if status.DailyCredit > 0 {
		display.Items = append(display.Items, sourceplugin.DisplayItem{Label: "每日积分", Value: trimFloat(status.DailyCredit)})
	}
	return display
}

func claimDisplay(claim *trae.CheckinClaimResult) *sourceplugin.Display {
	if claim == nil {
		return nil
	}
	display := &sourceplugin.Display{Summary: "签到请求完成"}
	if claim.ExtraCredits > 0 {
		display.Summary = "签到成功"
		display.Items = append(display.Items, sourceplugin.DisplayItem{Label: "获得积分", Value: trimFloat(claim.ExtraCredits)})
	}
	return display
}
