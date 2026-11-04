package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccessModeAndApproval(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir, "http://localhost", nil, nil, nil)
	if svc.AccessMode() != ModeSafe {
		t.Fatal("default should be safe")
	}
	if err := svc.SetAccessMode("bad"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	call := toolCall{Name: "run_command", Arguments: json.RawMessage(`{"command":"git reset --hard"}`)}
	events := make(chan Event, 1)
	done := make(chan error, 1)
	go func() {
		done <- svc.waitApproval(context.Background(), "session", nil, call, func(e Event) { events <- e })
	}()
	event := <-events
	if event.Approval == nil || event.Approval.Detail != "git reset --hard" {
		t.Fatalf("bad approval: %+v", event)
	}
	if svc.ResolveApproval("wrong", event.Approval.ID, true) == nil {
		t.Fatal("wrong session accepted")
	}
	if err := svc.ResolveApproval("session", event.Approval.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil || !strings.Contains(err.Error(), "拒绝") {
		t.Fatalf("denial lost: %v", err)
	}
	if err := svc.SetAccessMode(ModeFull); err != nil {
		t.Fatal(err)
	}
	if NewService(dir, "", nil, nil, nil).AccessMode() != ModeFull {
		t.Fatal("mode not persisted")
	}
	if err := svc.waitApproval(context.Background(), "session", nil, call, func(Event) { t.Error("unexpected approval") }); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalCancellation(t *testing.T) {
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 1)
	done := make(chan error, 1)
	go func() {
		done <- svc.waitApproval(ctx, "s", nil, toolCall{Name: "run_command", Arguments: json.RawMessage(`{"command":"rm -rf folder"}`)}, func(e Event) { events <- e })
	}()
	e := <-events
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation blocked")
	}
	if svc.ResolveApproval("s", e.Approval.ID, true) == nil {
		t.Fatal("canceled approval accepted")
	}
}

func TestLargeFileOverwriteNeedsApproval(t *testing.T) {
	root := t.TempDir()
	w, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), make([]byte, 128*1024), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := needsApproval(w, "write_file", json.RawMessage(`{"path":"large.txt","content":"replacement"}`)); !ok {
		t.Fatal("large overwrite was not flagged")
	}
	if _, ok := needsApproval(w, "write_file", json.RawMessage(`{"path":"large.txt","content":"addition","append":true}`)); ok {
		t.Fatal("append flagged as overwrite")
	}
}

func TestCommandRiskClassification(t *testing.T) {
	for _, test := range []struct {
		command string
		want    bool
	}{{"npm run build", false}, {"npm install react", true}, {"git reset --hard", true}, {"go test ./...", false}} {
		raw, _ := json.Marshal(map[string]string{"command": test.command})
		_, got := needsApproval(nil, "run_command", raw)
		if got != test.want {
			t.Errorf("%q: got %v want %v", test.command, got, test.want)
		}
	}
}
