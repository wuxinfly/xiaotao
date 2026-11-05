package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

var browserActivityTitles = map[string]string{
	"new_page": "打开网页", "navigate_page": "跳转网页", "list_pages": "查看浏览器页面", "select_page": "切换网页", "close_page": "关闭网页",
	"take_snapshot": "读取网页结构", "take_screenshot": "截取网页画面", "click": "点击网页元素", "dblclick": "双击网页元素", "hover": "悬停网页元素", "drag": "拖动网页元素",
	"fill": "填写网页输入框", "fill_form": "填写网页表单", "press_key": "按下键盘按键", "upload_file": "上传文件", "handle_dialog": "处理网页对话框",
	"evaluate_script": "执行网页脚本", "list_console_messages": "检查控制台消息", "get_console_message": "查看控制台消息",
	"list_network_requests": "检查网络请求", "get_network_request": "查看网络请求详情", "get_css_styles": "查看网页样式", "wait_for": "等待网页内容",
	"resize_page": "调整浏览器窗口", "emulate": "模拟浏览器环境", "performance_start_trace": "开始性能录制", "performance_stop_trace": "结束性能录制", "performance_analyze_insight": "分析网页性能",
	"lighthouse_audit": "检查网页质量", "screencast_start": "开始网页录屏", "screencast_stop": "结束网页录屏", "screencast": "网页录屏",
}

func activityPreview(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…（预览已截断）"
	}
	return text
}

func describeMCPActivity(activity *Activity, connection *mcpConnection, call toolCall) {
	name := connection.names[call.Name]
	activity.Tool = connection.server + "/" + name
	activity.Title = browserActivityTitles[name]
	if activity.Title == "" {
		activity.Title = "调用 " + name
	}
	activity.Detail = activity.Tool
	var args map[string]any
	if json.Unmarshal(call.Arguments, &args) == nil {
		for _, key := range []string{"url", "pageId", "uid", "key", "type"} {
			if value, ok := args[key]; ok && value != nil {
				activity.Detail = activityPreview(key+": "+fmt.Sprint(value), 120)
				break
			}
		}
	}
	var value any
	if json.Unmarshal(call.Arguments, &value) == nil {
		if pretty, err := json.MarshalIndent(value, "", "  "); err == nil {
			activity.Input = activityPreview(string(pretty), 6000)
		}
	}
}

func mcpOutputPreview(output string) string {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal([]byte(output), &result) == nil && len(result.Content) > 0 {
		parts := make([]string, 0, len(result.Content))
		for _, part := range result.Content {
			if part.Type == "text" {
				parts = append(parts, part.Text)
			} else {
				parts = append(parts, "（"+part.Type+" 内容，未展开）")
			}
		}
		return activityPreview(strings.Join(parts, "\n\n"), 6000)
	}
	return activityPreview(output, 6000)
}
