package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type catalogTestProvider struct {
	mu    sync.Mutex
	calls int
	fail  bool
	empty bool
}

func (*catalogTestProvider) Info() providers.Info                    { return providers.Info{ID: "catalog-test"} }
func (*catalogTestProvider) ListModels(string) []providers.ModelInfo { return nil }
func (p *catalogTestProvider) FetchModels(_ context.Context, c *providers.Credential) ([]providers.ModelInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail {
		return nil, fmt.Errorf("unavailable")
	}
	if p.empty {
		return []providers.ModelInfo{}, nil
	}
	return []providers.ModelInfo{{ID: c.UID + "-" + c.Region, UpstreamID: "Upstream", Routes: map[string]string{"high": "Upstream-High"}}}, nil
}

func TestUnifiedCatalogPersistsWithoutExpiryAndSurvivesTokenRotation(t *testing.T) {
	p := &catalogTestProvider{}
	providers.Register(p)
	dir := t.TempDir()
	store, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	identity := func(b []byte) ([]byte, error) { return b, nil }
	a := storage.Account{ID: "persist", Provider: "catalog-test", Region: "cn", UID: "u", Enabled: true}
	if err := store.UpsertAccount(&a); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSecret(a.ID, []byte(`{"AccessToken":"one"}`), identity); err != nil {
		t.Fatal(err)
	}
	newService := func(store *storage.Store) *Service {
		return New(store, func(id string) ([]byte, error) { return store.LoadSecret(id, identity) }, "")
	}
	svc := newService(store)
	if models := svc.AccountModels(a); len(models) != 1 {
		t.Fatalf("first load: %+v", models)
	}
	old := svc.catalogs[a.ID]
	old.at = time.Now().Add(-24 * time.Hour)
	svc.catalogs[a.ID] = old
	if _, err := svc.Models(); err != nil {
		t.Fatal(err)
	}
	svc.chatModelRoutes(a)
	if p.calls != 1 {
		t.Fatalf("viewing or routing refetched: %d", p.calls)
	}
	store, err = storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc = newService(store)
	if models, source := svc.AccountModelCatalog(a); len(models) != 1 || source != "cached" || p.calls != 1 {
		t.Fatalf("restart missed cache: %s %+v calls=%d", source, models, p.calls)
	}
	if routes := svc.chatModelRoutes(a); routes["u-cn"] != "Upstream" || routes["u-cn@high"] != "Upstream-High" {
		t.Fatalf("restart lost internal routes: %+v", routes)
	}
	rev, _ := store.SecretRevision(a.ID)
	if err := store.ReplaceSecret(a.ID, rev, []byte(`{"AccessToken":"rotated"}`), identity); err != nil {
		t.Fatal(err)
	}
	svc.AccountModels(a)
	if p.calls != 1 {
		t.Fatal("token rotation invalidated directory")
	}
	if _, _, err := svc.RefreshAccountModelCatalog(a); err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 {
		t.Fatal("manual refresh did not fetch exactly once")
	}
	if err := store.SaveSecret(a.ID, []byte(`{"AccessToken":"new-login"}`), identity); err != nil {
		t.Fatal(err)
	}
	svc.AccountModels(a)
	if p.calls != 3 {
		t.Fatal("new login reused old catalog")
	}
	if err := store.DeleteAccount(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.ModelCatalog(a.ID); exists {
		t.Fatal("deleted account retained persisted models")
	}
	if models := svc.AccountModels(a); len(models) != 0 {
		t.Fatal("deleted account reused memory cache")
	}
}

func TestSuccessfulEmptyCatalogIsCached(t *testing.T) {
	p := &catalogTestProvider{empty: true}
	providers.Register(p)
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := storage.Account{ID: "empty", Provider: "catalog-test", Enabled: true}
	store.UpsertAccount(&a)
	store.SaveSecret(a.ID, []byte(`{"AccessToken":"one"}`), func(b []byte) ([]byte, error) { return b, nil })
	svc := New(store, func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}, "")
	for range 3 {
		if models := svc.AccountModels(a); len(models) != 0 {
			t.Fatal(models)
		}
	}
	if p.calls != 1 {
		t.Fatalf("empty directory fetched %d times", p.calls)
	}
}

func TestBuiltinFallbackRetriesAfterFailure(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := storage.Account{ID: "fallback", Provider: "codex", Enabled: true}
	identity := func(b []byte) ([]byte, error) { return b, nil }
	if err := store.UpsertAccount(&a); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSecret(a.ID, []byte(`{"AccessToken":"token"}`), identity); err != nil {
		t.Fatal(err)
	}
	svc := New(store, func(id string) ([]byte, error) { return store.LoadSecret(id, identity) }, "")
	p := &catalogTestProvider{fail: true}
	models, source, err := svc.dynamicCatalog(a, p, false)
	if err == nil || source != "builtin" || len(models) == 0 {
		t.Fatalf("fallback: %s %v %+v", source, err, models)
	}
	p.fail = false
	if _, source, _ = svc.dynamicCatalog(a, p, false); source != "builtin" || p.calls != 1 {
		t.Fatalf("fallback was not rate limited: %s calls=%d", source, p.calls)
	}
	snapshot := svc.catalogs[a.ID]
	snapshot.at = time.Now().Add(-builtinRetryInterval)
	svc.catalogs[a.ID] = snapshot
	models, source, err = svc.dynamicCatalog(a, p, false)
	if err != nil || source != "live" || len(models) != 1 || p.calls != 2 {
		t.Fatalf("recovery: %s %v %+v calls=%d", source, err, models, p.calls)
	}
}

func TestDynamicCatalogIsolationAndStale(t *testing.T) {
	p := &catalogTestProvider{}
	providers.Register(p)
	store, _ := storage.Open(t.TempDir())
	svc := New(store, func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}, "")
	for _, a := range []storage.Account{{ID: "a", Provider: "catalog-test", UID: "u", Region: "cn", Enabled: true}, {ID: "b", Provider: "catalog-test", UID: "u", Region: "global", Enabled: true}} {
		store.UpsertAccount(&a)
		raw, _ := json.Marshal(providers.Credential{AccessToken: "token"})
		store.SaveSecret(a.ID, raw, func(b []byte) ([]byte, error) { return b, nil })
	}
	a, _ := store.Account("a")
	b, _ := store.Account("b")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); svc.AccountModels(a) }()
	}
	wg.Wait()
	if p.calls != 1 {
		t.Fatalf("concurrent fetches: %d", p.calls)
	}
	if got := svc.AccountModels(b); got[0].ID != "u-global" {
		t.Fatal("region cache leaked")
	}
	p.fail = true
	models, source, err := svc.RefreshAccountModelCatalog(a)
	if err == nil || source != "stale" || models[0].ID != "u-cn" {
		t.Fatal("last good catalog lost")
	}
	store.SaveSecret("a", []byte(`{"AccessToken":"new-login"}`), func(b []byte) ([]byte, error) { return b, nil })
	models, source = svc.AccountModelCatalog(a)
	if source != "error" || len(models) != 0 {
		t.Fatal("new login reused old catalog")
	}
}
