package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestAgentHistoryRoute(t *testing.T) {
	svc := agent.NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	item, err := svc.Create(agent.Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAgentRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://localhost" },
		func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) },
		func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
	path := "/api/agent/sessions/" + item.ID + "/history"
	for _, tc := range []struct {
		path, origin string
		status       int
	}{
		{path, "http://localhost", 200}, {path, "http://other", 403},
		{path + "?before=bad", "http://localhost", 400},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if w.Code == 200 {
			var page agent.HistoryPage
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Session == nil || page.Session.ID != item.ID || page.Before != "" {
				t.Fatalf("page: %s %v", w.Body.String(), err)
			}
		}
	}
}
