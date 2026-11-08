package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPruneToolResultVariousKinds(t *testing.T) {
	// 1. Short content is kept intact
	short := "执行成功，退出码 0"
	if got := pruneToolResult("run_command", "call_1", short); got != short {
		t.Fatalf("short content should not be pruned: got %q", got)
	}

	// 2. Long run_command output (> 2048 runes)
	longCommand := "Starting build process...\n" + strings.Repeat("Compiling package x/y/z\n", 200) + "FAIL: test failed at line 142\nexit status 1"
	prunedCommand := pruneToolResult("run_command", "call_cmd", longCommand)
	if !strings.Contains(prunedCommand, "Starting build process") {
		t.Fatalf("expected head of command output to be retained")
	}
	if !strings.Contains(prunedCommand, "exit status 1") || !strings.Contains(prunedCommand, "FAIL: test failed") {
		t.Fatalf("expected tail error to be retained")
	}
	if !strings.Contains(prunedCommand, "call_cmd") {
		t.Fatalf("expected call ID to be referenced for lookup")
	}
	if len(prunedCommand) >= len(longCommand) {
		t.Fatalf("pruned length should be smaller: got %d want < %d", len(prunedCommand), len(longCommand))
	}

	// 3. Long read_file output (> 2048 runes)
	longLines := make([]string, 120)
	for i := range longLines {
		longLines[i] = strings.Repeat("code_line_with_extra_symbols_", 3)
	}
	longFile := strings.Join(longLines, "\n")
	prunedFile := pruneToolResult("read_file", "call_read", longFile)
	if !strings.Contains(prunedFile, "文件读取已精简") || !strings.Contains(prunedFile, "call_read") {
		t.Fatalf("expected read_file pruning notice with call id")
	}

	// 4. Long search_files output (> 2048 runes)
	matchLines := make([]string, 80)
	for i := range matchLines {
		matchLines[i] = "path/to/very/long/file/name/file.go:10: match text line " + strings.Repeat("detail_", 5)
	}
	longSearch := strings.Join(matchLines, "\n")
	prunedSearch := pruneToolResult("search_files", "call_search", longSearch)
	if !strings.Contains(prunedSearch, "搜索结果已精简") || !strings.Contains(prunedSearch, "call_search") {
		t.Fatalf("expected search_files pruning notice")
	}
}

func TestModelHistoryPrunesOlderTurnsOnly(t *testing.T) {
	longOutput := "START\n" + strings.Repeat("long log output line here\n", 150) + "END"

	messages := []Message{
		// Turn 1 (Old turn)
		{ID: "u1", Role: "user", Content: "第一轮用户问题"},
		{
			ID: "a1", Role: "assistant", Content: "运行命令",
			ToolCalls: []map[string]any{
				{"id": "call_old", "type": "function", "function": map[string]any{"name": "run_command", "arguments": "{}"}},
			},
		},
		{ID: "t1", Role: "tool", ToolCallID: "call_old", ToolName: "run_command", Content: longOutput},
		{ID: "a1_final", Role: "assistant", Content: "第一轮完成"},

		// Turn 2 (Recent turn)
		{ID: "u2", Role: "user", Content: "第二轮用户问题"},
		{
			ID: "a2", Role: "assistant", Content: "读取文件",
			ToolCalls: []map[string]any{
				{"id": "call_recent", "type": "function", "function": map[string]any{"name": "read_file", "arguments": "{}"}},
			},
		},
		{ID: "t2", Role: "tool", ToolCallID: "call_recent", ToolName: "read_file", Content: longOutput},
		{ID: "a2_final", Role: "assistant", Content: "第二轮完成"},

		// Turn 3 (Current turn)
		{ID: "u3", Role: "user", Content: "第三轮当前问题"},
	}

	// Projecting model view for current turn u3
	projected, err := modelHistoryWith(messages, "u3", nil)
	if err != nil {
		t.Fatalf("modelHistoryWith failed: %v", err)
	}

	// 1. Verify original slice was NOT modified
	if messages[2].Content != longOutput {
		t.Fatalf("original history message was modified! persistent data must stay intact")
	}

	// 2. Locate tool results in projected model view
	var tool1Content, tool2Content string
	for _, m := range projected {
		if m["role"] == "tool" {
			if m["tool_call_id"] == "call_old" {
				tool1Content, _ = m["content"].(string)
			}
			if m["tool_call_id"] == "call_recent" {
				tool2Content, _ = m["content"].(string)
			}
		}
	}

	// Turn 1 is older (> 1 user turn before u3) -> should be pruned
	if !strings.Contains(tool1Content, "命令输出已精简") {
		t.Fatalf("expected Turn 1 tool result to be pruned, got %q", tool1Content)
	}
	if len(tool1Content) >= len(longOutput) {
		t.Fatalf("Turn 1 output should be significantly smaller: %d vs %d", len(tool1Content), len(longOutput))
	}

	// Turn 2 is recent (within preserveRecentToolTurns) -> kept intact
	if tool2Content != longOutput {
		t.Fatalf("expected Turn 2 tool result to stay unpruned, got length %d want %d", len(tool2Content), len(longOutput))
	}

	// 3. Verify tool call IDs and pairing
	var callIDsInAssistant []string
	var callIDsInTool []string
	for _, m := range projected {
		if calls, ok := m["tool_calls"].([]map[string]any); ok {
			for _, c := range calls {
				callIDsInAssistant = append(callIDsInAssistant, c["id"].(string))
			}
		}
		if m["role"] == "tool" {
			callIDsInTool = append(callIDsInTool, m["tool_call_id"].(string))
		}
	}
	if len(callIDsInAssistant) != len(callIDsInTool) {
		t.Fatalf("tool call pairing mismatch: %d calls vs %d tool returns", len(callIDsInAssistant), len(callIDsInTool))
	}
	for i := range callIDsInAssistant {
		if callIDsInAssistant[i] != callIDsInTool[i] {
			t.Fatalf("mismatched tool_call_id at %d: %q vs %q", i, callIDsInAssistant[i], callIDsInTool[i])
		}
	}
}

