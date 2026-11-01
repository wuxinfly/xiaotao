package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xiaotao/xt-client/internal/authsessions"
	"xiaotao/xt-client/internal/desktop"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/antigravity"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/proxy"
	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

const Version = "0.1.0-dev"

const defaultPort = "5455"

//go:embed web/*
var assets embed.FS

type State struct {
	URL     string
	Token   string
	PID     int
	Version string
}
type Config struct {
	Name string `json:"name"`
}

var traeProvider = trae.Provider{}
var workbuddyProvider = workbuddy.Provider{}
var antigravityProvider = antigravity.Provider{}

func Key(data string) string { h := sha256.Sum256([]byte(data)); return hex.EncodeToString(h[:12]) }
func Save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func Read(data string) (State, error) {
	var s State
	b, e := os.ReadFile(filepath.Join(data, "runtime.json"))
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func Alive(s State) bool {
	c := http.Client{Timeout: time.Second}
	req, e := http.NewRequest("GET", s.URL+"/health", nil)
	if e != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	r, e := c.Do(req)
	if e != nil {
		return false
	}
	defer r.Body.Close()
	var v struct {
		App string
		PID int
	}
	return r.StatusCode == 200 && json.NewDecoder(r.Body).Decode(&v) == nil && v.App == "xiaotao" && v.PID == s.PID
}
func Handler(s State, data string, stop func()) (http.Handler, error) {
	cfg := Config{Name: "小涛"}
	path := filepath.Join(data, "config.json")
	if b, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(b, &cfg); e != nil {
			return nil, fmt.Errorf("invalid config: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	} else if e = Save(path, cfg); e != nil {
		return nil, e
	}
	var mu sync.Mutex
	mux := http.NewServeMux()
	started := time.Now()
	store, err := storage.Open(filepath.Join(data, "accounts"))
	if err != nil {
		return nil, err
	}
	keyring, err := secrets.Open(filepath.Join(data, "keys"))
	if err != nil {
		return nil, err
	}
	proxy.SetEncryptor(keyring.Encrypt)
	authSvc := authsessions.NewService(s.URL)
	var proxyKey string
	if b, err := store.LoadSecret("proxy", keyring.Decrypt); err == nil {
		// Keys are always stored as their hex-string form.
		proxyKey = string(b)
		if len(b) != 48 {
			proxyKey = hex.EncodeToString(b)
		}
	} else {
		key := make([]byte, 24)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		proxyKey = hex.EncodeToString(key)
		if err = store.SaveSecret("proxy", key, keyring.Encrypt); err != nil {
			return nil, err
		}
	}
	proxySvc := proxy.New(store, func(id string) ([]byte, error) { return store.LoadSecret(id, keyring.Decrypt) }, proxyKey)
	accountSvc := proxy.NewAccountService(store, func(id string) ([]byte, error) { return store.LoadSecret(id, keyring.Decrypt) })
	usageSvc, err := usage.New(filepath.Join(data, "data"), Version)
	if err != nil {
		return nil, err
	}
	proxySvc.SetUsage(usageSvc)
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(v)
	}
	apiError := func(w http.ResponseWriter, status int, message string) {
		send(w, map[string]any{"error": map[string]any{"message": message}})
	}
	requireOrigin := func(r *http.Request) bool {
		if origin := r.Header.Get("Origin"); origin != "" {
			return origin == s.URL
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			return auth == "Bearer "+s.Token
		}
		return r.Method == http.MethodGet || r.Method == http.MethodHead
	}
	registerProviders := func() {
		providers.Register(antigravityProvider)
		providers.Register(traeProvider)
		providers.Register(workbuddyProvider)
	}
	registerProviders()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"App": "xiaotao", "PID": s.PID, "version": Version})
	})
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"providers": providers.List()})
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		snap, err := usageSvc.Snapshot()
		if err != nil {
			apiError(w, 500, "snapshot failed: "+err.Error())
			return
		}
		send(w, snap)
	})
	mux.HandleFunc("GET /api/dashboard/events", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		usageSvc.HandleSSE(w, r)
	})
	mux.HandleFunc("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"accounts": store.Accounts(), "proxy": store.Proxy()})
	})
	mux.HandleFunc("POST /api/accounts/{id}/rename", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var body struct {
			Alias   string `json:"alias"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			apiError(w, 400, "invalid request")
			return
		}
		id := r.PathValue("id")
		if _, ok := store.Account(id); !ok {
			apiError(w, 404, "account not found")
			return
		}
		if len([]rune(body.Alias)) > 40 {
			apiError(w, 400, "alias too long")
			return
		}
		err := store.UpdateAccount(id, func(a *storage.Account) {
			if body.Alias != "" {
				a.Alias = body.Alias
			}
			if body.Enabled != nil {
				a.Enabled = *body.Enabled
			}
		})
		if err != nil {
			apiError(w, 500, "update failed")
			return
		}
		if acct, ok := store.Account(id); ok {
			send(w, acct)
		}
	})
	mux.HandleFunc("POST /api/accounts/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		if _, ok := store.Account(id); !ok {
			apiError(w, 404, "account not found")
			return
		}
		if err := store.DeleteAccount(id); err != nil {
			apiError(w, 500, "delete failed")
			return
		}
		send(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/accounts/{id}/quota", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		if _, ok := store.Account(id); !ok {
			apiError(w, 404, "account not found")
			return
		}
		snap, err := accountSvc.RefreshQuota(id)
		if err != nil {
			apiError(w, 502, "quota refresh failed")
			return
		}
		send(w, snap)
	})
	mux.HandleFunc("POST /api/auth/trae", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sess, err := authSvc.Start("trae", "")
		if err != nil {
			apiError(w, 500, "start login failed")
			return
		}
		if err := desktop.Browser(sess.LoginURL); err != nil {
			apiError(w, 500, "cannot open default browser")
			return
		}
		send(w, sess)
	})

	startProviderLogin := func(w http.ResponseWriter, r *http.Request, providerID string) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sess, err := authSvc.Start(providerID, "")
		if err != nil {
			apiError(w, 500, "start login failed")
			return
		}
		if sess.LoginURL != "" {
			if err := desktop.Browser(sess.LoginURL); err != nil {
				apiError(w, 500, "cannot open default browser")
				return
			}
		}
		send(w, sess)
	}

	mux.HandleFunc("POST /api/auth/workbuddy", func(w http.ResponseWriter, r *http.Request) {
		startProviderLogin(w, r, "workbuddy")
	})
	mux.HandleFunc("POST /api/auth/antigravity", func(w http.ResponseWriter, r *http.Request) {
		startProviderLogin(w, r, "antigravity")
	})

	saveProviderAccount := func(w http.ResponseWriter, r *http.Request, providerID string) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sess, cred := authSvc.Poll(r.PathValue("id"))
		if sess.Status == "not_found" {
			apiError(w, 404, "session not found")
			return
		}
		if sess.Status == "succeeded" && cred != nil {
			uid := cred.UID
			nickname := cred.Nickname
			if providerID == "trae" {
				var err error
				uid, nickname, err = trae.UserInfo(cred)
				if err != nil || uid == "" {
					uid = trae.JWTUserID(cred.AccessToken)
				}
				if uid == "" {
					apiError(w, 502, "cannot determine account identity")
					return
				}
			}
			if uid == "" {
				apiError(w, 502, "cannot determine account identity")
				return
			}
			id := providerID + ":cn:" + uid
			now := time.Now().Unix()
			acct := &storage.Account{
				ID: id, Provider: providerID, Region: cred.Region, UID: uid,
				Nickname: nickname, Enabled: true, AuthState: "valid",
				MachineID: cred.MachineID, DeviceID: cred.DeviceID,
				AddedAt: now, UpdatedAt: now,
			}
			if existing, ok := store.Account(id); ok {
				acct.Alias = existing.Alias
				acct.Enabled = existing.Enabled
				acct.AddedAt = existing.AddedAt
			}
			if err := store.UpsertAccount(acct); err != nil {
				apiError(w, 500, "save account failed")
				return
			}
			b, err := json.Marshal(cred)
			if err != nil {
				apiError(w, 500, "encode credential failed")
				return
			}
			if err := store.SaveSecret(id, b, keyring.Encrypt); err != nil {
				apiError(w, 500, "save credential failed")
				return
			}
			if p := store.Proxy(); p.DefaultAccountID == "" {
				p.DefaultAccountID = id
				if err := store.SetProxy(p); err != nil {
					apiError(w, 500, "save default account failed")
					return
				}
			}
			send(w, map[string]any{"status": "succeeded", "accountId": id, "nickname": nickname})
			return
		}
		send(w, map[string]any{"status": sess.Status, "error": sess.Error})
	}

	mux.HandleFunc("GET /api/auth/workbuddy/{id}", func(w http.ResponseWriter, r *http.Request) {
		saveProviderAccount(w, r, "workbuddy")
	})
	mux.HandleFunc("GET /api/auth/antigravity/{id}", func(w http.ResponseWriter, r *http.Request) {
		saveProviderAccount(w, r, "antigravity")
	})

	mux.HandleFunc("DELETE /api/auth/workbuddy/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		authSvc.Cancel(r.PathValue("id"))
		send(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("DELETE /api/auth/antigravity/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		authSvc.Cancel(r.PathValue("id"))
		send(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/auth/trae/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sess, cred := authSvc.Poll(r.PathValue("id"))
		if sess.Status == "not_found" {
			apiError(w, 404, "session not found")
			return
		}
		if sess.Status == "succeeded" && cred != nil {
			uid, nickname, err := trae.UserInfo(cred)
			if err != nil || uid == "" {
				uid = trae.JWTUserID(cred.AccessToken)
			}
			if uid == "" {
				apiError(w, 502, "cannot determine account identity")
				return
			}
			id := "trae:" + uid
			now := time.Now().Unix()
			acct := &storage.Account{
				ID: id, Provider: "trae", Region: cred.Region, UID: uid,
				Nickname: nickname, Enabled: true, AuthState: "valid",
				MachineID: cred.MachineID, DeviceID: cred.DeviceID,
				AddedAt: now, UpdatedAt: now,
			}
			if existing, ok := store.Account(id); ok {
				acct.Alias = existing.Alias
				acct.Enabled = existing.Enabled
				acct.AddedAt = existing.AddedAt
			}
			if err := store.UpsertAccount(acct); err != nil {
				apiError(w, 500, "save account failed")
				return
			}
			b, err := json.Marshal(cred)
			if err != nil {
				apiError(w, 500, "encode credential failed")
				return
			}
			if err := store.SaveSecret(id, b, keyring.Encrypt); err != nil {
				apiError(w, 500, "save credential failed")
				return
			}
			if p := store.Proxy(); p.DefaultAccountID == "" {
				p.DefaultAccountID = id
				if err := store.SetProxy(p); err != nil {
					apiError(w, 500, "save default account failed")
					return
				}
			}
			send(w, map[string]any{"status": "succeeded", "accountId": id, "nickname": nickname})
			return
		}
		send(w, map[string]any{"status": sess.Status, "error": sess.Error})
	})
	mux.HandleFunc("DELETE /api/auth/trae/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		authSvc.Cancel(r.PathValue("id"))
		send(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("PUT /api/proxy/settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var body struct {
			DefaultAccountID string `json:"defaultAccountId"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			apiError(w, 400, "invalid request")
			return
		}
		next := store.Proxy()
		if body.DefaultAccountID != "" {
			if _, ok := store.Account(body.DefaultAccountID); !ok {
				apiError(w, 404, "account not found")
				return
			}
			next.DefaultAccountID = body.DefaultAccountID
		}
		if err := store.SetProxy(next); err != nil {
			apiError(w, 500, "save failed")
			return
		}
		send(w, next)
	})
	mux.HandleFunc("POST /api/proxy/key", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		key, err := proxySvc.RotateKey()
		if err != nil {
			apiError(w, 500, "rotate failed")
			return
		}
		send(w, map[string]any{"key": key})
	})
	mux.HandleFunc("GET /api/proxy/key", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"key": proxySvc.APIKey()})
	})
	mux.HandleFunc("POST /api/proxy/models", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		accs := store.Accounts()
		if len(accs) == 0 {
			apiError(w, 409, "no accounts")
			return
		}
		seen := map[string]bool{}
		models := []providers.ModelInfo{}
		for _, a := range accs {
			if !a.Enabled {
				continue
			}
			for _, m := range providerModels(a) {
				if !seen[m.ID] {
					seen[m.ID] = true
					models = append(models, m)
				}
			}
		}
		send(w, map[string]any{"models": models})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"version": Version, "url": s.URL, "pid": s.PID, "uptime": int(time.Since(started).Seconds()), "dataDir": data, "modelReady": false})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { mu.Lock(); defer mu.Unlock(); send(w, cfg) })
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var next Config
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&next); e != nil || len([]rune(next.Name)) < 1 || len([]rune(next.Name)) > 40 {
			http.Error(w, "invalid name", 400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if e := Save(path, next); e != nil {
			http.Error(w, "save failed", 500)
			return
		}
		cfg = next
		send(w, cfg)
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]bool{"ok": true})
		usageSvc.Close()
		go stop()
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !proxySvc.Authorize(r) {
			writeProxyError(w, 401, "invalid_api_key", "代理访问密钥无效")
			return
		}
		models, err := proxySvc.Models()
		if err != nil {
			writeProxyError(w, 500, "models_error", "获取模型失败")
			return
		}
		send(w, map[string]any{"object": "list", "data": models})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		proxySvc.HandleChat(w, r)
	})
	sub, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.URL[len("http://"):] {
			http.Error(w, "invalid host", 403)
			return
		}
		// The OpenAI-compatible proxy authenticates with its own bearer key
		// and must accept non-browser clients, so it is exempt from the
		// same-origin guard below.
		if r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/v1/models" {
			mux.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'")
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.URL {
			http.Error(w, "same-origin required", 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Authorization") != "Bearer "+s.Token && r.Header.Get("Origin") != s.URL {
			http.Error(w, "same-origin required", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return &closeableHandler{Handler: baseHandler, usage: usageSvc}, nil
}

type closeableHandler struct {
	http.Handler
	usage *usage.Service
}

func (c *closeableHandler) Close() error {
	if c.usage != nil {
		return c.usage.Close()
	}
	return nil
}

func writeProxyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message, "type": code}})
}

func providerModels(a *storage.Account) []providers.ModelInfo {
	p, err := providers.Get(a.Provider)
	if err != nil {
		return []providers.ModelInfo{}
	}
	return p.ListModels(a.Region)
}
func Serve(ctx context.Context, data string) error {
	release, e := desktop.Lock("Local\\XiaoTao.Service." + Key(data))
	if e != nil {
		return e
	}
	defer release()
	l, e := net.Listen("tcp4", "127.0.0.1:"+defaultPort)
	if e != nil {
		l, e = net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			return e
		}
	}
	defer l.Close()
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		return e
	}
	s := State{URL: "http://" + l.Addr().String(), Token: hex.EncodeToString(token), PID: os.Getpid(), Version: Version}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	h, e := Handler(s, data, cancel)
	if e != nil {
		return e
	}
	if closer, ok := h.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	srv := http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if e = Save(filepath.Join(data, "runtime.json"), s); e != nil {
		return e
	}
	defer os.Remove(filepath.Join(data, "runtime.json"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		srv.Shutdown(c)
	}()
	e = srv.Serve(l)
	cancel()
	<-done
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
