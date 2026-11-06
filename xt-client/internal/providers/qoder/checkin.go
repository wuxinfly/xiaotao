package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

const campaignsPath = "/sash/api/v1/me/campaigns"

type campaign struct {
	ID      string `json:"campaignId"`
	Action  string `json:"actionType"`
	Status  string `json:"claimStatus"`
	Start   int64  `json:"startAt"`
	End     int64  `json:"endAt"`
	Benefit struct {
		Kind   string  `json:"kind"`
		Amount float64 `json:"amount"`
	} `json:"benefit"`
}

// CheckinClient uses the desktop activity API, not the COSY inference protocol.
type CheckinClient struct{ HTTP *http.Client }

type CheckinResult struct {
	OK           bool
	CheckedIn    bool
	ExtraCredits float64
	Message      string
}

func (c CheckinClient) request(ctx context.Context, cred *providers.Credential, method, suffix string, result any) error {
	if cred == nil || cred.AccessToken == "" {
		return fmt.Errorf("Qoder 签到缺少登录凭据")
	}
	reg, ok := Regions[cred.Region]
	if !ok {
		return fmt.Errorf("unsupported Qoder region")
	}
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewBufferString("{}")
	}
	req, err := http.NewRequestWithContext(ctx, method, reg.OpenAPIBase+campaignsPath+suffix, body)
	if err != nil {
		return fmt.Errorf("invalid Qoder activity request")
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Qoder")
	req.Header.Set("Cosy-ClientType", "10")
	req.Header.Set("Cosy-Version", "0.4.3")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Qoder 活动请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &providers.UpstreamError{Status: resp.StatusCode, Message: fmt.Sprintf("Qoder 活动请求失败 (HTTP %d)", resp.StatusCode)}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(result) != nil {
		return fmt.Errorf("invalid Qoder activity response")
	}
	return nil
}

func (c CheckinClient) list(ctx context.Context, cred *providers.Credential) ([]campaign, error) {
	var data struct {
		Campaigns *[]campaign `json:"campaigns"`
	}
	if err := c.request(ctx, cred, http.MethodGet, "?forceRefresh=true", &data); err != nil {
		return nil, err
	}
	if data.Campaigns == nil {
		return nil, fmt.Errorf("Qoder activity response missing campaigns")
	}
	return *data.Campaigns, nil
}

func (a campaign) current(now int64) bool {
	return a.ID != "" && a.Action == "CLAIM_BENEFIT" && a.Benefit.Kind == "CREDITS" && (a.Start == 0 || now >= a.Start) && (a.End == 0 || now < a.End)
}

func campaignStatus(rows []campaign) storage.CheckinStatus {
	now := time.Now().Unix()
	st := storage.CheckinStatus{Supported: true, FetchedAt: now, Message: "暂无可领取活动"}
	seen := map[string]bool{}
	for _, a := range rows {
		if !a.current(now) || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		switch a.Status {
		case "CLAIMABLE":
			st.Active = true
			st.Message = "有活动奖励可领取"
			if a.Benefit.Kind == "CREDITS" && a.Benefit.Amount > 0 {
				st.DailyCredit += a.Benefit.Amount
			}
		case "CLAIMED":
			st.CheckedIn = true
			st.Active = true
		}
	}
	// A claimed promotion must not hide another currently claimable reward.
	for _, a := range rows {
		if a.current(now) && a.Status == "CLAIMABLE" {
			st.CheckedIn = false
			return st
		}
	}
	if st.CheckedIn {
		st.Message = "当前活动奖励已领取"
	}
	return st
}

func (c CheckinClient) Status(ctx context.Context, cred *providers.Credential) (storage.CheckinStatus, error) {
	rows, err := c.list(ctx, cred)
	if err != nil {
		return storage.CheckinStatus{}, err
	}
	return campaignStatus(rows), nil
}

func (c CheckinClient) Claim(ctx context.Context, cred *providers.Credential) (CheckinResult, error) {
	rows, err := c.list(ctx, cred)
	if err != nil {
		return CheckinResult{}, err
	}
	st := campaignStatus(rows)
	out := CheckinResult{CheckedIn: st.CheckedIn, Message: st.Message}
	seen := map[string]bool{}
	for _, a := range rows {
		if !a.current(time.Now().Unix()) || a.Status != "CLAIMABLE" || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		var result struct {
			Status   string `json:"status"`
			Replayed bool   `json:"replayed"`
			Benefit  struct {
				Kind   string  `json:"kind"`
				Amount float64 `json:"amount"`
			} `json:"benefit"`
		}
		if err := c.request(ctx, cred, http.MethodPost, "/"+url.PathEscape(a.ID)+"/claim", &result); err != nil {
			return out, err
		}
		if result.Status != "CLAIMED" {
			return out, fmt.Errorf("Qoder 活动奖励未确认领取")
		}
		out.CheckedIn = true
		if !result.Replayed {
			out.OK = true
			if result.Benefit.Kind == "" {
				result.Benefit = a.Benefit
			}
			if result.Benefit.Kind == "CREDITS" && result.Benefit.Amount > 0 && !math.IsInf(result.Benefit.Amount, 0) {
				out.ExtraCredits += result.Benefit.Amount
			}
		}
	}
	if out.OK {
		out.Message = "活动奖励领取成功"
	} else if out.CheckedIn {
		out.Message = "当前活动奖励已领取"
	}
	return out, nil
}
