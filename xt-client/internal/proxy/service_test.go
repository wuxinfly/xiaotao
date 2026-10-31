package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/storage"
	"xiaotao/xt-client/internal/usage"
)

func TestProxy_UnauthorizedDoesNotTrack(t *testing.T) {
	tmp := t.TempDir()
	store, err := storage.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	usageSvc, err := usage.New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	defer usageSvc.Close()

	svc := New(store, func(id string) ([]byte, error) { return nil, nil }, "valid-key")
	svc.SetUsage(usageSvc)

	// Request with invalid API key
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Authorization", "Bearer invalid-key")
	w := httptest.NewRecorder()

	svc.HandleChat(w, r)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	snap, err := usageSvc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.ActiveCount != 0 {
		t.Errorf("expected 0 active requests, got %d", snap.ActiveCount)
	}
	if len(snap.RecentRequests) != 0 {
		t.Errorf("expected 0 recent requests, got %d", len(snap.RecentRequests))
	}
}

func TestProxy_AuthorizedWithNoReadyAccount(t *testing.T) {
	tmp := t.TempDir()
	store, err := storage.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	usageSvc, err := usage.New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	defer usageSvc.Close()

	svc := New(store, func(id string) ([]byte, error) { return nil, nil }, "valid-key")
	svc.SetUsage(usageSvc)

	// Request with valid API key but no account configured
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()

	svc.HandleChat(w, r)
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	snap, err := usageSvc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	// Should have 1 failed request recorded
	if len(snap.RecentRequests) != 1 {
		t.Fatalf("expected 1 recent request recorded, got %d", len(snap.RecentRequests))
	}
	rec := snap.RecentRequests[0]
	if rec.Status != usage.StatusFailed {
		t.Errorf("expected status %s, got %s", usage.StatusFailed, rec.Status)
	}
	if !strings.Contains(rec.ErrorSummary, "缺少可用账号") {
		t.Errorf("expected error summary to mention missing account, got %s", rec.ErrorSummary)
	}
}

func TestProxy_DetectAction(t *testing.T) {
	// 1. User message
	uMsgs := []map[string]any{
		{"role": "user", "content": "帮我优化项目结构并整理代码"},
	}
	tag, title := detectAction(uMsgs, nil)
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
	rTag, rTitle := detectAction(readMsgs, nil)
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
	wTag, wTitle := detectAction(writeMsgs, nil)
	if wTag != "写" || !strings.Contains(wTitle, "README.md") {
		t.Errorf("unexpected write action: tag=%s, title=%s", wTag, wTitle)
	}

	// 4. User message with prompt template wrapper
	wrappedMsgs := []map[string]any{
		{"role": "user", "content": "# Files mentioned:\n## codex-123.png\n\n## My request:\n请修复输入输出时间样式的布局"},
	}
	wTag2, wTitle2 := detectAction(wrappedMsgs, nil)
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
	tTag, tTitle := detectAction(toolRespMsgs, nil)
	if tTag != "读" || !strings.Contains(tTitle, "分析") {
		t.Errorf("unexpected tool response action: tag=%s, title=%s", tTag, tTitle)
	}
}
