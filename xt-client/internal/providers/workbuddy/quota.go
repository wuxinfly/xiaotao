package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type resourceSummaryPackage struct {
	PackageCode         string          `json:"PackageCode"`
	CycleUsedCapacity   json.RawMessage `json:"CycleUsedCapacity"`
	CycleTotalCapacity  json.RawMessage `json:"CycleTotalCapacity"`
	CycleRemainCapacity json.RawMessage `json:"CycleRemainCapacity"`
	CapacityUnit        string          `json:"CapacityUnit"`
}

type resourceSummary struct {
	SubscriptionPackageCode string                   `json:"SubscriptionPackageCode"`
	IsPaidUser              bool                     `json:"IsPaidUser"`
	Packages                []resourceSummaryPackage `json:"Packages"`
}

type resourceInstance struct {
	PackageCode  string `json:"PackageCode"`
	CycleEndTime string `json:"CycleEndTime"`
	Status       int    `json:"Status"`
}

func FetchQuota(ctx context.Context, baseURL string, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if cred == nil || cred.AccessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}
	raw, err := post(ctx, baseURL+"/billing/meter/get-user-resource-summary", map[string]any{}, headersFor(cred))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Code any             `json:"code"`
		Msg  string          `json:"msg"`
		Data resourceSummary `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid quota response")
	}
	if code := number(envelope.Code); code != 0 {
		return nil, fmt.Errorf("quota code %d: %s", code, envelope.Msg)
	}
	now := time.Now().Unix()
	snap := &storage.QuotaSnapshot{Status: "ok", Items: []storage.QuotaItem{}, FetchedAt: now}
	for _, p := range envelope.Data.Packages {
		total := optionalNumber(p.CycleTotalCapacity)
		remaining := optionalNumber(p.CycleRemainCapacity)
		if total == nil {
			continue
		}
		used := optionalNumber(p.CycleUsedCapacity)
		label := "资源包"
		if p.PackageCode == envelope.Data.SubscriptionPackageCode {
			label = "订阅套餐"
		}
		item := storage.QuotaItem{Label: label, Total: total, Unit: "credit"}
		if used != nil {
			item.Used = used
		}
		if remaining != nil {
			item.Remaining = remaining
		} else if used != nil {
			derived := *total - *used
			item.Remaining = &derived
		}
		snap.Items = append(snap.Items, item)
	}
	if len(snap.Items) == 0 {
		return nil, fmt.Errorf("quota response has no packages")
	}
	expiry, expiryErr := FetchPackageExpiry(ctx, baseURL, cred)
	if expiryErr == nil {
		for i := range snap.Items {
			code := packageCode(envelope.Data.Packages, i)
			if code != "" && expiry[code] > 0 {
				snap.Items[i].ExpireAt = expiry[code]
			}
		}
	}
	return snap, nil
}

func packageCode(packages []resourceSummaryPackage, index int) string {
	if index < len(packages) {
		return packages[index].PackageCode
	}
	return ""
}

func optionalNumber(raw json.RawMessage) *float64 {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	switch n := value.(type) {
	case float64:
		out := n
		return &out
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return &parsed
		}
	}
	return nil
}
