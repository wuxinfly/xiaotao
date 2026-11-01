package antigravity

import (
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/providers"
)

// Model entries mirror buddy-proxy's curated antigravity model table.  The
// proxy layer uses the external id, upstream carries the real API name, and
// effort suffixes are appended per model.
type Model struct {
	ExternalID    string
	Upstream      string
	Group         string
	Display       string
	Multimodal    bool
	Efforts       []string
	DefaultEffort string
	Verified      bool
}

var modelCatalog = []Model{
	{ExternalID: "gemini-3.1-pro", Upstream: "gemini-3.1-pro", Group: "gemini", Display: "Gemini 3.1 Pro", Efforts: []string{"low", "high"}, DefaultEffort: "low", Verified: true},
	{ExternalID: "gemini-3.6-flash", Upstream: "gemini-3.6-flash", Group: "gemini", Display: "Gemini 3.6 Flash", Efforts: []string{"low", "medium", "high"}, DefaultEffort: "medium", Verified: true},
	{ExternalID: "gemini-3.8-flash", Upstream: "gemini-3.8-flash-tiered", Group: "gemini", Display: "Gemini 3.8 Flash (tiered)", Verified: true},
	{ExternalID: "claude-opus-4-6-thinking", Upstream: "claude-opus-4-6-thinking", Group: "claude-gpt", Display: "Claude Opus 4.6 (Thinking)", Verified: true},
	{ExternalID: "claude-sonnet-4-6", Upstream: "claude-sonnet-4-6", Group: "claude-gpt", Display: "Claude Sonnet 4.6", Verified: true},
	{ExternalID: "gpt-oss-120b", Upstream: "gpt-oss-120b-medium", Group: "claude-gpt", Display: "GPT-OSS 120B", Verified: true},
}

var modelIndex = func() map[string]Model {
	out := map[string]Model{}
	for _, m := range modelCatalog {
		out[m.ExternalID] = m
	}
	return out
}()

func ListModels() []providers.ModelInfo {
	out := make([]providers.ModelInfo, 0, len(modelCatalog))
	for _, m := range modelCatalog {
		out = append(out, providers.ModelInfo{ID: m.ExternalID, DisplayName: m.Display, Multimodal: m.Multimodal})
	}
	return out
}

func lookupModel(id string) (Model, error) {
	m, ok := modelIndex[strings.TrimSpace(id)]
	if !ok {
		return Model{}, fmt.Errorf("antigravity 未知模型: %s", id)
	}
	return m, nil
}

// resolveUpstreamModel maps external model id + reasoning effort to the
// upstream name.  Unsupported explicit efforts fall back to the table default
// (matching buddy-proxy: a usable tier is better than a hard 400).
func resolveUpstreamModel(m Model, reasoningEffort string) string {
	base := m.Upstream
	if base == "" {
		base = m.ExternalID
	}
	if len(m.Efforts) == 0 {
		return base
	}
	effort := strings.ToLower(strings.TrimSpace(reasoningEffort))
	if !contains(m.Efforts, effort) {
		effort = m.DefaultEffort
		if effort == "" && len(m.Efforts) > 0 {
			effort = m.Efforts[0]
		}
	}
	if effort == "" {
		return base
	}
	return base + "-" + effort
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
