package codex

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const (
	defaultBaseURL      = "https://chatgpt.com/backend-api/codex"
	originator          = "codex-tui"
	userAgent           = "codex-tui/0.154.0 (Mac OS 26.5.2; arm64) iTerm.app/3.6.11 (codex-tui; 0.154.0)"
	minMaxOutputTokens = 16
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeBaseURL(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		text = defaultBaseURL
	}
	text = strings.TrimRight(text, "/")
	if strings.HasSuffix(text, "/responses") {
		text = strings.TrimSuffix(text, "/responses")
	}
	return text
}

func buildHeaders(accessToken, accountID, sessionID string, stream bool) map[string]string {
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	h := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
		"Originator":    originator,
		"User-Agent":    userAgent,
		"Accept":        accept,
	}
	if accountID != "" {
		h["Chatgpt-Account-Id"] = accountID
	}
	if sessionID != "" {
		h["Session-Id"] = sessionID
	}
	return h
}

func textOf(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if parts, ok := content.([]any); ok {
		var sb strings.Builder
		for _, part := range parts {
			if s, ok := part.(string); ok {
				sb.WriteString(s)
			} else if m, ok := part.(map[string]any); ok {
				kind, _ := m["type"].(string)
				if kind == "text" || kind == "input_text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
		return sb.String()
	}
	return ""
}

func chatImages(content any) []map[string]any {
	var out []map[string]any
	parts, ok := content.([]any)
	if !ok {
		return out
	}
	for _, part := range parts {
		m, ok := part.(map[string]any)
		if !ok || m["type"] != "image_url" {
			continue
		}
		image := m["image_url"]
		url := ""
		if imgMap, ok := image.(map[string]any); ok {
			url, _ = imgMap["url"].(string)
		} else if imgStr, ok := image.(string); ok {
			url = imgStr
		}
		url = strings.TrimSpace(url)
		if url != "" {
			out = append(out, map[string]any{
				"type":      "input_image",
				"image_url": url,
			})
		}
	}
	return out
}

func userContent(content any) any {
	images := chatImages(content)
	if len(images) == 0 {
		return textOf(content)
	}
	var parts []any
	txt := textOf(content)
	if txt != "" {
		parts = append(parts, map[string]any{
			"type": "input_text",
			"text": txt,
		})
	}
	for _, img := range images {
		parts = append(parts, img)
	}
	return parts
}

func assistantContent(content any) any {
	images := chatImages(content)
	if len(images) == 0 {
		return textOf(content)
	}
	var parts []any
	txt := textOf(content)
	if txt != "" {
		parts = append(parts, map[string]any{
			"type": "output_text",
			"text": txt,
		})
	}
	for _, img := range images {
		parts = append(parts, img)
	}
	return parts
}

func convertTools(tools any) []map[string]any {
	var out []map[string]any
	list, ok := tools.([]any)
	if !ok {
		if mapList, ok := tools.([]map[string]any); ok {
			list = make([]any, len(mapList))
			for i, v := range mapList {
				list[i] = v
			}
		} else {
			return out
		}
	}
	for _, t := range list {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["type"].(string)
		if kind == "function" {
			fn, _ := m["function"].(map[string]any)
			if fn == nil {
				continue
			}
			name, _ := fn["name"].(string)
			if name == "" {
				continue
			}
			desc, _ := fn["description"].(string)
			params := fn["parameters"]
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			strict, _ := fn["strict"].(bool)
			out = append(out, map[string]any{
				"type":        "function",
				"name":        name,
				"description": desc,
				"parameters":  params,
				"strict":       strict,
			})
		} else if kind == "web_search" || kind == "web_search_preview" {
			out = append(out, map[string]any{"type": "web_search"})
		}
	}
	return out
}

func convertToolChoice(choice any) any {
	if s, ok := choice.(string); ok {
		switch s {
		case "auto", "none", "required":
			return s
		default:
			return "auto"
		}
	}
	if m, ok := choice.(map[string]any); ok {
		if fn, ok := m["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				return map[string]any{"type": "function", "name": name}
			}
		}
	}
	return "auto"
}

func convertMessages(messages any) (string, []map[string]any) {
	var systemTexts []string
	var items []map[string]any

	var list []any
	if rawList, ok := messages.([]any); ok {
		list = rawList
	} else if mapList, ok := messages.([]map[string]any); ok {
		list = make([]any, len(mapList))
		for i, v := range mapList {
			list[i] = v
		}
	} else {
		return "", items
	}

	for _, item := range list {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "" {
			role = "user"
		}
		content := msg["content"]

		if role == "system" || role == "developer" {
			txt := textOf(content)
			if txt != "" {
				systemTexts = append(systemTexts, txt)
			}
			continue
		}

		if role == "tool" {
			callID, _ := msg["tool_call_id"].(string)
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  textOf(content),
			})
			continue
		}

		if role == "assistant" {
			txt := textOf(content)
			if txt != "" {
				items = append(items, map[string]any{
					"type":    "message",
					"role":    "assistant",
					"content": assistantContent(content),
				})
			}
			var toolCalls []any
			if tcList, ok := msg["tool_calls"].([]any); ok {
				toolCalls = tcList
			} else if tcMaps, ok := msg["tool_calls"].([]map[string]any); ok {
				toolCalls = make([]any, len(tcMaps))
				for i, v := range tcMaps {
					toolCalls[i] = v
				}
			}
			for _, callItem := range toolCalls {
				call, ok := callItem.(map[string]any)
				if !ok {
					continue
				}
				fn, _ := call["function"].(map[string]any)
				if fn == nil {
					continue
				}
				name, _ := fn["name"].(string)
				if name == "" {
					continue
				}
				callID, _ := call["id"].(string)
				args, _ := fn["arguments"].(string)
				if args == "" {
					args = "{}"
				}
				items = append(items, map[string]any{
					"type":      "function_call",
					"call_id":   callID,
					"name":      name,
					"arguments": args,
				})
			}
			continue
		}

		// user and others
		txt := textOf(content)
		imgs := chatImages(content)
		if txt == "" && len(imgs) == 0 {
			continue
		}
		items = append(items, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": userContent(content),
		})
	}

	return strings.Join(systemTexts, "\n\n"), items
}

