package app

import (
	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/modelsource"
)

// A saved source is the catalog. Reading it never performs upstream discovery.
func agentModelsForSources(sources []modelsource.Source) []agent.ModelOption {
	out := []agent.ModelOption{}
	for _, source := range sources {
		if !source.Enabled || !source.HasAPIKey {
			continue
		}
		for _, model := range source.Models {
			out = append(out, agent.ModelOption{
				Model: model.ID, Label: model.Label + " · " + source.Label,
				SourceID: source.ID, SourceLabel: source.Label, UpstreamModelID: model.UpstreamModelID,
				ContextWindow: int64(model.ContextWindow), MaxOutputTokens: int64(model.MaxOutputTokens),
				SupportsImages: model.SupportsImages, SupportsReasoning: model.SupportsReasoning,
				ReasoningEfforts: append([]string(nil), model.ReasoningEfforts...),
			})
		}
	}
	return out
}