func TestReadHistoricalToolResult(t *testing.T) {
	svc := &Service{}
	longOutput := "Line 1: init\nLine 2: setup target\nLine 3: ERROR at component Foo\nLine 4: finished\n"
	messages := []Message{
		{ID: "m1", Role: "tool", ToolCallID: "call_abc", ToolName: "run_command", Content: longOutput},
	}

	// 1. Query by keyword
	queryRaw, _ := json.Marshal(map[string]any{
		"tool_call_id": "call_abc",
		"query":        "ERROR",
	})
	res, err := svc.readHistoricalToolResult(queryRaw, messages)
	if err != nil {
		t.Fatalf("readHistoricalToolResult failed: %v", err)
	}
	if !strings.Contains(res, "ERROR at component Foo") {
		t.Fatalf("expected query match in snippet: %s", res)
	}

	// 2. Query by offset/limit
	rangeRaw, _ := json.Marshal(map[string]any{
		"tool_call_id": "call_abc",
		"offset":       2,
		"limit":        2,
	})
	res2, err := svc.readHistoricalToolResult(rangeRaw, messages)
	if err != nil {
		t.Fatalf("readHistoricalToolResult range failed: %v", err)
	}
	if !strings.Contains(res2, "Line 2: setup target") || !strings.Contains(res2, "Line 3: ERROR") {
		t.Fatalf("expected lines 2 and 3: %s", res2)
	}

	// 3. Unknown call id
	badRaw, _ := json.Marshal(map[string]any{"tool_call_id": "non_existent"})
	_, err = svc.readHistoricalToolResult(badRaw, messages)
	if err == nil {
		t.Fatalf("expected error for non-existent call id")
	}
}

func TestTokenSavingsComparisonOnLongSessionSample(t *testing.T) {
	// Sample with 3 older turns having large tool outputs (e.g. 50 KiB command logs + 20 KiB read_file)
	bigLog := strings.Repeat("INFO: processing step\nDEBUG: detailed trace dump line 0123456789\n", 600) // ~40 KiB
	bigFile := strings.Repeat("package main\n\nfunc Example() {\n\t// code line...\n}\n", 300)          // ~15 KiB

	messages := []Message{
		{ID: "u1", Role: "user", Content: "排查测试失败"},
		{ID: "a1", Role: "assistant", Content: "运行测试", ToolCalls: []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "run_command", "arguments": "{}"}}}},
		{ID: "t1", Role: "tool", ToolCallID: "c1", ToolName: "run_command", Content: bigLog},
		{ID: "a1_done", Role: "assistant", Content: "测试失败了，看日志"},

		{ID: "u2", Role: "user", Content: "看看文件实现"},
		{ID: "a2", Role: "assistant", Content: "读取文件", ToolCalls: []map[string]any{{"id": "c2", "type": "function", "function": map[string]any{"name": "read_file", "arguments": "{}"}}}},
		{ID: "t2", Role: "tool", ToolCallID: "c2", ToolName: "read_file", Content: bigFile},
		{ID: "a2_done", Role: "assistant", Content: "找到问题所在"},

		{ID: "u3", Role: "user", Content: "再跑一次命令"},
		{ID: "a3", Role: "assistant", Content: "重试命令", ToolCalls: []map[string]any{{"id": "c3", "type": "function", "function": map[string]any{"name": "run_command", "arguments": "{}"}}}},
		{ID: "t3", Role: "tool", ToolCallID: "c3", ToolName: "run_command", Content: bigLog},
		{ID: "a3_done", Role: "assistant", Content: "依然失败"},

		// Current turn 4
		{ID: "u4", Role: "user", Content: "继续修复"},
	}

	// 1. Calculate tokens BEFORE pruning (simulate unpruned full history)
	unprunedProjected := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		p, _ := messageToModelWith(m, nil)
		unprunedProjected = append(unprunedProjected, p)
	}
	bdBefore := calculateContextBreakdown(unprunedProjected, toolDefinitions)

	// 2. Calculate tokens AFTER pruning (using modelHistoryWith for current turn u4)
	prunedProjected, err := modelHistoryWith(messages, "u4", nil)
	if err != nil {
		t.Fatalf("modelHistoryWith failed: %v", err)
	}
	bdAfter := calculateContextBreakdown(prunedProjected, toolDefinitions)

	t.Logf("Before pruning: TotalTokens=%d (ToolResultsTokens=%d)", bdBefore.TotalTokens, bdBefore.ToolResultsTokens)
	t.Logf("After pruning:  TotalTokens=%d (ToolResultsTokens=%d)", bdAfter.TotalTokens, bdAfter.ToolResultsTokens)

	if bdAfter.TotalTokens >= bdBefore.TotalTokens {
		t.Fatalf("expected token reduction after pruning: before=%d after=%d", bdBefore.TotalTokens, bdAfter.TotalTokens)
	}
	savedTokens := bdBefore.TotalTokens - bdAfter.TotalTokens
	reductionRatio := float64(savedTokens) / float64(bdBefore.TotalTokens) * 100
	t.Logf("Tokens saved: %d (%.1f%% reduction on total context)", savedTokens, reductionRatio)

	if reductionRatio < 40.0 {
		t.Fatalf("expected at least 40%% token reduction on this heavy tool sample, got %.1f%%", reductionRatio)
	}
}
