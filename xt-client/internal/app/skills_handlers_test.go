package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestSkillsRoutesOriginValidationAndDraftIsolation(t *testing.T) {
	svc := agent.NewService(t.TempDir(), "", nil, nil, nil)
	mux := http.NewServeMux()
	registerAgentRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://localhost" }, func(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }, func(w http.ResponseWriter, status int, msg string) { http.Error(w, msg, status) })
	for _, tc := range []struct {
		method, path, body string
		origin             bool
		status             int
	}{
		{"GET", "/api/agent/skills", "", false, 403},
		{"PUT", "/api/agent/skills", `{"directories":[]}`, false, 403},
		{"POST", "/api/agent/skills/scan", `{"directories":[]}`, false, 403},
		{"PUT", "/api/agent/skills", `{"directories":[{"path":"relative"}]}`, true, 400},
		{"PUT", "/api/agent/skills", `{"directories":[],"execute":"command"}`, true, 400},
		{"PUT", "/api/agent/skills", `{"directories":[]} {}`, true, 400},
		{"POST", "/api/agent/skills/scan", `{"directories":[]}`, true, 200},
		{"PUT", "/api/agent/skills", `{"directories":[],"disabledSkills":[]}`, true, 200},
		{"GET", "/api/agent/skills", "", true, 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.origin {
			r.Header.Set("Origin", "http://localhost")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	settings := agent.SkillsSettings{Directories: []agent.SkillDirectory{{Path: t.TempDir()}}}
	body, _ := json.Marshal(settings)
	r := httptest.NewRequest("POST", "/api/agent/skills/scan", strings.NewReader(string(body)))
	r.Header.Set("Origin", "http://localhost")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	saved, _ := svc.SkillsSettings()
	if len(saved.Directories) != 0 {
		t.Fatal("scan saved draft")
	}
}
