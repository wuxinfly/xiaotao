package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise real Agent routes, proxy authentication, and key rotation.
func TestAgentUsesCurrentProxyKey(t *testing.T) {
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
	raw := call("/api/agent/sessions", "{\"model\":\"trae/test\"}")
	var session struct{ ID string }
	if err = json.Unmarshal([]byte(raw), &session); err != nil || session.ID == "" {
		t.Fatalf("create: %s %v", raw, err)
	}
	send := func() {
		t.Helper()
		result := call("/api/agent/sessions/"+session.ID+"/messages", "{\"content\":\"hello\"}")
		if strings.Contains(result, "代理访问密钥无效") || !strings.Contains(result, "该模型没有可用的已启用账号") {
			t.Fatalf("proxy authentication/routing failed: %s", result)
		}
	}
	send()
	call("/api/proxy/key", "{}")
	send()
}
