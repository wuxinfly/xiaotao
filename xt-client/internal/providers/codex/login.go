package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
)

type LoginFlow struct {
	ID          string
	LoginURL    string
	State       string
	PKCE        *PKCECodes
	server      *http.Server
	listener    net.Listener
	mu          sync.Mutex
	cred        *providers.Credential
	err         error
	done        chan struct{}
	manualMode  bool
}

func StartLoginFlow() (*LoginFlow, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("生成 PKCE 失败: %w", err)
	}
	state := GenerateState()
	loginURL := BuildAuthorizeURL(pkce, state)

	flow := &LoginFlow{
		ID:       "codex-" + randHex(8),
		LoginURL: loginURL,
		State:    state,
		PKCE:     pkce,
		done:     make(chan struct{}),
	}

	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", CallbackPort))
	if err != nil {
		// Port occupied: fallback to manual callback URL pasting mode
		flow.manualMode = true
		return flow, nil
	}
	flow.listener = l

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		gotState := r.URL.Query().Get("state")
		errParam := r.URL.Query().Get("error")
		errDesc := r.URL.Query().Get("error_description")

		if errParam != "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "授权失败: %s %s", errParam, errDesc)
			flow.finish(nil, fmt.Errorf("授权失败: %s: %s", errParam, errDesc))
			return
		}

		if gotState != "" && gotState != flow.State {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "State mismatch")
			flow.finish(nil, errors.New("state mismatch"))
			return
		}

		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing code")
			flow.finish(nil, errors.New("missing code in callback"))
			return
		}

		tokens, err := ExchangeCode(r.Context(), code, flow.PKCE.Verifier)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, "换取 Token 失败: %v", err)
			flow.finish(nil, fmt.Errorf("换取 Token 失败: %w", err))
			return
		}

		cred := CredFromTokenResponse(tokens)
		flow.finish(cred, nil)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><meta charset='utf-8'><title>登录成功</title><body style='background:#0b1622;color:#dce8f5;font-family:system-ui;text-align:center;padding-top:20vh'><h2>Codex 登录成功</h2><p>可以回到小涛查看账号状态并关闭此网页。</p></body>")
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	flow.server = server
	go func() {
		_ = server.Serve(l)
	}()

	return flow, nil
}

func (f *LoginFlow) finish(cred *providers.Credential, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.done:
		return
	default:
	}
	f.cred = cred
	f.err = err
	close(f.done)
	if f.server != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = f.server.Shutdown(ctx)
		}()
	}
}

func (f *LoginFlow) CompleteManual(rawCallback string) (*providers.Credential, error) {
	code, state, err := ParseCallbackInput(rawCallback)
	if err != nil {
		return nil, err
	}
	if state != "" && state != f.State {
		return nil, errors.New("state 不匹配，回调可能来自其他授权请求")
	}
	tokens, err := ExchangeCode(context.Background(), code, f.PKCE.Verifier)
	if err != nil {
		return nil, fmt.Errorf("换取 Token 失败: %w", err)
	}
	cred := CredFromTokenResponse(tokens)
	f.finish(cred, nil)
	return cred, nil
}

func (f *LoginFlow) Cancel() {
	f.finish(nil, errors.New("login cancelled"))
}

func (f *LoginFlow) Result() (*providers.Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cred, f.err
}

func (f *LoginFlow) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

func (f *LoginFlow) IsManual() bool {
	return f.manualMode
}

func CredFromTokenResponse(tokens *TokenResponse) *providers.Credential {
	now := time.Now().Unix()
	expiresAt := now + int64(tokens.ExpiresIn)
	claims := DecodeIDTokenClaims(tokens.IDToken)
	accountID := AccountIDFromClaims(claims)
	email := EmailFromClaims(claims)
	plan := PlanTypeFromClaims(claims)
	userID := UserIDFromClaims(claims)

	uid := accountID
	if uid == "" {
		uid = userID
	}
	if uid == "" {
		h := sha256.Sum256([]byte(tokens.RefreshToken))
		uid = hex.EncodeToString(h[:8])
	}

	nickname := email
	if nickname == "" {
		nickname = uid
	}

	return &providers.Credential{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		UID:          uid,
		Nickname:     nickname,
		AuthTier:     plan,
		EnterpriseID: accountID, // Stored for Chatgpt-Account-Id header
		Region:       "global",
	}
}

func ImportAuthPayload(raw []byte) (*providers.Credential, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("导入内容不是合法 JSON: %w", err)
	}

	source := payload
	if tokens, ok := payload["tokens"].(map[string]any); ok {
		source = tokens
	}

	accessToken, _ := source["access_token"].(string)
	refreshToken, _ := source["refresh_token"].(string)
	idToken, _ := source["id_token"].(string)
	accountID, _ := source["account_id"].(string)
	if accountID == "" {
		accountID, _ = source["chatgpt_account_id"].(string)
	}

	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("导入内容缺少 access_token 或 refresh_token")
	}

	claims := DecodeIDTokenClaims(idToken)
	if accountID == "" {
		accountID = AccountIDFromClaims(claims)
	}
	email := EmailFromClaims(claims)
	userID := UserIDFromClaims(claims)

	uid := accountID
	if uid == "" {
		uid = userID
	}
	if uid == "" {
		h := sha256.Sum256([]byte(refreshToken))
		uid = hex.EncodeToString(h[:8])
	}
	nickname := email
	if nickname == "" {
		nickname = uid
	}

	// Verify refresh_token by performing a refresh immediately
	tokens, err := RefreshToken(context.Background(), refreshToken)
	if err != nil {
		return nil, fmt.Errorf("导入凭据验证失败（refresh_token 无效）: %w", err)
	}

	refreshedCred := CredFromTokenResponse(tokens)
	if accountID != "" && refreshedCred.EnterpriseID == "" {
		refreshedCred.EnterpriseID = accountID
	}
	if refreshedCred.Nickname == "" && nickname != "" {
		refreshedCred.Nickname = nickname
	}
	return refreshedCred, nil
}
