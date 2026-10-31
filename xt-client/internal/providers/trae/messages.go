package trae

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
)

// Ported from buddy-proxy trae/text_protocol._content_blocks: the native
// chat channel requires content to be an array of typed blocks; a plain
// OpenAI string must be wrapped, image_url blocks passed through, and the
// result must never be empty.
func contentBlocks(content any) []map[string]any {
	switch v := content.(type) {
	case string:
		return []map[string]any{{"type": "text", "text": v}}
	case []any:
		out := []map[string]any{}
		for _, p := range v {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			switch pm["type"] {
			case "text":
				if text, _ := pm["text"].(string); text != "" {
					out = append(out, map[string]any{"type": "text", "text": text})
				}
			case "image_url":
				switch iu := pm["image_url"].(type) {
				case map[string]any:
					if url, _ := iu["url"].(string); url != "" {
						out = append(out, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
					}
				case string:
					if iu != "" {
						out = append(out, map[string]any{"type": "image_url", "image_url": map[string]any{"url": iu}})
					}
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []map[string]any{{"type": "text", "text": ""}}
}

// Ported from buddy-proxy trae/native_tools._native_messages: structured
// history conversion. assistant.tool_calls become the function_call shape
// the upstream accepts; tool results keep the tool role with tool_call_id.
func NativeMessages(messages []map[string]any) []map[string]any {
	out := []map[string]any{}
	var pendingImages []map[string]any
	flushImages := func() {
		if len(pendingImages) > 0 {
			blocks := append([]map[string]any{{"type": "text", "text": "[images returned by the tool calls above]"}}, pendingImages...)
			out = append(out, map[string]any{"role": "user", "content": blocks})
			pendingImages = nil
		}
	}
	for _, m := range messages {
		role, _ := m["role"].(string)
		if role != "tool" && role != "function" {
			flushImages()
		}
		content := contentBlocks(m["content"])
		toolCalls, _ := m["tool_calls"].([]any)
		if role == "assistant" && len(toolCalls) > 0 {
			tcs := []map[string]any{}
			for j, c := range toolCalls {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				fn, _ := cm["function"].(map[string]any)
				if fn == nil {
					fn = cm
				}
				args, _ := fn["arguments"].(string)
				if args == "" {
					if raw, err := json.Marshal(fn["arguments"]); err == nil && len(raw) > 0 {
						args = string(raw)
					}
				}
				id, _ := cm["id"].(string)
				if id == "" {
					id = "call_" + randHex()
				}
				name, _ := fn["name"].(string)
				tcs = append(tcs, map[string]any{
					"index":         j,
					"id":            id,
					"type":          "function",
					"function_call": map[string]any{"name": name, "arguments": args},
				})
			}
			out = append(out, map[string]any{"role": "assistant", "content": content, "tool_calls": tcs})
			continue
		}
		if role == "tool" || role == "function" {
			images := []map[string]any{}
			kept := []map[string]any{}
			for _, b := range content {
				if b["type"] == "image_url" {
					images = append(images, b)
				} else {
					kept = append(kept, b)
				}
			}
			if len(images) > 0 {
				kept = append(kept, map[string]any{"type": "text", "text": "[image attached in the following user message]"})
				pendingImages = append(pendingImages, images...)
			}
			toolCallID, _ := m["tool_call_id"].(string)
			name, _ := m["name"].(string)
			if name == "" {
				name = "tool"
			}
			out = append(out, map[string]any{
				"role": "tool", "tool_call_id": toolCallID, "name": name, "content": kept,
			})
			continue
		}
		out = append(out, map[string]any{"role": role, "content": content})
	}
	flushImages()
	return out
}

func randHex() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
