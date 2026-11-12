package app

import (
	"encoding/json"
	"errors"
	"net/http"

	"xiaotao/xt-client/internal/office"
)

// registerOfficeRoutes exposes project-scoped office tasks. Every route is
// same-origin checked like the rest of the management API.
func registerOfficeRoutes(mux *http.ServeMux, svc *office.Service, requireOrigin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/office/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		snapshot, err := svc.Snapshot(r.URL.Query().Get("projectPath"), r.URL.Query().Get("projectName"))
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, snapshot)
	})
	mux.HandleFunc("POST /api/office/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var req office.CreateTaskRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&req); err != nil {
			apiError(w, 400, "任务格式无效")
			return
		}
		task, err := svc.CreateTask(req)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, task)
	})
	mux.HandleFunc("GET /api/office/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		detail, err := svc.TaskDetail(r.URL.Query().Get("projectPath"), r.PathValue("id"))
		if err != nil {
			officeError(w, err, apiError)
			return
		}
		send(w, detail)
	})
	mux.HandleFunc("GET /api/office/tasks/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		page, err := svc.TaskHistory(r.URL.Query().Get("projectPath"), r.PathValue("id"), r.URL.Query().Get("before"))
		if err != nil {
			officeError(w, err, apiError)
			return
		}
		send(w, page)
	})
	mux.HandleFunc("POST /api/office/tasks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if err := svc.CancelTask(r.URL.Query().Get("projectPath"), r.PathValue("id")); err != nil {
			officeError(w, err, apiError)
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/office/tasks/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var input struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&input); err != nil {
			apiError(w, 400, "消息格式无效")
			return
		}
		message, err := svc.PostMessage(r.URL.Query().Get("projectPath"), r.PathValue("id"), input.Content)
		if err != nil {
			officeError(w, err, apiError)
			return
		}
		send(w, message)
	})
	mux.HandleFunc("POST /api/office/tasks/{id}/approval", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var input struct {
			ID       string `json:"id"`
			Approved bool   `json:"approved"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			apiError(w, 400, "确认格式无效")
			return
		}
		if err := svc.ResolveApproval(r.URL.Query().Get("projectPath"), r.PathValue("id"), input.ID, input.Approved); err != nil {
			officeError(w, err, apiError)
			return
		}
		send(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/office/artifacts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		detail, err := svc.ArtifactDetail(r.URL.Query().Get("projectPath"), r.PathValue("id"))
		if err != nil {
			officeError(w, err, apiError)
			return
		}
		send(w, detail)
	})
}

func officeError(w http.ResponseWriter, err error, apiError func(http.ResponseWriter, int, string)) {
	switch {
	case errors.Is(err, office.ErrTaskNotFound), errors.Is(err, office.ErrArtifactNotFound):
		apiError(w, 404, err.Error())
	case errors.Is(err, office.ErrNotRunning):
		apiError(w, 409, err.Error())
	case errors.Is(err, office.ErrClosed):
		apiError(w, 503, err.Error())
	default:
		apiError(w, 400, err.Error())
	}
}
