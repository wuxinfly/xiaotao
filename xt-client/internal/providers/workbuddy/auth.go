package workbuddy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

const (
	DefaultBaseURL = "https://www.workbuddy.cn"
	authStatePath  = "/v2/plugin/auth/state"
	authTokenPath  = "/v2/plugin/auth/token"
	refreshPath    = "/v2/plugin/auth/token/refresh"
	chatPath       = "/v2/chat/completions"
)

var (
	defaultClient = &http.Client{Timeout: 30 * time.Second}
	chatClient    = newChatClient()
	testBaseURL   string
	dsmlBuffers   = map[string]*dsmlBuffer{}
)

// SetBaseURL lets tests point the provider at a local httptest server.
func SetBaseURL(url string) func() {
	prev := testBaseURL
	testBaseURL = url
	return func() { testBaseURL = prev }
}

func client() *http.Client {
	return defaultClient
}

func newChatClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = idleTimeout
	return &http.Client{Transport: transport}
}

type authStateResponse struct {
	Data *struct {
		AuthURL string `json:"authUrl"`
		State   string `json:"state"`
	} `json:"data"`
}

type authStateData struct {
	AuthURL string
	State   string
}

func parseAuthState(payload []byte) (authStateData, error) {
	var outer authStateResponse
	if err := json.Unmarshal(payload, &outer); err != nil {
		return authStateData{}, fmt.Errorf("invalid auth state response")
	}
	if outer.Data != nil && outer.Data.AuthURL != "" && outer.Data.State != "" {
		return authStateData{outer.Data.AuthURL, outer.Data.State}, nil
	}
	var generic map[string]any
	if err := json.Unmarshal(payload, &generic); err == nil {
		obj, _ := unwrapData(generic).(map[string]any)
		if obj != nil {
			authURL, _ := obj["authUrl"].(string)
			state, _ := obj["state"].(string)
			if authURL != "" && state != "" {
				return authStateData{authURL, state}, nil
			}
		}
	}
	return authStateData{}, fmt.Errorf("auth state response missing authUrl or state")
}

func StartAuth(ctx context.Context, baseURL string) (string, string, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	payload, err := post(ctx, baseURL+authStatePath+"?platform=workbuddy", map[string]any{}, baseHeaders())
	if err != nil {
		return "", "", err
	}
	state, err := parseAuthState(payload)
	if err != nil {
		return "", "", err
	}
	return state.AuthURL, state.State, err
}

type credentialPayload struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	Domain       string `json:"domain"`
	EnterpriseID string `json:"enterpriseId"`
}

func parseToken(payload []byte) (credentialPayload, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return credentialPayload{}, fmt.Errorf("invalid token response")
	}
	value := unwrapData(raw)
	obj, _ := value.(map[string]any)
	if obj == nil {
		return credentialPayload{}, fmt.Errorf("token response missing accessToken")
	}
	p := credentialPayload{}
	p.AccessToken, _ = obj["accessToken"].(string)
	p.RefreshToken, _ = obj["refreshToken"].(string)
	p.ExpiresAt = msToSeconds(number(obj["expiresAt"]))
	p.Domain, _ = obj["domain"].(string)
	p.EnterpriseID, _ = obj["enterpriseId"].(string)
	if p.AccessToken == "" {
		return credentialPayload{}, fmt.Errorf("token response missing accessToken")
	}
	return p, nil
}

