package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MapUsage converts Responses usage structure to OpenAI Chat usage format.
func MapUsage(usage any) map[string]any {
	u, ok := usage.(map[string]any)
	if !ok || u == nil {
		return nil
	}
	mapped := map[string]any{}

	var promptTokens, completionTokens int64
	var hasPrompt, hasCompletion bool

	if pt, ok := toInt64(u["input_tokens"]); ok {
		promptTokens = pt
		hasPrompt = true
		mapped["prompt_tokens"] = pt
		if details, ok := u["input_tokens_details"].(map[string]any); ok {
			if cached, ok := toInt64(details["cached_tokens"]); ok {
				mapped["prompt_tokens_details"] = map[string]any{"cached_tokens": cached}
				mapped["cached_tokens"] = cached
			}
		}
	}

	if ct, ok := toInt64(u["output_tokens"]); ok {
		completionTokens = ct
		hasCompletion = true
		mapped["completion_tokens"] = ct
		if details, ok := u["output_tokens_details"].(map[string]any); ok {
			if reasoning, ok := toInt64(details["reasoning_tokens"]); ok {
				mapped["completion_tokens_details"] = map[string]any{"reasoning_tokens": reasoning}
			}
		}
	}

	if tt, ok := toInt64(u["total_tokens"]); ok {
		mapped["total_tokens"] = tt
	} else if hasPrompt && hasCompletion {
		mapped["total_tokens"] = promptTokens + completionTokens
	}

	return mapped
}

func toInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int64:
		return val, true
	case float64:
		return int64(val), true
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i, true
		}
	}
	return 0, false
}

func isErrorEvent(eventName string, event map[string]any) (int, string) {
	if eventName == "error" {
		code := event["code"]
		if code == nil {
			code = event["status"]
		}
		msg := ""
		if m, ok := event["message"].(string); ok {
			msg = m
		} else if m, ok := event["detail"].(string); ok {
			msg = m
		}
		return statusFromCode(code), msg
	}

	if eventName == "response.failed" || eventName == "response.incomplete" {
		response, _ := event["response"].(map[string]any)
		if response == nil {
			response = event
		}
		if errObj, ok := response["error"].(map[string]any); ok {
			code := errObj["code"]
			if code == nil {
				code = errObj["status"]
			}
			msg, _ := errObj["message"].(string)
			return statusFromCode(code), msg
		}
		if eventName == "response.incomplete" {
			if details, ok := response["incomplete_details"].(map[string]any); ok {
				reason, _ := details["reason"].(string)
				if reason == "max_output_tokens" {
					return 0, ""
				}
				return 500, "incomplete: " + reason
			}
		}
	}
	return 0, ""
}

func statusFromCode(code any) int {
	if n, ok := toInt64(code); ok && n >= 400 && n <= 599 {
		return int(n)
	}
	text := strings.ToLower(fmt.Sprintf("%v", code))
	if strings.Contains(text, "rate_limit") || strings.Contains(text, "rate limit") ||
		strings.Contains(text, "quota") || strings.Contains(text, "usage_limit") {
		return 429
	}
	if strings.Contains(text, "unauthorized") || strings.Contains(text, "invalid_api_key") ||
		strings.Contains(text, "auth") {
		return 401
	}
	if strings.Contains(text, "forbidden") || strings.Contains(text, "permission") {
		return 403
	}
	return 500
}

func outputTextFromItem(item map[string]any) string {
	content := item["content"]
	if s, ok := content.(string); ok {
		return s
	}
	var sb strings.Builder
	if list, ok := content.([]any); ok {
		for _, part := range list {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := m["type"].(string)
			if kind == "output_text" || kind == "text" || kind == "input_text" {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			} else if kind == "refusal" {
				if r, ok := m["refusal"].(string); ok {
					sb.WriteString(r)
				}
			}
		}
	}
	return sb.String()
}

