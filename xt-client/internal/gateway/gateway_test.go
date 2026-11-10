package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/usage"
)

func setupGateway(t *testing.T, handler http.HandlerFunc) (*Service, modelsource.Source, *usage.Service) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	crypt := func(b []byte) ([]byte, error) {
		out := append([]byte(nil), b...)
		for i := range out {
			out[i] ^= 0x5a
		}
		return out, nil
	}
	store, err := modelsource.Open(t.TempDir(), crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	key := "isolated-upstream-secret"
	source, err := store.Save("", modelsource.Input{Label: "Test", Protocol: "openai-chat", BaseURL: server.URL + "/v1/chat/completions", APIKey: &key, Models: []modelsource.Model{{UpstreamModelID: "provider/model", Label: "Configured", ContextWindow: 4096, MaxOutputTokens: 256, SupportsImages: true, SupportsReasoning: true, ReasoningEfforts: []string{"low", "high"}}}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := usage.New(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u.Close() })
	service := New(store, "local-key", func(string) error { return nil })
	service.SetUsage(u)
	return service, source, u
}
func chatRequest(model string, stream bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[{"role":"user","content":"hello"}]}`, model, stream)))
	r.Header.Set("Authorization", "Bearer local-key")
	return r
}
func summary(t *testing.T, u *usage.Service) usage.RequestSummary {
	t.Helper()
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.RecentRequests) != 1 {
		t.Fatalf("expected one finished request: %+v", snap)
	}
	return snap.RecentRequests[0]
}
func completion(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"id":"upstream-id","object":"chat.completion","model":"provider/model","choices":[{"index":0,"message":{"role":"assistant","content":"ok","tool_calls":[{"id":"call1","type":"function","function":{"name":"run","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":8}}}`)
}

func TestNonStreamingPreservesStandardPayloadAndRealUsage(t *testing.T) {
	receivedPayload := make(chan map[string]any, 1)
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("duplicate/wrong path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer isolated-upstream-secret" {
			t.Error("wrong upstream auth")
		}
		var received map[string]any
		_ = json.NewDecoder(r.Body).Decode(&received)
		receivedPayload <- received
		completion(w)
	})
	raw := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,example"}}]}],"tools":[{"type":"function","function":{"name":"run","parameters":{"type":"object"}}}],"tool_choice":{"type":"function","function":{"name":"run"}},"parallel_tool_calls":true,"reasoning_effort":"high"}`, src.Models[0].ID)
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
	r.Header.Set("Authorization", "Bearer local-key")
	w := httptest.NewRecorder()
	svc.HandleChat(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	received := <-receivedPayload
	if received["model"] != "provider/model" || received["max_tokens"] != float64(256) || received["reasoning_effort"] != "high" || received["tools"] == nil || received["tool_choice"] == nil || received["parallel_tool_calls"] != true {
		t.Fatalf("payload lost: %+v", received)
	}
	messages := received["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["type"] != "image_url" {
		t.Fatal("image dropped")
	}
	var result map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result["model"] != src.Models[0].ID || !strings.HasPrefix(result["id"].(string), "chatcmpl-") {
		t.Fatal("public route IDs not restored")
	}
	rec := summary(t, u)
	if rec.Status != usage.StatusCompleted || rec.FirstTokenAt == nil || rec.Tokens.Prompt == nil || *rec.Tokens.Prompt != 20 || *rec.Tokens.Completion != 4 || *rec.Tokens.Cached != 8 {
		t.Fatalf("wrong usage: %+v", rec)
	}
}

