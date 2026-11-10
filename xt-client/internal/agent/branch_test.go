package agent

import (
	"strings"
	"testing"
	"time"
)

func seedBranchSession(t *testing.T, svc *Service) *Session {
	t.Helper()
	now := time.Now().Add(-time.Hour)
	ended := now.Add(time.Minute)
	session := &Session{
		ID:      "run_source",
		Project: Project{Path: t.TempDir(), Name: "demo"},
		Title:   "来源会话",
		Model:   "test/model",
		Status:  StatusCompleted,
		Messages: []Message{
			{ID: "u1", Role: "user", Content: "第一个问题", RunID: "run1", CreatedAt: now},
			{ID: "a1", Role: "assistant", RunID: "run1", CreatedAt: now.Add(time.Second), ToolCalls: []map[string]any{{"id": "call_1", "type": "function"}}},
			{ID: "t1", Role: "tool", Content: "文件列表", ToolCallID: "call_1", ToolName: "list_files", RunID: "run1", CreatedAt: now.Add(2 * time.Second)},
			{ID: "ans1", Role: "assistant", Content: "第一个回答", RunID: "run1", CreatedAt: now.Add(3 * time.Second)},
			{ID: "u2", Role: "user", Content: "第二个问题", RunID: "run2", CreatedAt: now.Add(4 * time.Second)},
			{ID: "ans2", Role: "assistant", Content: "第二个回答", RunID: "run2", CreatedAt: now.Add(5 * time.Second)},
		},
		Activity: []Activity{
			{ID: "act1", RunID: "run1", Kind: "read", Title: "读取目录", Status: "completed", StartedAt: now.Add(time.Second), EndedAt: &ended},
			{ID: "act2", RunID: "run2", Kind: "read", Title: "读取目录", Status: "completed", StartedAt: now.Add(4 * time.Second), EndedAt: &ended},
		},
		Thoughts: []ThoughtStep{
			{ID: "th1", RunID: "run1", Kind: "note", Text: "先看结构", Status: "completed", CreatedAt: now.Add(time.Second)},
			{ID: "th2", RunID: "run2", Kind: "note", Text: "再看结构", Status: "completed", CreatedAt: now.Add(4 * time.Second)},
		},
		RunStats: []RunStats{
			{RunID: "run1", UserMessageID: "u1", StartedAt: now, EndedAt: &ended, ToolCalls: 1},
			{RunID: "run2", UserMessageID: "u2", StartedAt: now.Add(4 * time.Second), EndedAt: &ended},
		},
		CreatedAt: now,
		UpdatedAt: now.Add(5 * time.Second),
	}
	if err := svc.saveSession(session); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[session.ID] = &run{}
	svc.mu.Unlock()
	return session
}

func TestBranchCopiesFinishedTurns(t *testing.T) {
	svc := NewService(t.TempDir(), "unused", nil, nil, nil)
	source := seedBranchSession(t, svc)

	branch, err := svc.Branch(source.ID, "ans1")
	if err != nil {
		t.Fatal(err)
	}
	if branch.ID == source.ID || !strings.HasPrefix(branch.ID, "run_") {
		t.Fatalf("unexpected branch id: %s", branch.ID)
	}
	if branch.Title != "来源会话 · 分支" {
		t.Fatalf("unexpected title: %q", branch.Title)
	}
	// The copy stops at the chosen answer: the second turn stays behind.
	if len(branch.Messages) != 4 || branch.Messages[3].ID != "ans1" {
		t.Fatalf("unexpected messages: %+v", branch.Messages)
	}
	// Tool calls keep their original IDs so the result stays paired.
	if len(branch.Messages[1].ToolCalls) != 1 || branch.Messages[2].ToolCallID != "call_1" {
		t.Fatalf("tool linkage was not preserved: %+v", branch.Messages[1:3])
	}
	if len(branch.Activity) != 1 || branch.Activity[0].ID != "act1" {
		t.Fatalf("unexpected copied activity: %+v", branch.Activity)
	}
	if len(branch.Thoughts) != 1 || branch.Thoughts[0].ID != "th1" {
		t.Fatalf("unexpected copied thoughts: %+v", branch.Thoughts)
	}
	if len(branch.RunStats) != 1 || branch.RunStats[0].RunID != "run1" || branch.RunStats[0].EndedAt == nil {
		t.Fatalf("unexpected copied stats: %+v", branch.RunStats)
	}
	if branch.Status != StatusIdle || branch.Model != "test/model" || branch.Project.Path != source.Project.Path {
		t.Fatalf("unexpected branch metadata: %+v", branch)
	}
	if !tsEqual(branch.CreatedAt, branch.UpdatedAt) || !branch.CreatedAt.After(source.UpdatedAt) {
		t.Fatalf("branch timestamps should be new: %+v", branch)
	}

	loaded, err := svc.Get(branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != len(branch.Messages) || len(loaded.RunStats) != 1 {
		t.Fatalf("branch was not persisted: %+v", loaded)
	}
	if _, ok := svc.runs[branch.ID]; !ok {
		t.Fatal("branch must be registered for future sends")
	}

	// The source session must not change.
	original, err := svc.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(original.Messages) != 6 || len(original.Activity) != 2 || len(original.RunStats) != 2 || original.Title != "来源会话" {
		t.Fatalf("source session was modified: %+v", original)
	}
}

func tsEqual(a, b time.Time) bool { return a.Equal(b) }

func TestBranchRejectsInvalidPoints(t *testing.T) {
	svc := NewService(t.TempDir(), "unused", nil, nil, nil)
	source := seedBranchSession(t, svc)

	if _, err := svc.Branch("run_missing", "ans1"); err == nil {
		t.Fatal("expected missing session to fail")
	}
	if _, err := svc.Branch(source.ID, "missing"); err == nil || !strings.Contains(err.Error(), "没有找到") {
		t.Fatalf("unexpected missing message error: %v", err)
	}
	if _, err := svc.Branch(source.ID, "u1"); err == nil || !strings.Contains(err.Error(), "回答") {
		t.Fatalf("user message must not be a branch point: %v", err)
	}
	if _, err := svc.Branch(source.ID, "a1"); err == nil || !strings.Contains(err.Error(), "工具调用") {
		t.Fatalf("tool-call message must not be a branch point: %v", err)
	}
	if _, err := svc.Branch(source.ID, "t1"); err == nil {
		t.Fatalf("tool result must not be a branch point")
	}

	svc.mu.Lock()
	r := svc.runs[source.ID]
	r.cancel = func() {}
	svc.mu.Unlock()
	if _, err := svc.Branch(source.ID, "ans1"); err == nil || !strings.Contains(err.Error(), "停止") {
		t.Fatalf("expected running session to be rejected: %v", err)
	}

	svc.mu.Lock()
	r.cancel = nil
	svc.mu.Unlock()
	loaded, err := svc.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.RunStats[0].EndedAt = nil
	if err := svc.saveSession(loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Branch(source.ID, "ans1"); err == nil || !strings.Contains(err.Error(), "尚未完成") {
		t.Fatalf("expected unfinished answer to be rejected: %v", err)
	}
}