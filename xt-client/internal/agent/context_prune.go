package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// toolPruneThreshold is the character length above which older tool outputs
	// are pruned when projecting the model view. Short outputs stay intact.
	toolPruneThreshold = 2048

	// preserveRecentToolTurns is the number of completed user turns before the
	// current turn whose tool results remain unpruned.
	preserveRecentToolTurns = 1

	// maxHistoricalToolReadRunes limits the maximum content returned by one lookup.
	maxHistoricalToolReadRunes = 3000
)

var historicalToolLookupDefinition = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "read_historical_tool_result",
		"description": "Read bounded text from an earlier pruned tool result in the current session. Use this only when the pruned summary is insufficient.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"tool_call_id"},
			"properties": map[string]any{
				"tool_call_id": map[string]any{"type": "string", "description": "The tool_call_id of the pruned result to inspect"},
				"query":        map[string]any{"type": "string", "description": "Optional keyword to locate within the original output"},
				"offset":       map[string]any{"type": "integer", "description": "Starting line number (1-based), default 1"},
				"limit":        map[string]any{"type": "integer", "description": "Maximum lines to return, default 80, max 160"},
			},
		},
	},
}

// pruneToolResult generates a deterministic, compact summary of an older tool output
// while preserving key context, status, errors, and an explicit reference for lookup.
func pruneToolResult(toolName, callID, content string) string {
	runes := []rune(content)
	if len(runes) <= toolPruneThreshold {
		return content
	}

	totalLen := len(runes)
	switch toolName {
	case "run_command":
		// Retain initial command startup (first 400 runes) and critical trailing errors (last 1000 runes).
		head := string(runes[:400])
		tail := string(runes[totalLen-1000:])
		return fmt.Sprintf("%s\n\n...[命令输出已精简，共 %d 字符。关键结尾见下文；若需完整细节可调用 read_historical_tool_result(tool_call_id=%q)]...\n\n%s", head, totalLen, callID, tail)

	case "read_file":
		lines := strings.Split(content, "\n")
		if len(lines) > 40 {
			head := strings.Join(lines[:20], "\n")
			tail := strings.Join(lines[len(lines)-15:], "\n")
			return fmt.Sprintf("%s\n\n...[文件读取已精简：共 %d 行 (%d 字符)。代码仍存在于项目文件；细节可重新读取或调用 read_historical_tool_result(tool_call_id=%q)]...\n\n%s", head, len(lines), totalLen, callID, tail)
		}
		// If few lines but very long lines:
		head := string(runes[:400])
		tail := string(runes[totalLen-600:])
		return fmt.Sprintf("%s\n\n...[内容已精简，共 %d 字符。可调用 read_historical_tool_result(tool_call_id=%q)]...\n\n%s", head, totalLen, callID, tail)

	case "search_files":
		lines := strings.Split(content, "\n")
		if len(lines) > 15 {
			head := strings.Join(lines[:12], "\n")
			return fmt.Sprintf("%s\n\n...[搜索结果已精简：展示前 12 项，共 %d 项匹配。建议缩小关键词重新搜索，或调用 read_historical_tool_result(tool_call_id=%q)]", head, len(lines), callID)
		}
		head := string(runes[:600])
		return fmt.Sprintf("%s\n\n...[搜索结果已精简：共 %d 字符。可调用 read_historical_tool_result(tool_call_id=%q)]", head, totalLen, callID)

	case "list_files":
		lines := strings.Split(content, "\n")
		if len(lines) > 30 {
			head := strings.Join(lines[:25], "\n")
			return fmt.Sprintf("%s\n\n...[目录列表已精简：展示前 25 项，共 %d 项。可调用 read_historical_tool_result(tool_call_id=%q)]", head, len(lines), callID)
		}
		head := string(runes[:600])
		return fmt.Sprintf("%s\n\n...[目录列表已精简：共 %d 字符。可调用 read_historical_tool_result(tool_call_id=%q)]", head, totalLen, callID)

	default:
		head := string(runes[:400])
		tail := string(runes[totalLen-400:])
		return fmt.Sprintf("%s\n\n...[工具结果已精简：共 %d 字符。可调用 read_historical_tool_result(tool_call_id=%q)]...\n\n%s", head, totalLen, callID, tail)
	}
}

