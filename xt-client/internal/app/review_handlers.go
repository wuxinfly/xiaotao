package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"xiaotao/xt-client/internal/agent"
)

func registerReviewRoutes(mux *http.ServeMux, svc *agent.Service, requireOrigin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	// 1. Create behavior review
	mux.HandleFunc("POST /api/agent/sessions/{id}/reviews", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sessionID := r.PathValue("id")
		var req agent.CreateReviewRequest
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
				apiError(w, 400, "请求格式无效")
				return
			}
		}
		review, err := svc.CreateReview(sessionID, req)
		if err != nil {
			if errors.Is(err, agent.ErrReviewActiveExists) || errors.Is(err, agent.ErrReviewConflict) {
				apiError(w, 409, err.Error())
				return
			}
			if errors.Is(err, agent.ErrReviewGlobalLimit) {
				apiError(w, 429, err.Error())
				return
			}
			apiError(w, 400, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(review)
	})

	// 2. List reviews
	mux.HandleFunc("GET /api/agent/reviews", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		sessionID := r.URL.Query().Get("sessionId")
		cursor := r.URL.Query().Get("cursor")
		limit := 20
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
				limit = l
			}
		}
		page, err := svc.ListReviews(sessionID, cursor, limit)
		if err != nil {
			apiError(w, 500, "读取复盘列表失败: "+err.Error())
			return
		}
		send(w, page)
	})

	// 3. Get review details
	mux.HandleFunc("GET /api/agent/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		review, err := svc.GetReview(id)
		if err != nil {
			if errors.Is(err, agent.ErrReviewNotFound) {
				apiError(w, 404, "复盘记录不存在")
				return
			}
			apiError(w, 500, err.Error())
			return
		}
		send(w, review)
	})

	// 4. Cancel review
	mux.HandleFunc("POST /api/agent/reviews/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		if err := svc.CancelReview(id); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})

	// 5. Retry review
	mux.HandleFunc("POST /api/agent/reviews/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		var req agent.RetryReviewRequest
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
				apiError(w, 400, "请求格式无效")
				return
			}
		}
		newReview, err := svc.RetryReview(id, req)
		if err != nil {
			if errors.Is(err, agent.ErrReviewActiveExists) || errors.Is(err, agent.ErrReviewConflict) {
				apiError(w, 409, err.Error())
				return
			}
			if errors.Is(err, agent.ErrReviewGlobalLimit) {
				apiError(w, 429, err.Error())
				return
			}
			apiError(w, 400, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(newReview)
	})

	// 6. Export review as Markdown
	mux.HandleFunc("GET /api/agent/reviews/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		filename, content, err := svc.GenerateExportMarkdown(id)
		if err != nil {
			if errors.Is(err, agent.ErrReviewNotFound) {
				apiError(w, 404, "复盘记录不存在")
				return
			}
			apiError(w, 400, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	})

	// 7. Delete review
	mux.HandleFunc("DELETE /api/agent/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		if err := svc.DeleteReview(id); err != nil {
			if errors.Is(err, agent.ErrReviewRunningDelete) {
				apiError(w, 409, err.Error())
				return
			}
			if errors.Is(err, agent.ErrReviewNotFound) {
				apiError(w, 404, err.Error())
				return
			}
			apiError(w, 400, err.Error())
			return
		}
		send(w, map[string]bool{"ok": true})
	})

	// 8. Add issue to suggestion box
	mux.HandleFunc("POST /api/agent/reviews/{id}/issues/{issueId}/suggestion", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		issueID := r.PathValue("issueId")
		var req agent.AddSuggestionRequest
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req); err != nil {
				apiError(w, 400, "请求格式无效")
				return
			}
		}
		link, err := svc.AddIssueSuggestion(id, issueID, req)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, link)
	})

	// 9. Get existing suggestions for the project associated with review
	mux.HandleFunc("GET /api/agent/reviews/{id}/suggestions", func(w http.ResponseWriter, r *http.Request) {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return
		}
		id := r.PathValue("id")
		review, err := svc.GetReview(id)
		if err != nil {
			apiError(w, 404, "复盘记录不存在")
			return
		}
		sess, err := svc.Get(review.SessionID)
		if err != nil || sess.Project.Path == "" {
			send(w, map[string]any{"suggestions": []any{}})
			return
		}
		items, err := svc.ListProjectSuggestions(sess.Project.Path)
		if err != nil {
			// If not enabled or other non-fatal, return empty list gracefully
			send(w, map[string]any{"suggestions": []any{}})
			return
		}
		send(w, map[string]any{"suggestions": items})
	})
}
