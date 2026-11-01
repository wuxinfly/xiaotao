package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPModelClientTranslatesErrorsAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"README.md\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer server.Close()
	client := &httpModelClient{endpoint: server.URL + "/v1/chat/completions", key: func() string { return "test-key" }, client: server.Client()}
	result, err := client.Complete(context.Background(), "test/model", []map[string]any{{"role": "user", "content": "read"}}, []map[string]any{{"type": "function"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool call not decoded: %+v", result)
	}
	var args map[string]string
	if err := json.Unmarshal(result.ToolCalls[0].Arguments, &args); err != nil || args["path"] != "README.md" {
		t.Fatalf("bad arguments: %s", result.ToolCalls[0].Arguments)
	}
}

func TestHTTPModelClientSurfacesBackendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"message":"该模型没有可用的已启用账号"}}`))
	}))
	defer server.Close()
	client := &httpModelClient{endpoint: server.URL + "/v1/chat/completions", client: server.Client()}
	_, err := client.Complete(context.Background(), "test/model", []map[string]any{{"role": "user", "content": "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "没有可用") {
		t.Fatalf("expected backend message, got %v", err)
	}
}
