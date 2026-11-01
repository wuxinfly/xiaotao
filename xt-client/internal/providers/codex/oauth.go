package codex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	AuthURL      = "https://auth.openai.com/oauth/authorize"
	TokenURL     = "https://auth.openai.com/oauth/token"
	ClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	RedirectURI  = "http://localhost:1455/auth/callback"
	CallbackPort = 1455
	CallbackPath = "/auth/callback"
	Scope        = "openid email profile offline_access"
)

type PKCECodes struct {
	Verifier  string
	Challenge string
}

type TokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	IDToken      string  `json:"id_token"`
	ExpiresIn    float64 `json:"expires_in"`
	TokenType    string  `json:"token_type"`
	Scope        string  `json:"scope"`
}

func GeneratePKCE() (*PKCECodes, error) {
	b := make([]byte, 96)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])
	return &PKCECodes{Verifier: verifier, Challenge: challenge}, nil
}

func GenerateState() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func BuildAuthorizeURL(pkce *PKCECodes, state string) string {
	q := url.Values{}
	q.Set("client_id", ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", RedirectURI)
	q.Set("scope", Scope)
	q.Set("state", state)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("prompt", "login")
	return AuthURL + "?" + q.Encode()
}

func ParseCallbackInput(raw string) (string, string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", "", errors.New("粘贴内容为空")
	}
	if strings.Contains(text, "=") || strings.Contains(text, "://") {
		query := text
		if idx := strings.Index(text, "?"); idx >= 0 {
			query = text[idx+1:]
		}
		fields := strings.Fields(query)
		if len(fields) > 0 {
			query = fields[0]
		}
		vals, err := url.ParseQuery(query)
		if err != nil {
			return "", "", fmt.Errorf("解析参数失败: %w", err)
		}
		code := vals.Get("code")
		state := vals.Get("state")
		if code == "" {
			errName := vals.Get("error")
			errDesc := vals.Get("error_description")
			if errName != "" {
				if errDesc != "" {
					return "", "", fmt.Errorf("授权失败: %s: %s", errName, errDesc)
				}
				return "", "", fmt.Errorf("授权失败: %s", errName)
			}
			return "", "", errors.New("回调 URL 里没有 code 参数")
		}
		return code, state, nil
	}
	return text, "", nil
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func postForm(ctx context.Context, data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errObj struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			Message          string `json:"message"`
		}
		_ = json.Unmarshal(body, &errObj)
		detail := errObj.ErrorDescription
		if detail == "" {
			detail = errObj.Error
		}
		if detail == "" {
			detail = errObj.Message
		}
		if detail == "" {
			detail = string(body)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, detail)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("响应不是合法 JSON: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("响应缺少 access_token")
	}
	return &tokenResp, nil
}

func ExchangeCode(ctx context.Context, code, verifier string) (*TokenResponse, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("授权码为空")
	}
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", ClientID)
	data.Set("code", code)
	data.Set("redirect_uri", RedirectURI)
	data.Set("code_verifier", verifier)
	return postForm(ctx, data)
}

func RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errors.New("refresh_token 为空")
	}
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", ClientID)
	data.Set("refresh_token", refreshToken)
	data.Set("scope", "openid profile email")
	return postForm(ctx, data)
}

func DecodeIDTokenClaims(idToken string) map[string]any {
	token := strings.TrimSpace(idToken)
	if token == "" {
		return map[string]any{}
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	payload := parts[1]
	if rem := len(payload) % 4; rem != 0 {
		payload += strings.Repeat("=", 4-rem)
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return map[string]any{}
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return map[string]any{}
	}
	return claims
}

func nestedClaim(claims map[string]any, key string) any {
	if val, ok := claims[key]; ok {
		return val
	}
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if val, ok := auth[key]; ok {
			return val
		}
	}
	return nil
}

func AccountIDFromClaims(claims map[string]any) string {
	for _, key := range []string{"chatgpt_account_id", "chatgptaccountid"} {
		if val, ok := nestedClaim(claims, key).(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if org, ok := auth["organization_id"].(string); ok && strings.TrimSpace(org) != "" {
			return strings.TrimSpace(org)
		}
	}
	if sub, ok := claims["sub"].(string); ok && strings.TrimSpace(sub) != "" {
		return strings.TrimSpace(sub)
	}
	return ""
}

func EmailFromClaims(claims map[string]any) string {
	for _, key := range []string{"email", "preferred_username"} {
		if val, ok := claims[key].(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func PlanTypeFromClaims(claims map[string]any) string {
	if val, ok := nestedClaim(claims, "chatgpt_plan_type").(string); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return "free"
}

func UserIDFromClaims(claims map[string]any) string {
	if val, ok := nestedClaim(claims, "chatgpt_user_id").(string); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	if sub, ok := claims["sub"].(string); ok && strings.TrimSpace(sub) != "" {
		return strings.TrimSpace(sub)
	}
	return ""
}
