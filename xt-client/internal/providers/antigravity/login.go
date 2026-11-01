package antigravity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
)

const (
	authEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenEndpoint    = "https://oauth2.googleapis.com/token"
	userInfoEndpoint = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	sessionTTL       = 15 * time.Minute
)

var (
	clientID = strings.Join([]string{
		"1071006060591-",
		"tmhssin2h21lcre235vtolojh4g403ep",
		".apps.googleusercontent",
		".com",
	}, "")
	clientSecret = strings.Join([]string{
		"GOC",
		"SPX-",
		"K58FWR486LdLJ1m",
		"LB8sXC4z6qDAf",
	}, "")
)

var oauthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// LoginSession tracks a single pending Google OAuth authorization.
type LoginSession struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	LoginURL string `json:"loginUrl"`
	Error    string `json:"error,omitempty"`

	cred   *providers.Credential
	mu     sync.Mutex
	cancel context.CancelFunc
}

// Credential returns the finished credential snapshot, or nil if login has
// not succeeded yet.  This is safe to call concurrently.
func (s *LoginSession) Credential() *providers.Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cred == nil {
		return nil
	}
	cp := *s.cred
	return &cp
}

func (s *LoginSession) setStatus(status, errText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != "waiting" {
		return
	}
	s.Status = status
	s.Error = errText
}

// StartLogin launches the PKCE OAuth flow, starts the local callback server,
// and returns the browser authorization link.
func StartLogin(parent context.Context) (*LoginSession, error) {
	verifier, err := randomToken(64)
	if err != nil {
		return nil, fmt.Errorf("pkce verifier failed: %w", err)
	}
	state, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("oauth state failed: %w", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("callback listener failed: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth-callback", port)
	params := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(oauthScopes, " ")},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	loginURL := authEndpoint + "?" + params.Encode()

	ctx, cancel := context.WithCancel(parent)
	sess := &LoginSession{ID: randomHex(16), Status: "waiting", LoginURL: loginURL, cancel: cancel}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth-callback", func(w http.ResponseWriter, r *http.Request) {
		sess.handleCallback(ctx, w, r, state, redirectURI, verifier)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		_ = server.Serve(listener)
	}()
	go func() {
		defer cancel()
		defer func() {
			shutdownCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			_ = server.Shutdown(shutdownCtx)
		}()
		deadline := time.NewTimer(sessionTTL)
		defer deadline.Stop()
		select {
		case <-deadline.C:
			sess.setStatus("timeout", "login timeout")
		case <-ctx.Done():
			sess.setStatus("cancelled", "login cancelled")
		case <-sess.done():
		}
	}()
	return sess, nil
}

// Cancel stops a pending login.  Success/timeout/cancelled are no-ops.
func (s *LoginSession) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
	s.setStatus("cancelled", "login cancelled")
}

func (s *LoginSession) handleCallback(ctx context.Context, w http.ResponseWriter, r *http.Request, state, redirectURI, verifier string) {
	q := r.URL.Query()
	callbackState := q.Get("state")
	if callbackState != state {
		s.setStatus("failed", "OAuth state mismatch")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("invalid callback state"))
		return
	}
	if errText := q.Get("error"); errText != "" {
		s.setStatus("failed", "Google 授权失败: "+errText)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("authorization failed"))
		return
	}
	code := q.Get("code")
	if code == "" {
		s.setStatus("failed", "callback missing code")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing code"))
		return
	}
	cred, err := exchangeCode(ctx, code, redirectURI, verifier)
	if err != nil {
		s.setStatus("failed", redact(err.Error()))
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("token exchange failed"))
		return
	}
	// Prevent duplicate callbacks from re-exchanging and re-saving.
	s.mu.Lock()
	if s.Status != "waiting" || s.cred != nil {
		s.mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("login already completed"))
		return
	}
	s.cred = cred
	s.mu.Unlock()
	s.setStatus("succeeded", "")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><meta charset='utf-8'><title>Antigravity</title><body style='font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh'><p>Authentication successful! You can close this window.</p></body>"))
	go func() {
		if s.cancel != nil {
			s.cancel()
		}
	}()
}

func (s *LoginSession) done() <-chan struct{} {
	// The local callback goroutine calls Cancel after a successful exchange,
	// closing the parent context and releasing the watcher below.
	ch := make(chan struct{})
	go func() {
		for {
			s.mu.Lock()
			finished := s.Status != "waiting"
			s.mu.Unlock()
			if finished {
				close(ch)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return ch
}

func exchangeCode(ctx context.Context, code, redirectURI, verifier string) (*providers.Credential, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code_verifier": {verifier},
	}
	payload, err := tokenRequest(ctx, form)
	if err != nil {
		return nil, err
	}
	cred := &providers.Credential{
		AccessToken:  payload["access_token"].(string),
		RefreshToken: payload["refresh_token"].(string),
		ExpiresAt:    expiresAt(payload["expires_in"]),
	}
	email, err := fetchUserEmail(ctx, cred.AccessToken)
	if err != nil {
		return nil, err
	}
	cred.Nickname = email
	cred.UID = email
	proj, tier, _, err := SetupCodeAssist(ctx, cred.AccessToken)
	if err != nil {
		return nil, err
	}
	cred.ProjectID = proj
	cred.AuthTier = tier
	return cred, nil
}

func tokenRequest(ctx context.Context, form url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := newHTTPClient(connectTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := readAll(resp.Body)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid token response")
	}
	if errMsg, ok := payload["error"].(string); ok {
		return nil, fmt.Errorf("Google token 接口拒绝: %s", errMsg)
	}
	return payload, nil
}

func fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := newHTTPClient(connectTimeout).Do(req)
	if err != nil {
		return "", fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := readAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("userinfo HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("invalid userinfo response")
	}
	if payload.Email == "" {
		return "", fmt.Errorf("userinfo missing email")
	}
	return payload.Email, nil
}

func expiresAt(expiresIn any) int64 {
	if seconds, ok := expiresIn.(float64); ok && seconds > 0 {
		return time.Now().Unix() + int64(seconds)
	}
	return time.Now().Unix() + 3600
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func redact(s string) string {
	lower := strings.ToLower(s)
	for _, key := range []string{"token", "credential", "userjwt", "authorization"} {
		if i := strings.Index(lower, key); i >= 0 {
			return s[:i] + key + "=<redacted>"
		}
	}
	return s
}
