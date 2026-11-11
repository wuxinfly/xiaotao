package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	"xiaotao/xt-client/internal/sourceplugin"
)

func registerPluginRoutes(mux *http.ServeMux, plugins *sourceplugin.Manager, origin func(*http.Request) bool) {
	handle := func(pattern string, fn func(http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !origin(r) {
				http.Error(w, "same-origin required", 403)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fn(w, r)
		})
	}
	handle("GET /api/source-plugins", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"plugins": plugins.List()})
	})
	handle("POST /api/source-plugins/install", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Path string `json:"path"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil {
			http.Error(w, "invalid package path", 400)
			return
		}
		entry, err := plugins.Install(input.Path)
		if err != nil {
			http.Error(w, "插件安装失败：请检查包、入口及重复安装", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(entry)
	})
	handle("POST /api/source-plugins/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
			http.Error(w, "invalid state", 400)
			return
		}
		if plugins.Enable(r.PathValue("id"), input.Enabled) != nil {
			http.Error(w, "无法修改插件状态", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	handle("POST /api/source-plugins/{id}/action", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Method string          `json:"method"`
			Input  json.RawMessage `json:"input"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil {
			http.Error(w, "invalid action", 400)
			return
		}
		switch input.Method {
		case "login/import", "models/refresh", "quota/get", "checkin/status", "checkin/claim":
		default:
			http.Error(w, "unsupported action", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		if input.Method != "login/import" {
			if _, err := plugins.Run(ctx, r.PathValue("id"), "credentials/refresh", nil, nil, "", nil); err != nil {
				http.Error(w, "凭据续期失败，请重新导入登录", 502)
				return
			}
		}
		result, err := plugins.Run(ctx, r.PathValue("id"), input.Method, input.Input, nil, "", nil)
		if err != nil {
			http.Error(w, "插件操作失败，请检查登录状态和上游连接", 502)
			return
		}
		// Credentials never cross the management HTTP boundary.
		_ = json.NewEncoder(w).Encode(map[string]any{"models": result.Models, "value": result.Value, "ok": true})
	})
}
