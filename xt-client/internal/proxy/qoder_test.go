package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/qoder"
	"xiaotao/xt-client/internal/storage"
)

func TestQoderQueueRetriesSameAccount(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrapped403=%t", wrapped), func(t *testing.T) {
			testQoderQueueRetriesSameAccount(t, wrapped)
		})
	}
}

func testQoderQueueRetriesSameAccount(t *testing.T, wrapped bool) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "model/list") {
			fmt.Fprint(w, `{"chat":[{"key":"qfmodel","display_name":"Qwen3.8-Flash"}]}`)
			return
		}
		hits++
		if hits == 1 {
			payload := map[string]any{"code": "10605", "message": `{"retryAfterSeconds":1,"subCode":"RESOURCE_LIMIT"}`}
			if wrapped {
				inner, _ := json.Marshal(payload)
				payload = map[string]any{"code": "403", "message": string(inner)}
			}
			inner, _ := json.Marshal(payload)
			frame, _ := json.Marshal(map[string]any{"body": string(inner)})
			fmt.Fprintf(w, "data: %s\n\n", frame)
			return
		}
		frame, _ := json.Marshal(map[string]any{"body": `{"choices":[{"delta":{"content":"after-queue"},"finish_reason":"stop"}]}`})
		fmt.Fprintf(w, "data: %s\n\ndata: {\"body\":\"[DONE]\"}\n\n", frame)
	}))
	defer server.Close()
	originalRegion, originalWait, originalSleep := qoder.Regions["cn"], qoderQueueWait, qoderQueueSleep
	reg := originalRegion
	reg.InferBase = server.URL
	qoder.Regions["cn"] = reg
	var waits []time.Duration
	qoderQueueWait = func(err error) time.Duration {
		wait := originalWait(err)
		waits = append(waits, wait)
		return wait
	}
	qoderQueueSleep = func(ctx context.Context, wait time.Duration) error { return nil }
	defer func() {
		qoder.Regions["cn"] = originalRegion
		qoderQueueWait = originalWait
		qoderQueueSleep = originalSleep
	}()
	store, _ := storage.Open(t.TempDir())
	acct := storage.Account{ID: "qoder:cn:u", Provider: "qoder", UID: "u", Region: "cn", Enabled: true}
	store.UpsertAccount(&acct)
	raw, _ := json.Marshal(providers.Credential{AccessToken: "t"})
	store.SaveSecret(acct.ID, raw, func(b []byte) ([]byte, error) { return b, nil })
	svc := New(store, func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}, "key")
	for _, stream := range []bool{false, true} {
		hits = 0
		waits = nil
		request := fmt.Sprintf(`{"model":"qoder/qwen3.8-flash","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream)
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(request))
		req = req.WithContext(context.Background())
		req.Header.Set("Authorization", "Bearer key")
		w := httptest.NewRecorder()
		svc.HandleChat(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "after-queue") {
			t.Fatalf("stream=%t failed %d %s", stream, w.Code, w.Body.String())
		}
		if hits != 2 || len(waits) != 1 || waits[0] <= 0 {
			t.Fatalf("stream=%t hits=%d waits=%v", stream, hits, waits)
		}
	}
}

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

func TestQoderQueueDoesNotReplayOutput(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"partial"}}]}`+"\n\n")
		inner, _ := json.Marshal(map[string]any{"code": "10605", "message": `{"retryAfterSeconds":1}`})
		frame, _ := json.Marshal(map[string]any{"code": "403", "message": string(inner)})
		fmt.Fprintf(w, "data: %s\n\n", frame)
	}))
	defer server.Close()
	client := qoder.ChatClient{Region: qoder.Region{InferBase: server.URL}, HTTP: server.Client()}
	events := 0
	err := qoderWithQueue(context.Background(), client, &providers.Credential{}, providers.ModelInfo{UpstreamID: "m"}, nil, func(qoder.ChatEvent) error {
		events++
		return nil
	})
	if err == nil || hits != 1 || events != 1 {
		t.Fatalf("err=%v hits=%d events=%d", err, hits, events)
	}
}
