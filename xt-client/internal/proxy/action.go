package proxy

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func detectAction(msgs []map[string]any, tools []map[string]any) (string, string) {
	if len(msgs) == 0 {
		return "用户", "对话交互"
	}

	// 1. Check if the latest message is a tool execution result
	last := msgs[len(msgs)-1]
	role, _ := last["role"].(string)

	if role == "tool" {
		name, _ := last["name"].(string)
		args := ""
		if name == "" {
			callID, _ := last["tool_call_id"].(string)
			name, args = findToolByCallID(msgs, callID)
		}
		if name != "" {
			return classifyTool(name, args)
		}
	}

	// Check for tool_result block in user message
	if callID := extractToolResultCallID(last["content"]); callID != "" {
		name, args := findToolByCallID(msgs, callID)
		if name != "" {
			return classifyTool(name, args)
		}
	}

	// 2. If the last message is a user prompt, extract the specific user request
	if role == "user" {
		txt := extractContentText(last["content"])
		sum := cleanSummary(txt)
		if sum != "" {
			return "用户", sum
		}
	}

	// 3. Scan backwards for the latest meaningful action
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		r, _ := m["role"].(string)
		if r == "assistant" {
			if tcList, ok := m["tool_calls"].([]any); ok && len(tcList) > 0 {
				if tc, ok := tcList[0].(map[string]any); ok {
					if fn, ok := tc["function"].(map[string]any); ok {
						name, _ := fn["name"].(string)
						args, _ := fn["arguments"].(string)
						return classifyTool(name, args)
					}
				}
			}
		}
		if r == "tool" {
			name, _ := m["name"].(string)
			args := ""
			if name == "" {
				callID, _ := m["tool_call_id"].(string)
				name, args = findToolByCallID(msgs, callID)
			}
			if name != "" {
				return classifyTool(name, args)
			}
		}
		if r == "user" {
			txt := extractContentText(m["content"])
			sum := cleanSummary(txt)
			if sum != "" {
				return "用户", sum
			}
		}
	}

	txt := extractContentText(last["content"])
	sum := cleanSummary(txt)
	if sum != "" {
		return "用户", sum
	}
	return "用户", "对话交互"
}

func classifyTool(name, args string) (string, string) {
	cleanName := name
	if idx := strings.LastIndex(name, "__"); idx >= 0 && idx < len(name)-2 {
		cleanName = name[idx+2:]
	}
	cleanLow := strings.ToLower(cleanName)
	target := extractPathOrTarget(args)

	if strings.Contains(cleanLow, "read") || strings.Contains(cleanLow, "view") ||
		strings.Contains(cleanLow, "cat") || strings.Contains(cleanLow, "grep") ||
		strings.Contains(cleanLow, "search") || strings.Contains(cleanLow, "list") ||
		strings.Contains(cleanLow, "get") {
		if target != "" {
			return "读", "分析: " + target
		}
		return "读", "分析项目文件"
	}
	if strings.Contains(cleanLow, "write") || strings.Contains(cleanLow, "edit") ||
		strings.Contains(cleanLow, "patch") || strings.Contains(cleanLow, "apply") ||
		strings.Contains(cleanLow, "save") || strings.Contains(cleanLow, "create") {
		if target != "" {
			return "写", "更新: " + target
		}
		return "写", "更新代码文件"
	}
	if strings.Contains(cleanLow, "exec") || strings.Contains(cleanLow, "cmd") ||
		strings.Contains(cleanLow, "bash") || strings.Contains(cleanLow, "sh") ||
		strings.Contains(cleanLow, "terminal") || strings.Contains(cleanLow, "run") {
		if target != "" {
			return "执行", target
		}
		return "执行", "执行命令"
	}
	if cleanName != "" {
		return "工具", cleanName
	}
	return "工具", "工具调用"
}

func extractPathOrTarget(args string) string {
	if args == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) == nil {
		if patchInput, ok := m["input"].(string); ok {
			for _, prefix := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
				if idx := strings.Index(patchInput, prefix); idx >= 0 {
					sub := patchInput[idx+len(prefix):]
					if endIdx := strings.Index(sub, "\n"); endIdx > 0 {
						fn := strings.TrimSpace(sub[:endIdx])
						return filepath.Base(fn)
					}
				}
			}
		}
		for _, k := range []string{"path", "file", "filename", "cmd", "command", "query", "url"} {
			if v, ok := m[k].(string); ok && v != "" {
				if strings.Contains(v, "/") || strings.Contains(v, "\\") {
					v = filepath.Base(v)
				}
				runes := []rune(v)
				if len(runes) > 28 {
					v = string(runes[:28]) + "..."
				}
				return v
			}
		}
	}
	return ""
}

func findToolByCallID(msgs []map[string]any, callID string) (string, string) {
	if callID == "" {
		return "", ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if tcList, ok := m["tool_calls"].([]any); ok {
			for _, item := range tcList {
				if tc, ok := item.(map[string]any); ok {
					if id, _ := tc["id"].(string); id == callID {
						if fn, ok := tc["function"].(map[string]any); ok {
							name, _ := fn["name"].(string)
							args, _ := fn["arguments"].(string)
							return name, args
						}
					}
				}
			}
		}
	}
	return "", ""
}

func extractToolResultCallID(content any) string {
	if list, ok := content.([]any); ok {
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if t, _ := m["type"].(string); t == "tool_result" {
					if id, ok := m["tool_use_id"].(string); ok && id != "" {
						return id
					}
				}
			}
		}
	}
	return ""
}

func extractContentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if txt, ok := m["text"].(string); ok && txt != "" {
					return txt
				}
			}
		}
	}
	return ""
}

func cleanSummary(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if idx := strings.Index(s, "## My request:"); idx >= 0 {
		s = strings.TrimSpace(s[idx+len("## My request:"):])
	} else if idx := strings.Index(s, "My request:"); idx >= 0 {
		s = strings.TrimSpace(s[idx+len("My request:"):])
	}

	lines := strings.Split(s, "\n")
	found := ""
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		cleaned := strings.TrimLeft(trimmed, "#*-`> \t")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}
		if strings.HasPrefix(cleaned, "Files mentioned") ||
			strings.HasPrefix(cleaned, "Distinguish instructions") ||
			strings.HasPrefix(cleaned, "Image attachment") ||
			strings.HasPrefix(cleaned, "Response annotations") {
			continue
		}
		found = cleaned
		break
	}

	if found == "" {
		found = s
	}

	found = strings.TrimSpace(found)
	runes := []rune(found)
	if len(runes) > 30 {
		return string(runes[:30]) + "..."
	}
	return found
}
