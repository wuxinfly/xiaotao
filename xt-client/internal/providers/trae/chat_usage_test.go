package trae

import (
	"encoding/json"
	"testing"
)

func usageEvent(t *testing.T, raw string) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func cachedOf(t *testing.T, usage map[string]any) (int64, bool) {
	t.Helper()
	v, ok := usage["cached_tokens"].(int64)
	return v, ok
}

func TestNormalizeTokenUsageReadsTraeCacheFields(t *testing.T) {
	// Shape observed on the live Trae CN upstream: Anthropic-style cache
	// counters instead of OpenAI cached_tokens.
	data := usageEvent(t, `{"prompt_tokens":100000,"completion_tokens":512,"total_tokens":100512,`+
		`"cache_read_input_tokens":68928,"cache_creation_input_tokens":0,"cluster":"normal_context"}`)
	usage := normalizeTokenUsage(data)

	if got, ok := cachedOf(t, usage); !ok || got != 68928 {
		t.Fatalf("cached_tokens = %v (present=%v), want 68928", got, ok)
	}
	details, ok := usage["prompt_tokens_details"].(map[string]any)
	if !ok {
		t.Fatal("prompt_tokens_details missing")
	}
	if details["cached_tokens"] != int64(68928) {
		t.Fatalf("prompt_tokens_details.cached_tokens = %v, want 68928", details["cached_tokens"])
	}
	if usage["cache_creation_input_tokens"] != int64(0) {
		t.Fatalf("cache_creation_input_tokens = %v, want 0", usage["cache_creation_input_tokens"])
	}
	if usage["prompt_tokens"] != int64(100000) || usage["completion_tokens"] != int64(512) {
		t.Fatalf("base counters lost: %v", usage)
	}
}

func TestNormalizeTokenUsageReportsZeroCache(t *testing.T) {
	usage := normalizeTokenUsage(usageEvent(t, `{"prompt_tokens":39046,"completion_tokens":33,"cache_read_input_tokens":0}`))
	if got, ok := cachedOf(t, usage); !ok || got != 0 {
		t.Fatalf("explicit zero must be recorded, got %v (present=%v)", got, ok)
	}
}

func TestNormalizeTokenUsagePrefersPromptTokensDetails(t *testing.T) {
	usage := normalizeTokenUsage(usageEvent(t, `{"prompt_tokens":10,"cached_tokens":3,`+
		`"prompt_tokens_details":{"cached_tokens":7},"cache_read_input_tokens":5}`))
	if got, _ := cachedOf(t, usage); got != 7 {
		t.Fatalf("cached_tokens = %v, want 7", got)
	}
}

func TestNormalizeTokenUsageLegacyFallbacks(t *testing.T) {
	if got, ok := cachedOf(t, normalizeTokenUsage(usageEvent(t, `{"prompt_tokens":10,"prompt_cache_hit_tokens":4}`))); !ok || got != 4 {
		t.Fatalf("prompt_cache_hit_tokens fallback = %v (present=%v), want 4", got, ok)
	}
	if got, ok := cachedOf(t, normalizeTokenUsage(usageEvent(t, `{"prompt_tokens":10,"cache_tokens":6}`))); !ok || got != 6 {
		t.Fatalf("cache_tokens fallback = %v (present=%v), want 6", got, ok)
	}
}

func TestNormalizeTokenUsageWithoutCacheFieldsKeepsUnknown(t *testing.T) {
	usage := normalizeTokenUsage(usageEvent(t, `{"prompt_tokens":10,"completion_tokens":2}`))
	if _, ok := cachedOf(t, usage); ok {
		t.Fatal("cached_tokens must stay absent when upstream reports no cache counter")
	}
	if _, ok := usage["prompt_tokens_details"]; ok {
		t.Fatal("prompt_tokens_details must stay absent when upstream reports no cache counter")
	}
}