func WaitForToken(ctx context.Context, baseURL, state string) (*providers.Credential, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		payload, err := get(ctx, baseURL+authTokenPath+"?state="+url.QueryEscape(state), baseHeaders())
		if err == nil {
			token, parseErr := parseToken(payload)
			if parseErr == nil {
				return &providers.Credential{
					AccessToken:  token.AccessToken,
					RefreshToken: token.RefreshToken,
					ExpiresAt:    token.ExpiresAt,
					Region:       "cn",
					Domain:       token.Domain,
					EnterpriseID: token.EnterpriseID,
				}, nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("login authorization timed out")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func baseHeaders() map[string]string {
	return map[string]string{
		"User-Agent":       "WorkBuddy/5.8.3 WorkBuddy/5.8.3 CLI/2.161.0",
		"X-Product":        "SaaS",
		"X-Requested-With": "XMLHttpRequest",
		"Origin":           DefaultBaseURL,
		"Referer":          DefaultBaseURL + "/",
	}
}

func Refresh(ctx context.Context, baseURL string, cred *providers.Credential) (*providers.Credential, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if cred == nil || cred.RefreshToken == "" {
		return nil, fmt.Errorf("missing refresh token")
	}
	if testBaseURL == "" {
		if err := ValidateCredential(cred); err != nil {
			return nil, err
		}
	}
	headers := headersFor(cred)
	delete(headers, "Authorization")
	headers["X-Refresh-Token"] = cred.RefreshToken
	headers["X-Auth-Refresh-Source"] = "plugin"
	payload, err := post(ctx, baseURL+refreshPath, map[string]any{}, headers)
	if err != nil {
		return nil, err
	}
	token, err := parseToken(payload)
	if err != nil {
		return nil, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = cred.RefreshToken
	}
	next := &providers.Credential{
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		ExpiresAt:      token.ExpiresAt,
		UID:            cred.UID,
		Nickname:       cred.Nickname,
		MachineID:      cred.MachineID,
		Region:         cred.Region,
		Domain:         token.Domain,
		EnterpriseID:   cred.EnterpriseID,
		DepartmentInfo: cred.DepartmentInfo,
	}
	if next.Domain == "" {
		next.Domain = cred.Domain
	}
	if testBaseURL == "" {
		if err := NativeAccount(next); err != nil {
			return nil, err
		}
		if next.UID != cred.UID {
			return nil, fmt.Errorf("WorkBuddy refresh changed account identity")
		}
	}
	return next, nil
}

func headersFor(cred *providers.Credential) map[string]string {
	machineID := cred.MachineID
	if machineID == "" {
		machineID = stableMachineID()
	}
	headers := map[string]string{
		"User-Agent":          "WorkBuddy/5.8.3 WorkBuddy/5.8.3 CLI/2.161.0",
		"X-Requested-With":    "XMLHttpRequest",
		"X-IDE-Type":          "WorkBuddy",
		"X-IDE-Name":          "WorkBuddy",
		"X-IDE-Version":       "5.8.3",
		"X-Product":           "SaaS",
		"X-Agent-Intent":      "craft",
		"X-Agent-Purpose":     "conversation",
		"X-Agent-Type":        "main",
		"X-Request-ID":        stableMachineID(),
		"Origin":              DefaultBaseURL,
		"Referer":             DefaultBaseURL + "/",
		"X-CodeBuddy-Request": "1",
		"Accept-Language":     "zh-CN",
		"X-Machine-Id":        machineID,
	}
	if cred.UID != "" {
		headers["X-User-Id"] = cred.UID
	}
	if cred.AccessToken != "" {
		headers["Authorization"] = "Bearer " + cred.AccessToken
	}
	if cred.EnterpriseID != "" {
		headers["X-Enterprise-Id"] = cred.EnterpriseID
		headers["X-Tenant-Id"] = cred.EnterpriseID
	}
	if cred.DepartmentInfo != "" {
		headers["X-Department-Info"] = cred.DepartmentInfo
	}
	if cred.Domain != "" {
		headers["X-Domain"] = cred.Domain
	}
	return headers
}

func unwrapData(raw map[string]any) any {
	if data, ok := raw["data"].(map[string]any); ok {
		if nested, ok := data["data"].(map[string]any); ok {
			return nested
		}
		return data
	}
	return raw
}

func number(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return int64(n)
	case int:
		return int64(n)
	case json.Number:
		if parsed, err := n.Int64(); err == nil {
			return parsed
		}
	}
	return 0
}

func msToSeconds(ms int64) int64 {
	if ms > 0 && ms < 100000000000 {
		return ms
	}
	if ms <= 0 {
		return 0
	}
	return ms / 1000
}

func stableMachineID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func post(ctx context.Context, endpoint string, body any, headers map[string]string) ([]byte, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return do(req)
}

func get(ctx context.Context, endpoint string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return do(req)
}

func do(req *http.Request) ([]byte, error) {
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, statusError(resp.StatusCode, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200)))
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) == nil {
		if code := number(envelope["code"]); code != 0 {
			return nil, fmt.Errorf("WorkBuddy upstream code %d", code)
		}
	}
	return raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// NativeAccount reads metadata only; the upstream verifies the JWT signature.
func NativeAccount(cred *providers.Credential) error {
	parts := strings.Split(cred.AccessToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("WorkBuddy token 缺少账号身份")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid WorkBuddy token metadata")
	}
	var claims struct {
		Sub      string `json:"sub"`
		Exp      int64  `json:"exp"`
		Nickname string `json:"nickname"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Sub == "" {
		return fmt.Errorf("WorkBuddy token 缺少账号身份")
	}
	cred.UID = claims.Sub
	if cred.Nickname == "" {
		cred.Nickname = claims.Nickname
	}
	if cred.ExpiresAt == 0 {
		cred.ExpiresAt = claims.Exp
	}
	if cred.Domain == "" {
		cred.Domain = "www.workbuddy.cn"
	}
	return ValidateCredential(cred)
}

func ValidateCredential(cred *providers.Credential) error {
	if cred == nil || cred.AccessToken == "" {
		return fmt.Errorf("missing WorkBuddy access token")
	}
	if cred.Domain != "www.workbuddy.cn" {
		return fmt.Errorf("此账号不是 WorkBuddy 国内原生登录，请在账号页重新登录 WorkBuddy")
	}
	return nil
}

func billingHeaders(cred *providers.Credential) map[string]string {
	h := headersFor(cred)
	for _, key := range []string{"X-IDE-Type", "X-IDE-Name", "X-IDE-Version", "X-Agent-Intent", "X-Agent-Type", "X-Agent-Purpose", "X-Machine-Id"} {
		delete(h, key)
	}
	h["User-Agent"] = "WorkBuddy/5.8.3"
	h["Accept-Language"] = "zh"
	return h
}

func chatHeaders(cred *providers.Credential) map[string]string {
	h := headersFor(cred)
	h["X-Stainless-Lang"] = "js"
	h["X-Stainless-Package-Version"] = "6.25.0"
	h["X-Stainless-OS"] = "Windows"
	h["X-Stainless-Arch"] = "x64"
	h["X-Stainless-Runtime"] = "node"
	h["X-Stainless-Runtime-Version"] = "v22.21.1"
	h["X-Stainless-Retry-Count"] = "0"
	h["X-Stainless-Timeout"] = "600"
	if cred.EnterpriseID != "" {
		raw, _ := json.Marshal(map[string]string{"uin": cred.UID, "owner_uin": cred.EnterpriseID})
		h["X-Userinfo"] = base64.StdEncoding.EncodeToString(raw)
	}
	return h
}
