package agent

import (
	"encoding/json"
	"strings"
)

func activityTitle(name string) string {
	switch name {
	case "list_files":
		return "查看项目结构"
	case "search_files":
		return "搜索代码"
	case "read_file":
		return "阅读文件"
	case "write_file":
		return "更新文件"
	case "str_replace":
		return "替换文件内容"
	case "run_command":
		return "运行命令"
	case "read_historical_tool_result":
		return "回查历史工具输出"
	case mcpEnableToolName:
		return "启用浏览器工具"
	default:
		return "调用工具"
	}
}

func activityDetail(args json.RawMessage) string {
	var value map[string]any
	if json.Unmarshal(args, &value) != nil {
		return ""
	}
	for _, key := range []string{"path", "command", "query"} {
		if text, ok := value[key].(string); ok && text != "" {
			runes := []rune(text)
			if len(runes) > 80 {
				return string(runes[:80]) + "..."
			}
			return text
		}
	}
	return ""
}

func localActivityInput(name string, args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var value map[string]any
	if json.Unmarshal(args, &value) != nil {
		return string(args)
	}
	switch name {
	case "read_file":
		var input struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if json.Unmarshal(args, &input) == nil {
			value = map[string]any{"path": input.Path}
			if input.Offset > 0 {
				value["offset"] = input.Offset
			}
			if input.Limit > 0 {
				value["limit"] = input.Limit
			}
		}
	case "write_file":
		var input struct {
			Path    string `json:"path"`
			Content string `json:"content"`
			Append  bool   `json:"append"`
		}
		if json.Unmarshal(args, &input) == nil {
			value = map[string]any{"path": input.Path, "content_bytes": len(input.Content), "append": input.Append}
		}
	case "str_replace":
		var input struct {
			Path   string `json:"path"`
			OldStr string `json:"old_str"`
			NewStr string `json:"new_str"`
		}
		if json.Unmarshal(args, &input) == nil {
			value = map[string]any{"path": input.Path, "old_str_bytes": len(input.OldStr), "new_str_bytes": len(input.NewStr)}
		}
	case "run_command":
		var input struct {
			Command    string `json:"command"`
			TimeoutSec int    `json:"timeout_sec"`
		}
		if json.Unmarshal(args, &input) == nil {
			value = map[string]any{"command": input.Command}
			if input.TimeoutSec > 0 {
				value["timeout_sec"] = input.TimeoutSec
			}
		}
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(args)
	}
	return string(pretty)
}

func outputPreview(output string) string {
	return activityPreview(strings.TrimSpace(output), 6000)
}
