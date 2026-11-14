package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"xiaotao/xt-client/internal/agent"
)

func registerAgentRoutes(mux *http.ServeMux, svc *agent.Service, requireOrigin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	registerSkillRoutes(mux, svc, requireOrigin, send, apiError)
	registerReviewRoutes(mux, svc, requireOrigin, send, apiError)
	mux.HandleFunc("GET /api/agent/mcp", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		settings, err := svc.MCPSettings()
		if err != nil {
			apiError(w, 500, err.Error())
			return
		}
		send(w, settings)
	})
	mux.HandleFunc("PUT /api/agent/mcp", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var settings agent.MCPSettings
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&settings); err != nil {
			apiError(w, 400, "MCP 配置格式无效")
			return
		}
		if err := svc.SaveMCPSettings(settings); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/agent/mcp/test", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var server agent.MCPServer
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&server); err != nil {
			apiError(w, 400, "MCP 服务格式无效")
			return
		}
		tools, err := svc.TestMCP(r.Context(), server)
		if err != nil {
			apiError(w, 400, "连接失败: "+err.Error())
			return
		}
		send(w, map[string]any{"tools": tools})
	})
	mux.HandleFunc("GET /api/agent/access-mode", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]string{"mode": svc.AccessMode()})
	})
	mux.HandleFunc("PUT /api/agent/access-mode", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Mode string `json:"mode"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		if err := svc.SetAccessMode(input.Mode); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]string{"mode": input.Mode})
	})
	mux.HandleFunc("POST /api/agent/sessions/{id}/approval", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			ID       string `json:"id"`
			Approved bool   `json:"approved"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		if err := svc.ResolveApproval(r.PathValue("id"), input.ID, input.Approved); err != nil {
			apiError(w, 409, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/agent/models", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"models": svc.AvailableModels()})
	})
	mux.HandleFunc("GET /api/agent/attachments/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		path, err := svc.AttachmentPath(r.PathValue("name"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".jpg":
			w.Header().Set("Content-Type", "image/jpeg")
		case ".gif":
			w.Header().Set("Content-Type", "image/gif")
		case ".webp":
			w.Header().Set("Content-Type", "image/webp")
		}
		http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
	})
	mux.HandleFunc("GET /api/agent/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var sessions []*agent.Session
		var err error
		if r.URL.Query().Get("summary") == "1" {
			sessions, err = svc.ListSummaries()
		} else {
			sessions, err = svc.List()
		}
		if err != nil {
			apiError(w, 500, "读取任务失败: "+err.Error())
			return
		}
		// Office executors are dispatched sessions: they belong to a task and
		// are read through the office API, not as user chat sessions.
		visible := make([]*agent.Session, 0, len(sessions))
		for _, session := range sessions {
			if session.Role == "" {
				visible = append(visible, session)
			}
		}
		send(w, map[string]any{"sessions": visible})
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
		options.Member = nil
		session, err := svc.Create(options)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, session)
	})
	mux.HandleFunc("POST /api/agent/sessions/{id}/branch", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			MessageID string `json:"messageId"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		session, err := svc.Branch(r.PathValue("id"), input.MessageID)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, session)
	})
	mux.HandleFunc("GET /api/agent/sessions/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		page, err := svc.History(r.PathValue("id"), r.URL.Query().Get("before"))
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, page)
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
	mux.HandleFunc("DELETE /api/agent/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		if err := svc.Delete(r.PathValue("id")); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("PATCH /api/agent/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		item, err := svc.Rename(r.PathValue("id"), input.Title)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, item)
	})
	mux.HandleFunc("POST /api/agent/sessions/{id}/title", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Model         string               `json:"model"`
			ModelSettings *agent.ModelSettings `json:"modelSettings"`
		}
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
				apiError(w, 400, "请求格式无效")
				return
			}
		}
		title, err := svc.GenerateTitleWithModel(r.Context(), r.PathValue("id"), input.Model, input.ModelSettings)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]string{"title": title})
	})
	mux.HandleFunc("POST /api/agent/sessions/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Content       string               `json:"content"`
			Model         string               `json:"model"`
			ModelSettings *agent.ModelSettings `json:"modelSettings"`
			References    []string             `json:"references"`
			Images        []agent.ImageInput   `json:"images"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&input); err != nil {
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
		err := svc.SendWithModelSettings(r.Context(), r.PathValue("id"), input.Content, input.Model, input.ModelSettings, input.References, input.Images, write)
		if err != nil {
			write(agent.Event{Type: "error", SessionID: r.PathValue("id"), Error: err.Error()})
		}
	})
	mux.HandleFunc("GET /api/agent/references/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		scope := r.URL.Query().Get("scope")
		if scope == "" {
			scope = "project"
		}
		if scope != "project" && scope != "all" {
			apiError(w, 400, "引用搜索范围无效")
			return
		}
		exclude := []string{}
		for _, id := range r.URL.Query()["exclude"] {
			if id = strings.TrimSpace(id); id != "" {
				exclude = append(exclude, id)
			}
		}
		results, err := svc.SearchConversationReferences(agent.ReferenceSearch{
			ProjectPath: r.URL.Query().Get("projectPath"),
			Query:       r.URL.Query().Get("query"),
			Exclude:     exclude,
			AllProjects: scope == "all",
		})
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]any{"sessions": results})
	})
	mux.HandleFunc("GET /api/agent/references/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		detail, err := svc.ConversationReference(r.PathValue("id"))
		if err != nil {
			apiError(w, 404, err.Error())
			return
		}
		send(w, detail)
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
	mux.HandleFunc("PATCH /api/agent/project/name", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		if err := svc.RenameProject(input.Path, input.Name); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/agent/projects", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		projects, err := svc.ListProjects()
		if err != nil {
			apiError(w, 500, err.Error())
			return
		}
		send(w, map[string]any{"projects": projects})
	})
	mux.HandleFunc("POST /api/agent/projects", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		var input struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			apiError(w, 400, "请求格式无效")
			return
		}
		if err := svc.SaveProject(input.Path, input.Name); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /api/agent/projects", func(w http.ResponseWriter, r *http.Request) {
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
		if err := svc.DeleteProject(input.Path); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/agent/project/open", func(w http.ResponseWriter, r *http.Request) {
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
		if err = openProjectFolder(project.Path); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/agent/project/pick", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		send(w, map[string]any{"desktopOnly": true})
	})
}
