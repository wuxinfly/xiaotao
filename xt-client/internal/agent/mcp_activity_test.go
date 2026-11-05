package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMCPActivityDescription(t *testing.T) {
	c := &mcpConnection{server: "browser", names: map[string]string{"mcp_0": "new_page", "mcp_1": "vendor_custom_tool", "mcp_2": "fill"}}
	activity := Activity{}
	describeMCPActivity(&activity, c, toolCall{Name: "mcp_0", Arguments: json.RawMessage(`{"url":"https://example.com"}`)})
	if activity.Title != "打开网页" || activity.Detail != "url: https://example.com" || activity.Tool != "browser/new_page" || !strings.Contains(activity.Input, `"url"`) {
		t.Fatalf("description: %+v", activity)
	}
	describeMCPActivity(&activity, c, toolCall{Name: "mcp_1", Arguments: json.RawMessage(`{}`)})
	if activity.Title != "调用 vendor_custom_tool" || activity.Detail != "browser/vendor_custom_tool" {
		t.Fatalf("fallback: %+v", activity)
	}
	describeMCPActivity(&activity, c, toolCall{Name: "mcp_2", Arguments: json.RawMessage(`{"uid":"input_1","value":"private input"}`)})
	if activity.Title != "填写网页输入框" || strings.Contains(activity.Detail, "private input") {
		t.Fatalf("input leaked into summary: %+v", activity)
	}
	if output := mcpOutputPreview(`{"content":[{"type":"text","text":"Example Domain"},{"type":"image","data":"BASE64"}]}`); !strings.Contains(output, "Example Domain") || strings.Contains(output, "BASE64") {
		t.Fatalf("preview: %s", output)
	}
	if output := mcpOutputPreview(strings.Repeat("长", 7000)); !strings.HasSuffix(output, "…（预览已截断）") || len([]rune(output)) > 6020 {
		t.Fatal("unbounded preview")
	}
}

func TestMCPActivityEventsAndHistory(t *testing.T) {
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
	model := &fakeModel{replies: []*completion{{ToolCalls: []toolCall{{ID: "call-echo", Name: "mcp_0", Arguments: json.RawMessage(`{}`)}}}, {Content: "done"}}}
	var activities []Activity
	err = svc.loop(context.Background(), nil, model, history, nil, func(event Event) {
		if event.Activity != nil {
			activities = append(activities, *event.Activity)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != 2 || activities[0].Tool != "fixture/echo" || activities[0].Title != "调用 echo" || activities[0].Status != "running" || activities[1].Output != "fixture result" {
		t.Fatalf("events: %+v", activities)
	}
	saved, err := svc.Get(history.ID)
	if err != nil || len(saved.Activity) != 1 || saved.Activity[0].Tool != "fixture/echo" || saved.Activity[0].Output != "fixture result" {
		t.Fatalf("history: %+v %v", saved, err)
	}
	if len(model.messages) < 2 || model.messages[1][len(model.messages[1])-1]["tool_call_id"] != "call-echo" {
		t.Fatal("tool call ID changed")
	}
}
