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
	first, err := svc.SendChat(project, "xiaotao", input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, "xiaotao"); return e == nil && c.Status == StatusCompleted })
	designer, err := svc.SendChat(project, "designer", input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, "designer"); return e == nil && c.Status == StatusCompleted })
	otherChat, err := svc.SendChat(other, "xiaotao", input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(other, "xiaotao"); return e == nil && c.Status == StatusCompleted })
	if first.SessionID == designer.SessionID || first.SessionID == otherChat.SessionID {
		t.Fatal("sessions are not isolated")
	}
	empty, err := svc.Chat(other, "developer")
	if err != nil || empty.SessionID != "" {
		t.Fatalf("unexpected chat: %+v %v", empty, err)
	}
	svc.Close()
	svc = create()
	defer svc.Close()
	restored, err := svc.Chat(project, "xiaotao")
	if err != nil || restored.SessionID != first.SessionID {
		t.Fatalf("session not restored: %+v %v", restored, err)
	}
	malicious.Store(true)
	if _, err = svc.SendChat(project, "xiaotao", input); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, "xiaotao"); return e == nil && c.Status == StatusFailed })
	failed, _ := svc.Chat(project, "xiaotao")
	if !strings.Contains(failed.Error, "只读") {
		t.Fatalf("missing read-only error: %+v", failed)
	}
	if _, err = os.Stat(filepath.Join(project, "bad.txt")); !os.IsNotExist(err) {
		t.Fatalf("write occurred: %v", err)
	}
	page, err := svc.ChatHistory(project, "xiaotao", "")
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
	_, err := svc.SendChat(project, "developer", ChatInput{Content: "hi", Model: "test/model"})
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
		if member.ID == "developer" {
			found = member.Status == StatusRunning
		}
	}
	if !found {
		t.Fatal("running chat missing from snapshot")
	}
	if _, err = svc.SendChat(project, "developer", ChatInput{Content: "again"}); err != ErrChatBusy {
		t.Fatalf("busy: %v", err)
	}
	if err = svc.CancelChat(project, "developer"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { c, e := svc.Chat(project, "developer"); return e == nil && c.Status == StatusCanceled })
	if len(exec.created) != 1 || !exec.created[0].ReadOnly {
		t.Fatal("missing read-only flag")
	}
}
