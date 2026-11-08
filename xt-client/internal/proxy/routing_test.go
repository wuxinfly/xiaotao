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
	svc := routingService(t, store)

	acct, model, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-1" || model != "glm-5.3-flash" {
		t.Fatalf("expected workbuddy account, got ok=%v acct=%s model=%s", ok, acct.ID, model)
	}
	acct, _, ok = svc.SelectAccount("trae/glm-5.3-flash")
	if !ok || acct.ID != "trae-1" {
		t.Fatalf("expected trae account, got ok=%v acct=%s", ok, acct.ID)
	}
}

func TestSelectAccountRoutesTraeMaxVariantFromCachedCatalog(t *testing.T) {
	store := newRoutingStore(t,
		storage.Account{ID: "trae-max", Provider: "trae", Region: "cn", Enabled: true},
	)
	svc := routingService(t, store)
	svc.catalogs["trae-max"] = catalogSnapshot{
		region: "cn",
		at:     time.Now(),
		models: []providers.ModelInfo{
			{ID: "glm-5.3", UpstreamID: "GLM-5.3"},
			{ID: "glm-5.3-max", UpstreamID: "GLM-5.3__max"},
		},
	}

	acct, model, ok := svc.SelectAccount("trae/glm-5.3-max")
	if !ok || acct.ID != "trae-max" || model != "glm-5.3-max" {
		t.Fatalf("max variant rejected: ok=%v acct=%s model=%s", ok, acct.ID, model)
	}
}

func TestSelectAccountUsesStoreOrder(t *testing.T) {
	store := newRoutingStore(t,
		storage.Account{ID: "wb-first", Provider: "workbuddy", Enabled: true},
		storage.Account{ID: "wb-second", Provider: "workbuddy", Enabled: true},
	)
	svc := routingService(t, store)

	acct, _, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-first" {
		t.Fatalf("expected first account, got ok=%v acct=%s", ok, acct.ID)
	}
}

func TestSelectAccountUsesManualOrder(t *testing.T) {
	store := newRoutingStore(t,
		storage.Account{ID: "wb-first", Provider: "workbuddy", Enabled: true},
		storage.Account{ID: "wb-second", Provider: "workbuddy", Enabled: true},
	)
	if err := store.ReorderAccounts("workbuddy", []string{"wb-second", "wb-first"}); err != nil {
		t.Fatal(err)
	}
	svc := routingService(t, store)

	acct, _, ok := svc.SelectAccount("workbuddy/glm-5.3-flash")
	if !ok || acct.ID != "wb-second" {
		t.Fatalf("expected manually reordered account, got ok=%v acct=%s", ok, acct.ID)
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
	svc := routingService(t, store)

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
	svc := routingService(t, store)

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
	svc := routingService(t, store)

	if _, _, ok := svc.SelectAccount("qoder/gpt-4"); ok {
		t.Fatal("expected unknown provider to have no account")
	}
}

func TestAccountCandidatesWithReasons(t *testing.T) {
	rem := 0.0
	exhausted := storage.QuotaSnapshot{
		Status: "ok",
		Items: []storage.QuotaItem{
			{Remaining: &rem},
		},
	}
	store := newRoutingStore(t,
		storage.Account{ID: "acct-disabled", Provider: "workbuddy", Enabled: false},
		storage.Account{ID: "acct-unsupported", Provider: "workbuddy", Enabled: true},
		storage.Account{ID: "acct-exhausted", Provider: "workbuddy", Enabled: true, Quota: &exhausted},
		storage.Account{ID: "acct-cooling", Provider: "workbuddy", Enabled: true},
		storage.Account{ID: "acct-valid", Provider: "workbuddy", Enabled: true},
	)
	svc := routingService(t, store)
	rev, _ := store.SecretRevision("acct-unsupported")
	svc.catalogs["acct-unsupported"] = catalogSnapshot{region: "", revision: rev, source: "cached", at: time.Now(), models: []providers.ModelInfo{{ID: "other-model"}}}
	// Mark acct-cooling as cooling
	svc.markAccountCooling("acct-cooling", false)

	candidates, reasons := svc.accountCandidatesWithReasons("workbuddy/glm-5.3-flash")
	if len(candidates) != 1 || candidates[0].ID != "acct-valid" {
		t.Fatalf("expected only acct-valid, got %v", candidates)
	}

	foundReasons := map[string]string{}
	for _, r := range reasons {
		foundReasons[r.Account] = r.Reason
	}
	if foundReasons["acct-disabled"] != "disabled" {
		t.Errorf("expected disabled, got %s", foundReasons["acct-disabled"])
	}
	if foundReasons["acct-unsupported"] != "model_unsupported" {
		t.Errorf("expected model_unsupported, got %s", foundReasons["acct-unsupported"])
	}
	if foundReasons["acct-exhausted"] != "quota_exhausted" {
		t.Errorf("expected quota_exhausted, got %s", foundReasons["acct-exhausted"])
	}
	if foundReasons["acct-cooling"] != "cooling" {
		t.Errorf("expected cooling, got %s", foundReasons["acct-cooling"])
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

func routingService(t *testing.T, store *storage.Store) *Service {
	t.Helper()
	svc := New(store, nil, "")
	for _, a := range store.Accounts() {
		if a.Provider != "workbuddy" {
			continue
		}
		if err := store.SaveSecret(a.ID, []byte(`{"AccessToken":"fixture"}`), func(b []byte) ([]byte, error) { return b, nil }); err != nil {
			t.Fatal(err)
		}
		rev, err := store.SecretRevision(a.ID)
		if err != nil {
			t.Fatal(err)
		}
		svc.catalogs[a.ID] = catalogSnapshot{region: a.Region, revision: rev, at: time.Now(), models: []providers.ModelInfo{{ID: "glm-5.3-flash"}}}
	}
	return svc
}
