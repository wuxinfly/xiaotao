package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type titleModel struct {
	mu       sync.Mutex
	messages [][]map[string]any
	settings []ModelSettings
	results  []titleResult
}

type titleResult struct {
	reply *completion
	err   error
}

func (m *titleModel) Complete(_ context.Context, model string, messages []map[string]any, tools []map[string]any, settings ModelSettings, _ func(string)) (*completion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isSummaryRequest(messages) {
		return &completion{Content: "确认请求。"}, nil
	}
	m.messages = append(m.messages, messages)
	m.settings = append(m.settings, settings)
	if len(m.results) == 0 {
		return nil, nil
	}
	result := m.results[0]
	m.results = m.results[1:]
	return result.reply, result.err
}

func TestGenerateTitleUsesLatestConversationRecords(t *testing.T) {
	svc := NewService(t.TempDir(), "http://unused", nil, nil, nil)
	item, err := svc.Create(Options{Model: "test/title"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	history.Messages = []Message{
		{ID: "u0", Role: "user", Content: "最早的需求：先做登录", CreatedAt: time.Now().Add(-20 * time.Minute)},
		{ID: "a0", Role: "assistant", Content: "登录已经完成", CreatedAt: time.Now().Add(-19 * time.Minute)},
		{ID: "u2", Role: "user", Content: "现在改成做会话重命名", CreatedAt: time.Now().Add(-2 * time.Minute)},
		{ID: "a2", Role: "assistant", Content: "正在接入 AI 标题", CreatedAt: time.Now().Add(-time.Minute)},
		{ID: "u3", Role: "user", Content: "标题要短一些", CreatedAt: time.Now().Add(-50 * time.Second)},
		{ID: "a3", Role: "assistant", Content: "正在收紧标题长度", CreatedAt: time.Now().Add(-40 * time.Second)},
		{ID: "u4", Role: "user", Content: "最后确认按钮位置", CreatedAt: time.Now().Add(-20 * time.Second)},
		{ID: "a4", Role: "assistant", Content: "按钮已经放在输入框右侧", CreatedAt: time.Now().Add(-10 * time.Second)},
	}
	history.Thoughts = []ThoughtStep{
		{ID: "old", Text: "旧的执行进展", CreatedAt: time.Now().Add(-21 * time.Minute)},
		{ID: "new", Text: "最新的执行进展：已经读取旧标题逻辑", CreatedAt: time.Now().Add(-5 * time.Second)},
	}
	if err = svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	model := &titleModel{results: []titleResult{
		{reply: &completion{Content: "用户要求增加 AI 生成会话标题，并确定标题要短、由用户确认保存。"}},
		{reply: &completion{Content: "「AI 会话标题」\n多余解释"}},
	}}
	svc.SetModelClientForTest(model)
	title, err := svc.GenerateTitle(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if title != "AI 会话标题" {
		t.Fatalf("unexpected cleaned title: %q", title)
	}
	if len(model.messages) != 2 || len(model.messages[0]) != 2 || len(model.messages[1]) != 2 {
		t.Fatalf("expected summary and title requests: %+v", model.messages)
	}
	material := model.messages[0][1]["content"].(string)
	if !strings.Contains(material, "最后确认按钮位置") || !strings.Contains(material, "按钮已经放在输入框右侧") || !strings.Contains(material, "最新的执行进展") {
		t.Fatalf("latest records missing from material: %s", material)
	}
	if strings.Contains(material, "最早的需求：先做登录") || strings.Contains(material, "登录已经完成") || strings.Contains(material, "现在改成做会话重命名") {
		t.Fatalf("older records should be bounded out: %s", material)
	}
	if got := model.messages[1][1]["content"].(string); !strings.Contains(got, "用户要求增加 AI 生成会话标题") {
		t.Fatalf("title request must use the generated summary: %s", got)
	}
	loaded, err := svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "新任务" {
		t.Fatalf("generate must not persist the title, got %q", loaded.Title)
	}
}

func TestGenerateTitleFallsBackWhenSummaryRequestFails(t *testing.T) {
	svc := NewService(t.TempDir(), "http://unused", nil, nil, nil)
	item, err := svc.Create(Options{Model: "test/title"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	history.Messages = []Message{{ID: "u", Role: "user", Content: "讨论标题生成", CreatedAt: time.Now()}}
	if err = svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	model := &titleModel{results: []titleResult{
		{err: context.DeadlineExceeded},
		{reply: &completion{Content: "标题生成"}},
	}}
	svc.SetModelClientForTest(model)
	title, err := svc.GenerateTitle(context.Background(), item.ID)
	if err != nil || title != "标题生成" {
		t.Fatalf("expected fallback title, got %q %v", title, err)
	}
	if len(model.messages) != 2 || !strings.Contains(model.messages[1][1]["content"].(string), "讨论标题生成") {
		t.Fatalf("fallback must retry from bounded records: %+v", model.messages)
	}
}

func TestGenerateTitleUsesRequestedUtilityModel(t *testing.T) {
	svc := NewService(t.TempDir(), "http://unused", nil, nil, nil)
	item, err := svc.Create(Options{Model: "session/model"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	history.Messages = []Message{{ID: "u", Role: "user", Content: "测试默认标题模型", CreatedAt: time.Now()}}
	if err = svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	model := &titleModel{results: []titleResult{
		{reply: &completion{Content: "总结"}},
		{reply: &completion{Content: "默认标题"}},
	}}
	svc.SetModelClientForTest(model)
	title, err := svc.GenerateTitleWithModel(context.Background(), item.ID, "utility/model", &ModelSettings{ContextWindow: 32000, MaxOutputTokens: 512})
	if err != nil || title != "默认标题" {
		t.Fatalf("title=%q err=%v", title, err)
	}
	if len(model.settings) != 2 || model.settings[0].MaxOutputTokens != 512 || model.settings[1].ContextWindow != 32000 {
		t.Fatalf("utility settings were not passed: %+v", model.settings)
	}
}

func TestGenerateTitleRejectsRunningAndEmptySessions(t *testing.T) {
	svc := NewService(t.TempDir(), "http://unused", nil, nil, nil)
	empty, err := svc.Create(Options{Model: "test/title"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.GenerateTitle(context.Background(), empty.ID); err == nil || !strings.Contains(err.Error(), "没有可用于生成标题") {
		t.Fatalf("expected empty-session error, got %v", err)
	}
	running, err := svc.Create(Options{Model: "test/title"})
	if err != nil {
		t.Fatal(err)
	}
	svc.runs[running.ID] = &run{cancel: func() {}}
	if _, err = svc.GenerateTitle(context.Background(), running.ID); err == nil || !strings.Contains(err.Error(), "请先停止") {
		t.Fatalf("expected running-session error, got %v", err)
	}
}

func TestCleanGeneratedTitleBoundsAndNormalizesOutput(t *testing.T) {
	cases := map[string]string{
		"标题：修复登录":           "修复登录",
		"“AI 标题”。":          "AI 标题",
		"```text\n短标题\n```": "短标题",
		"":                  "",
	}
	for input, want := range cases {
		if got := cleanGeneratedTitle(input); got != want {
			t.Errorf("cleanGeneratedTitle(%q) = %q, want %q", input, got, want)
		}
	}
	if got := cleanGeneratedSummary("```text\n会话最新总结\n```"); got != "会话最新总结" {
		t.Fatalf("summary fence was not removed: %q", got)
	}
	long := strings.Repeat("字", autoTitleMaxRunes+8)
	if got := cleanGeneratedTitle(long); len([]rune(got)) != autoTitleMaxRunes {
		t.Fatalf("long title was not bounded: %d", len([]rune(got)))
	}
}
