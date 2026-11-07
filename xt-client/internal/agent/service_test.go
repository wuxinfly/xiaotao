package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeModel struct {
	mu       sync.Mutex
	replies  []*completion
	messages [][]map[string]any
}

func (f *fakeModel) Complete(_ context.Context, _ string, messages []map[string]any, tools []map[string]any, _ func(string)) (*completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if isSummaryRequest(messages) {
		return &completion{Content: "确认请求。"}, nil
	}
	f.messages = append(f.messages, messages)
	if len(f.replies) == 0 {
		return &completion{Content: "done"}, nil
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	return reply, nil
}

type statsModel struct {
	mu      sync.Mutex
	replies []*completion
}

func (m *statsModel) Complete(_ context.Context, _ string, messages []map[string]any, _ []map[string]any, _ func(string)) (*completion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isSummaryRequest(messages) {
		return &completion{Content: "确认请求。"}, nil
	}
	if len(m.replies) == 0 {
		return &completion{Content: "done"}, nil
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	return reply, nil
}

func TestRunStatsCountModelRequestsAndMeasuredOutputSpeed(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-2 * time.Second)
	first := started.Add(500 * time.Millisecond)
	ended := started.Add(1500 * time.Millisecond)
	model := &statsModel{replies: []*completion{{
		Content:        "完成",
		OutputTokens:   80,
		FirstOutputAt:  &first,
		RequestEndedAt: ended,
	}}}
	stats := RunStats{RunID: "turn_stats", StartedAt: started}
	var outputSeconds float64
	if err := svc.loopWithRun(context.Background(), nil, model, history, nil, nil, stats.RunID, &stats, &outputSeconds, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	stats.OutputSpeed = float64(stats.OutputTokens) / outputSeconds
	if stats.ModelRequests != 1 || stats.OutputTokens != 80 || stats.OutputSpeed != 80 {
		t.Fatalf("unexpected stats: %+v outputSeconds=%v", stats, outputSeconds)
	}
}

// streamingModel replays deltas so the run can measure live output.
type streamingModel struct {
	mu      sync.Mutex
	started time.Time
	first   time.Time
	delays  []time.Duration
}

func (m *streamingModel) Complete(_ context.Context, _ string, messages []map[string]any, _ []map[string]any, onDelta func(string)) (*completion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isSummaryRequest(messages) {
		return &completion{Content: "确认请求。"}, nil
	}
	m.started = time.Now()
	for i, delay := range m.delays {
		time.Sleep(delay)
		m.first = time.Now()
		if onDelta != nil {
			onDelta(strings.Repeat("字", 30))
		}
		_ = i
	}
	return &completion{Content: "done", OutputTokens: 60, RequestStartedAt: m.started, FirstOutputAt: &m.first, RequestEndedAt: time.Now()}, nil
}

func TestRunStatsReportsFirstTokenLatencyAndLiveOutput(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	model := &streamingModel{delays: []time.Duration{60 * time.Millisecond}}
	stats := RunStats{RunID: "turn_live", StartedAt: time.Now()}
	var outputSeconds float64
	if err := svc.loopWithRun(context.Background(), nil, model, history, nil, nil, stats.RunID, &stats, &outputSeconds, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if stats.FirstTokenMs <= 0 {
		t.Fatalf("expected a first-token latency, got %+v", stats)
	}
	if stats.LiveTokens != 60 {
		t.Fatalf("expected the live token count to fall back to the reported usage, got %+v", stats)
	}
}

// streamingMetricsModel replays deltas so the run can be observed while it
// is still producing output.
type streamingMetricsModel struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (m *streamingMetricsModel) Complete(_ context.Context, _ string, messages []map[string]any, _ []map[string]any, onDelta func(string)) (*completion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isSummaryRequest(messages) {
		return &completion{Content: "确认请求。"}, nil
	}
	started := time.Now()
	var first *time.Time
	for _, delay := range m.delays {
		time.Sleep(delay)
		at := time.Now()
		if first == nil {
			first = &at
		}
		if onDelta != nil {
			onDelta(strings.Repeat("字", 60))
		}
	}
	return &completion{Content: "done", OutputTokens: 160, RequestStartedAt: started, FirstOutputAt: first, RequestEndedAt: time.Now()}, nil
}

func TestRunStatsEventsCarryLiveMetricsWhileStreaming(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	model := &streamingMetricsModel{delays: []time.Duration{80 * time.Millisecond, 80 * time.Millisecond, 80 * time.Millisecond}}
	var events []RunStats
	stats := RunStats{RunID: "turn_metrics", StartedAt: time.Now()}
	var outputSeconds float64
	if err := svc.loopWithRun(context.Background(), nil, model, history, nil, nil, stats.RunID, &stats, &outputSeconds, func(event Event) {
		if event.RunStats != nil {
			events = append(events, *event.RunStats)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected run_stats events")
	}
	sawLiveTokens := false
	sawLiveSpeed := false
	sawFirstToken := false
	for _, event := range events {
		if event.LiveTokens > 0 {
			sawLiveTokens = true
		}
		if event.LiveSpeed > 0 {
			sawLiveSpeed = true
		}
		if event.FirstTokenMs > 0 {
			sawFirstToken = true
		}
	}
	if !sawLiveTokens {
		t.Fatalf("expected live token counts while streaming, got %+v", events)
	}
	if !sawLiveSpeed {
		t.Fatalf("expected a live output speed while streaming, got %+v", events)
	}
	if !sawFirstToken {
		t.Fatalf("expected a first-token latency while streaming, got %+v", events)
	}
	last := events[len(events)-1]
	if last.FirstTokenMs <= 0 {
		t.Fatalf("expected the final event to keep the first-token latency, got %+v", last)
	}
}

func TestSendCarriesKnownModelContextWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(payload.Tools) == 0 {
			w.Write([]byte(`{"choices":[{"message":{"content":"确认请求。"}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), func() []ModelOption {
		return []ModelOption{{Model: "test/window", Label: "Window", ContextWindow: 1000}}
	})
	session, err := svc.Create(Options{Model: "test/window"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(context.Background(), session.ID, "hello", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.RunStats) != 1 || loaded.RunStats[0].ContextWindow != 1000 || loaded.RunStats[0].ContextTokens <= 0 {
		t.Fatalf("context stats missing: %+v", loaded.RunStats)
	}
}

func isSummaryRequest(messages []map[string]any) bool {
	return len(messages) > 0 && strings.Contains(fmt.Sprint(messages[0]["content"]), "小涛的进度摘要器")
}

func TestLoopPersistsToolHistoryAndReturnsFinalMessage(t *testing.T) {
	data := t.TempDir()
	root := t.TempDir()
	svc := NewService(data, "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{ProjectPath: root, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := OpenWorkspace(root)
	history, _ := svc.Get(session.ID)
	history.Messages = append(history.Messages, Message{ID: "u", Role: "user", Content: "create a file"})
	args, _ := json.Marshal(map[string]any{"path": "notes.txt", "content": "hello"})
	fake := &fakeModel{replies: []*completion{
		{ToolCalls: []toolCall{{ID: "call_1", Name: "write_file", Arguments: args}}},
		{Content: "已创建并检查。"},
	}}
	var events []Event
	err = svc.loop(context.Background(), workspace, fake, history, []map[string]any{{"role": "user", "content": "create a file"}}, func(event Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil || string(text) != "hello" {
		t.Fatalf("file not written: %q %v", text, err)
	}
	if len(history.Messages) != 4 {
		t.Fatalf("expected user/tool/final history, got %+v", history.Messages)
	}
	if len(fake.messages) != 2 {
		t.Fatalf("expected tool and final turns, got %d", len(fake.messages))
	}
	second := fake.messages[1]
	if len(second) != 3 || second[2]["role"] != "tool" || second[2]["tool_call_id"] != "call_1" {
		t.Fatalf("tool output was not returned to the model: %+v", second)
	}
	foundFinal := false
	for _, event := range events {
		if event.Type == "answer_end" && event.Message != nil && strings.Contains(event.Message.Content, "已创建") {
			foundFinal = true
		}
	}
	if !foundFinal {
		t.Fatal("final assistant event missing")
	}
}

func TestLoopContinuesPastFortyToolRounds(t *testing.T) {
	data := t.TempDir()
	root := t.TempDir()
	svc := NewService(data, "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{ProjectPath: root, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := OpenWorkspace(root)
	history, _ := svc.Get(session.ID)
	fake := &fakeModel{}
	for i := 0; i < 45; i++ {
		fake.replies = append(fake.replies, &completion{ToolCalls: []toolCall{{ID: fmt.Sprintf("call_%d", i), Name: "list_files", Arguments: json.RawMessage(`{"path":"."}`)}}})
	}
	fake.replies = append(fake.replies, &completion{Content: "完成"})
	err = svc.loop(context.Background(), workspace, fake, history, nil, func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	// The send flow persists the final answer after loopWithRun returns.
	if err := svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 46 || len(loaded.Messages) != 91 || len(loaded.Activity) != 45 {
		t.Fatalf("unexpected request or history counts: requests=%d messages=%d activities=%d", len(fake.messages), len(loaded.Messages), len(loaded.Activity))
	}
	for i := 0; i < 45; i++ {
		call, result := loaded.Messages[i*2], loaded.Messages[i*2+1]
		id := fmt.Sprintf("call_%d", i)
		if call.Role != "assistant" || len(call.ToolCalls) != 1 || call.ToolCalls[0]["id"] != id || result.Role != "tool" || result.ToolCallID != id || loaded.Activity[i].Status != "completed" {
			t.Fatalf("round %d not paired or successful: %+v %+v", i, call, result)
		}
	}
	if loaded.Messages[90].Content != "完成" {
		t.Fatalf("final answer missing: %+v", loaded.Messages[90])
	}
	checkpoints := 0
	for _, thought := range loaded.Thoughts {
		if thought.Kind == "checkpoint" {
			checkpoints++
		}
	}
	if checkpoints != 5 {
		t.Fatalf("expected five checkpoints, got %d", checkpoints)
	}
}

func TestLoopCanCancelAfterFortyToolRounds(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{ProjectPath: t.TempDir(), Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(session.Project.Path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeModel{}
	for i := 0; i < 45; i++ {
		fake.replies = append(fake.replies, &completion{ToolCalls: []toolCall{{ID: fmt.Sprintf("call_%d", i), Name: "list_files", Arguments: json.RawMessage(`{"path":"."}`)}}})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := 0
	done := make(chan error, 1)
	go func() {
		done <- svc.loop(ctx, workspace, fake, history, nil, func(event Event) {
			if event.Type == "activity" && event.Activity != nil && event.Activity.Status == "completed" {
				completed++
				if completed == 42 {
					cancel()
				}
			}
		})
	}()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("canceled loop did not return")
	}
	if len(fake.messages) != 42 || len(history.Activity) != 42 {
		t.Fatalf("work continued after cancellation: requests=%d activities=%d", len(fake.messages), len(history.Activity))
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 84 || loaded.Messages[83].ToolCallID != "call_41" {
		t.Fatalf("canceled history missing tool result: %+v", loaded.Messages)
	}
}

func TestLoopAllowsRepeatedPolling(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{ProjectPath: t.TempDir(), Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(session.Project.Path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeModel{}
	for i := 0; i < 4; i++ {
		fake.replies = append(fake.replies, &completion{ToolCalls: []toolCall{{ID: fmt.Sprintf("call_%d", i), Name: "list_files", Arguments: json.RawMessage(`{"path":"."}`)}}})
	}
	fake.replies = append(fake.replies, &completion{Content: "完成"})
	if err := svc.loop(context.Background(), workspace, fake, history, nil, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(history.Activity) != 4 {
		t.Fatalf("expected four polls, got %d", len(history.Activity))
	}
}

func TestIntermediateToolNarrationIsNotPersistedAsReply(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if len(request.Tools) == 0 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"先看当前状态。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		calls++
		if calls == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"我先看看当前状态。\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"list_files\",\"arguments\":\"{\\\"path\\\":\\\".\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"检查完了。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	session, err := svc.Create(Options{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(context.Background(), session.ID, "检查一下", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var replies []string
	for _, message := range saved.Messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 0 {
			replies = append(replies, message.Content)
		}
	}
	if len(replies) != 1 || replies[0] != "检查完了。" {
		t.Fatalf("intermediate narration was persisted as a reply: %+v", replies)
	}
	foundNarration := false
	for _, thought := range saved.Thoughts {
		if thought.Text == "我先看看当前状态。" {
			foundNarration = true
		}
	}
	if !foundNarration {
		t.Fatalf("intermediate narration missing from thought trail: %+v", saved.Thoughts)
	}
}

func TestCreateKeepsProjectDisplayName(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	root := t.TempDir()
	session, err := svc.Create(Options{ProjectPath: root, ProjectName: "小涛 UI 重构", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	if session.Project.Path != root || session.Project.Name != "小涛 UI 重构" {
		t.Fatalf("unexpected project: %+v", session.Project)
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.Name != "小涛 UI 重构" {
		t.Fatalf("project name was not persisted: %+v", loaded.Project)
	}
}

func TestVisibleChineseThinkingRejectsRawModelReasoning(t *testing.T) {
	for _, value := range []string{
		"Let me inspect the actual diff before deciding the next step.",
		"Use rg to find README references",
		"",
	} {
		if visibleChineseThinking(value) {
			t.Fatalf("accepted non-visible reasoning: %q", value)
		}
	}
	for _, value := range []string{
		"已确认测试文件，下一步检查失败路径。",
		"先核对 git 变更，再补充测试。",
		"当前上下文够了，继续修改 Agent 页面。",
	} {
		if !visibleChineseThinking(value) {
			t.Fatalf("rejected Chinese progress: %q", value)
		}
	}
}

func TestDeleteSession(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	session, err := svc.Create(Options{Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	session.Messages = []Message{{ID: "m", Content: "hello"}}
	session.Activity = []Activity{{ID: "a"}}
	if err := svc.saveSession(session); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[session.ID].cancel = func() {}
	svc.mu.Unlock()
	if err := svc.Delete(session.ID); err == nil {
		t.Fatal("running session was deleted")
	}
	svc.mu.Lock()
	svc.runs[session.ID].cancel = nil
	svc.mu.Unlock()
	if err := svc.Delete(session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(session.ID); err == nil {
		t.Fatal("deleted session remains")
	}
	db, err := svc.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"messages", "activities"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM "+table+" WHERE session_id=?", session.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s: count=%d err=%v", table, count, err)
		}
	}
	if err := svc.Delete(session.ID); err == nil {
		t.Fatal("missing session deletion succeeded")
	}
}

func TestLoopDenialReturnsToolResult(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	root := t.TempDir()
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := OpenWorkspace(root)
	fake := &fakeModel{replies: []*completion{
		{ToolCalls: []toolCall{{ID: "call_danger", Name: "run_command", Arguments: json.RawMessage(`{"command":"git reset --hard"}`)}}},
		{Content: "操作已取消"},
	}}
	err = svc.loop(context.Background(), workspace, fake, session, nil, func(event Event) {
		if event.Approval != nil {
			if err := svc.ResolveApproval(session.ID, event.Approval.ID, false); err != nil {
				t.Error(err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 2 {
		t.Fatalf("expected tool and next model turns: %d", len(fake.messages))
	}
	messages := fake.messages[1]
	result := messages[len(messages)-1]
	if result["tool_call_id"] != "call_danger" || !strings.Contains(result["content"].(string), "拒绝") {
		t.Fatalf("denial result missing: %+v", result)
	}
}