func TestStreamPreservesToolsReasoningStableIDsAndUsage(t *testing.T) {
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stream_options"].(map[string]any)["include_usage"] != true {
			t.Error("real usage not requested")
		}
		if _, exists := body["reasoning_effort"]; exists {
			t.Error("capability list must not select an effort default")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"id":"one","model":"private","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"thinking"},"finish_reason":null}]}`,
			`{"id":"two","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call1","type":"function","function":{"name":"run","arguments":"{\"a\":"}}]},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":3}}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			w.(http.Flusher).Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	r := chatRequest(src.Models[0].ID, true)
	body := fmt.Sprintf(`{"model":%q,"stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hello"}]}`, src.Models[0].ID)
	r.Body = io.NopCloser(strings.NewReader(body))
	w := httptest.NewRecorder()
	svc.HandleChat(w, r)
	if w.Code != 200 || !strings.HasSuffix(w.Body.String(), "data: [DONE]\n\n") {
		t.Fatal(w.Code, w.Body.String())
	}
	var events []map[string]any
	var id string
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var event map[string]any
		_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event)
		events = append(events, event)
		if id == "" {
			id = event["id"].(string)
		}
		if event["id"] != id || event["model"] != src.Models[0].ID {
			t.Fatal("unstable public IDs")
		}
	}
	if len(events) != 5 {
		t.Fatalf("lost event: %s", w.Body.String())
	}
	choices := events[1]["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	tool := delta["tool_calls"].([]any)[0].(map[string]any)
	if tool["id"] != "call1" || tool["function"].(map[string]any)["arguments"] != `{"a":` {
		t.Fatalf("tool delta changed: %+v", tool)
	}
	if !strings.Contains(w.Body.String(), "thinking") {
		t.Fatal("reasoning lost")
	}
	rec := summary(t, u)
	if rec.Status != usage.StatusCompleted || rec.Tokens.Prompt == nil || *rec.Tokens.Prompt != 11 || *rec.Tokens.Completion != 7 || *rec.Tokens.Cached != 3 {
		t.Fatalf("usage lost: %+v", rec)
	}
}

func TestMissingUsageStaysUnknownAndUsageChunkIsOptional(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if stream {
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"many many words\"},\"finish_reason\":\"stop\"}]}\n\n")
				} else {
					_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"many many words"},"finish_reason":"stop"}]}`)
				}
			})
			w := httptest.NewRecorder()
			svc.HandleChat(w, chatRequest(src.Models[0].ID, stream))
			rec := summary(t, u)
			if rec.Status != usage.StatusCompleted || rec.Tokens.Prompt != nil || rec.Tokens.Completion != nil {
				t.Fatalf("missing usage fabricated: %+v", rec)
			}
		})
	}
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0}}\n\ndata: [DONE]\n\n")
	})
	w := httptest.NewRecorder()
	svc.HandleChat(w, chatRequest(src.Models[0].ID, true))
	if strings.Contains(w.Body.String(), `"usage"`) {
		t.Fatal("usage sent when include_usage is false")
	}
	rec := summary(t, u)
	if rec.Tokens.Prompt == nil || *rec.Tokens.Prompt != 0 || rec.Tokens.Completion == nil {
		t.Fatal("real zero usage lost")
	}
}

func TestStreamFailuresNeverEmitSuccessDelimiter(t *testing.T) {
	cases := []struct {
		name, body string
		status     string
	}{
		{"disconnect", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n", usage.StatusInterrupted},
		{"inband-error", "data: {\"error\":{\"message\":\"isolated-upstream-secret\"}}\n\n", usage.StatusFailed},
		{"malformed", "data: not-json\n\n", usage.StatusFailed},
		{"done-without-completion", "data: [DONE]\n\n", usage.StatusFailed},
		{"finish-error", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"error\"}]}\n\n", usage.StatusFailed},
		{"truncated-final-frame", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[]}", usage.StatusInterrupted},
		{"one-of-two-unfinished", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"},{\"index\":1,\"delta\":{},\"finish_reason\":null}]}\n\n", usage.StatusInterrupted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tc.body) })
			w := httptest.NewRecorder()
			svc.HandleChat(w, chatRequest(src.Models[0].ID, true))
			if strings.Contains(w.Body.String(), "[DONE]") || strings.Contains(w.Body.String(), "isolated-upstream-secret") {
				t.Fatal("failed stream claimed success or leaked secret", w.Body.String())
			}
			rec := summary(t, u)
			if rec.Status != tc.status {
				t.Fatal(rec.Status, tc.status, w.Body.String())
			}
		})
	}
}

