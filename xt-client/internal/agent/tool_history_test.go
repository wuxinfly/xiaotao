package agent

import "testing"

func TestModelHistoryToolPairing(t *testing.T) {
	call := map[string]any{"id": "call", "type": "function", "function": map[string]any{"name": "read_file", "arguments": `{"path":"a"}`}}
	for _, interrupted := range []bool{false, true} {
		messages := []Message{{Role: "assistant", ToolCalls: []map[string]any{call}}}
		if !interrupted {
			messages = append(messages, Message{Role: "tool", ToolCallID: "call", ToolName: "read_file", Content: "evidence"})
		}
		messages = append(messages, Message{Role: "user", Content: "continue"})
		out := modelHistory(messages, "")
		if len(out) != 3 || out[0]["tool_calls"] == nil || out[1]["tool_call_id"] != "call" || out[2]["role"] != "user" {
			t.Fatalf("invalid pairing after interruption=%v: %+v", interrupted, out)
		}
	}
	out := modelHistory([]Message{{Role: "tool", ToolCallID: "legacy", ToolName: "read_file", Content: "old evidence"}}, "")
	if len(out) != 1 || out[0]["role"] != "user" || out[0]["tool_call_id"] != nil {
		t.Fatalf("orphan result leaked: %+v", out)
	}
}

func TestToolHistorySurvivesReload(t *testing.T) {
	svc := NewService(t.TempDir(), "unused", nil, nil, nil)
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	model := &fakeModel{replies: []*completion{
		{ToolCalls: []toolCall{{ID: "read", Name: "list_files", Arguments: []byte(`{}`)}}},
		{ToolCalls: []toolCall{{ID: "finish", Name: "list_files", Arguments: []byte(`{}`)}}},
		{Content: "done"},
	}}
	if err := svc.loop(t.Context(), nil, model, history, nil, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if err := svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.Get(history.ID)
	if err != nil {
		t.Fatal(err)
	}
	out := modelHistory(saved.Messages, "")
	for i, id := range []string{"read", "finish"} {
		if out[i*2]["tool_calls"] == nil || out[i*2+1]["tool_call_id"] != id {
			t.Fatalf("lost tool pair after reload: %+v", out)
		}
	}
}
