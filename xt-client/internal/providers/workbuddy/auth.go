package workbuddy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"xiaotao/xt-client/internal/providers"
)

const (
	DefaultBaseURL = "https://copilot.tencent.com"
	authStatePath  = "/v2/plugin/auth/state"
	authTokenPath  = "/v2/plugin/auth/token"
	loginAccPath   = "/v2/plugin/login/account"
	refreshPath    = "/v2/plugin/auth/token/refresh"
	chatPath       = "/v2/chat/completions"
)

var (
	defaultClient = &http.Client{Timeout: 30 * time.Second}
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
	payload, err := post(ctx, baseURL+authStatePath+"?platform=VSCode", map[string]any{}, baseHeaders())
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

func WaitForAccount(ctx context.Context, baseURL, state, accessToken string) (string, string, string, string, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	deadline := time.Now().Add(time.Minute)
	if accessToken == "" {
		return "", "", "", "", fmt.Errorf("missing access token")
	}
	authHeaders := baseHeaders()
	authHeaders["Authorization"] = "Bearer " + accessToken
	if domain := tokenDomain(ctx, baseURL, state, accessToken); domain != "" {
		authHeaders["X-Domain"] = domain
	}
	authHeaders["X-No-User-Id"] = "true"
	authHeaders["X-No-Enterprise-Id"] = "true"
	authHeaders["X-No-Department-Info"] = "true"
	for {
		payload, err := get(ctx, baseURL+loginAccPath+"?state="+url.QueryEscape(state), authHeaders)
		if err == nil {
			uid, nickname, enterprise, department, parseErr := parseAccount(payload)
			if parseErr == nil && uid != "" {
				return uid, nickname, enterprise, department, nil
			}
		}
		if time.Now().After(deadline) {
			return "", "", "", "", fmt.Errorf("account information timed out")
		}
		select {
		case <-ctx.Done():
			return "", "", "", "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func tokenDomain(ctx context.Context, baseURL, state, accessToken string) string {
	headers := baseHeaders()
	headers["Authorization"] = "Bearer " + accessToken
	payload, err := get(ctx, baseURL+authTokenPath+"?state="+url.QueryEscape(state), headers)
	if err != nil {
		return ""
	}
	token, err := parseToken(payload)
	if err != nil {
		return ""
	}
	return token.Domain
}

func baseHeaders() map[string]string {
	return map[string]string{
		"User-Agent":     "Mozilla/5.0 (compatible; Genie-IDE/1.0)",
		"X-Product-Code": "codebuddy",
	}
}

func parseAccount(payload []byte) (string, string, string, string, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return "", "", "", "", fmt.Errorf("invalid account response")
	}
	obj, _ := unwrapData(raw).(map[string]any)
	if obj == nil {
		return "", "", "", "", fmt.Errorf("account response missing uid")
	}
	uid, _ := obj["uid"].(string)
	if uid == "" {
		if n := number(obj["uid"]); n != 0 {
			uid = fmt.Sprintf("%d", n)
		}
	}
	nickname, _ := obj["nickname"].(string)
	enterprise, _ := obj["enterpriseId"].(string)
	department, _ := obj["departmentInfo"].(string)
	if uid == "" {
		return "", "", "", "", fmt.Errorf("account response missing uid")
	}
	return uid, nickname, enterprise, department, nil
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
	return &providers.Credential{
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
	}, nil
}

func headersFor(cred *providers.Credential) map[string]string {
	machineID := cred.MachineID
	if machineID == "" {
		machineID = stableMachineID()
	}
	headers := map[string]string{
		"User-Agent":        "Mozilla/5.0 (compatible; Genie-IDE/1.0)",
		"X-Product-Code":    "codebuddy",
		"X-IDE-Type":        "vscode",
		"X-IDE-Name":        "Visual Studio Code",
		"X-IDE-Version":     "1.70.2",
		"X-Product-Version": "4.10.33259736",
		"X-Machine-Id":      machineID,
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
	if ms <= 0 {
		return 0
	}
	return ms / 1000
}

func stableMachineID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("wb-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
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
	return raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
