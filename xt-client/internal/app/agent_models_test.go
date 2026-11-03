package app

import (
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

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
