package app

import (
	"sync"
	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

func agentModelsForAccounts(accounts []*storage.Account, fetch func(storage.Account) []providers.ModelInfo) []agent.ModelOption {
	results := make([][]agent.ModelOption, len(accounts))
	var wg sync.WaitGroup
	for i, acct := range accounts {
		if !acct.Enabled {
			continue
		}
		wg.Add(1)
		go func(i int, acct storage.Account) {
			defer wg.Done()
			for _, model := range fetch(acct) {
				id := acct.Provider + "/" + model.ID
				label := id
				if model.DisplayName != "" {
					label = model.DisplayName + " · " + acct.Provider
				}
				results[i] = append(results[i], agent.ModelOption{Model: id, Label: label})
			}
		}(i, *acct)
	}
	wg.Wait()
	out := []agent.ModelOption{}
	for _, result := range results {
		out = append(out, result...)
	}
	return out
}
