package workbuddy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"xiaotao/xt-client/internal/providers"
)

var testOnce sync.Once
var testCred *providers.Credential

func setupChatTest(t *testing.T) *providers.Credential {
	t.Helper()
	if testCred != nil {
		return testCred
	}
	return &providers.Credential{AccessToken: "test-token"}
}

func TestCollector_SendsOpenAIRequestBody(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}],\"usage\":null}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	defer SetBaseURL(server.URL)()

	cred := &providers.Credential{AccessToken: "test-token"}
	body := map[string]any{
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
		"tools":    []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file", "parameters": map[string]any{}}}},
	}
	result, err := Collector{}.Collect(t.Context(), cred, "auto", body)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := gotBody["function"]; ok {
		t.Errorf("request body should not contain trae function field, got: %v", gotBody)
	}
	if gotBody["stream"] != true {
		t.Errorf("expected stream=true, got %v", gotBody["stream"])
	}
	if gotBody["model"] != "auto" {
		t.Errorf("expected model auto, got %v", gotBody["model"])
	}

	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg, _ := msgs[0].(map[string]any)
	if msg["role"] != "user" || msg["content"] != "hi" {
		t.Errorf("expected raw OpenAI message, got %v", msg)
	}

	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	fn, _ := tool["function"].(map[string]any)
	if fn["name"] != "read_file" {
		t.Errorf("expected tool function name read_file, got %v", fn["name"])
	}
	if _, isString := fn["parameters"].(string); isString {
		t.Errorf("parameters should stay as JSON object for workbuddy, got string")
	}

	choice0, _ := result["choices"].([]any)
	first, _ := choice0[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	if message["content"] != "hi" {
		t.Errorf("expected content hi, got %v", message["content"])
	}
	usage, _ := result["usage"].(map[string]any)
	if usage["prompt_tokens"] != int64(1) || usage["completion_tokens"] != int64(2) {
		t.Errorf("unexpected usage: %v", usage)
	}
}

func TestStreamer_ReceivesStreamedDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		flusher.Flush()
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"},\"finish_reason\":null}]}\n\n"))
		flusher.Flush()
		time.Sleep(20 * time.Millisecond)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4}}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	defer SetBaseURL(server.URL)()

	cred := &providers.Credential{AccessToken: "test-token"}
	var content string
	var usage map[string]any
	events := &providers.ChatEvents{
		OnDelta: func(delta map[string]any) {
			if c, ok := delta["content"].(string); ok {
				content += c
			}
		},
		OnUsage: func(u map[string]any) {
			usage = u
		},
	}
	err := Streamer{}.Stream(t.Context(), cred, "auto", map[string]any{"messages": []any{}}, events)
	if err != nil {
		t.Fatal(err)
	}
	if content != "Hello" {
		t.Errorf("expected Hello, got %q", content)
	}
	if usage["prompt_tokens"] != int64(3) {
		t.Errorf("expected prompt_tokens 3, got %v", usage)
	}
}

func TestStreamer_DoesNotUseAuthClientTimeout(t *testing.T) {
	previous := defaultClient
	defaultClient = &http.Client{Timeout: 20 * time.Millisecond}
	defer func() { defaultClient = previous }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		flusher.Flush()
		time.Sleep(60 * time.Millisecond)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"slow\"},\"finish_reason\":\"stop\"}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	defer SetBaseURL(server.URL)()

	cred := &providers.Credential{AccessToken: "test-token"}
	var content string
	events := &providers.ChatEvents{
		OnDelta: func(delta map[string]any) {
			if c, ok := delta["content"].(string); ok {
				content += c
			}
		},
	}
	err := Streamer{}.Stream(t.Context(), cred, "auto", map[string]any{"messages": []any{}}, events)
	if err != nil {
		t.Fatal(err)
	}
	if content != "slow" {
		t.Errorf("expected slow, got %q", content)
	}
}

func TestStreamer_IdleTimeoutInterruptsBlockedRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	defer SetBaseURL(server.URL)()

	cred := &providers.Credential{AccessToken: "test-token"}
	err := streamWithTimeouts(t.Context(), cred, "auto", map[string]any{"messages": []any{}}, &providers.ChatEvents{}, 25*time.Millisecond, time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected idle deadline error, got %v", err)
	}
	if err.Error() != errStreamIdleTimeout.Error() {
		t.Fatalf("expected idle timeout message, got %v", err)
	}
}

func TestStreamer_ParentCancellationIsNotReportedAsIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	defer SetBaseURL(server.URL)()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cred := &providers.Credential{AccessToken: "test-token"}
	done := make(chan error, 1)
	go func() {
		done <- streamWithTimeouts(ctx, cred, "auto", map[string]any{"messages": []any{}}, &providers.ChatEvents{}, time.Second, time.Second)
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected request cancellation, got %v", err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancellation should not be reported as idle timeout: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not stop")
	}
}

func TestCollector_UpstreamErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer server.Close()
	defer SetBaseURL(server.URL)()

	cred := &providers.Credential{AccessToken: "test-token"}
	_, err := Collector{}.Collect(t.Context(), cred, "auto", map[string]any{"messages": []any{}})
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*providers.UpstreamError)
	if !ok {
		t.Fatalf("expected *providers.UpstreamError, got %T: %v", err, err)
	}
	if pe.Status != 429 {
		t.Errorf("expected status 429, got %d", pe.Status)
	}
}
