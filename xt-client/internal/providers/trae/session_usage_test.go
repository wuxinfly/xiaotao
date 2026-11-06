package trae

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/providers"
)

func TestParseSessionUsageReadsNestedCacheCounters(t *testing.T) {
	raw := []byte(`{"code":0,"data":{"user_usage_group_by_session":{` +
		`"input_token":22857,"output_token":211,"cache_read_token":22784,"cache_write_token":0}}}`)
	usage, err := parseSessionUsage(raw, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if usage["prompt_tokens"] != int64(22857) || usage["completion_tokens"] != int64(211) {
		t.Fatalf("unexpected token counters: %#v", usage)
	}
	details, ok := usage["prompt_tokens_details"].(map[string]any)
	if !ok || details["cached_tokens"] != int64(22784) {
		t.Fatalf("unexpected cache details: %#v", usage)
	}
}

func TestParseSessionUsageReadsExtraInfoAndCamelCase(t *testing.T) {
	raw := []byte(`{"data":{"usageGroupDetails":[` +
		`{"sessionId":"turn-1","extraInfo":{"inputToken":"100","outputToken":"10","cacheReadToken":"80"}}]}}`)
	usage, err := parseSessionUsage(raw, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if usage["prompt_tokens"] != int64(100) || usage["completion_tokens"] != int64(10) {
		t.Fatalf("unexpected token counters: %#v", usage)
	}
	details := usage["prompt_tokens_details"].(map[string]any)
	if details["cached_tokens"] != int64(80) {
		t.Fatalf("unexpected cache details: %#v", usage)
	}
}

func TestParseSessionUsageRejectsMismatchedDetailTurn(t *testing.T) {
	raw := []byte(`{"data":{"usage_group_details":[` +
		`{"session_id":"other-turn","extra_info":{"input_token":100,"output_token":10,"cache_read_token":80}}]}}`)
	if _, err := parseSessionUsage(raw, "turn-1"); err == nil {
		t.Fatal("expected mismatched detail turn to be rejected")
	}
}

func TestSessionUsageMatchRequiresExactTotals(t *testing.T) {
	usage := map[string]any{"prompt_tokens": int64(100), "completion_tokens": int64(10)}
	bill := map[string]any{
		"prompt_tokens": int64(100), "completion_tokens": int64(10),
		"prompt_tokens_details": map[string]any{"cached_tokens": int64(80)},
	}
	if cached, ok := SessionUsageMatch(usage, bill); !ok || cached != 80 {
		t.Fatalf("expected 80 cached tokens, got %d (ok=%v)", cached, ok)
	}
	bill["completion_tokens"] = int64(11)
	if _, ok := SessionUsageMatch(usage, bill); ok {
		t.Fatal("mismatched completion must not be accepted")
	}
}

func TestSessionUsageMatchRejectsInvalidCache(t *testing.T) {
	usage := map[string]any{"prompt_tokens": int64(100), "completion_tokens": int64(10)}
	for _, cached := range []int64{-1, 101} {
		bill := map[string]any{
			"prompt_tokens": int64(100), "completion_tokens": int64(10),
			"prompt_tokens_details": map[string]any{"cached_tokens": cached},
		}
		if _, ok := SessionUsageMatch(usage, bill); ok {
			t.Fatalf("invalid cache %d must not be accepted", cached)
		}
	}
}

func TestSessionUsageMatchRequiresPromptAndCompletion(t *testing.T) {
	usage := map[string]any{"prompt_tokens": int64(100), "completion_tokens": int64(10)}
	bill := map[string]any{
		"prompt_tokens_details": map[string]any{"cached_tokens": int64(80)},
	}
	if _, ok := SessionUsageMatch(usage, bill); ok {
		t.Fatal("bill without both prompt and completion totals must not match")
	}
}

func TestSessionUsageMatchRejectsZeroTotals(t *testing.T) {
	usage := map[string]any{"prompt_tokens": int64(100), "completion_tokens": int64(10)}
	bill := map[string]any{
		"prompt_tokens": int64(0), "completion_tokens": int64(0),
		"prompt_tokens_details": map[string]any{"cached_tokens": int64(0)},
	}
	if _, ok := SessionUsageMatch(usage, bill); ok {
		t.Fatal("a missing session bill must not match a real request")
	}
}

func TestParseSessionUsageRejectsCacheOnlyPayload(t *testing.T) {
	raw := []byte(`{"data":{"extra_info":{"cache_read_token":80}}}`)
	if _, err := parseSessionUsage(raw, "turn-1"); err == nil {
		t.Fatal("cache-only bill must not be treated as a complete usage match")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSessionUsageSendsTurnAndAuthorization(t *testing.T) {
	previous := sessionUsageHTTPClient
	defer func() { sessionUsageHTTPClient = previous }()
	sessionUsageHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api5-normal.mchost.guru/api/v1/commercial/get_session_usage" {
			t.Fatalf("unexpected URL %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Cloud-IDE-JWT token-1" {
			t.Fatalf("unexpected Authorization %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"session_id":"turn-1"`) {
			t.Fatalf("unexpected body %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"code":0,"data":{"user_usage_group_by_session":{"input_token":100,"output_token":10,"cache_read_token":80}}}`)),
		}, nil
	})}
	usage, err := SessionUsage(context.Background(), &providers.Credential{AccessToken: "token-1"}, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if usage["prompt_tokens"] != int64(100) {
		t.Fatalf("unexpected usage %#v", usage)
	}
}
