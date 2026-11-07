package app

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

func TestRefreshAgentModelsCoversEnabledSourcesOnceAndKeepsStale(t *testing.T) {
	accounts := []*storage.Account{
		{ID: "one", Provider: "first", Enabled: true},
		{ID: "two", Provider: "second", Enabled: true},
		{ID: "disabled", Provider: "third"},
	}
	var mu sync.Mutex
	calls := map[string]int{}
	models, failures := refreshAgentModelsForAccounts(accounts, func(a storage.Account) ([]providers.ModelInfo, string, error) {
		mu.Lock()
		calls[a.ID]++
		mu.Unlock()
		if a.ID == "two" {
			return []providers.ModelInfo{{ID: "old"}}, "stale", fmt.Errorf("offline")
		}
		return []providers.ModelInfo{{ID: "new"}}, "live", nil
	})
	if calls["one"] != 1 || calls["two"] != 1 || calls["disabled"] != 0 {
		t.Fatal(calls)
	}
	if len(models) != 2 || models[0].Model != "first/new" || models[1].Model != "second/old" || len(failures) != 1 {
		t.Fatalf("models=%+v failures=%v", models, failures)
	}
	_, failures = refreshAgentModelsForAccounts(accounts, func(a storage.Account) ([]providers.ModelInfo, string, error) {
		mu.Lock()
		calls[a.ID]++
		mu.Unlock()
		return nil, "error", fmt.Errorf("offline")
	})
	if calls["one"] != 2 || calls["two"] != 2 || len(failures) != 2 {
		t.Fatalf("refresh retried empty failures: %v %v", calls, failures)
	}
}

func TestAgentModelsFetchConcurrentlyAndKeepAccountOrder(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan []string, 1)
	accounts := []*storage.Account{{Provider: "first", Enabled: true}, {Provider: "disabled"}, {Provider: "second", Enabled: true}}
	go func() {
		models := agentModelsForAccounts(accounts, func(acct storage.Account) []providers.ModelInfo {
			started <- acct.Provider
			<-release
			return []providers.ModelInfo{{ID: "model"}}
		})
		ids := make([]string, len(models))
		for i, model := range models {
			ids[i] = model.Model
		}
		done <- ids
	}()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("enabled accounts did not start concurrently")
		}
	}
	close(release)
	released = true
	select {
	case ids := <-done:
		if len(ids) != 2 || ids[0] != "first/model" || ids[1] != "second/model" {
			t.Fatalf("unexpected models: %v", ids)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fetch did not finish")
	}
}

func TestAgentModelsPricingFollowsFirstAccount(t *testing.T) {
	accounts := []*storage.Account{{ID: "first", Provider: "qoder", Enabled: true}, {ID: "second", Provider: "qoder", Enabled: true}}
	models := agentModelsForAccounts(accounts, func(a storage.Account) []providers.ModelInfo {
		factor := 0.5
		if a.ID == "second" {
			factor = 2
		}
		return []providers.ModelInfo{{ID: "m", PriceFactor: &factor}}
	})
	if len(models) != 1 || models[0].PriceFactor == nil || *models[0].PriceFactor != 0.5 {
		t.Fatalf("wrong routing price: %v", models)
	}
}

func TestAgentModelsExposeContextWindow(t *testing.T) {
	accounts := []*storage.Account{{ID: "first", Provider: "codex", Enabled: true}}
	models := agentModelsForAccounts(accounts, func(storage.Account) []providers.ModelInfo {
		return []providers.ModelInfo{{ID: "m", MaxInputTokens: 272000}}
	})
	if len(models) != 1 || models[0].ContextWindow != 272000 {
		t.Fatalf("model context window missing: %+v", models)
	}
}
