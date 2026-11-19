package gateway

import (
	"fmt"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/usage"
)

// Keep bounded labels and token estimates without copying message text.
func contextItems(messages, tools []map[string]any) map[string][]usage.ContextItem {
	items := map[string][]usage.ContextItem{}
	add := func(group, label string, tokens int64) {
		if tokens == 0 {
			return
		}
		groupItems := items[group]
		if len(groupItems) == 8 {
			groupItems[7].Label = "其余条目"
			groupItems[7].Tokens += tokens
			return
		}
		items[group] = append(groupItems, usage.ContextItem{Label: label, Tokens: tokens})
	}
	for i, tool := range tools {
		fn, _ := tool["function"].(map[string]any)
		label := shortTitle(textValue(fn["name"]))
		if label == "" {
			label = fmt.Sprintf("工具 %d", i+1)
		}
		size := agent.EstimateRequestContext(nil, tools[i:i+1])
		add("tools", label, size.ToolsTokens)
	}
	for i, msg := range messages {
		role := textValue(msg["role"])
		size := agent.EstimateRequestContext(messages[i:i+1], nil)
		labels := map[string]string{"system": "系统", "developer": "开发者", "user": "用户", "assistant": "助手", "tool": "工具"}
		label := fmt.Sprintf("%s消息 %d", labels[role], i+1)
		add("system", label, size.SystemTokens)
		add("messages", label, size.MessagesTokens)
		add("toolCalls", label, size.ToolResultsTokens)
		add("references", label, size.ReferenceTokens)
		add("images", label, size.ImageTokens)
	}
	return items
}
