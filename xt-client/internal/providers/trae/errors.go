package trae

import "fmt"

func errText(data map[string]any) string {
	code, _ := data["code"].(string)
	msg, _ := data["message"].(string)
	if code == "" {
		if f, ok := data["code"].(float64); ok {
			code = fmt.Sprintf("%.0f", f)
		}
	}
	hints := map[string]string{
		"1005": "Trae plan 权益不足",
		"3003": "Trae 模型暂不可用或额度用完",
		"4008": "Trae 账号请求额度已超限",
		"4011": "Trae 今日问答用量已达上限",
		"4010": "Trae 检测到风险账号，请重新登录",
		"4031": "Trae 今日请求额度已用完",
	}
	text := hints[code]
	if text == "" {
		if msg != "" {
			return "Trae 错误: " + msg
		}
		return "Trae 未知错误"
	}
	if msg != "" && msg != text {
		text += "（" + msg + "）"
	}
	if code != "" {
		text += fmt.Sprintf(" (code: %s)", code)
	}
	return text
}
