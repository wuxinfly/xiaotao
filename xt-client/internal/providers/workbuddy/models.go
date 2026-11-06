package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"xiaotao/xt-client/internal/providers"
)

// FetchModels exposes only the selected desktop agent's chat catalog.
func (Provider) FetchModels(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	base := testBaseURL
	if base == "" {
		if err := ValidateCredential(cred); err != nil {
			return nil, err
		}
		base = DefaultBaseURL
	}
	raw, err := get(ctx, base+"/v3/config", headersFor(cred))
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		return nil, fmt.Errorf("invalid WorkBuddy config")
	}
	if code := number(envelope["code"]); code != 0 {
		return nil, fmt.Errorf("WorkBuddy config code %d", code)
	}
	data, err := json.Marshal(unwrapData(envelope))
	if err != nil {
		return nil, err
	}
	var config struct {
		Models []struct {
			ID                string   `json:"id"`
			Name              string   `json:"name"`
			SupportsImages    bool     `json:"supportsImages"`
			SupportsReasoning bool     `json:"supportsReasoning"`
			Tags              []string `json:"tags"`
		} `json:"models"`
		Agents []struct {
			Name   string   `json:"name"`
			Tags   []string `json:"tags"`
			Models []string `json:"models"`
		} `json:"agents"`
	}
	if json.Unmarshal(data, &config) != nil {
		return nil, fmt.Errorf("invalid WorkBuddy model config")
	}
	selected := -1
	for i, a := range config.Agents {
		for _, tag := range a.Tags {
			if tag == "default" {
				selected = i
				break
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		for i, a := range config.Agents {
			if a.Name == "cli" {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		for i, a := range config.Agents {
			if len(a.Models) > 0 {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return nil, fmt.Errorf("WorkBuddy config has no client model list")
	}
	out := []providers.ModelInfo{}
	seen := map[string]bool{}
	for _, id := range config.Agents[selected].Models {
		for _, m := range config.Models {
			if m.ID != id && m.Name != id {
				continue
			}
			image := false
			for _, tag := range m.Tags {
				if tag == "text-to-image" || tag == "image-to-image" {
					image = true
				}
			}
			if m.ID == "" || seen[m.ID] || image {
				continue
			}
			seen[m.ID] = true
			name := m.Name
			if name == "" {
				name = m.ID
			}
			out = append(out, providers.ModelInfo{ID: m.ID, DisplayName: name, Multimodal: m.SupportsImages, Reasoning: m.SupportsReasoning})
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("WorkBuddy client model list is empty")
	}
	return out, nil
}
