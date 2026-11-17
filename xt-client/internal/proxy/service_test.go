package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

func TestProxy_UnauthorizedDoesNotTrack(t *testing.T) {
	tmp := t.TempDir()
	store, err := storage.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	usageSvc, err := usage.New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	defer usageSvc.Close()

	svc := New(store, func(id string) ([]byte, error) { return nil, nil }, "valid-key")
	svc.SetUsage(usageSvc)

	// Request with invalid API key
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Authorization", "Bearer invalid-key")
	w := httptest.NewRecorder()

	svc.HandleChat(w, r)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	snap, err := usageSvc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.ActiveCount != 0 {
		t.Errorf("expected 0 active requests, got %d", snap.ActiveCount)
	}
	if len(snap.RecentRequests) != 0 {
		t.Errorf("expected 0 recent requests, got %d", len(snap.RecentRequests))
	}
}

func TestProxy_AuthorizedWithNoReadyAccount(t *testing.T) {
	tmp := t.TempDir()
	store, err := storage.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	usageSvc, err := usage.New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	defer usageSvc.Close()

	svc := New(store, func(id string) ([]byte, error) { return nil, nil }, "valid-key")
	svc.SetUsage(usageSvc)

	// Request with valid API key but no account configured
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"trae/gpt-4","messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()

	svc.HandleChat(w, r)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	snap, err := usageSvc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	// Should have 1 failed request recorded
	if len(snap.RecentRequests) != 1 {
		t.Fatalf("expected 1 recent request recorded, got %d", len(snap.RecentRequests))
	}
	rec := snap.RecentRequests[0]
	if rec.Status != usage.StatusFailed {
		t.Errorf("expected status %s, got %s", usage.StatusFailed, rec.Status)
	}
	if !strings.Contains(rec.ErrorSummary, "没有可用的已启用账号") {
		t.Errorf("expected error summary to mention missing account, got %s", rec.ErrorSummary)
	}
}
