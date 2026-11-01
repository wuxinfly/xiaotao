package agent

import (
	"context"
	"encoding/json"
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

func (f *fakeModel) Complete(_ context.Context, _ string, messages []map[string]any, _ []map[string]any) (*completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, messages)
	if len(f.replies) == 0 {
		return &completion{Content: "done"}, nil
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	return reply, nil
}

func TestLoopPersistsToolHistoryAndReturnsFinalMessage(t *testing.T) {
	data := t.TempDir()
	root := t.TempDir()
	svc := NewService(data, "http://127.0.0.1:9999", "", nil, nil)
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
	if len(history.Messages) != 3 {
		t.Fatalf("expected user/tool/final history, got %+v", history.Messages)
	}
	if len(fake.messages) != 2 {
		t.Fatalf("expected two model turns, got %d", len(fake.messages))
	}
	second := fake.messages[1]
	if len(second) != 3 || second[2]["role"] != "tool" || second[2]["tool_call_id"] != "call_1" {
		t.Fatalf("tool output was not returned to the model: %+v", second)
	}
	foundFinal := false
	for _, event := range events {
		if event.Type == "message" && event.Message != nil && strings.Contains(event.Message.Content, "已创建") {
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
	svc := NewService(data, "http://127.0.0.1:9999", "", nil, nil)
	session, err := svc.Create(Options{ProjectPath: root, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := OpenWorkspace(root)
	history, _ := svc.Get(session.ID)
	fake := &fakeModel{}
	for i := 0; i < maxTurns+1; i++ {
		args, _ := json.Marshal(map[string]any{"path": "."})
		fake.replies = append(fake.replies, &completion{ToolCalls: []toolCall{{ID: "call", Name: "list_files", Arguments: args}}})
	}
	err = svc.loop(context.Background(), workspace, fake, history, nil, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "超过") {
		t.Fatalf("expected bounded loop error, got %v", err)
	}
}
