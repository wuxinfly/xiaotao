package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"xiaotao/xt-client/internal/desktop"
	"xiaotao/xt-client/internal/gateway"
	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/runtimeenv"
	"xiaotao/xt-client/internal/usage"
)

type runtimeRoutes struct {
	state    State
	data     string
	stop     func()
	cfg      Config
	sources  *modelsource.Store
	gateway  *gateway.Service
	usage    *usage.Service
	send     func(http.ResponseWriter, any)
	apiError func(http.ResponseWriter, int, string)
}

type hudState struct {
	mu      sync.Mutex
	enabled bool
}

func registerRuntimeRoutes(mux *http.ServeMux, rt runtimeRoutes) {
	started := time.Now()
	var mu sync.Mutex
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		rt.send(w, map[string]any{"App": "xiaotao", "PID": rt.state.PID, "version": Version})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		rt.send(w, map[string]any{"version": Version, "url": rt.state.URL, "pid": rt.state.PID, "uptime": int(time.Since(started).Seconds()), "dataDir": rt.data, "modelReady": len(agentModelsForSources(rt.sources.List())) > 0})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { mu.Lock(); defer mu.Unlock(); rt.send(w, rt.cfg) })
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var next Config
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&next); e != nil || len([]rune(next.Name)) < 1 || len([]rune(next.Name)) > 40 {
			http.Error(w, "invalid name", 400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if err := Save(filepath.Join(rt.data, "config.json"), next); err != nil {
			rt.apiError(w, 500, "save failed")
			return
		}
		rt.cfg = next
		rt.send(w, rt.cfg)
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		rt.send(w, map[string]bool{"ok": true})
		// Serve owns handler cleanup after requests have drained.
		go rt.stop()
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := rt.usage.Snapshot()
		if err != nil {
			rt.apiError(w, 500, "snapshot failed")
			return
		}
		rt.send(w, snapshot)
	})
	mux.HandleFunc("GET /api/dashboard/events", rt.usage.HandleSSE)
	mux.HandleFunc("GET /api/proxy/key", func(w http.ResponseWriter, r *http.Request) {
		rt.send(w, map[string]string{"key": rt.gateway.APIKey()})
	})
	mux.HandleFunc("POST /api/proxy/key", func(w http.ResponseWriter, r *http.Request) {
		key, err := rt.gateway.RotateKey()
		if err != nil {
			rt.apiError(w, 500, "更新本地访问密钥失败")
			return
		}
		rt.send(w, map[string]string{"key": key})
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !rt.gateway.Authorize(r) {
			writeProxyError(w, 401, "invalid_api_key", "本地访问密钥无效")
			return
		}
		rt.send(w, map[string]any{"object": "list", "data": rt.gateway.Models()})
	})
	mux.HandleFunc("POST /v1/chat/completions", rt.gateway.HandleChat)
	registerHUDRoutes(mux, rt)
}

func registerHUDRoutes(mux *http.ServeMux, rt runtimeRoutes) {
	hud := &hudState{}
	title := desktop.HUDWindowTitle(runtimeenv.Current().DisplayName)
	mux.HandleFunc("GET /api/request-hud", func(w http.ResponseWriter, r *http.Request) {
		hud.mu.Lock()
		enabled := hud.enabled
		hud.mu.Unlock()
		rt.send(w, map[string]bool{"enabled": enabled})
	})
	mux.HandleFunc("POST /api/request-hud", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
			rt.apiError(w, 400, "invalid request")
			return
		}
		var exe string
		if body.Enabled {
			var err error
			exe, err = os.Executable()
			if err != nil {
				rt.apiError(w, 500, "cannot locate executable")
				return
			}
		}
		hud.mu.Lock()
		hud.enabled = body.Enabled
		hud.mu.Unlock()
		if body.Enabled {
			go func() { _ = desktop.Start(exe, "--hud-window", "--data-dir", rt.data) }()
		} else {
			go desktop.CloseWindow(title)
		}
		rt.send(w, map[string]bool{"enabled": body.Enabled})
	})
}
