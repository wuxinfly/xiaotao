package agent

import (
	"strings"
	"testing"
)

// A pasted image used to be counted as its raw base64 payload, which made the
// context meter report hundreds of thousands of tokens for a single picture.
func TestEstimateContextTokensIgnoresImagePayloadSize(t *testing.T) {
	small := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": []map[string]any{
			{"type": "text", "text": "看这张图"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + strings.Repeat("A", 64)}},
		}},
	}
	huge := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": []map[string]any{
			{"type": "text", "text": "看这张图"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + strings.Repeat("A", 4_000_000)}},
		}},
	}

	smallTokens := estimateContextTokens(small)
	hugeTokens := estimateContextTokens(huge)
	if smallTokens <= 0 {
		t.Fatalf("expected a positive estimate, got %d", smallTokens)
	}
	if hugeTokens != smallTokens {
		t.Fatalf("image payload size must not change the estimate: small=%d huge=%d", smallTokens, hugeTokens)
	}
	if hugeTokens > 10_000 {
		t.Fatalf("one image must not blow up the estimate, got %d tokens", hugeTokens)
	}
}

// Text still counts, and the multipart shape must be handled for both the
// in-memory []map[string]any form and the decoded []any form.
func TestEstimateContentBytesCountsTextOnly(t *testing.T) {
	text := "一段普通文字"
	multipart := []map[string]any{
		{"type": "text", "text": text},
		{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}},
	}
	got := estimateContentBytes(multipart)
	if got < int64(len(text)) {
		t.Fatalf("text bytes missing: got %d want >= %d", got, len(text))
	}
	if got > int64(len(text))+estimatedImageTokens*3 {
		t.Fatalf("image payload counted as text: got %d", got)
	}

	decoded := []any{
		map[string]any{"type": "text", "text": text},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}},
	}
	if estimateContentBytes(decoded) != got {
		t.Fatalf("decoded []any must match []map[string]any: got %d want %d", estimateContentBytes(decoded), got)
	}
}