// isOlderToolMessage determines if a tool message is old enough to prune.
// Messages in the current turn and within the last `preserveRecentToolTurns` user turns
// are kept completely unpruned.
func isOlderToolMessage(messages []Message, targetIndex int, currentUserID string) bool {
	if targetIndex < 0 || targetIndex >= len(messages) {
		return false
	}
	if messages[targetIndex].Role != "tool" {
		return false
	}

	// Count how many user turns occur between targetIndex and currentUserID.
	var laterUserTurns int
	for i := targetIndex + 1; i < len(messages); i++ {
		if messages[i].Role == "user" {
			laterUserTurns++
		}
	}

	return laterUserTurns > preserveRecentToolTurns
}

func hasPrunedToolResults(messages []Message, currentUserID string) bool {
	for i, m := range messages {
		if m.Role == "tool" && isOlderToolMessage(messages, i, currentUserID) && len([]rune(m.Content)) > toolPruneThreshold {
			return true
		}
	}
	return false
}

func (s *Service) readHistoricalToolResult(raw json.RawMessage, messages []Message) (string, error) {
	var args struct {
		ToolCallID string `json:"tool_call_id"`
		Query      string `json:"query"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("回查参数无效")
	}
	callID := strings.TrimSpace(args.ToolCallID)
	if callID == "" {
		return "", fmt.Errorf("请提供 tool_call_id")
	}

	// Only search within the current session's messages
	var found *Message
	for i := range messages {
		if messages[i].Role == "tool" && messages[i].ToolCallID == callID {
			found = &messages[i]
			break
		}
	}
	if found == nil {
		return "", fmt.Errorf("未在当前会话中找到对应的历史工具结果 (ID: %s)", callID)
	}

	text := found.Content
	query := strings.TrimSpace(args.Query)
	runes := []rune(text)

	if query != "" {
		lowerText := strings.ToLower(text)
		lowerQuery := strings.ToLower(query)
		idx := strings.Index(lowerText, lowerQuery)
		if idx < 0 {
			return fmt.Sprintf("历史结果中未找到关键词 %q。原始输出共 %d 字符。", query, len(runes)), nil
		}
		runeOffset := len([]rune(text[:idx]))
		start := runeOffset - 200
		if start < 0 {
			start = 0
		}
		end := start + maxHistoricalToolReadRunes
		if end > len(runes) {
			end = len(runes)
		}
		return fmt.Sprintf("历史工具结果片段（工具：%s，ID：%s，定位关键词：%q）：\n\n%s", found.ToolName, callID, query, string(runes[start:end])), nil
	}

	// Offset / limit based extraction
	lines := strings.Split(text, "\n")
	offset := args.Offset
	if offset < 1 {
		offset = 1
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 80
	}
	if limit > 160 {
		limit = 160
	}

	if offset > len(lines) {
		return fmt.Sprintf("行号超出范围：共 %d 行，请求起始行 %d", len(lines), offset), nil
	}
	endLine := offset - 1 + limit
	if endLine > len(lines) {
		endLine = len(lines)
	}

	snippet := strings.Join(lines[offset-1:endLine], "\n")
	snippetRunes := []rune(snippet)
	if len(snippetRunes) > maxHistoricalToolReadRunes {
		snippet = string(snippetRunes[:maxHistoricalToolReadRunes]) + "\n...[单次提取已达上限]"
	}

	return fmt.Sprintf("历史工具结果片段（工具：%s，ID：%s，第 %d-%d 行，共 %d 行）：\n\n%s", found.ToolName, callID, offset, endLine, len(lines), snippet), nil
}
