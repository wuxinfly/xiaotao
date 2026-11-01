package antigravity

import (
	"testing"
	"xiaotao/xt-client/internal/providers"
)

func TestDiscoveredCatalogRoutes(t *testing.T) {
	models, err := parseAvailableModels(map[string]any{"models": map[string]any{
		"gemini-4-flash-low": map[string]any{}, "gemini-4-flash-high": map[string]any{},
		"chat_internal": map[string]any{}, "tab_internal": map[string]any{},
		"gemini-2.5-flash": map[string]any{}, "gemini-3.1-flash-low": map[string]any{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != len(ListModels())+1 {
		t.Fatalf("unexpected models: %+v", models)
	}
	m := models[len(models)-1]
	if m.ID != "gemini-4-flash" || m.UpstreamID != "gemini-4-flash-low" {
		t.Fatalf("wrong default: %+v", m)
	}
	cred := &providers.Credential{ModelRoutes: map[string]string{m.ID: m.UpstreamID, m.ID + "@high": m.Routes["high"]}}
	route, err := requestModel(cred, m.ID, "high")
	if err != nil || route != "gemini-4-flash-high" {
		t.Fatalf("route %s, %v", route, err)
	}
	curated, err := requestModel(cred, "gemini-3.1-pro", "")
	if err != nil || curated != "gemini-3.1-pro-low" {
		t.Fatalf("lost curated route %s %v", curated, err)
	}
	if _, err := parseAvailableModels(map[string]any{"models": map[string]any{"tab_internal": nil}}); err == nil {
		t.Fatal("accepted internal-only catalog")
	}
}
