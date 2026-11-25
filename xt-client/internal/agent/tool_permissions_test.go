package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmployeeToolPermissionsFilterAndEnforce(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "allowed.txt"), []byte("允许读取"), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(project)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	session := &Session{Project: Project{Path: project}, Member: &MemberIdentity{ToolPermissions: &ToolPermissions{Read: true}}}
	model := &recordingModel{replies: []*completion{{ToolCalls: []toolCall{
		{ID: "read", Name: "read_file", Arguments: json.RawMessage(`{"path":"allowed.txt"}`)},
		{ID: "write", Name: "write_file", Arguments: json.RawMessage(`{"path":"blocked.txt","content":"不能写"}`)},
		{ID: "cmd", Name: "run_command", Arguments: json.RawMessage(`{"command":"echo blocked"}`)},
		{ID: "mcp", Name: mcpEnableToolName, Arguments: json.RawMessage(`{}`)},
	}}}}
	if err := svc.loopWithRun(t.Context(), workspace, model, session, nil, nil, "run", &RunStats{}, nil, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if !hasTool(model.toolSets[0], "read_file") {
		t.Fatal("允许的读文件工具未提供")
	}
	for _, tool := range []string{"write_file", "run_command", mcpEnableToolName} {
		if hasTool(model.toolSets[0], tool) {
			t.Errorf("模型仍能看到已禁用工具 %s", tool)
		}
	}
	for _, message := range session.Messages {
		if message.Role != "tool" {
			continue
		}
		if message.ToolCallID == "read" && !strings.Contains(message.Content, "允许读取") ||
			message.ToolCallID != "read" && message.Content != "员工没有调用此工具的权限" {
			t.Errorf("工具权限执行结果不符合配置: %+v", message)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("未经允许的写入发生了: %v", err)
	}
	session.Member.ToolPermissions = nil
	defaultTools := toolsForTurn(workspace, session, nil, &RunStats{}, &lazyMCP{})
	coordinator := toolsForTurn(workspace, &Session{}, nil, &RunStats{}, &lazyMCP{})
	if strings.Join(toolNames(defaultTools), ",") != strings.Join(toolNames(coordinator), ",") {
		t.Fatal("默认员工工具与小涛不一致")
	}
	session.ReadOnly = true
	if len(toolsForTurn(workspace, session, nil, &RunStats{}, &lazyMCP{})) != 0 {
		t.Fatal("普通闲聊开放了工具")
	}
}
