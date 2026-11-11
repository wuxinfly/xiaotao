package app

import (
	"testing"
	"xiaotao/xt-client/internal/modelsource"
)

func TestAgentModelsUseStableRoutesAndSavedCapabilities(t *testing.T) {
	factor, free := 0.31, false
	sources := []modelsource.Source{
		{ID: "source_1", Label: "接口一", Enabled: true, HasAPIKey: true, Models: []modelsource.Model{{ID: "model_1", UpstreamModelID: "org/model", Label: "模型", ContextWindow: 32000, MaxOutputTokens: 2048, SupportsImages: true, SupportsReasoning: true, ReasoningEfforts: []string{"low", "high"}, PriceFactor: &factor, IsFree: &free}}},
		{ID: "source_2", Label: "接口二", Enabled: true, HasAPIKey: true, Models: []modelsource.Model{{ID: "model_2", UpstreamModelID: "org/model", Label: "模型"}}},
		{ID: "disabled", Models: []modelsource.Model{{ID: "model_3"}}},
		{ID: "missing-key", Enabled: true, Models: []modelsource.Model{{ID: "model_4"}}},
	}
	models := agentModelsForSources(sources)
	if len(models) != 2 || models[0].Model != "model_1" || models[1].Model != "model_2" {
		t.Fatalf("routes collapsed: %+v", models)
	}
	first := models[0]
	if first.SourceID != "source_1" || first.SourceLabel != "接口一" || first.UpstreamModelID != "org/model" || first.Label != "模型 · 接口一" {
		t.Fatalf("source identity lost: %+v", first)
	}
	if first.ContextWindow != 32000 || first.MaxOutputTokens != 2048 || !first.SupportsImages || !first.SupportsReasoning {
		t.Fatalf("capabilities lost: %+v", first)
	}
	if first.PriceFactor == nil || *first.PriceFactor != 0.31 || first.IsFree == nil || *first.IsFree {
		t.Fatalf("price metadata lost: %+v", first)
	}
	first.ReasoningEfforts[0] = "changed"
	if sources[0].Models[0].ReasoningEfforts[0] != "low" {
		t.Fatal("catalog aliases mutable storage")
	}
}

func TestAgentModelsEmptyDirectoryIsJSONArray(t *testing.T) {
	if got := agentModelsForSources(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty models: %#v", got)
	}
}
