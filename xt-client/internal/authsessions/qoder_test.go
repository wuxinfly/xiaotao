package authsessions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/providers"
)

func TestQoderSessionCancellationAndSerialization(t *testing.T) {
	svc := NewService("")
	sess, err := svc.Start("qoder", "global")
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Cancel(sess.ID)
	raw, _ := json.Marshal(sess)
	if strings.Contains(string(raw), "verifier") || strings.Contains(string(raw), "Credential") {
		t.Fatal("private authorization state exposed")
	}
	svc.Cancel(sess.ID)
	svc.finishQoder(svc.sessions[sess.ID], &providers.Credential{UID: "u", AccessToken: "t"}, nil)
	current, cred := svc.Poll(sess.ID)
	if current.Status != "cancelled" || cred != nil {
		t.Fatal("late login revived cancelled session")
	}
}

func TestQoderCompletionStates(t *testing.T) {
	svc := NewService("")
	sess := &Session{Status: "waiting"}
	svc.finishQoder(sess, nil, context.DeadlineExceeded)
	if sess.Status != "expired" {
		t.Fatal("timeout not marked expired")
	}
	sess = &Session{Status: "waiting"}
	svc.finishQoder(sess, &providers.Credential{AccessToken: "t"}, nil)
	if sess.Status != "failed" {
		t.Fatal("accepted missing identity")
	}
}
