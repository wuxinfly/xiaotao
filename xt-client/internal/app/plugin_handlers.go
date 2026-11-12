package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xiaotao/xt-client/internal/desktop"
	"xiaotao/xt-client/internal/sourceplugin"
)

var actionCapabilities = map[string]string{
	"login/import":   "login",
	"login/browser":  "browser-login",
	"models/refresh": "models",
	"quota/get":      "quota",
	"checkin/status": "checkin",
	"checkin/claim":  "checkin",
}

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
	handle("GET /api/source-plugins/index", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"plugins": localPluginIndex(plugins)})
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
		if input.Method == "login/host" {
			result, err := sourceplugin.HostLogin(r.Context(), plugins, r.PathValue("id"), desktop.Browser)
			if err != nil {
				writePluginError(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"account": result.Account, "ok": true})
			return
		}
		capability, ok := actionCapabilities[input.Method]
		if !ok || !plugins.Supports(r.PathValue("id"), capability) {
			writePluginError(w, sourceplugin.NewError(sourceplugin.CodeUnsupported, "插件不支持该操作"))
			return
		}
		timeout := 60 * time.Second
		if input.Method == "login/browser" {
			timeout = 6 * time.Minute
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if input.AccountID == "" {
			input.AccountID = plugins.PrimaryAccount(r.PathValue("id"))
		}
		if input.Method != "login/import" && input.Method != "login/browser" && plugins.Supports(r.PathValue("id"), "refresh") {
			if _, err := plugins.RunForAccount(ctx, r.PathValue("id"), input.AccountID, "credentials/refresh", nil, nil, "", nil); err != nil {
				writePluginError(w, err)
				return
			}
		}
		result, err := plugins.RunForAccount(ctx, r.PathValue("id"), input.AccountID, input.Method, input.Input, nil, "", nil)
		if err != nil {
			writePluginError(w, err)
			return
		}
		// Credentials never cross the management HTTP boundary.
		_ = json.NewEncoder(w).Encode(map[string]any{"models": result.Models, "value": result.Value, "display": result.Display, "ok": true})
	})
	handle("POST /api/source-plugins/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Values map[string]string `json:"values"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&input) != nil || input.Values == nil {
			writePluginError(w, sourceplugin.NewError(sourceplugin.CodeInvalid, "插件设置无效"))
			return
		}
		if err := plugins.SaveSettings(r.PathValue("id"), input.Values); err != nil {
			writePluginError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	handle("POST /api/source-plugins/{id}/uninstall", func(w http.ResponseWriter, r *http.Request) {
		if err := plugins.Uninstall(r.PathValue("id")); err != nil {
			writePluginError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	handle("POST /api/source-plugins/{id}/rollback", func(w http.ResponseWriter, r *http.Request) {
		if _, err := plugins.Rollback(r.PathValue("id")); err != nil {
			writePluginError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	handle("POST /api/source-plugins/{id}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
			writePluginError(w, sourceplugin.NewError(sourceplugin.CodeInvalid, "诊断设置无效"))
			return
		}
		if err := plugins.SetDiagnostics(r.PathValue("id"), input.Enabled); err != nil {
			writePluginError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
}

func writePluginError(w http.ResponseWriter, err error) {
	code := sourceplugin.ErrorCode(err)
	status := http.StatusBadGateway
	message := "插件操作失败"
	switch code {
	case sourceplugin.CodeAuthExpired:
		status, message = http.StatusUnauthorized, "登录已失效，请重新授权账号"
	case sourceplugin.CodeRateLimited:
		status, message = http.StatusTooManyRequests, "上游限流，请稍后再试"
	case sourceplugin.CodeUnsupported:
		status, message = http.StatusBadRequest, "插件不支持该操作"
	case sourceplugin.CodeInvalid:
		status, message = http.StatusBadRequest, "插件操作无效，请刷新后重试"
	case sourceplugin.CodeTimeout:
		status, message = http.StatusGatewayTimeout, "插件操作超时"
	case sourceplugin.CodeUnavailable:
		status, message = http.StatusServiceUnavailable, "插件不可用，可能已停止或崩溃"
	case sourceplugin.CodeUpstream:
		status, message = http.StatusBadGateway, "上游服务异常，请稍后再试"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code, status, message = sourceplugin.CodeTimeout, http.StatusGatewayTimeout, "插件操作超时"
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message}})
}
