package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFindCompactionCutoffBoundaries(t *testing.T) {
	// Not enough messages
	short := []Message{
		{ID: "u1", Role: "user", Content: "hello"},
		{ID: "a1", Role: "assistant", Content: "world"},
	}
	if cutoff := findCompactionCutoff(short, 2); cutoff != -1 {
		t.Fatalf("expected cutoff=-1 for short history, got %d", cutoff)
	}

	// 3 user turns: u1, u2, u3. Preserving 2 turns means keeping u2 and u3.
	// u1 turn should be candidate for cutoff.
	messages := []Message{
		{ID: "u1", Role: "user", Content: "Turn 1 question"},
		{ID: "a1", Role: "assistant", Content: "Turn 1 answer", ToolCalls: []map[string]any{
			{"id": "c1", "type": "function", "function": map[string]any{"name": "run_command"}},
		}},
		{ID: "t1", Role: "tool", ToolCallID: "c1", ToolName: "run_command", Content: "out 1"},
		{ID: "a1_f", Role: "assistant", Content: "Turn 1 finished"},

		// Preserved turn 2
		{ID: "u2", Role: "user", Content: "Turn 2 question"},
		{ID: "a2", Role: "assistant", Content: "Turn 2 answer"},

		// Preserved turn 3
		{ID: "u3", Role: "user", Content: "Turn 3 question"},
	}

	cutoff := findCompactionCutoff(messages, 2)
	if cutoff < 0 {
		t.Fatalf("expected positive cutoff, got %d", cutoff)
	}

	// The cutoff must be at or after a1_f (index 3), and strictly before u2 (index 4)
	if cutoff != 3 {
		t.Fatalf("expected cutoff at index 3 (a1_f), got %d (id: %s)", cutoff, messages[cutoff].ID)
	}
	if messages[cutoff].ID != "a1_f" {
		t.Fatalf("cutoff message must be a1_f, got %s", messages[cutoff].ID)
	}
}

func TestShouldAutoCompactThreshold(t *testing.T) {
	messages := make([]Message, 0, 10)
	for i := 1; i <= 4; i++ {
		messages = append(messages,
			Message{ID: fmt.Sprintf("u%d", i), Role: "user", Content: fmt.Sprintf("user %d", i)},
			Message{ID: fmt.Sprintf("a%d", i), Role: "assistant", Content: fmt.Sprintf("assistant %d", i)},
		)
	}

	// 1. ContextWindow known (32000): 80% threshold = 25600
	stats := &RunStats{ContextWindow: 32000, ContextTokens: 20000}
	if shouldAutoCompact(stats, messages) {
		t.Fatalf("should not auto compact below 80%% threshold")
	}

	stats.ContextTokens = 26000
	if !shouldAutoCompact(stats, messages) {
		t.Fatalf("should auto compact above 80%% threshold")
	}

	// 2. ContextWindow unknown (0): fallback threshold = 40000
	unknownStats := &RunStats{ContextWindow: 0, ContextTokens: 35000}
	if shouldAutoCompact(unknownStats, messages) {
		t.Fatalf("should not compact unknown window below 40000")
	}
	unknownStats.ContextTokens = 42000
	if !shouldAutoCompact(unknownStats, messages) {
		t.Fatalf("should compact unknown window above 40000")
	}
}

func TestModelHistoryWithSummaryProjection(t *testing.T) {
	messages := []Message{
		{ID: "u1", Role: "user", Content: "旧问题 1"},
		{ID: "a1", Role: "assistant", Content: "旧回答 1"},
		{ID: "u2", Role: "user", Content: "旧问题 2"},
		{ID: "a2", Role: "assistant", Content: "旧回答 2"},
		{ID: "u3", Role: "user", Content: "新问题 3"},
		{ID: "a3", Role: "assistant", Content: "新回答 3"},
	}

	summary := &ContextSummary{
		SummaryID:     "sum_1",
		UpToMessageID: "a2",
		Content:       "【用户核心目标】：优化代码性能\n【已完成工作】：排查了内存泄漏",
		CreatedAt:     time.Now(),
	}

	projected, err := modelHistoryWithSummary(messages, "u3", summary, nil)
	if err != nil {
		t.Fatalf("modelHistoryWithSummary failed: %v", err)
	}

	// Expected: first message is summary, then u3, then a3
	if len(projected) != 3 {
		t.Fatalf("expected 3 projected messages (summary + u3 + a3), got %d", len(projected))
	}

	content0 := fmt.Sprint(projected[0]["content"])
	if !strings.Contains(content0, "前序历史工作结构化摘要") || !strings.Contains(content0, "优化代码性能") {
		t.Fatalf("first message must be summary block, got %s", content0)
	}

	content1 := fmt.Sprint(projected[1]["content"])
	if content1 != "新问题 3" {
		t.Fatalf("second message must be u3, got %s", content1)
	}

	// Fallback when summary upToMessageId does not match
	badSummary := &ContextSummary{
		SummaryID:     "sum_bad",
		UpToMessageID: "non_existent_id",
		Content:       "some summary",
	}
	fallbackProjected, err := modelHistoryWithSummary(messages, "u3", badSummary, nil)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if len(fallbackProjected) != len(messages) {
		t.Fatalf("expected full fallback projection when summary boundary missing, got %d", len(fallbackProjected))
	}
}

type fakeModelClient struct {
	content string
	err     error
}

func (f *fakeModelClient) Complete(ctx context.Context, model string, messages []map[string]any, tools []map[string]any, settings ModelSettings, onDelta func(string)) (*completion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &completion{Content: f.content, OutputTokens: 100}, nil
}

func TestTryAutoCompactExecution(t *testing.T) {
	svc := &Service{}
	messages := []Message{
		{ID: "u0", Role: "user", Content: "最初的讨论"},
		{ID: "a0", Role: "assistant", Content: "最初的回答"},
		{ID: "u1", Role: "user", Content: "分析架构"},
		{ID: "a1", Role: "assistant", Content: "已分析"},
		{ID: "u2", Role: "user", Content: "编写测试"},
		{ID: "a2", Role: "assistant", Content: "已编写"},
		{ID: "u3", Role: "user", Content: "现在遇到报错"},
	}
	history := &Session{
		ID:       "sess_comp",
		Model:    "test-model",
		Messages: messages,
	}
	stats := &RunStats{RunID: "turn_comp", ContextTokens: 25000, ContextWindow: 30000}

	fakeClient := &fakeModelClient{
		content: "【用户核心目标】：搭建系统架构并跑通测试\n【已完成工作】：已分析并编写了测试",
	}

	var events []Event
	emit := func(e Event) { events = append(events, e) }

	summary, err := svc.tryAutoCompact(context.Background(), fakeClient, history, stats, emit)
	if err != nil {
		t.Fatalf("tryAutoCompact failed: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected generated summary, got nil")
	}
	if summary.UpToMessageID != "a1" {
		t.Fatalf("expected upToMessageId=a1, got %s", summary.UpToMessageID)
	}
	if !strings.Contains(summary.Content, "搭建系统架构") {
		t.Fatalf("expected summary content: %s", summary.Content)
	}

	// Verify thought event announced
	var foundThought bool
	for _, e := range events {
		if e.Type == "thought" && strings.Contains(e.Thought.Text, "自动压缩较早历史") {
			foundThought = true
			break
		}
	}
	if !foundThought {
		t.Fatalf("expected compaction thought event to be emitted")
	}
}