func TestCancellationStopsUpstreamAndRecordsCanceled(t *testing.T) {
	entered := make(chan struct{})
	stopped := make(chan struct{})
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
		close(stopped)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := chatRequest(src.Models[0].ID, true).WithContext(ctx)
	w := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() { svc.HandleChat(w, r); close(finished) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream never entered")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("gateway did not cancel blocked stream")
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream context still alive")
	}
	if strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatal("cancel emitted success")
	}
	if rec := summary(t, u); rec.Status != usage.StatusCanceled {
		t.Fatal(rec.Status)
	}
}

func TestRedirectCannotForwardSecret(t *testing.T) {
	var hit atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		t.Error("redirect followed with credentials")
	}))
	defer other.Close()
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	})
	w := httptest.NewRecorder()
	svc.HandleChat(w, chatRequest(src.Models[0].ID, false))
	if w.Code != 502 || hit.Load() {
		t.Fatal("redirect not blocked")
	}
	if summary(t, u).Status != usage.StatusFailed {
		t.Fatal("redirect not failed")
	}
}

func TestAuthenticationRoutesAndKeyRotation(t *testing.T) {
	var hit atomic.Bool
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { hit.Store(true); completion(w) })
	r := chatRequest(src.Models[0].ID, false)
	r.Header.Del("Authorization")
	w := httptest.NewRecorder()
	svc.HandleChat(w, r)
	if w.Code != 401 || hit.Load() {
		t.Fatal("unauthorized request passed")
	}
	snapshot, _ := u.Snapshot()
	if len(snapshot.RecentRequests) != 0 || snapshot.ActiveCount != 0 {
		t.Fatal("unauthorized request tracked")
	}
	for _, model := range []string{"provider/model", "trae/provider/model", "model_missing"} {
		w := httptest.NewRecorder()
		svc.HandleChat(w, chatRequest(model, false))
		if w.Code != 400 || hit.Load() {
			t.Fatal("unknown/legacy route accepted")
		}
	}
	models := svc.Models()
	if len(models) != 1 || models[0]["id"] != src.Models[0].ID {
		t.Fatal("wrong models")
	}
	if models[0]["supports_images"] != true || models[0]["max_input_tokens"] != 4096 || models[0]["max_output_tokens"] != 256 || models[0]["source_id"] != src.ID {
		t.Fatalf("public model capability contract lost: %+v", models[0])
	}
	saved := ""
	svc.saveKey = func(key string) error { saved = key; return nil }
	old := svc.APIKey()
	key, err := svc.RotateKey()
	if err != nil || key == old || key != saved {
		t.Fatal("rotation failed", err)
	}
	svc.saveKey = func(string) error { return errors.New("disk failed") }
	if _, err := svc.RotateKey(); err == nil || svc.APIKey() != key {
		t.Fatal("failed key save changed runtime key")
	}
}

func TestUpstreamErrorsAndTestEndpointAreSafe(t *testing.T) {
	for _, mode := range []string{"http", "json", "incomplete", "valid"} {
		t.Run(mode, func(t *testing.T) {
			svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "http":
					w.WriteHeader(401)
					_, _ = io.WriteString(w, "isolated-upstream-secret")
				case "json":
					_, _ = io.WriteString(w, `{"error":{"message":"isolated-upstream-secret"}}`)
				case "incomplete":
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"partial"},"finish_reason":null}]}`)
				default:
					completion(w)
				}
			})
			err := svc.Test(context.Background(), src.ID)
			if (err == nil) != (mode == "valid") {
				t.Fatal("unexpected test result", mode, err)
			}
			if err != nil && strings.Contains(err.Error(), "isolated-upstream-secret") {
				t.Fatal("test leaked key")
			}
			snap, _ := u.Snapshot()
			if len(snap.RecentRequests) != 0 {
				t.Fatal("test generated usage record")
			}
			w := httptest.NewRecorder()
			svc.HandleChat(w, chatRequest(src.Models[0].ID, false))
			if mode != "valid" {
				if w.Code != 502 || strings.Contains(w.Body.String(), "isolated-upstream-secret") || summary(t, u).Status != usage.StatusFailed {
					t.Fatal("unsafe upstream failure", w.Code, w.Body.String())
				}
			}
		})
	}
}
