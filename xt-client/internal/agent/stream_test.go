package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamAssemblesFragmentedToolsAndRejectsTruncation(t *testing.T) {
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"你好","tool_calls":[{"index":0,"id":"c1","function":{"name":"read_","arguments":"{\"pa"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"世界","tool_calls":[{"index":0,"function":{"name":"file","arguments":"th\":\"x\"}"}},{"index":1,"id":"c2","function":{"name":"list_files","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		``, `data: [DONE]`, ``,
	}, "\n")
	var fragments []string
	result, err := readModelStream(strings.NewReader(input), func(s string) { fragments = append(fragments, s) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fragments, "|") != "你好|世界" || result.Content != "你好世界" || len(result.ToolCalls) != 2 {
		t.Fatalf("unexpected stream: %+v %v", result, fragments)
	}
	if result.ToolCalls[0].ID != "c1" || result.ToolCalls[0].Name != "read_file" || string(result.ToolCalls[0].Arguments) != `{"path":"x"}` || result.ToolCalls[1].ID != "c2" {
		t.Fatalf("bad tools: %+v", result.ToolCalls)
	}
	if _, err := readModelStream(strings.NewReader(strings.Replace(input, "data: [DONE]", "", 1)), nil); err == nil {
		t.Fatal("truncated stream accepted")
	}
	invalid := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c","function":{"name":"write_file","arguments":"{"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	if _, err := readModelStream(strings.NewReader(invalid), nil); err == nil {
		t.Fatal("incomplete arguments accepted")
	}
}

func TestStreamWithoutToolIndicesKeepsDistinctCalls(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"tool_calls":[{"id":"one","function":{"name":"read_file","arguments":"{}"}},{"id":"two","function":{"name":"list_files","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	result, err := readModelStream(strings.NewReader(stream), nil)
	if err != nil || len(result.ToolCalls) != 2 || result.ToolCalls[0].ID != "one" || result.ToolCalls[1].ID != "two" {
		t.Fatalf("unindexed calls: %+v %v", result, err)
	}
}

func TestStreamCapturesOutputUsageAndFirstToken(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"第一段"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"第二段"},"finish_reason":"stop"}]}`,
		``,
		`data: {"choices":[],"usage":{"completion_tokens":42}}`,
		``, `data: [DONE]`, ``,
	}, "\n")
	result, err := readModelStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputTokens != 42 || result.FirstOutputAt == nil || result.Content != "第一段第二段" {
		t.Fatalf("unexpected output metrics: %+v", result)
	}
}

func TestServiceStreamsBeforeCompletionAndPersists(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req["stream"] != true {
			t.Errorf("stream request missing: %v %v", req, err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if tools, _ := req["tools"].([]any); len(tools) == 0 {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"理解请求"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"第二段\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	session, err := svc.Create(Options{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 10)
	finished := make(chan error, 1)
	go func() { finished <- svc.Send(context.Background(), session.ID, "hello", func(e Event) { events <- e }) }()
	var first Event
	deadline := time.After(3 * time.Second)
	for first.Type != "message_delta" {
		select {
		case first = <-events:
		case <-deadline:
			close(release)
			t.Fatal("no text before stream finished")
		}
	}
	if first.Message.Content != "第一段" {
		t.Fatalf("wrong first delta: %+v", first)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 || loaded.Messages[1].Content != "第一段第二段" || loaded.Messages[1].ID != first.Message.ID {
		t.Fatalf("bad stored messages: %+v", loaded.Messages)
	}
}

func TestStreamedToolCallsAreExecutedAfterCompleteArguments(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if len(req.Tools) == 0 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"先查看项目。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		calls++
		if calls == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"先看看。\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"list_files\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\".\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			found := false
			for _, m := range req.Messages {
				if m["role"] == "tool" && m["tool_call_id"] == "call_1" {
					found = true
				}
			}
			if !found {
				t.Error("tool result missing from next model request")
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"完成。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	session, err := svc.Create(Options{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err := svc.Send(context.Background(), session.ID, "查看", func(e Event) { events = append(events, e) }); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(saved.Activity) != 1 || saved.Activity[0].Status != "completed" {
		t.Fatalf("bad tool execution: %+v", saved)
	}
	count := 0
	var replies []string
	for _, message := range saved.Messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 0 {
			count++
			replies = append(replies, message.Content)
		}
	}
	if count != 1 || replies[0] != "完成。" {
		t.Fatalf("intermediate narration should not be a reply: %+v", saved.Messages)
	}
}

func TestCanceledStreamKeepsPartialReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		if tools, _ := req["tools"].([]any); len(tools) == 0 {
			fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"理解请求"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"部分内容\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	session, err := svc.Create(Options{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Send(context.Background(), session.ID, "测试", func(e Event) {
		if e.Type == "message_delta" {
			svc.Cancel(session.ID)
		}
	})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	saved, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != StatusCanceled || len(saved.Messages) != 2 || saved.Messages[1].Content != "部分内容" {
		t.Fatalf("partial output lost: %+v", saved)
	}
}
