package office

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestChatRealKernelIsolationAndReadOnlyRestart(t *testing.T) {
	var malicious atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
		}
		if len(payload.Tools) != 0 {
			t.Errorf("read-only session exposed tools: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		if malicious.Load() {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"bad","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"bad.txt\",\"content\":\"bad\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer server.Close()
	root := t.TempDir()
	project := t.TempDir()
	other := t.TempDir()
	create := func() *Service {
		kernel := agent.NewService(filepath.Join(root, "agent"), server.URL, nil, server.Client(), func() []agent.ModelOption { return []agent.ModelOption{{Model: "test/model"}} })
		svc, err := New(filepath.Join(root, "office"), NewAgentExecutor(kernel))
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	svc := create()
	input := ChatInput{Content: "hello", Model: "test/model"}
	ids := makeTeamForChat(t, svc, project)
	first, err := svc.SendChat(project, ids[0], input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, ids[0]); return e == nil && c.Status == StatusCompleted })
	designer, err := svc.SendChat(project, ids[1], input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, ids[1]); return e == nil && c.Status == StatusCompleted })
	otherIDs := makeTeamForChat(t, svc, other)
	otherChat, err := svc.SendChat(other, otherIDs[0], input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(other, otherIDs[0]); return e == nil && c.Status == StatusCompleted })
	if first.SessionID == designer.SessionID || first.SessionID == otherChat.SessionID {
		t.Fatal("sessions are not isolated")
	}
	empty, err := svc.Chat(other, otherIDs[2])
	if err != nil || empty.SessionID != "" {
		t.Fatalf("unexpected chat: %+v %v", empty, err)
	}
	svc.Close()
	svc = create()
	defer svc.Close()
	restored, err := svc.Chat(project, ids[0])
	if err != nil || restored.SessionID != first.SessionID {
		t.Fatalf("session not restored: %+v %v", restored, err)
	}
	malicious.Store(true)
	if _, err = svc.SendChat(project, ids[0], input); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, ids[0]); return e == nil && c.Status == StatusFailed })
	failed, _ := svc.Chat(project, ids[0])
	if !strings.Contains(failed.Error, "只读") {
		t.Fatalf("missing read-only error: %+v", failed)
	}
	if _, err = os.Stat(filepath.Join(project, "bad.txt")); !os.IsNotExist(err) {
		t.Fatalf("write occurred: %v", err)
	}
	page, err := svc.ChatHistory(project, ids[0], "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if !bytes.Contains(raw, []byte("hello")) {
		t.Fatalf("missing history: %s", raw)
	}
}

func TestChatBusyCancel(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	exec.run = func(ctx context.Context, _ string, _ string, _ func(agent.Event)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	svc, project := newTestService(t, exec)
	ids := makeTeamForChat(t, svc, project)
	memberID := ids[2]
	_, err := svc.SendChat(project, memberID, ChatInput{Content: "hi", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	snapshot, err := svc.Snapshot(project, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, member := range snapshot.Members {
		if member.ID == memberID {
			found = member.Status == StatusRunning
		}
	}
	if !found {
		t.Fatal("running chat missing from snapshot")
	}
	if _, err = svc.SendChat(project, memberID, ChatInput{Content: "again"}); err != ErrChatBusy {
		t.Fatalf("busy: %v", err)
	}
	if err = svc.CancelChat(project, memberID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, memberID); return e == nil && c.Status == StatusCanceled })
	if len(exec.created) != 1 || !exec.created[0].ReadOnly {
		t.Fatal("missing read-only flag")
	}
}

func makeTeamForChat(t *testing.T, svc *Service, project string) []string {
	t.Helper()
	before, err := svc.teams.Snapshot(project)
	if err != nil {
		t.Fatal(err)
	}
	drafts := TeamTemplates()[0].Roles
	for index := range drafts {
		drafts[index].Model.ModelID = "test/model"
	}
	team, err := svc.teams.Bootstrap(project, drafts, revisions(before))
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(team.Members))
	for index, member := range team.Members {
		ids[index] = member.ID
	}
	return ids
}
