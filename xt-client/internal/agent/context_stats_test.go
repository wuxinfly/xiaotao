package agent

import (
	"strings"
	"testing"
)

func TestCalculateContextBreakdownItemization(t *testing.T) {
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "run_command",
				"description": "Run a command",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"command": map[string]any{"type": "string"}},
				},
			},
		},
	}

	messages := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "[引用历史对话：会话 A]\n历史讨论细节"},
		{"role": "user", "content": "帮我运行测试"},
		{
			"role":    "assistant",
			"content": "正在运行命令",
			"tool_calls": []map[string]any{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "run_command",
						"arguments": `{"command":"go test ./..."}`,
					},
				},
			},
		},
		{
			"role":         "tool",
			"tool_call_id": "call_1",
			"name":         "run_command",
			"content":      "PASS: all tests passed",
		},
		{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "再看这张图"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + strings.Repeat("B", 1000)}},
			},
		},
	}

	breakdown := calculateContextBreakdown(messages, tools)

	if breakdown.SystemTokens <= 0 {
		t.Fatalf("expected SystemTokens > 0, got %d", breakdown.SystemTokens)
	}
	if breakdown.ToolsTokens <= 0 {
		t.Fatalf("expected ToolsTokens > 0, got %d", breakdown.ToolsTokens)
	}
	if breakdown.ReferenceTokens <= 0 {
		t.Fatalf("expected ReferenceTokens > 0, got %d", breakdown.ReferenceTokens)
	}
	if breakdown.MessagesTokens <= 0 {
		t.Fatalf("expected MessagesTokens > 0, got %d", breakdown.MessagesTokens)
	}
	if breakdown.ToolResultsTokens <= 0 {
		t.Fatalf("expected ToolResultsTokens > 0, got %d", breakdown.ToolResultsTokens)
	}
	if !breakdown.HasImages {
		t.Fatalf("expected HasImages to be true")
	}
	if breakdown.ImageTokens != estimatedImageTokens {
		t.Fatalf("expected ImageTokens=%d, got %d", estimatedImageTokens, breakdown.ImageTokens)
	}

	sum := breakdown.SystemTokens + breakdown.ToolsTokens + breakdown.MessagesTokens +
		breakdown.ToolResultsTokens + breakdown.ReferenceTokens + breakdown.ImageTokens
	if breakdown.TotalTokens != sum {
		t.Fatalf("breakdown sum mismatch: sum=%d TotalTokens=%d", sum, breakdown.TotalTokens)
	}
}

func TestCalculateContextBreakdownEmpty(t *testing.T) {
	breakdown := calculateContextBreakdown(nil, nil)
	if breakdown.TotalTokens != 0 || breakdown.HasImages {
		t.Fatalf("expected all zeros for empty messages and tools, got %+v", breakdown)
	}
}

func TestRecordCompletionStatsHandlesCacheHits(t *testing.T) {
	stats := &RunStats{}
	var outputSec float64

	// First completion without cache hits
	promptTokens := int64(100)
	res1 := &completion{
		OutputTokens: 20,
		InputTokens:  &promptTokens,
		CachedTokens: nil,
	}
	recordCompletionStats(stats, res1, &outputSec)
	if stats.InputTokens != 100 || !stats.InputKnown {
		t.Fatalf("expected InputTokens=100 InputKnown=true, got %d %v", stats.InputTokens, stats.InputKnown)
	}
	if stats.CacheKnown || stats.CachedTokens != 0 {
		t.Fatalf("expected CacheKnown=false CachedTokens=0, got %v %d", stats.CacheKnown, stats.CachedTokens)
	}

	// Second completion with cache hits
	cached := int64(80)
	res2 := &completion{
		OutputTokens: 10,
		InputTokens:  &promptTokens,
		CachedTokens: &cached,
	}
	recordCompletionStats(stats, res2, &outputSec)
	if stats.InputTokens != 200 {
		t.Fatalf("expected InputTokens=200, got %d", stats.InputTokens)
	}
	if !stats.CacheKnown || stats.CachedTokens != 80 {
		t.Fatalf("expected CacheKnown=true CachedTokens=80, got %v %d", stats.CacheKnown, stats.CachedTokens)
	}
}
