package requestaction

import (
	"strings"
	"testing"
)

func TestProxy_DetectAction(t *testing.T) {
	// 1. User message
	uMsgs := []map[string]any{
		{"role": "user", "content": "帮我优化项目结构并整理代码"},
	}
	tag, title := Detect(uMsgs, nil)
	if tag != "用户" || !strings.Contains(title, "优化项目结构") {
		t.Errorf("unexpected user action: tag=%s, title=%s", tag, title)
	}

	// 2. Tool read action
	readMsgs := []map[string]any{
		{"role": "user", "content": "hello"},
		{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"function": map[string]any{
						"name":      "read_file",
						"arguments": `{"path":"src/config.json"}`,
					},
				},
			},
		},
	}
	rTag, rTitle := Detect(readMsgs, nil)
	if rTag != "读" || !strings.Contains(rTitle, "config.json") {
		t.Errorf("unexpected read action: tag=%s, title=%s", rTag, rTitle)
	}

	// 3. Tool write action
	writeMsgs := []map[string]any{
		{"role": "user", "content": "hello"},
		{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"function": map[string]any{
						"name":      "apply_patch",
						"arguments": `{"path":"xt-client/README.md"}`,
					},
				},
			},
		},
	}
	wTag, wTitle := Detect(writeMsgs, nil)
	if wTag != "写" || !strings.Contains(wTitle, "README.md") {
		t.Errorf("unexpected write action: tag=%s, title=%s", wTag, wTitle)
	}

	// 4. User message with prompt template wrapper
	wrappedMsgs := []map[string]any{
		{"role": "user", "content": "# Files mentioned:\n## codex-123.png\n\n## My request:\n请修复输入输出时间样式的布局"},
	}
	wTag2, wTitle2 := Detect(wrappedMsgs, nil)
	if wTag2 != "用户" || !strings.Contains(wTitle2, "修复输入输出时间样式") {
		t.Errorf("unexpected wrapped prompt action: tag=%s, title=%s", wTag2, wTitle2)
	}

	// 5. Tool response message matching preceding assistant tool call
	toolRespMsgs := []map[string]any{
		{"role": "user", "content": "hello"},
		{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id": "call_999",
					"function": map[string]any{
						"name":      "mcp__codex_app__read_thread",
						"arguments": `{"threadId":"thread_123"}`,
					},
				},
			},
		},
		{
			"role":         "tool",
			"tool_call_id": "call_999",
			"content":      "thread content...",
		},
	}
	tTag, tTitle := Detect(toolRespMsgs, nil)
	if tTag != "读" || !strings.Contains(tTitle, "分析") {
		t.Errorf("unexpected tool response action: tag=%s, title=%s", tTag, tTitle)
	}
}
