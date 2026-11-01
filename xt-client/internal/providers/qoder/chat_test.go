package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
)

func TestChatEnvelopeAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		plain, err := DecodeBody(string(raw))
		if err != nil {
			t.Error(err)
		}
		var request map[string]any
		json.Unmarshal([]byte(plain), &request)
		if request["temperature"] != 0.2 || request["model"] != "qfmodel" || request["business"] == nil || request["model_config"] == nil {
			t.Error("request fields missing")
		}
		msgs := request["messages"].([]any)
		m := msgs[0].(map[string]any)
		if m["role"] != "system" || m["content"] != "" || m["tool_calls"] != nil {
			t.Error("developer normalization failed")
		}
		frames := []string{
			`{"choices":[{"delta":{"reasoning_content":"think"}}]}`,
			`{"choices":[{"delta":{"content":"hello","tool_calls":[{"index":0,"id":"c","type":"function","function":{"name":"search","arguments":"{\"q\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":1}}}`,
			"[DONE]",
		}
		for _, f := range frames {
			data, _ := json.Marshal(map[string]any{"body": f})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	defer server.Close()
	cred := &providers.Credential{Region: "cn", UID: "u", AccessToken: "t", MachineID: "m"}
	c := ChatClient{Region: Region{InferBase: server.URL}, HTTP: server.Client()}
	body := map[string]any{"temperature": 0.2, "messages": []map[string]any{{"role": "developer", "content": nil, "tool_calls": []any{map[string]any{"id": "x"}}}}}
	result, err := c.Collect(context.Background(), cred, "qwen3.8-flash", providers.ModelInfo{UpstreamID: "qfmodel", Reasoning: true}, body)
	if err != nil {
		t.Fatal(err)
	}
	choice := result["choices"].([]any)[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if choice["finish_reason"] != "tool_calls" || msg["content"] != "hello" || msg["reasoning_content"] != "think" {
		t.Fatalf("wrong aggregate %#v", result)
	}
	tools := msg["tool_calls"].([]any)
	args := tools[0].(map[string]any)["function"].(map[string]any)["arguments"]
	if args != `{"q":1}` {
		t.Fatalf("fragmented arguments: %v", args)
	}
	if result["usage"].(map[string]any)["cached_tokens"] != int64(1) {
		t.Fatal("usage missing")
	}
}

func TestChatCancellationAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	for _, mode := range []string{"cancel", "idle", "total"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := ChatClient{Region: Region{InferBase: server.URL}, HTTP: server.Client()}
			switch mode {
			case "cancel":
				cancel()
			case "idle":
				c.IdleTimeout = 20 * time.Millisecond
			case "total":
				c.TotalTimeout = 20 * time.Millisecond
			}
			if err := c.Stream(ctx, &providers.Credential{}, providers.ModelInfo{UpstreamID: "m"}, nil, func(ChatEvent) error { return nil }); err == nil {
				t.Fatal("expected cancellation or timeout")
			}
		})
	}
}

func TestChatFailures(t *testing.T) {
	for _, frame := range []string{`{"body":"{\"code\":\"112\",\"details\":{\"error\":{\"message\":\"no entitlement\"}}}"}`, `{"body":"{\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "data: %s\n\n", frame) }))
		c := ChatClient{Region: Region{InferBase: server.URL}, HTTP: server.Client()}
		_, err := c.Collect(context.Background(), &providers.Credential{}, "m", providers.ModelInfo{UpstreamID: "m"}, map[string]any{})
		server.Close()
		if err == nil {
			t.Fatal("accepted business error or interrupted stream")
		}
	}
}
