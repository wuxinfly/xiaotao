package codex

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestPKCEAndAuthorizeURL(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}
	if len(pkce.Verifier) < 43 || pkce.Challenge == "" {
		t.Fatalf("invalid pkce: %+v", pkce)
	}

	state := GenerateState()
	authURL := BuildAuthorizeURL(pkce, state)

	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != ClientID {
		t.Fatalf("client_id mismatch: %s", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != RedirectURI {
		t.Fatalf("redirect_uri mismatch: %s", q.Get("redirect_uri"))
	}
	if q.Get("prompt") != "login" {
		t.Fatalf("prompt mismatch: %s", q.Get("prompt"))
	}
	if q.Get("state") != state {
		t.Fatalf("state mismatch: %s", q.Get("state"))
	}
	if q.Get("code_challenge") != pkce.Challenge {
		t.Fatalf("challenge mismatch")
	}
}

func TestParseCallbackInput(t *testing.T) {
	// Full callback URL
	code, state, err := ParseCallbackInput("http://localhost:1455/auth/callback?code=abc-123&state=xyz-789")
	if err != nil || code != "abc-123" || state != "xyz-789" {
		t.Fatalf("full URL parse failed: %s %s %v", code, state, err)
	}

	// Query only
	code, state, err = ParseCallbackInput("code=mycode&state=mystate")
	if err != nil || code != "mycode" || state != "mystate" {
		t.Fatalf("query parse failed: %s %s %v", code, state, err)
	}

	// Bare code
	code, state, err = ParseCallbackInput("bare-code-value")
	if err != nil || code != "bare-code-value" || state != "" {
		t.Fatalf("bare code parse failed: %s %s %v", code, state, err)
	}

	// Error callback
	_, _, err = ParseCallbackInput("http://localhost:1455/auth/callback?error=access_denied&error_description=User+cancelled")
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("expected error callback to fail, got %v", err)
	}

	// Empty input
	_, _, err = ParseCallbackInput("   ")
	if err == nil {
		t.Fatalf("expected error for empty input")
	}
}

func TestDecodeIDTokenClaims(t *testing.T) {
	claims := map[string]any{
		"email": "test@example.com",
		"sub":   "user-sub-123",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-openai-999",
			"chatgpt_plan_type":  "plus",
			"chatgpt_user_id":    "user-xyz",
		},
	}
	raw, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	fakeJWT := "header." + payload + ".sig"

	decoded := DecodeIDTokenClaims(fakeJWT)
	if EmailFromClaims(decoded) != "test@example.com" {
		t.Fatalf("email mismatch: %s", EmailFromClaims(decoded))
	}
	if AccountIDFromClaims(decoded) != "acct-openai-999" {
		t.Fatalf("account_id mismatch: %s", AccountIDFromClaims(decoded))
	}
	if PlanTypeFromClaims(decoded) != "plus" {
		t.Fatalf("plan mismatch: %s", PlanTypeFromClaims(decoded))
	}
	if UserIDFromClaims(decoded) != "user-xyz" {
		t.Fatalf("user_id mismatch: %s", UserIDFromClaims(decoded))
	}

	// Token without chatgpt_account_id falls back to sub
	fallbackClaims := map[string]any{
		"sub": "fallback-sub",
	}
	rawFB, _ := json.Marshal(fallbackClaims)
	payloadFB := base64.RawURLEncoding.EncodeToString(rawFB)
	decodedFB := DecodeIDTokenClaims("h." + payloadFB + ".s")
	if AccountIDFromClaims(decodedFB) != "fallback-sub" {
		t.Fatalf("fallback account_id mismatch: %s", AccountIDFromClaims(decodedFB))
	}
}

func TestCredFromTokenResponse(t *testing.T) {
	claims := map[string]any{
		"email":              "alex@example.com",
		"chatgpt_account_id": "org_alex",
		"chatgpt_plan_type":  "team",
	}
	raw, _ := json.Marshal(claims)
	jwt := "h." + base64.RawURLEncoding.EncodeToString(raw) + ".s"

	resp := &TokenResponse{
		AccessToken:  "at-1",
		RefreshToken: "rt-1",
		IDToken:      jwt,
		ExpiresIn:    3600,
	}

	cred := CredFromTokenResponse(resp)
	if cred.AccessToken != "at-1" || cred.RefreshToken != "rt-1" {
		t.Fatalf("tokens mismatch")
	}
	if cred.UID != "org_alex" || cred.EnterpriseID != "org_alex" {
		t.Fatalf("UID or EnterpriseID mismatch: %s / %s", cred.UID, cred.EnterpriseID)
	}
	if cred.Nickname != "alex@example.com" {
		t.Fatalf("nickname mismatch: %s", cred.Nickname)
	}
	if cred.AuthTier != "team" {
		t.Fatalf("tier mismatch: %s", cred.AuthTier)
	}
}
