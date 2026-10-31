package trae

import (
	"fmt"
	"strconv"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

func FetchQuota(cred *providers.Credential, machineID, deviceID string) (*storage.QuotaSnapshot, error) {
	r := RegionOf(cred.Region)
	body := []byte("{}")
	head := headers(cred, machineID, deviceID)
	head["X-User-Region"] = "CN"
	var out struct {
		IsDollar bool `json:"is_dollar_usage_billing"`
		IsCredit bool `json:"is_credits_billing"`
		Usage    struct {
			Total    *float64 `json:"total_amount"`
			Consumed *float64 `json:"consumed_amount"`
		} `json:"usage_summary"`
		Packs []struct {
			Hide bool   `json:"is_hide"`
			Desc string `json:"display_desc"`
			Base struct {
				EndTime int64          `json:"end_time"`
				EntID   string         `json:"entitlement_id"`
				Quota   map[string]any `json:"quota"`
			} `json:"entitlement_base_info"`
			Usage map[string]any `json:"usage"`
		} `json:"user_entitlement_pack_list"`
	}
	target := fmt.Sprintf("%s/trae/api/%s/pay/ide_user_ent_usage", r.UGBase, r.UGVersion)
	if err := postJSON(target, body, head, &out); err != nil {
		return nil, err
	}
	unit := "credit"
	if out.IsDollar {
		unit = "dollar"
	}
	snap := &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{}, FetchedAt: now()}
	if out.Usage.Total != nil {
		used := float64(0)
		if out.Usage.Consumed != nil {
			used = *out.Usage.Consumed
		}
		snap.Items = append(snap.Items, storage.QuotaItem{
			Label: "总额度", Used: &used, Total: out.Usage.Total, Unit: unit,
		})
	}
	seen := map[string]bool{}
	for _, p := range out.Packs {
		if p.Hide || p.Base.EndTime == 0 {
			continue
		}
		limit := firstFloat(p.Base.Quota, []string{"credits_limit", "total_amount", "basic_usage_limit"})
		used := firstFloat(p.Usage, []string{"credits_amount", "consumed_amount", "basic_usage_amount"})
		if limit == nil {
			continue
		}
		usedVal := 0.0
		if used != nil {
			usedVal = *used
		}
		label := p.Desc
		if label == "" {
			label = "权益包"
		}
		key := fmt.Sprintf("%s|%s|%d", label, p.Base.EntID, p.Base.EndTime)
		if seen[key] {
			continue
		}
		seen[key] = true
		snap.Items = append(snap.Items, storage.QuotaItem{
			Label: label, Used: &usedVal, Total: limit, Unit: unit, ExpireAt: p.Base.EndTime,
		})
	}
	return snap, nil
}

func firstFloat(m map[string]any, keys []string) *float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return &n
			case string:
				if f, err := strconv.ParseFloat(n, 64); err == nil {
					return &f
				}
			}
		}
	}
	return nil
}
