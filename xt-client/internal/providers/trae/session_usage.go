package trae

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

const sessionUsageBase = "https://api5-normal.mchost.guru"

var sessionUsageHTTPClient = http.DefaultClient

var sessionUsageAliases = map[string][]string{
	"prompt_tokens":      {"input_token", "input_tokens", "prompt_tokens", "inputToken"},
	"completion_tokens":  {"output_token", "output_tokens", "completion_tokens", "outputToken"},
	"cached_tokens":      {"cache_read_token", "cache_read_tokens", "cached_tokens", "cache_read_input_tokens", "cacheReadToken"},
	"cache_write_tokens": {"cache_write_token", "cache_write_tokens", "cache_creation_input_tokens", "cacheWriteToken"},
}

// SessionUsage reads the commercial bill for one completed chat turn.
// It only exposes numeric token counters; the upstream billing envelope is
// intentionally not returned to the caller.
func SessionUsage(ctx context.Context, cred *providers.Credential, sessionID string) (map[string]any, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("trae session id is required")
	}
	if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
		return nil, fmt.Errorf("trae access token is unavailable")
	}
	body, err := json.Marshal(map[string]any{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sessionUsageBase+"/api/v1/commercial/get_session_usage", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+cred.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Trae/"+appVersion)
	resp, err := sessionUsageHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("trae session usage http %d: %s", resp.StatusCode, redact(string(raw)))
	}
	return parseSessionUsage(raw, sessionID)
}

func parseSessionUsage(raw []byte, sessionID string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("trae session usage invalid json: %w", err)
	}
	if code, exists := payload["code"]; exists {
		if n, ok := numberValue(code); ok && n != 0 {
			return nil, fmt.Errorf("trae session usage code %v", code)
		}
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = payload
	}
	counters := findUsageCounters(data, sessionID, 0)
	if counters == nil {
		return nil, fmt.Errorf("trae session usage has no token counters")
	}
	if _, ok := counters["prompt_tokens"]; !ok {
		return nil, fmt.Errorf("trae session usage has no prompt tokens")
	}
	if _, ok := counters["completion_tokens"]; !ok {
		return nil, fmt.Errorf("trae session usage has no completion tokens")
	}
	usage := map[string]any{}
	for _, key := range []string{"prompt_tokens", "completion_tokens"} {
		if value, ok := counters[key]; ok {
			usage[key] = value
		}
	}
	if prompt, ok := usage["prompt_tokens"].(int64); ok {
		if completion, ok := usage["completion_tokens"].(int64); ok {
			usage["total_tokens"] = prompt + completion
		}
	}
	details := map[string]any{}
	if cached, ok := counters["cached_tokens"]; ok {
		details["cached_tokens"] = cached
	}
	if written, ok := counters["cache_write_tokens"]; ok {
		details["cache_write_tokens"] = written
	}
	if len(details) > 0 {
		usage["prompt_tokens_details"] = details
		if cached, ok := details["cached_tokens"].(int64); ok {
			usage["cached_tokens"] = cached
		}
	}
	return usage, nil
}

func findUsageCounters(value map[string]any, sessionID string, depth int) map[string]int64 {
	if value == nil || depth > 6 {
		return nil
	}
	for _, key := range []string{"user_usage_group_by_session", "userUsageGroupBySession"} {
		if nested, ok := value[key].(map[string]any); ok {
			if counters := findUsageCounters(nested, sessionID, depth+1); counters != nil {
				return counters
			}
		}
	}
	if counters := countersFrom(value); counters != nil {
		return counters
	}
	for _, key := range []string{"usage", "result", "data"} {
		if nested, ok := value[key].(map[string]any); ok {
			if counters := findUsageCounters(nested, sessionID, depth+1); counters != nil {
				return counters
			}
		}
	}
	details, _ := value["usage_group_details"].([]any)
	if len(details) == 0 {
		details, _ = value["usageGroupDetails"].([]any)
	}
	if len(details) == 0 {
		return nil
	}
	sums := map[string]int64{}
	for _, item := range details {
		row, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		rowSession := stringValue(row["session_id"])
		if rowSession == "" {
			rowSession = stringValue(row["sessionId"])
		}
		if rowSession != "" && rowSession != sessionID {
			return nil
		}
		part := countersFrom(row)
		if part == nil {
			return nil
		}
		for key, n := range part {
			sums[key] += n
		}
	}
	if len(sums) == 0 {
		return nil
	}
	return sums
}

func countersFrom(value map[string]any) map[string]int64 {
	if value == nil {
		return nil
	}
	sources := []map[string]any{value}
	for _, key := range []string{"extra_info", "extraInfo"} {
		if extra, ok := value[key].(map[string]any); ok {
			sources = append([]map[string]any{extra}, sources...)
		}
	}
	keys := map[string]bool{}
	for _, names := range sessionUsageAliases {
		for _, key := range names {
			keys[key] = true
		}
	}
	present := false
	for _, source := range sources {
		for key := range source {
			if keys[key] {
				present = true
				break
			}
		}
	}
	if !present {
		return nil
	}
	result := map[string]int64{}
	for field, names := range sessionUsageAliases {
		for _, source := range sources {
			found := false
			for _, key := range names {
				if raw, ok := source[key]; ok {
					if n, valid := numberValue(raw); valid && n >= 0 {
						result[field] = n
						found = true
					}
					break
				}
			}
			if found {
				break
			}
		}
	}
	return result
}

func numberValue(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || math.Trunc(v) != v {
			return 0, false
		}
		return int64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return v, true
	case int:
		if v < 0 {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	case string:
		n, err := parseNonNegativeInt(v)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func parseNonNegativeInt(value string) (int64, error) {
	var f float64
	if _, err := fmt.Sscan(strings.TrimSpace(value), &f); err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || math.Trunc(f) != f {
		return 0, fmt.Errorf("not a non-negative integer")
	}
	return int64(f), nil
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

// SessionUsageMatch returns the bill counters only when this turn's input
// and output totals exactly match the SSE usage. Billing can settle after
// the stream, so the caller decides how long to retry.
func SessionUsageMatch(usage map[string]any, bill map[string]any) (int64, bool) {
	prompt, okPrompt := usage["prompt_tokens"].(int64)
	completion, okCompletion := usage["completion_tokens"].(int64)
	billPrompt, billPromptOK := bill["prompt_tokens"].(int64)
	billCompletion, billCompletionOK := bill["completion_tokens"].(int64)
	if !okPrompt || !okCompletion || !billPromptOK || !billCompletionOK ||
		prompt <= 0 || billPrompt <= 0 || billPrompt != prompt || billCompletion != completion {
		return 0, false
	}
	details, _ := bill["prompt_tokens_details"].(map[string]any)
	cached, ok := details["cached_tokens"].(int64)
	if !ok || cached < 0 || cached > prompt {
		return 0, false
	}
	return cached, true
}

func SessionUsageLookupTimeout() time.Duration {
	return 3 * time.Second
}
