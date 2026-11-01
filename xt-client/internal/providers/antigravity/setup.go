package antigravity

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

const (
	ideTypeAntigravity = 9
	pluginTypeGemini   = 2
	pollInterval       = 5 * time.Second
	pollBudget         = 60 * time.Second
)

func platformEnum() int {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/amd64":
		return 1
	case "darwin/arm64":
		return 2
	case "linux/amd64":
		return 3
	case "linux/arm64":
		return 4
	case "windows/amd64":
		return 5
	}
	return 1
}

func coreMetadata() map[string]any {
	return map[string]any{
		"ideType":    ideTypeAntigravity,
		"platform":   platformEnum(),
		"pluginType": pluginTypeGemini,
	}
}

// SetupCodeAssist runs loadCodeAssist and, if necessary, onboardUser.  It
// returns the project ID once onboarding completes successfully.
func SetupCodeAssist(ctx context.Context, accessToken string) (projectID, tier, tierName string, err error) {
	loadBody := map[string]any{"metadata": coreMetadata()}
	loadRes, err := postJSON(ctx, accessToken, "loadCodeAssist", loadBody)
	if err != nil {
		return "", "", "", fmt.Errorf("loadCodeAssist failed: %w", err)
	}
	currentTier, _ := loadRes["currentTier"].(map[string]any)
	if len(currentTier) > 0 {
		proj := projectFromValue(loadRes["cloudaicompanionProject"])
		if proj == "" {
			return "", "", "", fmt.Errorf("账号已开通但未返回 cloudaicompanionProject")
		}
		paidTier, _ := loadRes["paidTier"].(map[string]any)
		tierValue := currentTier
		if len(paidTier) > 0 {
			tierValue = paidTier
		}
		id, _ := tierValue["id"].(string)
		name, _ := tierValue["name"].(string)
		return proj, id, name, nil
	}

	tierID := defaultTierID(loadRes)
	onboardBody := map[string]any{"tierId": tierID, "metadata": coreMetadata()}
	lro, err := postJSON(ctx, accessToken, "onboardUser", onboardBody)
	if err != nil {
		return "", "", "", fmt.Errorf("onboardUser failed: %w", err)
	}
	lro, err = waitForLRO(ctx, accessToken, lro, onboardBody)
	if err != nil {
		return "", "", "", err
	}
	if lroErr, ok := lro["error"].(map[string]any); ok {
		code, _ := lroErr["code"].(string)
		message, _ := lroErr["message"].(string)
		return "", "", "", fmt.Errorf("onboardUser failed: [%s] %s", code, message)
	}
	resp, _ := lro["response"].(map[string]any)
	proj := projectFromValue(resp["cloudaicompanionProject"])
	if proj == "" {
		proj = projectFromValue(lro["cloudaicompanionProject"])
	}
	if proj == "" {
		return "", "", "", fmt.Errorf("onboarding completed but no project ID returned")
	}
	return proj, tierID, "", nil
}

func waitForLRO(ctx context.Context, accessToken string, op map[string]any, body map[string]any) (map[string]any, error) {
	deadline := time.Now().Add(pollBudget)
	done, _ := op["done"].(bool)
	for !done {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("onboardUser timeout: LRO did not finish")
		}
		time.Sleep(pollInterval)
		var err error
		op, err = postJSON(ctx, accessToken, "onboardUser", body)
		if err != nil {
			return nil, fmt.Errorf("onboardUser polling failed: %w", err)
		}
		done, _ = op["done"].(bool)
	}
	return op, nil
}

func defaultTierID(loadRes map[string]any) string {
	tiers, _ := loadRes["allowedTiers"].([]any)
	for _, item := range tiers {
		tier, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := tier["id"].(string)
		if id == "" {
			continue
		}
		if isDefault, _ := tier["isDefault"].(bool); isDefault {
			return id
		}
	}
	for _, item := range tiers {
		if tier, ok := item.(map[string]any); ok {
			if id, ok := tier["id"].(string); ok && id != "" {
				return id
			}
		}
	}
	return "legacy-tier"
}

func projectFromValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		for _, key := range []string{"id", "projectId", "project"} {
			if inner, ok := v[key].(string); ok && inner != "" {
				return inner
			}
			if nested, ok := v[key].(map[string]any); ok {
				if id, ok := nested["id"].(string); ok && id != "" {
					return id
				}
			}
		}
	}
	return ""
}
