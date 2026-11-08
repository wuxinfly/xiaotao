package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ContextSummary holds a derived, structured summary of earlier message turns.
// The original messages remain intact in storage, while the summary allows the
// active model view to stay comfortably within the context window limit.
type ContextSummary struct {
	SummaryID     string    `json:"summaryId"`
	UpToMessageID string    `json:"upToMessageId"`
	UpToRunID     string    `json:"upToRunId"`
	Content       string    `json:"content"`
	Model         string    `json:"model"`
	CreatedAt     time.Time `json:"createdAt"`
}

const (
	// compactionThresholdRatio triggers automatic compaction when context usage
	// reaches 80% of the model window.
	compactionThresholdRatio = 0.80

	// fallbackWindowCompactionTokens is used when contextWindow is unknown (0).
	fallbackWindowCompactionTokens = 40000

	// preserveRecentCompactionTurns is the number of latest user turns preserved intact.
	preserveRecentCompactionTurns = 2
)

const compactionSystemPrompt = `你是小涛的历史上下文摘要助手。请将以下较早的多轮工作历史压缩为一份结构化、高保真的工作摘要。
要求严格包含以下几个维度（若没有相关内容可简记为无）：
1. 【用户核心目标】：用户想解决的根本问题或需求。
2. 【已确认决定与约束】：已经讨论确定的技术方案、规则约束或架构设计。
3. 【涉及关键文件与代码】：涉及的文件路径、核心函数或已修改的模块。
4. 【已完成工作与验证】：已执行的命令、验证/测试结果、已修复的缺陷。
5. 【未完成事项与后续步骤】：遗留的待办任务、已知阻碍或紧接着该做的事。

注意：保留事实与具体标识（文件名、退出状态等），不要虚构，不要丢失错误证据。`

// findCompactionCutoff finds the message index boundary to prune up to.
// The boundary strictly falls after a complete assistant/tool turn and keeps
// at least `preserveRecentCompactionTurns` user turns intact.
func findCompactionCutoff(messages []Message, preserveTurns int) int {
	if preserveTurns <= 0 {
		preserveTurns = 2
	}

	// Scan backwards to find the start of the N-th recent user turn
	userTurnCount := 0
	targetUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userTurnCount++
			if userTurnCount == preserveTurns {
				targetUserIdx = i
				break
			}
		}
	}

	if targetUserIdx <= 0 {
		return -1 // Not enough earlier turns to warrant compaction
	}

	// The cutoff point is the message right before targetUserIdx.
	// Ensure we do not cut between an assistant tool_call and its tool returns.
	cutoff := targetUserIdx - 1
	for cutoff >= 0 && messages[cutoff].Role == "tool" {
		cutoff--
	}

	// Only compact if there are at least 3 messages to summarize
	if cutoff < 2 {
		return -1
	}

	return cutoff
}

// shouldAutoCompact checks whether the session's context usage exceeds the safe threshold.
func shouldAutoCompact(stats *RunStats, messages []Message) bool {
	if stats == nil {
		return false
	}

	cutoff := findCompactionCutoff(messages, preserveRecentCompactionTurns)
	if cutoff < 0 {
		return false
	}

	if stats.ContextWindow > 0 {
		threshold := int64(float64(stats.ContextWindow) * compactionThresholdRatio)
		return stats.ContextTokens >= threshold
	}

	return stats.ContextTokens >= fallbackWindowCompactionTokens
}

// generateHistorySummary calls the model to produce a structured summary of messages up to cutoff.
func generateHistorySummary(ctx context.Context, client modelClient, model string, messagesToSummarize []Message) (string, error) {
	var sb strings.Builder
	for _, m := range messagesToSummarize {
		roleName := m.Role
		if roleName == "assistant" && len(m.ToolCalls) > 0 {
			calls := make([]string, 0, len(m.ToolCalls))
			for _, c := range m.ToolCalls {
				calls = append(calls, fmt.Sprint(c["function"]))
			}
			sb.WriteString(fmt.Sprintf("[%s]: %s (调用工具: %s)\n", roleName, m.Content, strings.Join(calls, ", ")))
			continue
		}
		if roleName == "tool" {
			// Limit individual tool result in summary prompt to keep summary request light
			content := m.Content
			if len([]rune(content)) > 400 {
				content = string([]rune(content)[:400]) + "...[已截断]"
			}
			sb.WriteString(fmt.Sprintf("[%s (%s)]: %s\n", roleName, m.ToolName, content))
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", roleName, m.Content))
	}

	promptMessages := []map[string]any{
		{"role": "system", "content": compactionSystemPrompt},
		{"role": "user", "content": "请对以下历史工作过程进行结构化摘要：\n\n" + sb.String()},
	}

	summaryCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := client.Complete(summaryCtx, model, promptMessages, nil, ModelSettings{MaxOutputTokens: 1500}, nil)
	if err != nil {
		return "", fmt.Errorf("摘要生成失败: %w", err)
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return "", fmt.Errorf("摘要模型返回内容为空")
	}

	return content, nil
}

func (s *Service) tryAutoCompact(ctx context.Context, client modelClient, history *Session, stats *RunStats, emit func(Event)) (*ContextSummary, error) {
	cutoff := findCompactionCutoff(history.Messages, preserveRecentCompactionTurns)
	if cutoff < 0 {
		return nil, nil
	}
	messagesToSummarize := history.Messages[:cutoff+1]
	upToMsg := history.Messages[cutoff]

	step := ThoughtStep{
		ID:        "thought_" + randomID(5),
		RunID:     stats.RunID,
		Kind:      "decision",
		Text:      "当前上下文已接近模型上限，自动压缩较早历史并生成工作摘要...",
		Status:    "completed",
		CreatedAt: time.Now(),
	}
	history.Thoughts = append(history.Thoughts, step)
	emit(Event{Type: "thought", SessionID: history.ID, Thought: &step})

	summaryContent, err := generateHistorySummary(ctx, client, history.Model, messagesToSummarize)
	if err != nil {
		return nil, err
	}

	summary := &ContextSummary{
		SummaryID:     "sum_" + randomID(6),
		UpToMessageID: upToMsg.ID,
		UpToRunID:     upToMsg.RunID,
		Content:       summaryContent,
		Model:         history.Model,
		CreatedAt:     time.Now(),
	}
	return summary, nil
}
