package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"xiaotao/xt-client/internal/gateway"
	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/sourceplugin"
)

func registerSourceRoutes(mux *http.ServeMux, store *modelsource.Store, gw *gateway.Service, origin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string), plugins *sourceplugin.Manager) {
	protect := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !origin(r) {
				http.Error(w, "same-origin required", 403)
				return
			}
			handler(w, r)
		}
	}
	mux.HandleFunc("GET /api/model-sources", protect(func(w http.ResponseWriter, r *http.Request) { send(w, map[string]any{"sources": store.List()}) }))
	save := protect(func(w http.ResponseWriter, r *http.Request) {
		var input modelsource.Input
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			apiError(w, 400, "接口配置格式无效")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			apiError(w, 400, "接口配置必须是单个 JSON 对象")
			return
		}
		source, err := store.Save(r.PathValue("id"), input)
		if err != nil {
			sourceAPIError(w, err, apiError)
			return
		}
		send(w, source)
	})
	mux.HandleFunc("POST /api/model-sources", save)
	mux.HandleFunc("PUT /api/model-sources/{id}", save)
	mux.HandleFunc("DELETE /api/model-sources/{id}", protect(func(w http.ResponseWriter, r *http.Request) {
		if err := store.Delete(r.PathValue("id")); err != nil {
			sourceAPIError(w, err, apiError)
			return
		}
		send(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/model-sources/{id}/test", protect(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()
		if err := gw.Test(ctx, r.PathValue("id")); err != nil {
			if errors.Is(err, modelsource.ErrNotFound) || errors.Is(err, modelsource.ErrInvalid) {
				sourceAPIError(w, err, apiError)
			} else {
				apiError(w, 502, "连接测试失败：请检查接口地址、密钥、模型 ID 和上游状态")
			}
			return
		}
		send(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/agent/models/refresh", protect(func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"models": agentModelsForSources(append(store.List(), plugins.Sources()...)), "errors": []string{}})
	}))
	// Compatibility view for dashboard/HUD account labels; it contains manual
	// sources only and never reads or refreshes legacy platform accounts.
	mux.HandleFunc("GET /api/accounts", protect(func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"accounts": sourceAccountLabels(store.List())})
	}))
}

func sourceAPIError(w http.ResponseWriter, err error, apiError func(http.ResponseWriter, int, string)) {
	switch {
	case errors.Is(err, modelsource.ErrNotFound):
		apiError(w, 404, "模型接口不存在或已停用")
	case errors.Is(err, modelsource.ErrInvalid):
		apiError(w, 400, "接口配置无效，请检查 URL、密钥、模型 ID 和参数范围")
	default:
		apiError(w, 500, "保存模型接口失败，原配置已保留")
	}
}

func sourceAccountLabels(sources []modelsource.Source) []map[string]any {
	out := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		out = append(out, map[string]any{"id": source.ID, "provider": "manual", "region": "", "uid": source.ID, "nickname": source.Label, "alias": source.Label, "enabled": source.Enabled, "authState": "configured", "addedAt": 0, "usageTokens": 0, "usageCalls": 0})
	}
	return out
}