func reasoningConfig(body map[string]any) map[string]any {
	effort, _ := body["reasoning_effort"].(string)
	if effort == "" {
		if rMap, ok := body["reasoning"].(map[string]any); ok {
			effort, _ = rMap["effort"].(string)
		}
	}
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return nil
	}
	return map[string]any{
		"effort":  effort,
		"summary": "auto",
	}
}

// BuildUpstreamBody projects an OpenAI chat completion body into Responses request body.
func BuildUpstreamBody(body map[string]any, model string) map[string]any {
	instructions, items := convertMessages(body["messages"])
	if items == nil {
		items = []map[string]any{}
	}
	upstreamModel := model
	if upstreamModel == "" {
		upstreamModel, _ = body["model"].(string)
	}

	upstream := map[string]any{
		"model":  upstreamModel,
		"input":  items,
		"stream": true,
		"store":  false,
	}

	var instructionParts []string
	if explicit, ok := body["instructions"].(string); ok && strings.TrimSpace(explicit) != "" {
		instructionParts = append(instructionParts, strings.TrimSpace(explicit))
	}
	if instructions != "" && (len(instructionParts) == 0 || instructionParts[0] != instructions) {
		instructionParts = append(instructionParts, instructions)
	}
	upstream["instructions"] = strings.Join(instructionParts, "\n\n")

	if tools := convertTools(body["tools"]); len(tools) > 0 {
		upstream["tools"] = tools
		upstream["tool_choice"] = convertToolChoice(body["tool_choice"])
		if ptc, ok := body["parallel_tool_calls"].(bool); ok {
			upstream["parallel_tool_calls"] = ptc
		}
	}

	if reasoning := reasoningConfig(body); reasoning != nil {
		upstream["reasoning"] = reasoning
	}

	var maxTokens int
	if mt, ok := body["max_completion_tokens"].(float64); ok && mt > 0 {
		maxTokens = int(mt)
	} else if mt, ok := body["max_tokens"].(float64); ok && mt > 0 {
		maxTokens = int(mt)
	} else if mt, ok := body["max_tokens"].(int); ok && mt > 0 {
		maxTokens = mt
	} else if mt, ok := body["max_completion_tokens"].(int); ok && mt > 0 {
		maxTokens = mt
	}
	if maxTokens > 0 {
		if maxTokens < minMaxOutputTokens {
			maxTokens = minMaxOutputTokens
		}
		upstream["max_output_tokens"] = maxTokens
	}

	for _, key := range []string{"temperature", "top_p", "user", "service_tier"} {
		if val, exists := body[key]; exists && val != nil {
			upstream[key] = val
		}
	}

	return upstream
}
