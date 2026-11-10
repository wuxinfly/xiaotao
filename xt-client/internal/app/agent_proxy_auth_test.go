package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Exercise the complete Agent -> local gateway -> configured upstream path,
// including key rotation. No platform accounts or real API keys are involved.
func TestAgentUsesCurrentProxyKey(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer upstream-fixture" || body["model"] != "org/model" {
			t.Errorf("unexpected upstream route/header: %s, model=%v", r.URL.Path, body["model"])
		}
		calls.Add(1)
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"chat-fixture\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"连接成功\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chat-fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"连接成功"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`)
		}
	}))
	defer upstream.Close()
	server := httptest.NewUnstartedServer(nil)
	url := "http://" + server.Listener.Addr().String()
	handler, err := Handler(State{URL: url, Token: "management-token"}, t.TempDir(), func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	call := func(path, body string) string {
		t.Helper()
		req, err := http.NewRequest("POST", url+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", url)
		req.Header.Set("Content-Type", "application/json")
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, res.StatusCode, raw)
		}
		return string(raw)
	}
	raw := call("/api/model-sources", `{"label":"测试接口","protocol":"openai-chat","baseURL":"`+upstream.URL+`/v1","apiKey":"upstream-fixture","models":[{"upstreamModelId":"org/model","contextWindow":32000,"maxOutputTokens":2048}]}`)
	var source struct{ Models []struct{ ID string } }
	if err = json.Unmarshal([]byte(raw), &source); err != nil || len(source.Models) != 1 {
		t.Fatalf("source: %s %v", raw, err)
	}
	raw = call("/api/agent/sessions", `{"model":"`+source.Models[0].ID+`"}`)
	var session struct{ ID string }
	if err = json.Unmarshal([]byte(raw), &session); err != nil || session.ID == "" {
		t.Fatalf("create: %s %v", raw, err)
	}
	send := func() {
		result := call("/api/agent/sessions/"+session.ID+"/messages", `{"content":"hello"}`)
		if !strings.Contains(result, "连接成功") || strings.Contains(result, `"status":"failed"`) {
			t.Fatalf("agent gateway call failed: %s", result)
		}
	}
	send()
	call("/api/proxy/key", "{}")
	send()
	if calls.Load() < 2 {
		t.Fatalf("upstream not called twice: %d", calls.Load())
	}
}
