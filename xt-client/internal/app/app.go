package app

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/gateway"
	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/usage"
)

var Version = "1.0.0-dev"

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
	if e = f.Sync(); e != nil {
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

// Handler assembles only generic model sources. Platform account packages are
// deliberately absent: they will be loaded through the future plugin boundary.
func Handler(s State, data string, stop func()) (http.Handler, error) {
	if err := os.MkdirAll(data, 0700); err != nil {
		return nil, err
	}
	cfg, err := loadConfig(data)
	if err != nil {
		return nil, err
	}
	keyring, err := secrets.Open(filepath.Join(data, "keys"))
	if err != nil {
		return nil, err
	}
	sources, err := modelsource.Open(data, keyring.Encrypt, keyring.Decrypt)
	if err != nil {
		return nil, err
	}
	if err = importManualModelSources(sources, keyring, data); err != nil {
		return nil, fmt.Errorf("import manual model sources: %w", err)
	}
	key, saveKey, err := localGatewayKey(data, keyring)
	if err != nil {
		return nil, err
	}
	gatewaySvc := gateway.New(sources, key, saveKey)
	agentSvc := agent.NewService(filepath.Join(data, "agent"), s.URL, gatewaySvc.APIKey, &http.Client{Timeout: 35 * time.Minute}, func() []agent.ModelOption { return agentModelsForSources(sources.List()) })
	agentSvc.SetClientVersion(Version)
	usageSvc, err := usage.New(filepath.Join(data, "data"), Version)
	if err != nil {
		return nil, err
	}
	gatewaySvc.SetUsage(usageSvc)
	mux := http.NewServeMux()
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(v)
	}
	apiError := func(w http.ResponseWriter, status int, message string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
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
	registerAgentRoutes(mux, agentSvc, requireOrigin, send, apiError)
	registerSourceRoutes(mux, sources, gatewaySvc, requireOrigin, send, apiError)
	registerRuntimeRoutes(mux, runtimeRoutes{state: s, data: data, stop: stop, cfg: cfg, sources: sources, gateway: gatewaySvc, usage: usageSvc, send: send, apiError: apiError})
	sub, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	// Removed management routes must never fall through to HTML/static resources.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 404, "接口不存在；平台账号接入已移至后续插件方案")
	})
	return &closeableHandler{Handler: accessLogMiddleware(guardLocalRequests(s, mux)), usage: usageSvc}, nil
}

func loadConfig(data string) (Config, error) {
	cfg := Config{Name: "小涛"}
	path := filepath.Join(data, "config.json")
	if b, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(b, &cfg); e != nil {
			return cfg, fmt.Errorf("invalid config: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return cfg, e
	} else if e = Save(path, cfg); e != nil {
		return cfg, e
	}
	return cfg, nil
}

func guardLocalRequests(s State, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != strings.TrimPrefix(s.URL, "http://") {
			http.Error(w, "invalid host", 403)
			return
		}
		// Model endpoints authenticate with a separate local gateway key.
		if r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/v1/models" {
			mux.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'")
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.URL {
			logging.Default().For(logging.ModuleClient).Warn("same-origin rejected", logging.F("path", r.URL.Path), logging.F("reason", "origin_mismatch"))
			http.Error(w, "same-origin required", 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Authorization") != "Bearer "+s.Token && r.Header.Get("Origin") != s.URL {
			http.Error(w, "same-origin required", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
