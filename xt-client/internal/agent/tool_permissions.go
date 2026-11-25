package agent

import "time"

// ToolPermissions is an allowlist for named employees. Nil inherits the
// coordinator's tool availability and approval rules.
type ToolPermissions struct {
	Inherit bool `json:"inherit,omitempty" yaml:"inherit,omitempty"`
	Read    bool `json:"read" yaml:"read"`
	Write   bool `json:"write" yaml:"write"`
	Command bool `json:"command" yaml:"command"`
	MCP     bool `json:"mcp" yaml:"mcp"`
}

func employeeToolPermissions(session *Session) *ToolPermissions {
	if session.Member == nil {
		return nil
	}
	return session.Member.ToolPermissions
}

func (p *ToolPermissions) allows(tool string) bool {
	if p == nil || p.Inherit {
		return true
	}
	switch tool {
	case "list_files", "search_files", "read_file", "read_historical_tool_result", "read_conversation_reference":
		return p.Read
	case "write_file", "str_replace":
		return p.Write
	case "run_command":
		return p.Command
	case mcpEnableToolName:
		return p.MCP
	default:
		return p.MCP && isMCPToolName(tool)
	}
}

func toolsForTurn(workspace *Workspace, history *Session, references []ConversationReference, stats *RunStats, mcp *lazyMCP) []map[string]any {
	if history.ReadOnly {
		return nil
	}
	var tools []map[string]any
	if workspace != nil {
		tools = append(tools, toolDefinitions...)
	}
	if len(references) > 0 {
		tools = append(tools, referenceToolDefinition)
	}
	if hasPrunedToolResults(history.Messages, stats.UserMessageID) {
		tools = append(tools, historicalToolLookupDefinition)
	}
	tools = append(tools, mcp.definitions()...)
	if permissions := employeeToolPermissions(history); permissions != nil {
		allowed := tools[:0]
		for _, tool := range tools {
			function, _ := tool["function"].(map[string]any)
			name, _ := function["name"].(string)
			if permissions.allows(name) {
				allowed = append(allowed, tool)
			}
		}
		return allowed
	}
	return tools
}

func recordToolResult(history *Session, messages []map[string]any, call toolCall, output string, end time.Time) []map[string]any {
	history.Messages = append(history.Messages, Message{
		ID: "msg_" + randomID(6), Role: "tool", Content: output,
		ToolCallID: call.ID, ToolName: call.Name, CreatedAt: end,
	})
	return append(messages, map[string]any{
		"role": "tool", "tool_call_id": call.ID, "name": call.Name, "content": output,
	})
}
