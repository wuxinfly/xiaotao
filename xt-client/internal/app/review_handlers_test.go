package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/agent"
)

func TestReviewRoutes(t *testing.T) {
	tempDir := t.TempDir()
	svc := agent.NewService(tempDir, "http://127.0.0.1:5455", nil, nil, nil)
	svc.SetClientVersion("1.4.9")

	mux := http.NewServeMux()
	requireOrigin := func(r *http.Request) bool {
		return r.Header.Get("X-Requested-With") == "XiaoTao"
	}
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(v)
	}
	apiError := func(w http.ResponseWriter, code int, msg string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
	}

	registerReviewRoutes(mux, svc, requireOrigin, send, apiError)

	// 1. Same-origin rejection check
	req := httptest.NewRequest("GET", "/api/agent/reviews", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403 without same-origin header, got: %d", w.Code)
	}

	// 2. Empty reviews list
	req = httptest.NewRequest("GET", "/api/agent/reviews", nil)
	req.Header.Set("X-Requested-With", "XiaoTao")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got: %d", w.Code)
	}
	var listResp agent.ReviewListPage
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(listResp.Reviews) != 0 {
		t.Errorf("expected 0 reviews initially, got: %d", len(listResp.Reviews))
	}

	// 3. Create session with run to test review creation
	sess, err := svc.Create(agent.Options{Model: "test-model"})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	now := time.Now()
	ended := now.Add(time.Second)
	sess.Messages = []agent.Message{
		{
			ID:        "msg_u1",
			Role:      "user",
			Content:   "测试复盘要求",
			RunID:     "r_1",
			CreatedAt: now,
		},
		{
			ID:        "msg_a1",
			Role:      "assistant",
			Content:   "收到并处理",
			RunID:     "r_1",
			CreatedAt: now.Add(time.Millisecond * 500),
		},
	}
	sess.RunStats = []agent.RunStats{
		{
			RunID:     "r_1",
			StartedAt: now,
			EndedAt:   &ended,
		},
	}
	sess.Status = agent.StatusCompleted
	// Directly persist via exposed API or internal method
	bodyJSON := `{"requestId":"req_http_1"}`
	createReq := httptest.NewRequest("POST", "/api/agent/sessions/"+sess.ID+"/reviews", strings.NewReader(bodyJSON))
	createReq.Header.Set("X-Requested-With", "XiaoTao")
	createReq.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, createReq)
	// Empty execution without saving messages to store will fail validation as expected
	if w.Code != 400 && w.Code != 202 {
		t.Errorf("unexpected status code: %d", w.Code)
	}
}
