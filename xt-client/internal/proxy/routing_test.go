package proxy

import (
	"encoding/json"
	"testing"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/qoder"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/storage"
)

func TestSelectAccountRoutesByModelPrefix(t *testing.T) {
	store := newRoutingStore(t,
		storage.Account{ID: "wb-1", Provider: "workbuddy", Enabled: true},
		storage.Account{ID: "trae-1", Provider: "trae", Enabled: true},
	)
	svc := New(store, nil, "")

	acct, model, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-1" || model != "glm-5.3-flash" {
		t.Fatalf("expected workbuddy account, got ok=%v acct=%s model=%s", ok, acct.ID, model)
	}
	acct, _, ok = svc.SelectAccount("trae/glm-5.3-flash")
	if !ok || acct.ID != "trae-1" {
		t.Fatalf("expected trae account, got ok=%v acct=%s", ok, acct.ID)
	}
}

func TestSelectAccountUsesStoreOrder(t *testing.T) {
	store := newRoutingStore(t,
		storage.Account{ID: "wb-first", Provider: "workbuddy", Enabled: true},
		storage.Account{ID: "wb-second", Provider: "workbuddy", Enabled: true},
	)
	svc := New(store, nil, "")

	acct, _, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-first" {
		t.Fatalf("expected first account, got ok=%v acct=%s", ok, acct.ID)
	}
}

func TestSelectAccountSkipsExhaustedQuota(t *testing.T) {
	zero := 0.0
	ten := 10.0
	store := newRoutingStore(t,
		storage.Account{
			ID: "wb-empty", Provider: "workbuddy", Enabled: true,
			Quota: &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{
				{Label: "credit", Total: &ten, Remaining: &zero},
			}},
		},
		storage.Account{ID: "wb-next", Provider: "workbuddy", Enabled: true},
	)
	svc := New(store, nil, "")

	acct, _, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-next" {
		t.Fatalf("expected exhausted account to be skipped, got ok=%v acct=%s", ok, acct.ID)
	}
}

func TestSelectAccountKeepsQuotaWithoutRemaining(t *testing.T) {
	ten := 10.0
	tenUsed := 10.0
	store := newRoutingStore(t,
		storage.Account{
			ID: "wb-used-only", Provider: "workbuddy", Enabled: true,
			Quota: &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{
				{Label: "credit", Used: &tenUsed, Total: &ten},
			}},
		},
		storage.Account{ID: "wb-fallback", Provider: "workbuddy", Enabled: true},
	)
	svc := New(store, nil, "")

	acct, _, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-used-only" {
		t.Fatalf("expected unknown remaining quota to stay eligible, got ok=%v acct=%s", ok, acct.ID)
	}
}

func TestSelectAccountKeepsZeroTotalQuota(t *testing.T) {
	zero := 0.0
	store := newRoutingStore(t,
		storage.Account{
			ID: "qoder-zero", Provider: "qoder", Enabled: true,
			Quota: &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{
				{Label: "订阅额度", Used: &zero, Total: &zero, Remaining: &zero},
			}},
		},
	)
	raw, _ := json.Marshal(providers.Credential{AccessToken: "test"})
	if err := store.SaveSecret("qoder-zero", raw, func(b []byte) ([]byte, error) { return b, nil }); err != nil {
		t.Fatal(err)
	}
	revision, err := store.SecretRevision("qoder-zero")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store, func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}, "")
	svc.catalogs = map[string]catalogSnapshot{
		"qoder-zero": {
			models:   []providers.ModelInfo{{ID: "qwen3.8-flash", UpstreamID: "qfmodel"}},
			revision: revision,
			at:       time.Now(),
		},
	}

	acct, model, ok := svc.SelectAccount("qoder/qwen3.8-flash")
	if !ok || acct.ID != "qoder-zero" || model != "qwen3.8-flash" {
		t.Fatalf("expected zero-total quota to stay eligible, got ok=%v acct=%s model=%s", ok, acct.ID, model)
	}
}

func TestSelectAccountKeepsQoderWithZeroQuota(t *testing.T) {
	zero := 0.0
	ten := 10.0
	store := newRoutingStore(t,
		storage.Account{
			ID: "qoder-zero", Provider: "qoder", Enabled: true,
			Quota: &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{
				{Label: "订阅额度", Total: &ten, Remaining: &zero},
			}},
		},
	)
	raw, _ := json.Marshal(providers.Credential{AccessToken: "test"})
	if err := store.SaveSecret("qoder-zero", raw, func(b []byte) ([]byte, error) { return b, nil }); err != nil {
		t.Fatal(err)
	}
	revision, err := store.SecretRevision("qoder-zero")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store, func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}, "")
	svc.catalogs = map[string]catalogSnapshot{
		"qoder-zero": {
			models:   []providers.ModelInfo{{ID: "qwen3.8-flash", UpstreamID: "qfmodel"}},
			revision: revision,
			at:       time.Now(),
		},
	}

	acct, model, ok := svc.SelectAccount("qoder/qwen3.8-flash")
	if !ok || acct.ID != "qoder-zero" || model != "qwen3.8-flash" {
		t.Fatalf("expected Qoder free model to stay eligible, got ok=%v acct=%s model=%s", ok, acct.ID, model)
	}
}

func TestSelectAccountRejectsUnknownProvider(t *testing.T) {
	store := newRoutingStore(t,
		storage.Account{ID: "wb-1", Provider: "workbuddy", Enabled: true},
	)
	svc := New(store, nil, "")

	if _, _, ok := svc.SelectAccount("qoder/gpt-4"); ok {
		t.Fatal("expected unknown provider to have no account")
	}
}

func newRoutingStore(t *testing.T, accounts ...storage.Account) *storage.Store {
	t.Helper()
	providers.Register(trae.Provider{})
	providers.Register(workbuddy.Provider{})
	providers.Register(qoder.Provider{})
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := range accounts {
		acct := accounts[i]
		if acct.AddedAt == 0 {
			acct.AddedAt = int64(i + 1)
		}
		if err := store.UpsertAccount(&acct); err != nil {
			t.Fatal(err)
		}
	}
	return store
}
