package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestAgentTitleRouteGeneratesWithoutRenaming(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"标题：会话标题生成"}}]}`))
	}))
	defer modelServer.Close()
	svc := agent.NewService(t.TempDir(), modelServer.URL, nil, modelServer.Client(), nil)
	item, err := svc.Create(agent.Options{Model: "test/title"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Send(t.Context(), item.ID, "给会话重命名加一个 AI 生成标题", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAgentRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://localhost" },
		func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) },
		func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
	path := "/api/agent/sessions/" + item.ID + "/title"
	for _, tc := range []struct {
		path, origin string
		status       int
		want         string
	}{
		{path, "http://other", 403, "same-origin"},
	} {
		r := httptest.NewRequest(http.MethodPost, tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"test/title","modelSettings":{"contextWindow":32000}}`))
	r.Header.Set("Origin", "http://localhost")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "会话标题生成") {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	loaded, err := svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title == "会话标题生成" {
		t.Fatal("title generation must not rename the session")
	}
}
