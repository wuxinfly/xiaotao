package agent

import (
	"context"
	"fmt"
	"strings"
)

// MCP tool schemas are expensive. The built-in Chrome DevTools server alone
// ships 32 tools worth roughly 8.5k tokens, and every model request would
// carry them even when the run never touches a browser. XiaoTao therefore
// starts each run with one small activation tool and only adds the real
// schemas once the model actually asks for them.

const mcpEnableToolName = "enable_mcp_tools"

const mcpEnableToolDescription = "启用浏览器与 MCP 工具。内置 Chrome DevTools 浏览器工具（打开网页、读取页面结构、点击、输入、检查控制台与网络）以及设置中配置的 MCP 服务在此之前不可用。调用后下一步会出现具体工具列表。"

const mcpEnabledToolDescription = "浏览器与 MCP 工具已经启用，具体工具见工具列表。只有需要重新确认可用工具时才调用它。"

func mcpEnableToolDefinition(description string) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        mcpEnableToolName,
			"description": description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

// historyUsesMCPTools reports whether the conversation already contains an
// MCP tool call. Such a run must expose the MCP tool names again, because
// the stored tool messages reference them by name.
func historyUsesMCPTools(messages []map[string]any) bool {
	for _, message := range messages {
		if name, _ := message["name"].(string); isMCPToolName(name) {
			return true
		}
	}
	return false
}

// isMCPToolName reports whether a tool call targets an MCP server tool.
func isMCPToolName(name string) bool {
	return strings.HasPrefix(name, "mcp_")
}

// lazyMCP keeps the configured MCP servers closed until a run needs them.
type lazyMCP struct {
	connections map[string]*mcpConnection
	tools       []map[string]any
	enabled     bool
}

func (m *lazyMCP) close() {
	closeMCPConnections(m.connections)
	m.connections = nil
	m.tools = nil
}

func (m *lazyMCP) connection(name string) *mcpConnection {
	if m.connections == nil {
		return nil
	}
	return m.connections[name]
}

// definitions returns the tool schemas advertised to the model right now.
// The activation tool stays visible so a later turn never references a tool
// the model was not offered.
func (m *lazyMCP) definitions() []map[string]any {
	description := mcpEnableToolDescription
	if m.enabled {
		description = mcpEnabledToolDescription
	}
	return append([]map[string]any{mcpEnableToolDefinition(description)}, m.tools...)
}

func (m *lazyMCP) aliases() []string {
	names := make([]string, 0, len(m.tools))
	for _, definition := range m.tools {
		function, _ := definition["function"].(map[string]any)
		alias, _ := function["name"].(string)
		if alias != "" {
			names = append(names, alias)
		}
	}
	return names
}

// enableMCP starts the configured MCP servers once and summarises what appeared.
func (s *Service) enableMCP(ctx context.Context, state *lazyMCP) (string, error) {
	if state.enabled {
		return summarizeMCPTools(state.tools), nil
	}
	connections, tools, err := s.openMCP(ctx)
	if err != nil {
		return "", fmt.Errorf("MCP 启动失败: %w", err)
	}
	state.connections = connections
	state.tools = tools
	state.enabled = true
	return summarizeMCPTools(tools), nil
}

// enableMCPTools activates the MCP servers behind the user approval that
// already guards every MCP tool call.
func (s *Service) enableMCPTools(ctx context.Context, state *lazyMCP, session string, workspace *Workspace, emit func(Event)) (string, error) {
	if err := s.waitApproval(ctx, session, workspace, toolCall{Name: mcpEnableToolName, Arguments: []byte(`{}`)}, emit); err != nil {
		return "", err
	}
	return s.enableMCP(ctx, state)
}

// resolveMCPTool returns the connection backing an MCP tool call, starting
// the servers on first use.
func (s *Service) resolveMCPTool(ctx context.Context, state *lazyMCP, call toolCall) (*mcpConnection, error) {
	if connection := state.connection(call.Name); connection != nil {
		return connection, nil
	}
	if _, err := s.enableMCP(ctx, state); err != nil {
		return nil, err
	}
	connection := state.connection(call.Name)
	if connection == nil {
		return nil, fmt.Errorf("未找到 MCP 工具 %s，可用工具：%s", call.Name, strings.Join(state.aliases(), "、"))
	}
	return connection, nil
}

// runMCPTool executes one MCP tool call after the caller resolved its connection.
func (s *Service) runMCPTool(ctx context.Context, session string, workspace *Workspace, call toolCall, connection *mcpConnection, emit func(Event)) (string, error) {
	if err := s.waitApproval(ctx, session, workspace, call, emit); err != nil {
		return "", err
	}
	return callMCP(ctx, connection, call.Name, call.Arguments)
}

func summarizeMCPTools(tools []map[string]any) string {
	if len(tools) == 0 {
		return "MCP 服务已启动，但没有暴露可用工具。"
	}
	return fmt.Sprintf("已启用 %d 个 MCP 工具，工具列表已更新，可以直接调用。", len(tools))
}
