package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type Provider struct{}

func (Provider) Info() providers.Info {
	return providers.Info{
		ID:          "antigravity",
		DisplayName: "Antigravity",
		LoginType:   "google",
		Regions:     []string{"global"},
		Billing:     "quota",
	}
}

func (Provider) ListModels(region string) []providers.ModelInfo {
	_ = region
	return ListModels()
}

// Refresh exchanges the stored refresh token for a new access token,
// preserving identity, project, and tier fields.
func Refresh(ctx context.Context, cred *providers.Credential) (*providers.Credential, error) {
	if cred == nil || cred.RefreshToken == "" {
		return nil, fmt.Errorf("missing refresh token")
	}
	form := strings.NewReader("grant_type=refresh_token&refresh_token=" + url.QueryEscape(cred.RefreshToken) +
		"&client_id=" + url.QueryEscape(clientID) + "&client_secret=" + url.QueryEscape(clientSecret))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, form)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := newHTTPClient(connectTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google token 接口请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := readAll(resp.Body)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid token response")
	}
	if errMsg, ok := payload["error"].(string); ok {
		return nil, fmt.Errorf("Google token 接口拒绝: %s", errMsg)
	}
	next := *cred
	if token, ok := payload["access_token"].(string); ok {
		next.AccessToken = token
	}
	next.ExpiresAt = expiresAt(payload["expires_in"])
	if scope, ok := payload["scope"].(string); ok {
		next.Region = scope
	}
	return &next, nil
}

func FetchQuota(ctx context.Context, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}
	payload, err := upstreamRequest(ctx, cred.AccessToken, "fetchAvailableModels", "", map[string]any{}, false)
	if err != nil {
		return nil, err
	}
	models, _ := payload["models"].(map[string]any)
	snap := &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{}, FetchedAt: time.Now().Unix()}
	for _, entry := range modelCatalog {
		group := entry.Group
		if group == "" {
			group = "gemini"
		}
		base := entry.Upstream
		if base == "" {
			base = entry.ExternalID
		}
		for name, raw := range models {
			if !strings.HasPrefix(name, base) {
				continue
			}
			info, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			quotaInfo, _ := info["quotaInfo"].(map[string]any)
			if quotaInfo == nil {
				continue
			}
			frac := firstNumber(quotaInfo, "remainingFraction")
			if frac == nil {
				continue
			}
			used := (1 - *frac) * 1000
			total := 1000.0
			remaining := *frac * 1000
			label := "Gemini 组"
			if group == "claude-gpt" {
				label = "Claude/GPT 组"
			}
			item := storage.QuotaItem{
				Label: label, Used: &used, Total: &total, Remaining: &remaining,
				Unit: "permille",
			}
			if reset, ok := quotaInfo["resetTime"].(string); ok {
				item.ExpireAt = parseISO8601(reset)
			}
			snap.Items = append(snap.Items, item)
			break
		}
	}
	if len(snap.Items) == 0 {
		return nil, fmt.Errorf("quota response has no models")
	}
	return snap, nil
}
