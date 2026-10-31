package authsessions

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
)

type Session struct {
	ID         string                `json:"id"`
	Provider   string                `json:"provider"`
	Region     string                `json:"region"`
	Status     string                `json:"status"`
	CreatedAt  int64                 `json:"createdAt"`
	ExpiresAt  int64                 `json:"expiresAt"`
	Port       int                   `json:"port"`
	Nonce      string                `json:"-"`
	LoginURL   string                `json:"loginUrl"`
	Error      string                `json:"error,omitempty"`
	Credential *providers.Credential `json:"-"`
}

type Service struct {
	mu       sync.Mutex
	sessions map[string]*Session
	baseURL  string
}

func NewService(baseURL string) *Service {
	return &Service{sessions: map[string]*Session{}, baseURL: baseURL}
}

func (s *Service) Start(providerID, region string) (Session, error) {
	p, err := providers.Get(providerID)
	if err != nil {
		return Session{}, err
	}
	if providerID != "trae" {
		return Session{}, fmt.Errorf("provider %q does not support web login yet", providerID)
	}
	if region == "" {
		region = defaultRegionOf(p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, old := range s.sessions {
		if old.Provider == providerID && old.Region == region && old.Status == "waiting" {
			old.Status = "cancelled"
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Session{}, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	nonce := randHex()
	sess := &Session{
		ID: randHex(), Provider: providerID, Region: region, Status: "waiting",
		CreatedAt: now.Unix(), ExpiresAt: now.Add(15 * time.Minute).Unix(),
		Port: port, Nonce: nonce, LoginURL: trae.LoginURL(region, port, nonce),
	}
	s.sessions[sess.ID] = sess
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		s.handleCallback(sess, w, r)
	})
	server := &http.Server{Handler: corsHandler(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		_ = server.Serve(listener)
	}()
	go func() {
		s.expireWatch(sess, server)
	}()
	cp := *sess
	return cp, nil
}

func (s *Service) handleCallback(sess *Session, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	nonce := first(q, "loginTraceID", "login_trace_id", "state", "nonce")
	s.mu.Lock()
	now := time.Now().Unix()
	if sess.Status != "waiting" || nonce != sess.Nonce {
		s.mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("invalid callback"))
		return
	}
	if now > sess.ExpiresAt {
		sess.Status = "expired"
		s.mu.Unlock()
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte("login session expired"))
		return
	}
	refresh := trae.ExtractRefreshToken(r.URL.String())
	if refresh == "" {
		sess.Status = "failed"
		sess.Error = "callback missing refresh token"
		s.mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing credentials"))
		return
	}
	s.mu.Unlock()
	cred, err := trae.Exchange(sess.Region, refresh)
	if err != nil {
		s.mu.Lock()
		sess.Status = "failed"
		sess.Error = redact(err.Error())
		s.mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("token exchange failed"))
		return
	}
	uid, nickname, err := trae.UserInfo(cred)
	if err != nil || uid == "" {
		uid = trae.JWTUserID(cred.AccessToken)
		nickname = ""
	}
	s.mu.Lock()
	sess.Status = "succeeded"
	sess.Credential = cred
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><meta charset='utf-8'><title>登录成功</title><body style='background:#0b1622;color:#dce8f5;font-family:system-ui;text-align:center;padding-top:20vh'><h2>登录成功</h2><p>可以回到小涛查看账号状态。</p></body>"))
	_ = nickname
}

func (s *Service) Poll(id string) (Session, *providers.Credential) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{Status: "not_found"}, nil
	}
	now := time.Now().Unix()
	if sess.Status == "waiting" && now > sess.ExpiresAt {
		sess.Status = "expired"
	}
	cp := *sess
	return cp, sess.Credential
}

func (s *Service) View(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	cp := *sess
	return cp, true
}

func (s *Service) Cancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok && sess.Status == "waiting" {
		sess.Status = "cancelled"
	}
}

func (s *Service) expireWatch(sess *Session, server *http.Server) {
	for {
		time.Sleep(time.Second)
		s.mu.Lock()
		status := sess.Status
		expired := time.Now().Unix() > sess.ExpiresAt
		s.mu.Unlock()
		if status != "waiting" {
			ctx, cancel := contextWithTimeout()
			_ = server.Shutdown(ctx)
			cancel()
			return
		}
		if expired {
			s.mu.Lock()
			sess.Status = "expired"
			s.mu.Unlock()
			s.mu.Lock()
			sess.Status = "expired"
			s.mu.Unlock()
			ctx, cancel := contextWithTimeout()
			_ = server.Shutdown(ctx)
			cancel()
			return
		}
	}
}

func corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

func first(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func randHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultRegionOf(p providers.Provider) string {
	if r := p.Info().Regions; len(r) > 0 {
		return r[0]
	}
	return "cn"
}

func redact(s string) string {
	low := strings.ToLower(s)
	for _, key := range []string{"token", "credential", "userjwt"} {
		if i := strings.Index(low, key); i >= 0 {
			return s[:i] + key + "=<redacted>"
		}
	}
	return s
}
