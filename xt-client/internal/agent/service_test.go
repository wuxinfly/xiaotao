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

func TestLoopStopsAtToolCallLimit(t *testing.T) {
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
	for i := 0; i < maxTurns+1; i++ {
		args, _ := json.Marshal(map[string]any{"path": fmt.Sprintf("dir-%d", i)})
		fake.replies = append(fake.replies, &completion{ToolCalls: []toolCall{{ID: "call", Name: "list_files", Arguments: args}}})
	}
	err = svc.loop(context.Background(), workspace, fake, history, nil, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "超过") {
		t.Fatalf("expected bounded loop error, got %v", err)
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
