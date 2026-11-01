package codex

import (
	"testing"
)

func TestParseCatalogPayload(t *testing.T) {
	raw := []byte(`{
		"models": [
			{
				"slug": "gpt-6-luna",
				"display_name": "GPT-6 Luna",
				"description": "fast",
				"visibility": "list",
				"supported_in_api": true,
				"context_window": 272000,
				"input_modalities": ["text", "image"]
			},
			{
				"slug": "gpt-6.1-sol",
				"display_name": "GPT-6.1 Sol",
				"description": "flagship",
				"visibility": "list",
				"supported_in_api": true,
				"context_window": 500000,
				"input_modalities": ["text", "image"]
			},
			{
				"slug": "internal-model",
				"visibility": "hide",
				"supported_in_api": true
			},
			{
				"slug": "deprecated-model",
				"visibility": "list",
				"supported_in_api": false
			}
		]
	}`)

	models, err := ParseCatalogPayload(raw)
	if err != nil {
		t.Fatalf("ParseCatalogPayload failed: %v", err)
	}

	// internal-model (hide) and deprecated-model (supported_in_api=false) must be filtered out
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	// gpt-6.1-sol should be prioritized to index 0
	if models[0].ID != "gpt-6.1-sol" {
		t.Fatalf("expected gpt-6.1-sol first, got %s", models[0].ID)
	}
	if models[0].MaxInputTokens != 500000 || !models[0].Multimodal || !models[0].Reasoning {
		t.Fatalf("unexpected model attrs for sol: %+v", models[0])
	}

	if models[1].ID != "gpt-6-luna" {
		t.Fatalf("expected gpt-6-luna second, got %s", models[1].ID)
	}
}

func TestParseCatalogEmpty(t *testing.T) {
	raw := []byte(`{"models": []}`)
	_, err := ParseCatalogPayload(raw)
	if err == nil {
		t.Fatalf("expected error for empty models list")
	}
}
