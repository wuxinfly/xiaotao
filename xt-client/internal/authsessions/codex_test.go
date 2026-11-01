package authsessions

import (
	"testing"
)

func TestCodexSessionStartAndCancel(t *testing.T) {
	svc := NewService("http://127.0.0.1:18080")
	sess, err := svc.Start("codex", "")
	if err != nil {
		t.Fatalf("Start codex failed: %v", err)
	}
	if sess.Provider != "codex" || sess.Status != "waiting" || sess.LoginURL == "" {
		t.Fatalf("unexpected codex session: %+v", sess)
	}

	polled, cred := svc.Poll(sess.ID)
	if polled.Status != "waiting" || cred != nil {
		t.Fatalf("expected waiting session, got: %+v, cred=%v", polled, cred)
	}

	svc.Cancel(sess.ID)
	cancelled, _ := svc.Poll(sess.ID)
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got: %s", cancelled.Status)
	}
}
