package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestMCPRoutesOriginAndValidation(t *testing.T) {
	svc := agent.NewService(t.TempDir(), "", nil, nil, nil)
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	mux := http.NewServeMux()
	registerAgentRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://localhost" }, func(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }, func(w http.ResponseWriter, status int, msg string) { http.Error(w, msg, status) })
	for _, tc := range []struct {
		method, path, body string
		origin             bool
		status             int
	}{
		{"GET", "/api/agent/mcp", "", false, 403},
		{"PUT", "/api/agent/mcp", `{"mcpServers":{}}`, false, 403},
		{"POST", "/api/agent/mcp/test", `{"command":"missing"}`, false, 403},
		{"PUT", "/api/agent/mcp", `{"mcpServers":{"x":{"command":""}}}`, true, 400},
		{"PUT", "/api/agent/mcp", `{"mcpServers":{"x":{"command":"node","env":{"SECRET":"value"}}}}`, true, 400},
		{"PUT", "/api/agent/mcp", `{"mcpServers":{"x":{"command":"node","args":[],"disabled":true}}}`, true, 200},
		{"GET", "/api/agent/mcp", "", true, 200},
		{"POST", "/api/agent/mcp/test", `{"command":""}`, true, 400},
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
}