func reasoningTextFromItem(item map[string]any) string {
	var sb strings.Builder
	if summaryList, ok := item["summary"].([]any); ok {
		for _, part := range summaryList {
			if m, ok := part.(map[string]any); ok {
				kind, _ := m["type"].(string)
				if kind == "summary_text" || kind == "text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
	}
	if contentList, ok := item["content"].([]any); ok {
		for _, part := range contentList {
			if m, ok := part.(map[string]any); ok {
				kind, _ := m["type"].(string)
				if kind == "reasoning_text" || kind == "text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
	}
	return sb.String()
}

// ResponsesToChatCompletion converts a final Responses response object to OpenAI chat completion format.
func ResponsesToChatCompletion(response map[string]any, model string) map[string]any {
	var textParts []string
	var reasoningParts []string
	var toolCalls []map[string]any

	if output, ok := response["output"].([]any); ok {
		for _, itemVal := range output {
			item, ok := itemVal.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := item["type"].(string)
			switch kind {
			case "message":
				textParts = append(textParts, outputTextFromItem(item))
			case "reasoning":
				reasoningParts = append(reasoningParts, reasoningTextFromItem(item))
			case "function_call":
				name, _ := item["name"].(string)
				callID, _ := item["call_id"].(string)
				if callID == "" {
					callID, _ = item["id"].(string)
				}
				if callID == "" {
					callID = "call_" + randHex(12)
				}
				args, _ := item["arguments"].(string)
				if args == "" {
					args = "{}"
				}
				toolCalls = append(toolCalls, map[string]any{
					"id":   callID,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": args,
					},
				})
			}
		}
	}

	text := strings.Join(textParts, "")
	reasoning := strings.Join(reasoningParts, "")

	message := map[string]any{"role": "assistant"}
	if text != "" {
		message["content"] = text
	} else if reasoning != "" {
		message["content"] = reasoning
		reasoning = ""
	} else {
		message["content"] = nil
	}

	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	if status, _ := response["status"].(string); status == "incomplete" {
		if details, ok := response["incomplete_details"].(map[string]any); ok {
			if reason, _ := details["reason"].(string); reason == "max_output_tokens" {
				finishReason = "length"
			}
		}
	}

	id, _ := response["id"].(string)
	if id == "" {
		id = "chatcmpl-" + randHex(12)
	}

	res := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
	}

	if u := MapUsage(response["usage"]); u != nil {
		res["usage"] = u
	}
	return res
}

// StreamState folds Responses SSE events into Chat delta chunks.
type StreamState struct {
	model         string
	started       bool
	finished      bool
	nextToolIndex int
	toolIndex     map[string]int
	toolBuf       map[string][]string
	reasoningBuf  []string
	reasoningSent bool
	usage         map[string]any
	finishReason  string
}

func NewStreamState(model string) *StreamState {
	return &StreamState{
		model:        model,
		toolIndex:    map[string]int{},
		toolBuf:      map[string][]string{},
		finishReason: "stop",
	}
}

func (s *StreamState) Feed(eventName string, event map[string]any) []map[string]any {
	if s.finished {
		return nil
	}
	var out []map[string]any

	switch eventName {
	case "response.output_text.delta":
		delta, _ := event["delta"].(string)
		if delta != "" {
			out = append(out, map[string]any{"content": delta})
		}
	case "response.reasoning_summary_text.delta":
		delta, _ := event["delta"].(string)
		if delta != "" {
			out = append(out, map[string]any{"reasoning_content": delta})
		}
	case "response.output_item.added":
		item, ok := event["item"].(map[string]any)
		if ok && item["type"] == "function_call" {
			name, _ := item["name"].(string)
			itemID, _ := item["call_id"].(string)
			if itemID == "" {
				itemID, _ = item["id"].(string)
			}
			if itemID == "" {
				itemID = "fc_" + randHex(12)
			}
			idx := s.nextToolIndex
			s.nextToolIndex++
			s.toolIndex[itemID] = idx
			out = append(out, map[string]any{
				"tool_calls": []any{
					map[string]any{
						"index": idx,
						"id":    itemID,
						"type":  "function",
						"function": map[string]any{
							"name":      name,
							"arguments": "",
						},
					},
				},
			})
		}
	case "response.function_call_arguments.delta":
		itemID, _ := event["item_id"].(string)
		delta, _ := event["delta"].(string)
		if delta != "" && itemID != "" {
			s.toolBuf[itemID] = append(s.toolBuf[itemID], delta)
		}
	case "response.output_item.done":
		item, ok := event["item"].(map[string]any)
		if ok && item["type"] == "function_call" {
			itemID, _ := item["call_id"].(string)
			if itemID == "" {
				itemID, _ = item["id"].(string)
			}
			idx, ok := s.toolIndex[itemID]
			if ok {
				args := strings.Join(s.toolBuf[itemID], "")
				delete(s.toolBuf, itemID)
				if args == "" {
					args, _ = item["arguments"].(string)
				}
				if args != "" {
					out = append(out, map[string]any{
						"tool_calls": []any{
							map[string]any{
								"index": idx,
								"function": map[string]any{
									"arguments": args,
								},
							},
						},
					})
				}
			}
		}
	case "response.completed":
		resp, _ := event["response"].(map[string]any)
		if resp == nil {
			resp = event
		}
		if u := MapUsage(resp["usage"]); u != nil {
			s.usage = u
		}
		if len(s.toolIndex) > 0 {
			s.finishReason = "tool_calls"
		} else {
			s.finishReason = "stop"
		}
		s.finished = true
	case "response.failed", "response.incomplete":
		s.finished = true
		s.finishReason = "stop"
	}

	return out
}

func (s *StreamState) FinalUsage() map[string]any {
	return s.usage
}

func (s *StreamState) FinishReason() string {
	return s.finishReason
}
