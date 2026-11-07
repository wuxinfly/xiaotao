package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/storage"
)

type catalogHTTPProvider struct {
	mu    sync.Mutex
	calls map[string]int
	fail  bool
}

func (*catalogHTTPProvider) Info() providers.Info                    { return providers.Info{ID: "catalog-http"} }
func (*catalogHTTPProvider) ListModels(string) []providers.ModelInfo { return nil }
func (p *catalogHTTPProvider) FetchModels(_ context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[cred.UID]++
	if p.fail {
		return nil, fmt.Errorf("offline")
	}
	return []providers.ModelInfo{{ID: cred.UID, DisplayName: fmt.Sprintf("version-%d", p.calls[cred.UID])}}, nil
}

func TestModelCatalogHTTPReadsAndRefreshScopes(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(filepath.Join(dir, "accounts"))
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := secrets.Open(filepath.Join(dir, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two", "disabled"} {
		if err := store.UpsertAccount(&storage.Account{ID: id, UID: id, Provider: "catalog-http", Enabled: id != "disabled"}); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveSecret(id, []byte(`{"AccessToken":"fixture"}`), keyring.Encrypt); err != nil {
			t.Fatal(err)
		}
	}
	p := &catalogHTTPProvider{calls: map[string]int{}}
	providers.Register(p)
	state := State{URL: "http://127.0.0.1:12345", Token: "fixture"}
	h, err := Handler(state, dir, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closer, ok := h.(interface{ Close() error }); ok {
			closer.Close()
		}
	})
	call := func(method, path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, state.URL+path, nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	check := func(method, path string) map[string]json.RawMessage {
		t.Helper()
		w := call(method, path, state.URL)
		if w.Code != 200 {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	check("GET", "/api/proxy/models?accountId=one")
	check("GET", "/api/proxy/models?accountId=one")
	check("GET", "/api/agent/models")
	check("GET", "/api/proxy/models")
	if p.calls["one"] != 1 || p.calls["two"] != 1 || p.calls["disabled"] != 0 {
		t.Fatalf("views refetched: %v", p.calls)
	}
	check("POST", "/api/proxy/models?accountId=one")
	if p.calls["one"] != 2 || p.calls["two"] != 1 {
		t.Fatalf("account refresh scope: %v", p.calls)
	}
	check("POST", "/api/agent/models/refresh")
	if p.calls["one"] != 3 || p.calls["two"] != 2 || p.calls["disabled"] != 0 {
		t.Fatalf("global refresh scope: %v", p.calls)
	}
	p.fail = true
	result := check("POST", "/api/agent/models/refresh")
	var failures []string
	if err := json.Unmarshal(result["errors"], &failures); err != nil || len(failures) != 2 {
		t.Fatalf("missing partial errors: %s", result["errors"])
	}
	var models []struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(result["models"], &models); err != nil || len(models) != 2 || models[0].Label != "version-3 · catalog-http" || models[1].Label != "version-2 · catalog-http" {
		t.Fatalf("old models lost: %s", result["models"])
	}
	check("GET", "/api/agent/models")
	if p.calls["one"] != 4 || p.calls["two"] != 3 {
		t.Fatalf("failure or later view retried: %v", p.calls)
	}
	if w := call("POST", "/api/agent/models/refresh", "http://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin refresh: %d", w.Code)
	}
	p.fail = false
	check("POST", "/api/proxy/models")
	if p.calls["one"] != 5 || p.calls["two"] != 4 || p.calls["disabled"] != 0 {
		t.Fatalf("legacy global refresh failed: %v", p.calls)
	}
}
