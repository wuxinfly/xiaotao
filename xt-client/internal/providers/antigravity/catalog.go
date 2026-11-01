package antigravity

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"xiaotao/xt-client/internal/providers"
)

// FetchModels merges public discoveries with curated routes. The upstream
// catalog omits some verified premium models and includes obsolete/internal ones.
func FetchModels(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	payload, err := upstreamRequest(ctx, cred.AccessToken, "fetchAvailableModels", "", map[string]any{}, false)
	if err != nil {
		return nil, err
	}
	return parseAvailableModels(payload)
}

func parseAvailableModels(payload map[string]any) ([]providers.ModelInfo, error) {
	remote, ok := payload["models"].(map[string]any)
	if !ok || len(remote) == 0 {
		return nil, fmt.Errorf("empty Antigravity catalog")
	}
	names := make([]string, 0, len(remote))
	for name := range remote {
		names = append(names, name)
	}
	sort.Strings(names)
	discovered := map[string]providers.ModelInfo{}
	for _, name := range names {
		base, effort := splitEffort(name)
		lower := strings.ToLower(base)
		blocked := false
		for _, prefix := range []string{"chat_", "tab_", "gemini-2.5-", "gemini-3-flash", "gemini-3.1-flash", "gemini-3.5-", "gemini-pro-agent"} {
			if strings.HasPrefix(lower, prefix) {
				blocked = true
			}
		}
		// Image models require a separate response path; do not advertise them here.
		if blocked || strings.Contains(lower, "image") || base == "" {
			continue
		}
		m := discovered[base]
		m.ID, m.DisplayName = base, base
		if m.Routes == nil {
			m.Routes = map[string]string{}
		}
		m.Routes[effort] = name
		discovered[base] = m
	}
	if len(discovered) == 0 {
		return nil, fmt.Errorf("no public chat models in Antigravity catalog")
	}
	out := ListModels()
	for _, m := range out {
		delete(discovered, m.ID)
	}
	keys := make([]string, 0, len(discovered))
	for id := range discovered {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		m := discovered[id]
		for _, effort := range []string{"", "medium", "tiered", "low", "high", "extra-low"} {
			if route := m.Routes[effort]; route != "" {
				m.UpstreamID = route
				break
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func splitEffort(name string) (string, string) {
	for _, effort := range []string{"extra-low", "low", "medium", "high", "tiered"} {
		if strings.HasSuffix(name, "-"+effort) {
			return strings.TrimSuffix(name, "-"+effort), effort
		}
	}
	return name, ""
}

func requestModel(cred *providers.Credential, model, effort string) (string, error) {
	if route := cred.ModelRoutes[model+"@"+effort]; route != "" {
		return route, nil
	}
	if route := cred.ModelRoutes[model]; route != "" {
		return route, nil
	}
	entry, err := lookupModel(model)
	if err != nil {
		return "", err
	}
	return resolveUpstreamModel(entry, effort), nil
}
