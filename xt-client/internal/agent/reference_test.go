package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestConversationReferenceSearchAndFreeze(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	project := t.TempDir()
	other := t.TempDir()
	source, err := svc.Create(Options{ProjectPath: project, ProjectName: "来源", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.Create(Options{ProjectPath: project, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := svc.Create(Options{ProjectPath: other, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	source.Title = "登录讨论"
	source.Messages = []Message{
		{ID: "u1", Role: "user", Content: "登录要用 A 方案", CreatedAt: at},
		{ID: "t1", Role: "tool", Content: "secret command output", ToolCallID: "c1", ToolName: "run_command", CreatedAt: at.Add(time.Second)},
		{ID: "a1", Role: "assistant", Content: "结论：先做 A，再补 B", CreatedAt: at.Add(2 * time.Second)},
	}
	source.UpdatedAt = at.Add(time.Second)
	if err := svc.saveSession(source); err != nil {
		t.Fatal(err)
	}
	foreign.Title = "另一个项目"
	if err := svc.saveSession(foreign); err != nil {
		t.Fatal(err)
	}

	current, err := svc.SearchConversationReferences(ReferenceSearch{ProjectPath: project, Query: "登录", Exclude: []string{target.ID}})
	if err != nil || len(current) != 1 || current[0].SessionID != source.ID {
		t.Fatalf("current project search: %+v %v", current, err)
	}
	all, err := svc.SearchConversationReferences(ReferenceSearch{AllProjects: true, Query: "另一个"})
	if err != nil || len(all) != 1 || all[0].SessionID != foreign.ID {
		t.Fatalf("all project search: %+v %v", all, err)
	}

	fake := &fakeModel{replies: []*completion{{Content: "继续 A"}}}
	history, err := svc.Get(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	refs, sources, err := svc.prepareConversationReferencesForSend([]string{source.ID, source.ID}, at)
	if err != nil || len(refs) != 1 || sources[refs[0].ID].Limit != 2 {
		t.Fatalf("freeze: %+v %+v %v", refs, sources, err)
	}
	if strings.Contains(sources[refs[0].ID].Text, "secret command output") {
		t.Fatal("tool output entered reference context")
	}
	history.Messages = append(history.Messages, Message{ID: "current", Role: "user", Content: "接着做", References: refs})
	modelMessages := modelHistory(history.Messages, "current")
	if len(modelMessages) != 2 || !strings.Contains(modelMessages[0]["content"].(string), "登录要用 A 方案") || modelMessages[1]["content"] != "接着做" {
		t.Fatalf("model history: %+v", modelMessages)
	}
	if len(fake.messages) != 0 {
		t.Fatal("unexpected model call")
	}
}

func TestConversationReferenceRejectsTooManyAndEmptySources(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	ids := make([]string, 0, maxConversationReferences+1)
	for i := 0; i < maxConversationReferences+1; i++ {
		item, err := svc.Create(Options{Model: "test"})
		if err != nil {
			t.Fatal(err)
		}
		item.Messages = []Message{{ID: "m", Role: "user", Content: "hello", CreatedAt: time.Now()}}
		if err := svc.saveSession(item); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	if _, _, err := svc.prepareConversationReferencesForSend(ids, time.Now()); err == nil || !strings.Contains(err.Error(), "最多引用") {
		t.Fatalf("expected reference count error, got %v", err)
	}
	empty, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.prepareConversationReferencesForSend([]string{empty.ID}, time.Now()); err == nil {
		t.Fatal("accepted source without text messages")
	}
}

func TestReadConversationReferenceIsDirectAndBounded(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	source, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	source.Title = "长讨论"
	source.Messages = []Message{{ID: "m", Role: "user", Content: strings.Repeat("前文", 800) + "关键结论在这里" + strings.Repeat("后文", 800), CreatedAt: time.Now()}}
	if err := svc.saveSession(source); err != nil {
		t.Fatal(err)
	}
	refs, sources, err := svc.prepareConversationReferencesForSend([]string{source.ID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.saveConversationReferences(refs, sources); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"reference_id": refs[0].ID, "query": "关键结论"})
	text, err := svc.readConversationReference(raw, refs)
	if err != nil || !strings.Contains(text, "关键结论") || len([]rune(text)) > 2500 {
		t.Fatalf("bounded read: %q %v", text, err)
	}
	if len(sources) != 1 {
		t.Fatal("source map missing")
	}
	if _, err := svc.readConversationReference(raw, nil); err == nil {
		t.Fatal("read undeclared reference")
	}
}

func TestSendWithReferencesHydratesDeletedSource(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	source, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	source.Messages = []Message{{ID: "m", Role: "user", Content: "旧结论", CreatedAt: time.Now()}}
	if err := svc.saveSession(source); err != nil {
		t.Fatal(err)
	}
	refs, sources, err := svc.prepareConversationReferencesForSend([]string{source.ID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	target.Messages = append(target.Messages, Message{ID: "u", Role: "user", Content: "引用旧结论", References: refs, CreatedAt: time.Now()})
	if err := svc.saveSessionWithReferences(target, refs, sources); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(source.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(target.ID)
	if err != nil || len(loaded.Messages) != 1 || len(loaded.Messages[0].References) != 1 || loaded.Messages[0].References[0].SourceAvailable {
		t.Fatalf("deleted source status: %+v %v", loaded, err)
	}
	history := &Session{ID: target.ID, Model: "test", Messages: loaded.Messages}
	fake := &fakeModel{replies: []*completion{{Content: "可以继续"}}}
	err = svc.loop(context.Background(), nil, fake, history, modelHistory(history.Messages, "u"), func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("unexpected model turns: %d", len(fake.messages))
	}
}

func TestConversationReferenceFreezesBeforeSourceUpdate(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	source, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	source.Messages = []Message{{ID: "old", Role: "user", Content: "旧内容", CreatedAt: time.Now()}}
	if err := svc.saveSession(source); err != nil {
		t.Fatal(err)
	}
	refs, sources, err := svc.prepareConversationReferencesForSend([]string{source.ID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.saveConversationReferences(refs, sources); err != nil {
		t.Fatal(err)
	}
	source, err = svc.Get(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	source.Messages = append(source.Messages, Message{ID: "new", Role: "assistant", Content: "新内容", CreatedAt: time.Now().Add(time.Second)})
	source.UpdatedAt = time.Now().Add(time.Second)
	if err := svc.saveSession(source); err != nil {
		t.Fatal(err)
	}
	target, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	target.Messages = append(target.Messages, Message{ID: "u", Role: "user", Content: "使用旧结论", References: refs, CreatedAt: time.Now()})
	if err := svc.saveSession(target); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(target.ID)
	if err != nil || len(loaded.Messages[0].References) != 1 || !loaded.Messages[0].References[0].HasNewContent {
		t.Fatalf("new source status: %+v %v", loaded, err)
	}
	text, err := svc.referenceSourceText(refs[0].ID)
	if err != nil || strings.Contains(text, "新内容") || !strings.Contains(text, "旧内容") {
		t.Fatalf("snapshot changed: %q %v", text, err)
	}
}

func TestModelHistoryDoesNotRepeatOldReferenceBody(t *testing.T) {
	refs := []ConversationReference{{ID: "ref", Title: "旧讨论", Summary: "旧结论", CreatedAt: time.Now()}}
	messages := []Message{
		{ID: "u1", Role: "user", Content: "第一条", References: refs},
		{ID: "a1", Role: "assistant", Content: "回复"},
		{ID: "u2", Role: "user", Content: "继续"},
	}
	modelMessages := modelHistory(messages, "u2")
	if len(modelMessages) != 3 || strings.Contains(modelMessages[0]["content"].(string), "旧结论") {
		t.Fatalf("old reference repeated: %+v", modelMessages)
	}
}
