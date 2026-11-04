package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/agent"
)

func TestAgentReferenceRoutes(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"结论 A"}}]}`))
	}))
	defer modelServer.Close()
	svc := agent.NewService(t.TempDir(), modelServer.URL, nil, modelServer.Client(), nil)
	project := t.TempDir()
	source, err := svc.Create(agent.Options{ProjectPath: project, ProjectName: "项目", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(t.Context(), source.ID, "登录讨论", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	source, err = svc.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	source.Title = "登录讨论"
	if _, err := svc.Rename(source.ID, source.Title); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAgentRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://localhost" },
		func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) },
		func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
	for _, tc := range []struct {
		path     string
		origin   string
		wantCode int
		wantText string
	}{
		{"/api/agent/references/sessions?scope=project&projectPath=" + project, "http://localhost", 200, "登录讨论"},
		{"/api/agent/references/sessions?scope=bad", "http://localhost", 400, "引用搜索范围无效"},
		{"/api/agent/references/sessions?scope=all", "http://other", 403, "same-origin"},
		{"/api/agent/references/missing", "http://localhost", 404, "引用不存在"},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.wantCode || !strings.Contains(w.Body.String(), tc.wantText) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
