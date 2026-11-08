package app

import (
	"sync"
	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

// Aggregate the refresh results directly: reading again would retry an empty,
// failed catalog and make one click send two upstream requests.
func refreshAgentModelsForAccounts(accounts []*storage.Account, refresh func(storage.Account) ([]providers.ModelInfo, string, error)) ([]agent.ModelOption, []string) {
	var mu sync.Mutex
	failed := map[string]bool{}
	models := agentModelsForAccounts(accounts, func(acct storage.Account) []providers.ModelInfo {
		models, _, err := refresh(acct)
		if err != nil {
			mu.Lock()
			failed[acct.ID] = true
			mu.Unlock()
		}
		return models
	})
	errors := []string{}
	for _, acct := range accounts {
		if failed[acct.ID] {
			errors = append(errors, acct.Provider+": 模型目录刷新失败，已保留原有目录")
		}
	}
	return models, errors
}

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
				results[i] = append(results[i], agent.ModelOption{Model: id, Label: label, PriceFactor: model.PriceFactor, IsFree: model.IsFree, SupportsImages: model.Multimodal, SupportsReasoning: model.Reasoning, ReasoningEfforts: model.Efforts, ContextWindow: int64(model.MaxInputTokens)})
			}
		}(i, *acct)
	}
	wg.Wait()
	out := []agent.ModelOption{}
	seen := map[string]bool{}
	for _, result := range results {
		for _, model := range result {
			if !seen[model.Model] {
				seen[model.Model] = true
				out = append(out, model)
			}
		}
	}
	return out
}
