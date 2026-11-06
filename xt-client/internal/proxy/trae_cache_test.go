package proxy

import (
	"context"
	"testing"

	"xiaotao/xt-client/internal/providers"
)

func TestEnrichTraeUsageReplacesZeroCacheOnExactMatch(t *testing.T) {
	svc := New(nil, nil, "key")
	svc.traeUsage = func(_ context.Context, _ *providers.Credential, sessionID string) (map[string]any, error) {
		if sessionID != "turn-1" {
			t.Fatalf("unexpected session id %q", sessionID)
		}
		return map[string]any{
			"prompt_tokens": int64(100), "completion_tokens": int64(10),
			"prompt_tokens_details": map[string]any{"cached_tokens": int64(80)},
		}, nil
	}
	usage := map[string]any{
		"prompt_tokens": int64(100), "completion_tokens": int64(10),
		"cached_tokens":         int64(0),
		"prompt_tokens_details": map[string]any{"cached_tokens": int64(0)},
	}
	got := svc.enrichTraeUsage(context.Background(), &providers.Credential{}, "turn-1", usage)
	if got["cached_tokens"] != int64(80) {
		t.Fatalf("cached_tokens = %v, want 80", got["cached_tokens"])
	}
	details := got["prompt_tokens_details"].(map[string]any)
	if details["cached_tokens"] != int64(80) {
		t.Fatalf("details.cached_tokens = %v, want 80", details["cached_tokens"])
	}
	if usage["cached_tokens"] != int64(0) {
		t.Fatal("enrichment must not mutate the original usage map")
	}
}

func TestEnrichTraeUsageKeepsOriginalOnTokenMismatch(t *testing.T) {
	svc := New(nil, nil, "key")
	svc.traeUsage = func(context.Context, *providers.Credential, string) (map[string]any, error) {
		return map[string]any{
			"prompt_tokens": int64(101), "completion_tokens": int64(10),
			"prompt_tokens_details": map[string]any{"cached_tokens": int64(80)},
		}, nil
	}
	usage := map[string]any{"prompt_tokens": int64(100), "completion_tokens": int64(10), "cached_tokens": int64(0)}
	got := svc.enrichTraeUsage(context.Background(), &providers.Credential{}, "turn-1", usage)
	if got["cached_tokens"] != int64(0) {
		t.Fatalf("mismatched bill must not replace cache, got %v", got["cached_tokens"])
	}
}

func TestEnrichTraeUsageDoesNotOverrideNonZeroCache(t *testing.T) {
	called := false
	svc := New(nil, nil, "key")
	svc.traeUsage = func(context.Context, *providers.Credential, string) (map[string]any, error) {
		called = true
		return nil, nil
	}
	usage := map[string]any{
		"prompt_tokens": int64(100), "completion_tokens": int64(10),
		"prompt_tokens_details": map[string]any{"cached_tokens": int64(40)},
	}
	got := svc.enrichTraeUsage(context.Background(), &providers.Credential{}, "turn-1", usage)
	if called {
		t.Fatal("must not query billing when SSE already reported a cache hit")
	}
	if got["prompt_tokens_details"].(map[string]any)["cached_tokens"] != int64(40) {
		t.Fatalf("unexpected cache: %#v", got)
	}
}

func TestTraeUsageSessionIDFallsBackToRequestID(t *testing.T) {
	if got := traeUsageSessionID("", "request-1"); got != "request-1" {
		t.Fatalf("got %q", got)
	}
	if got := traeUsageSessionID("upstream-1", "request-1"); got != "upstream-1" {
		t.Fatalf("got %q", got)
	}
}
