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
	handle("GET /api/source-plugins/packages", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"packages": localPluginPackages(plugins)})
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
	handle("POST /api/source-plugins/update", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Path string `json:"path"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil {
			http.Error(w, "插件包路径无效", 400)
			return
		}
		if _, err := plugins.Update(input.Path); err != nil {
			http.Error(w, "插件更新失败：请停用插件，并核对包的 ID、版本和入口", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
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
	handle("POST /api/source-plugins/{id}/account-order", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			IDs []string `json:"ids"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil || input.IDs == nil {
			http.Error(w, "账号顺序无效", 400)
			return
		}
		if err := plugins.ReorderAccounts(r.PathValue("id"), input.IDs); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	handle("POST /api/source-plugins/{id}/models", func(w http.ResponseWriter, r *http.Request) {
		var settings sourceplugin.ModelSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&settings) != nil {
			http.Error(w, "模型设置无效", 400)
			return
		}
		if err := plugins.SaveModelSettings(r.PathValue("id"), settings); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	handle("POST /api/source-plugins/{id}/action", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Method    string          `json:"method"`
			AccountID string          `json:"accountId"`
			Input     json.RawMessage `json:"input"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input) != nil {
			http.Error(w, "invalid action", 400)
			return
		}
		switch input.Method {
		case "login/import", "login/browser", "models/refresh", "quota/get", "checkin/status", "checkin/claim":
		default:
			http.Error(w, "unsupported action", 400)
			return
		}
		timeout := 60 * time.Second
		if input.Method == "login/browser" {
			if !plugins.Supports(r.PathValue("id"), "browser-login") {
				http.Error(w, "请先更新插件以使用浏览器授权", 400)
				return
			}
			timeout = 6 * time.Minute
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if input.AccountID == "" {
			input.AccountID = plugins.PrimaryAccount(r.PathValue("id"))
		}
		if input.Method != "login/import" && input.Method != "login/browser" {
			if _, err := plugins.RunForAccount(ctx, r.PathValue("id"), input.AccountID, "credentials/refresh", nil, nil, "", nil); err != nil {
				http.Error(w, "凭据续期失败，请重新导入登录", 502)
				return
			}
		}
		result, err := plugins.RunForAccount(ctx, r.PathValue("id"), input.AccountID, input.Method, input.Input, nil, "", nil)
		if err != nil {
			http.Error(w, "插件操作失败，请检查登录状态和上游连接", 502)
			return
		}
		// Credentials never cross the management HTTP boundary.
		_ = json.NewEncoder(w).Encode(map[string]any{"models": result.Models, "value": result.Value, "ok": true})
	})
}
