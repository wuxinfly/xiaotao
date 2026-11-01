package app

import (
	"encoding/json"
	"fmt"
	"net/http"

	"xiaotao/xt-client/internal/agent"
)

func registerAgentRoutes(mux *http.ServeMux, svc *agent.Service, requireOrigin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	mux.HandleFunc("GET /api/agent/models", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"models": svc.AvailableModels()})
	})
	mux.HandleFunc("GET /api/agent/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sessions, err := svc.List()
		if err != nil {
			apiError(w, 500, "读取任务失败: "+err.Error())
			return
		}
		send(w, map[string]any{"sessions": sessions})
	})
	mux.HandleFunc("POST /api/agent/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var options agent.Options
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&options); err != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		session, err := svc.Create(options)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, session)
	})
	mux.HandleFunc("GET /api/agent/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		session, err := svc.Get(r.PathValue("id"))
		if err != nil {
			apiError(w, 404, err.Error())
			return
		}
		send(w, session)
	})
	mux.HandleFunc("POST /api/agent/sessions/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
			apiError(w, 400, "消息格式无效")
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			apiError(w, 500, "当前环境不支持流式响应")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		write := func(event agent.Event) {
			b, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, b)
			flusher.Flush()
		}
		err := svc.Send(r.Context(), r.PathValue("id"), input.Content, write)
		if err != nil {
			write(agent.Event{Type: "error", SessionID: r.PathValue("id"), Error: err.Error()})
		}
	})
	mux.HandleFunc("POST /api/agent/sessions/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"ok": svc.Cancel(r.PathValue("id"))})
	})
	mux.HandleFunc("POST /api/agent/project", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		project, err := svc.Project(input.Path)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, project)
	})
	mux.HandleFunc("POST /api/agent/project/pick", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"desktopOnly": true})
	})
}
