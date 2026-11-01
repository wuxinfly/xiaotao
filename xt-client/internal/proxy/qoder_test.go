package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/qoder"
	"xiaotao/xt-client/internal/storage"
)

func TestQoderProxyEntry(t *testing.T) {
	failure := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "model/list") {
			fmt.Fprint(w, `{"chat":[{"key":"qfmodel","display_name":"Qwen3.8-Flash"}]}`)
			return
		}
		if failure {
			w.WriteHeader(403)
			return
		}
		frame, _ := json.Marshal(map[string]any{"body": `{"choices":[{"delta":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", frame)
	}))
	defer server.Close()
	original := qoder.Regions["cn"]
	reg := original
	reg.InferBase = server.URL
	qoder.Regions["cn"] = reg
	defer func() { qoder.Regions["cn"] = original }()
	store, _ := storage.Open(t.TempDir())
	acct := storage.Account{ID: "qoder:cn:u", Provider: "qoder", UID: "u", Region: "cn", Enabled: true}
	store.UpsertAccount(&acct)
	raw, _ := json.Marshal(providers.Credential{AccessToken: "t"})
	store.SaveSecret(acct.ID, raw, func(b []byte) ([]byte, error) { return b, nil })
	svc := New(store, func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}, "key")
	for _, stream := range []bool{false, true} {
		request := fmt.Sprintf(`{"model":"qoder/qwen3.8-flash","stream":%t,"messages":[{"role":"user","content":"hi"}],"temperature":0.2}`, stream)
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(request))
		req.Header.Set("Authorization", "Bearer key")
		w := httptest.NewRecorder()
		svc.HandleChat(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "hello") {
			t.Fatalf("proxy failed %d %s", w.Code, w.Body.String())
		}
		if stream && !strings.Contains(w.Body.String(), "[DONE]") {
			t.Fatal("missing completion")
		}
	}
	failure = true
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"qoder/qwen3.8-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer key")
	w := httptest.NewRecorder()
	svc.HandleChat(w, req)
	if w.Code != 403 || strings.Contains(w.Header().Get("Content-Type"), "event-stream") || strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("upstream failure disguised as stream: %d %s", w.Code, w.Body.String())
	}
}
