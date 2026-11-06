package proxy

import (
	"context"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
)

const traeUsageLookupBudget = 3 * time.Second

func (s *Service) enrichTraeUsage(ctx context.Context, cred *providers.Credential, sessionID string, usage map[string]any) map[string]any {
	if s == nil || s.traeUsage == nil || cred == nil || sessionID == "" || len(usage) == 0 {
		return usage
	}
	prompt, promptOK := usage["prompt_tokens"].(int64)
	_, completionOK := usage["completion_tokens"].(int64)
	if !promptOK || !completionOK || prompt <= 0 {
		return usage
	}
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if cached, ok := details["cached_tokens"].(int64); ok && cached > 0 {
			return usage
		}
	}
	lookupCtx, cancel := context.WithTimeout(ctx, traeUsageLookupBudget)
	defer cancel()
	for _, delay := range []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond} {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-lookupCtx.Done():
				timer.Stop()
				return usage
			case <-timer.C:
			}
		}
		bill, err := s.traeUsage(lookupCtx, cred, sessionID)
		if err != nil || bill == nil {
			continue
		}
		cached, ok := trae.SessionUsageMatch(usage, bill)
		if !ok {
			continue
		}
		recovered := cloneUsage(usage)
		details, _ := recovered["prompt_tokens_details"].(map[string]any)
		if details == nil {
			details = map[string]any{}
		} else {
			details = cloneUsage(details)
		}
		details["cached_tokens"] = cached
		if written, ok := billDetails(bill)["cache_write_tokens"].(int64); ok {
			details["cache_write_tokens"] = written
		}
		recovered["prompt_tokens_details"] = details
		recovered["cached_tokens"] = cached
		return recovered
	}
	return usage
}

func traeUsageSessionID(upstreamID, requestID string) string {
	if strings.TrimSpace(upstreamID) != "" {
		return upstreamID
	}
	return requestID
}

func billDetails(bill map[string]any) map[string]any {
	details, _ := bill["prompt_tokens_details"].(map[string]any)
	return details
}

func cloneUsage(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		if nested, ok := value.(map[string]any); ok {
			dst[key] = cloneUsage(nested)
			continue
		}
		dst[key] = value
	}
	return dst
}
