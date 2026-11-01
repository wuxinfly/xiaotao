package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/antigravity"
	"xiaotao/xt-client/internal/storage"
)

func TestAntigravityFailover429Collect(t *testing.T) {
	store := newAntigravityStore(t, "ag-first", "ag-second")
	svc := New(store, storeSecret(store), "key")
	setAntigravityRoute(t, svc)
	var mu sync.Mutex
	tried := []string{}
	svc.collect = func(_ context.Context, cred *providers.Credential, _ string, _ map[string]any) (map[string]any, error) {
		mu.Lock()
		tried = append(tried, cred.UID)
		mu.Unlock()
		if cred.UID == "ag-first" {
			return nil, &providers.UpstreamError{Status: http.StatusTooManyRequests, Message: "quota exhausted"}
		}
		return map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}},
		}, nil
	}

	req := antigravityRequest(false)
	w := httptest.NewRecorder()
	svc.HandleChat(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"content":"ok"`) {
		t.Fatalf("expected fallback success, got %d %s", w.Code, w.Body.String())
	}
	if got := strings.Join(tried, ","); got != "ag-first,ag-second" {
		t.Fatalf("expected both accounts in order, got %s", got)
	}
	if !svc.accountCooling("ag-first") {
		t.Fatal("429 account should be cooling")
	}
	if svc.accountCooling("ag-second") {
		t.Fatal("successful account must not be cooling")
	}

	req = antigravityRequest(false)
	w = httptest.NewRecorder()
	svc.HandleChat(w, req)
	if got := strings.Join(tried, ","); got != "ag-first,ag-second,ag-second" {
		t.Fatalf("cooling account should be skipped on next request, got %s", got)
	}
}

func TestAntigravityFailover429StreamBeforeCommit(t *testing.T) {
	store := newAntigravityStore(t, "ag-first", "ag-second")
	svc := New(store, storeSecret(store), "key")
	setAntigravityRoute(t, svc)
	var mu sync.Mutex
	tried := []string{}
	svc.stream = func(_ context.Context, cred *providers.Credential, _ string, _ map[string]any, events *providers.ChatEvents) error {
		mu.Lock()
		tried = append(tried, cred.UID)
		mu.Unlock()
		if cred.UID == "ag-first" {
			return &providers.UpstreamError{Status: http.StatusTooManyRequests, Message: "quota exhausted"}
		}
		events.OnFirstToken()
		events.OnDelta(map[string]any{"content": "hello"})
		return nil
	}

	req := antigravityRequest(true)
	w := httptest.NewRecorder()
	svc.HandleChat(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello") {
		t.Fatalf("expected streamed fallback, got %d %s", w.Code, w.Body.String())
	}
	if got := strings.Join(tried, ","); got != "ag-first,ag-second" {
		t.Fatalf("expected both accounts in order, got %s", got)
	}
	if strings.Contains(w.Body.String(), "quota exhausted") {
		t.Fatalf("first account error leaked into successful stream: %s", w.Body.String())
	}
}

func TestAntigravityFailoverAll429(t *testing.T) {
	store := newAntigravityStore(t, "ag-first", "ag-second")
	svc := New(store, storeSecret(store), "key")
	setAntigravityRoute(t, svc)
	svc.collect = func(context.Context, *providers.Credential, string, map[string]any) (map[string]any, error) {
		return nil, &providers.UpstreamError{Status: http.StatusTooManyRequests, Message: "quota exhausted"}
	}

	req := antigravityRequest(false)
	w := httptest.NewRecorder()
	svc.HandleChat(w, req)

	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "quota exhausted") {
		t.Fatalf("expected last 429, got %d %s", w.Code, w.Body.String())
	}
	if !svc.accountCooling("ag-first") || !svc.accountCooling("ag-second") {
		t.Fatal("all exhausted accounts should be cooling")
	}
}

func TestAntigravityFailoverDoesNotReplayCommittedStream(t *testing.T) {
	store := newAntigravityStore(t, "ag-first", "ag-second")
	svc := New(store, storeSecret(store), "key")
	setAntigravityRoute(t, svc)
	var mu sync.Mutex
	tried := []string{}
	svc.stream = func(_ context.Context, cred *providers.Credential, _ string, _ map[string]any, events *providers.ChatEvents) error {
		mu.Lock()
		tried = append(tried, cred.UID)
		mu.Unlock()
		events.OnFirstToken()
		events.OnDelta(map[string]any{"content": "partial"})
		return &providers.UpstreamError{Status: http.StatusTooManyRequests, Message: "quota exhausted"}
	}

	req := antigravityRequest(true)
	w := httptest.NewRecorder()
	svc.HandleChat(w, req)

	if got := strings.Join(tried, ","); got != "ag-first" {
		t.Fatalf("committed stream must not be replayed, got %s", got)
	}
	if !strings.Contains(w.Body.String(), "partial") || !strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("expected committed stream to finish, got %s", w.Body.String())
	}
}

func TestAntigravityFailoverReportsCooldown(t *testing.T) {
	store := newAntigravityStore(t, "ag-first")
	svc := New(store, storeSecret(store), "key")
	setAntigravityRoute(t, svc)
	svc.markAccountCooling("ag-first", true)

	req := antigravityRequest(false)
	w := httptest.NewRecorder()
	svc.HandleChat(w, req)

	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "冷却") {
		t.Fatalf("expected cooldown response, got %d %s", w.Code, w.Body.String())
	}
}

func antigravityRequest(stream bool) *http.Request {
	body := fmt.Sprintf(
		`{"model":"antigravity/gemini-3.8-flash","stream":%t,"messages":[{"role":"user","content":"hi"}]}`,
		stream,
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer key")
	return req
}

func newAntigravityStore(t *testing.T, ids ...string) *storage.Store {
	t.Helper()
	providers.Register(antigravity.Provider{})
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		acct := storage.Account{
			ID: id, Provider: "antigravity", UID: id, Enabled: true,
			AddedAt: int64(i + 1),
		}
		if err := store.UpsertAccount(&acct); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(providers.Credential{
			UID: id, AccessToken: "token-" + id, ProjectID: "project-" + id,
		})
		if err := store.SaveSecret(id, raw, func(b []byte) ([]byte, error) { return b, nil }); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func storeSecret(store *storage.Store) func(string) ([]byte, error) {
	return func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}
}

func setAntigravityRoute(t *testing.T, svc *Service) {
	t.Helper()
	for _, acct := range svc.store.Accounts() {
		svc.mu.Lock()
		svc.catalogs[acct.ID] = catalogSnapshot{
			models: []providers.ModelInfo{
				{ID: "gemini-3.8-flash", UpstreamID: "gemini-3.8-flash-tiered"},
			},
			at: time.Now(),
		}
		svc.mu.Unlock()
	}
}
