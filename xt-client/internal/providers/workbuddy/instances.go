package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"xiaotao/xt-client/internal/providers"
)

func FetchPackageExpiry(ctx context.Context, baseURL string, cred *providers.Credential) (map[string]int64, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	out := map[string]int64{}
	allEnds := map[string][]int64{}
	offset := 0
	for range 10 {
		raw, err := post(ctx, baseURL+"/billing/meter/get-user-resource", map[string]any{"Offset": offset, "Limit": 100}, headersFor(cred))
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Code any `json:"code"`
			Data struct {
				Response struct {
					Data struct {
						TotalCount int                `json:"TotalCount"`
						Accounts   []resourceInstance `json:"Accounts"`
					} `json:"Data"`
				} `json:"Response"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("invalid resource instance response")
		}
		if code := number(envelope.Code); code != 0 {
			return nil, fmt.Errorf("resource code %d", code)
		}
		for _, item := range envelope.Data.Response.Data.Accounts {
			if item.PackageCode == "" {
				continue
			}
			if end, ok := parseUpstreamTime(item.CycleEndTime); ok {
				allEnds[item.PackageCode] = append(allEnds[item.PackageCode], end)
			}
		}
		offset += len(envelope.Data.Response.Data.Accounts)
		if len(envelope.Data.Response.Data.Accounts) == 0 || offset >= envelope.Data.Response.Data.TotalCount {
			break
		}
	}
	for code, ends := range allEnds {
		active := []int64{}
		for _, end := range ends {
			if end > time.Now().Unix() {
				active = append(active, end)
			}
		}
		if len(active) > 0 {
			out[code] = maxTime(active)
			continue
		}
		out[code] = maxTime(ends)
	}
	return out, nil
}

func parseUpstreamTime(value string) (int64, bool) {
	if value == "" {
		return 0, false
	}
	layouts := []string{"2006-01-02 15:04:05", time.RFC3339}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.Unix(), true
		}
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return n, true
	}
	return 0, false
}

func maxTime(values []int64) int64 {
	var max int64
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}
