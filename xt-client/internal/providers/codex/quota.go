package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

func UsageURL(baseURL string) string {
	base := normalizeBaseURL(baseURL)
	if strings.HasSuffix(base, "/backend-api/codex") {
		base = strings.TrimSuffix(base, "/codex")
	} else if !strings.Contains(base, "/backend-api") {
		base = base + "/backend-api"
	}
	return base + "/wham/usage"
}

func windowLabel(windowKey string, minutes float64) string {
	switch int(minutes) {
	case 300:
		return "5 小时窗口"
	case 1440:
		return "1 天窗口"
	case 10080:
		return "周窗口"
	}
	if minutes > 0 && int(minutes)%1440 == 0 {
		return fmt.Sprintf("%d 天窗口", int(minutes)/1440)
	}
	if minutes > 0 && int(minutes)%60 == 0 {
		return fmt.Sprintf("%d 小时窗口", int(minutes)/60)
	}
	switch windowKey {
	case "primary":
		return "主窗口"
	case "secondary":
		return "次窗口"
	}
	if windowKey != "" {
		return windowKey
	}
	return "额度窗口"
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}

func ParseUsagePayload(raw []byte) (*storage.QuotaSnapshot, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析用量响应失败: %w", err)
	}

	snap := &storage.QuotaSnapshot{
		Status:    "ok",
		FetchedAt: time.Now().Unix(),
		Items:     []storage.QuotaItem{},
	}

	var limits map[string]any
	if rl, ok := payload["rate_limit"].(map[string]any); ok {
		limits = rl
	} else if rl, ok := payload["rateLimit"].(map[string]any); ok {
		limits = rl
	} else if rl, ok := payload["rate_limits"].(map[string]any); ok {
		limits = rl
	} else {
		limits = payload
	}

	windowKeys := []string{"primary_window", "secondary_window", "primary", "secondary"}
	seen := map[string]bool{}

	for _, key := range windowKeys {
		wObj, ok := limits[key].(map[string]any)
		if !ok {
			continue
		}
		usedVal, ok := wObj["used_percent"]
		if !ok {
			usedVal = wObj["usedPercent"]
		}
		used, ok := toFloat(usedVal)
		if !ok || used < 0 {
			continue
		}

		// Window duration in minutes
		var minutes float64
		if winSec, ok := toFloat(wObj["limit_window_seconds"]); ok && winSec > 0 {
			minutes = winSec / 60.0
		} else if winSec, ok := toFloat(wObj["limitWindowSeconds"]); ok && winSec > 0 {
			minutes = winSec / 60.0
		} else if winMin, ok := toFloat(wObj["window_minutes"]); ok && winMin > 0 {
			minutes = winMin
		} else if winMin, ok := toFloat(wObj["windowMinutes"]); ok && winMin > 0 {
			minutes = winMin
		}

		label := windowLabel(key, minutes)
		if seen[label] {
			continue
		}
		seen[label] = true

		if used > 100 {
			used = 100
		}
		rem := 100.0 - used
		tot := 100.0

		item := storage.QuotaItem{
			Label:     label,
			Used:      &used,
			Total:     &tot,
			Remaining: &rem,
			Unit:      "percent",
		}

		var resetTs int64
		if rAt, ok := toInt64(wObj["reset_at"]); ok && rAt > 0 {
			resetTs = rAt
		} else if rAt, ok := toInt64(wObj["resetAt"]); ok && rAt > 0 {
			resetTs = rAt
		} else if afterSec, ok := toInt64(wObj["reset_after_seconds"]); ok && afterSec >= 0 {
			resetTs = time.Now().Unix() + afterSec
		} else if afterSec, ok := toInt64(wObj["resetAfterSeconds"]); ok && afterSec >= 0 {
			resetTs = time.Now().Unix() + afterSec
		}
		if resetTs > 0 {
			item.ExpireAt = resetTs
		}
		snap.Items = append(snap.Items, item)
	}

	var credits map[string]any
	if c, ok := payload["credits"].(map[string]any); ok {
		credits = c
	} else if c, ok := limits["credits"].(map[string]any); ok {
		credits = c
	}
	if credits != nil {
		if has, _ := credits["has_credits"].(bool); has {
			unlimited, _ := credits["unlimited"].(bool)
			if unlimited {
				tot := 1.0
				rem := 1.0
				zero := 0.0
				snap.Items = append(snap.Items, storage.QuotaItem{
					Label:     "附加额度（不限额）",
					Used:      &zero,
					Total:     &tot,
					Remaining: &rem,
					Unit:      "credit",
				})
			} else if bal, ok := toFloat(credits["balance"]); ok {
				zero := 0.0
				snap.Items = append(snap.Items, storage.QuotaItem{
					Label:     "附加额度",
					Used:      &zero,
					Total:     &bal,
					Remaining: &bal,
					Unit:      "credit",
				})
			}
		}
	}

	return snap, nil
}

func FetchQuota(ctx context.Context, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}
	url := UsageURL("")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	accountID := cred.EnterpriseID
	for k, v := range buildHeaders(cred.AccessToken, accountID, "", false) {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求用量接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("用量接口返回 HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return ParseUsagePayload(body)
}
