package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type catalogTestProvider struct {
	mu    sync.Mutex
	calls int
	fail  bool
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
	return []providers.ModelInfo{{ID: c.UID + "-" + c.Region, UpstreamID: "upstream"}}, nil
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
