package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
)

// recordingModel captures the tool schemas offered on every request.
type recordingModel struct {
	mu       sync.Mutex
	replies  []*completion
	toolSets [][]map[string]any
}

func (m *recordingModel) Complete(_ context.Context, _ string, messages []map[string]any, tools []map[string]any, _ ModelSettings, _ func(string)) (*completion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isSummaryRequest(messages) {
		return &completion{Content: "确认请求。"}, nil
	}
	m.toolSets = append(m.toolSets, tools)
	if len(m.replies) == 0 {
		return &completion{Content: "done"}, nil
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	return reply, nil
}

func toolNames(tools []map[string]any) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		names = append(names, name)
	}
	return names
}

func hasTool(tools []map[string]any, name string) bool {
	for _, candidate := range toolNames(tools) {
		if candidate == name {
			return true
		}
	}
	return false
}

// A run that never touches a browser must not carry the browser tool schemas.
func TestLazyMCPKeepsBrowserToolsOutOfUnrelatedRuns(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	if err := svc.SetAccessMode(ModeFull); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMCPSettings(MCPSettings{Servers: map[string]MCPServer{"fixture": {Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}}}}); err != nil {
		t.Fatal(err)
	}
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	model := &recordingModel{replies: []*completion{{Content: "done"}}}
	if err := svc.loop(context.Background(), nil, model, history, nil, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(model.toolSets) == 0 {
		t.Fatal("no model request recorded")
	}
	for index, tools := range model.toolSets {
		if hasTool(tools, "mcp_0") {
			t.Fatalf("request %d advertised MCP tools before activation: %v", index, toolNames(tools))
		}
		if !hasTool(tools, mcpEnableToolName) {
			t.Fatalf("request %d lost the activation tool: %v", index, toolNames(tools))
		}
	}
}

// Activating the browser adds the real schemas to the following request.
func TestLazyMCPAddsToolsAfterActivation(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	if err := svc.SetAccessMode(ModeFull); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMCPSettings(MCPSettings{Servers: map[string]MCPServer{"fixture": {Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}}}}); err != nil {
		t.Fatal(err)
	}
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	model := &recordingModel{replies: []*completion{
		{ToolCalls: []toolCall{{ID: "call-enable", Name: mcpEnableToolName, Arguments: json.RawMessage(`{}`)}}},
		{Content: "done"},
	}}
	var activities []Activity
	if err := svc.loop(context.Background(), nil, model, history, nil, func(event Event) {
		if event.Activity != nil {
			activities = append(activities, *event.Activity)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(model.toolSets) < 2 {
		t.Fatalf("expected two requests, got %d", len(model.toolSets))
	}
	if hasTool(model.toolSets[0], "mcp_0") {
		t.Fatalf("first request advertised MCP tools: %v", toolNames(model.toolSets[0]))
	}
	if !hasTool(model.toolSets[1], "mcp_0") {
		t.Fatalf("second request lost MCP tools: %v", toolNames(model.toolSets[1]))
	}
	if !hasTool(model.toolSets[1], mcpEnableToolName) {
		t.Fatalf("activation tool disappeared: %v", toolNames(model.toolSets[1]))
	}
	if len(activities) != 2 || activities[0].Title != "启用浏览器工具" || activities[1].Status != "completed" {
		t.Fatalf("activities: %+v", activities)
	}
}

// The model may call an MCP tool without activating first; the server must
// still start and the call must succeed.
func TestLazyMCPStartsServerOnDirectToolCall(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	if err := svc.SetAccessMode(ModeFull); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMCPSettings(MCPSettings{Servers: map[string]MCPServer{"fixture": {Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}}}}); err != nil {
		t.Fatal(err)
	}
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	model := &recordingModel{replies: []*completion{
		{ToolCalls: []toolCall{{ID: "call-echo", Name: "mcp_0", Arguments: json.RawMessage(`{}`)}}},
		{Content: "done"},
	}}
	if err := svc.loop(context.Background(), nil, model, history, nil, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(model.toolSets) < 2 || !hasTool(model.toolSets[1], "mcp_0") {
		t.Fatalf("tools not activated: %v", model.toolSets)
	}
	saved, err := svc.Get(history.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, activity := range saved.Activity {
		if activity.Tool == "fixture/echo" && activity.Output == "fixture result" {
			found = true
		}
	}
	if !found {
		t.Fatalf("direct call not executed: %+v", saved.Activity)
	}
}

// Activation asks for the same user approval that MCP calls already require.
func TestLazyMCPActivationNeedsApproval(t *testing.T) {
	if detail, ok := needsApproval(nil, mcpEnableToolName, json.RawMessage(`{}`)); !ok || detail == "" {
		t.Fatal("activation bypasses approval")
	}
	if detail, ok := needsApproval(nil, "mcp_0", json.RawMessage(`{}`)); !ok || !strings.Contains(detail, "MCP") {
		t.Fatal("MCP approval lost")
	}
}

// A conversation that already used an MCP tool must expose those tool names
// again, otherwise the stored tool messages reference unknown tools.
func TestLazyMCPPreActivatesWhenHistoryUsedMCPTools(t *testing.T) {
	t.Setenv("XT_MCP_FIXTURE", "1")
	t.Setenv("XIAOTAO_MCP_CONFIG", "")
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	if err := svc.SetAccessMode(ModeFull); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMCPSettings(MCPSettings{Servers: map[string]MCPServer{"fixture": {Command: os.Args[0], Args: []string{"-test.run=^TestMCPFixture$"}}}}); err != nil {
		t.Fatal(err)
	}
	history, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	messages := []map[string]any{
		{"role": "user", "content": "打开网页"},
		{"role": "tool", "tool_call_id": "call-1", "name": "mcp_0", "content": "ok"},
	}
	model := &recordingModel{replies: []*completion{{Content: "done"}}}
	if err := svc.loop(context.Background(), nil, model, history, messages, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(model.toolSets) == 0 || !hasTool(model.toolSets[0], "mcp_0") {
		t.Fatalf("history MCP tools not restored: %v", toolNames(model.toolSets[0]))
	}
}

// The system prompt must advertise the browser, otherwise the model never
// reaches for the activation tool.
func TestSystemPromptAnnouncesBrowserTools(t *testing.T) {
	prompt := systemPrompt("E:/fixture")
	if !strings.Contains(prompt, mcpEnableToolName) {
		t.Fatalf("system prompt does not mention %s: %s", mcpEnableToolName, prompt)
	}
	if !strings.Contains(prompt, "\u6d4f\u89c8\u5668") {
		t.Fatalf("system prompt does not mention the browser: %s", prompt)
	}
	if !strings.Contains(systemPrompt(""), mcpEnableToolName) {
		t.Fatal("browser notice missing without a project")
	}
}
