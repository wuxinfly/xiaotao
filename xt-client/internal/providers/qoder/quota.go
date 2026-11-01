package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

func FetchQuota(ctx context.Context, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	reg, ok := Regions[cred.Region]
	if !ok {
		return nil, fmt.Errorf("unsupported Qoder region")
	}
	data, status, err := (AuthClient{}).request(ctx, http.MethodGet, reg.QuotaURL(), cred.AccessToken, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, &providers.UpstreamError{Status: status, Message: "Qoder quota request failed"}
	}
	raw, _ := json.Marshal(data)
	return parseQuota(raw)
}

func parseQuota(raw []byte) (*storage.QuotaSnapshot, error) {
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return nil, fmt.Errorf("invalid Qoder quota response")
	}
	snap := &storage.QuotaSnapshot{Status: "ok", FetchedAt: time.Now().Unix(), Items: []storage.QuotaItem{}}
	expiry := quotaExpiry(data["expiresAt"])
	valid := false
	item := func(node map[string]any, label string, expire int64) storage.QuotaItem {
		number := func(key string) *float64 {
			if n, ok := node[key].(float64); ok {
				return &n
			}
			return nil
		}
		return storage.QuotaItem{Label: label, Used: number("used"), Total: number("total"), Remaining: number("remaining"), Unit: "credit", ExpireAt: expire}
	}
	for _, entry := range []struct{ key, label string }{{"userQuota", "订阅额度"}, {"addOnQuota", "加购包"}} {
		if node, ok := data[entry.key].(map[string]any); ok {
			valid = true
			exp := quotaExpiry(node["expiresAt"])
			if exp == 0 {
				exp = expiry
			}
			snap.Items = append(snap.Items, item(node, entry.label, exp))
		}
	}
	if packages, ok := data["dedicatedResourcePackages"].([]any); ok {
		valid = true
		for _, value := range packages {
			pkg, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if available, ok := pkg["available"].(bool); ok && !available {
				continue
			}
			status := strings.ToUpper(stringField(pkg, "status"))
			active := true
			for _, hint := range []string{"EXPIRED", "INVALID", "INACTIVE", "DISABLED", "USED_UP", "DEPLETED"} {
				if strings.Contains(status, hint) {
					active = false
				}
			}
			if !active {
				continue
			}
			label := stringField(pkg, "name")
			if label == "" {
				label = "专属积分"
			}
			if labels, ok := pkg["displayLabels"].([]any); ok {
				for _, v := range labels {
					l, _ := v.(map[string]any)
					if l["dimension"] != "title" {
						continue
					}
					translated, _ := l["valueI18n"].(map[string]any)
					name := stringField(translated, "zh-CN", "en-US")
					if name == "" {
						name = stringField(l, "value")
					}
					if name != "" {
						label = name
						break
					}
				}
			}
			snap.Items = append(snap.Items, item(pkg, label, quotaExpiry(pkg["expiresAt"])))
		}
	}
	if !valid {
		return nil, fmt.Errorf("Qoder quota unavailable")
	}
	return snap, nil
}

func quotaExpiry(value any) int64 {
	n := parseExpiry(value)
	if n >= 4102444800 {
		return 0
	}
	return n
}
