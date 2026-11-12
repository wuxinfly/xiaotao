package wireprotocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func ToChatRequest(protocol string, b Object) (Object, error) {
	if protocol == Chat {
		return b, nil
	}
	if err := reject(b, "previous_response_id", "conversation", "background", "thinking", "context_management", "service_tier", "include", "truncation", "top_k", "output_config"); err != nil {
		return nil, err
	}
	out := Object{}
	copyFields(out, b, "model", "stream", "temperature", "top_p", "metadata", "user", "parallel_tool_calls")
	var messages []any
	if protocol == Responses {
		if b["store"] == true {
			return nil, errors.New("stored Responses are not supported; send full input history with store:false")
		}
		if v := b["instructions"]; v != nil {
			text, err := textParts(v)
			if err != nil {
				return nil, err
			}
			messages = append(messages, Object{"role": "system", "content": text})
		}
		input := b["input"]
		if s, ok := input.(string); ok {
			input = []any{Object{"role": "user", "content": s}}
		}
		for _, raw := range List(input) {
			item, err := responseInput(Obj(raw))
			if err != nil {
				return nil, err
			}
			messages = append(messages, item)
		}
		if v := b["max_output_tokens"]; v != nil {
			out["max_tokens"] = v
		}
		if effort := Obj(b["reasoning"])["effort"]; effort != nil {
			out["reasoning_effort"] = effort
		}
		if format := Obj(b["text"])["format"]; format != nil {
			out["response_format"] = responseFormatToChat(Obj(format))
		}
	} else {
		if v := b["system"]; v != nil {
			text, err := textParts(v)
			if err != nil {
				return nil, err
			}
			messages = append(messages, Object{"role": "system", "content": text})
		}
		for _, raw := range List(b["messages"]) {
			items, err := anthropicInput(Obj(raw))
			if err != nil {
				return nil, err
			}
			messages = append(messages, items...)
		}
		copyFields(out, b, "max_tokens")
		if v := b["stop_sequences"]; v != nil {
			out["stop"] = v
		}
	}
	if len(messages) == 0 {
		return nil, errors.New("input/messages must contain at least one message")
	}
	out["messages"] = messages
	if err := toolsToChat(protocol, b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func responseInput(item Object) (Object, error) {
	switch Str(item["type"]) {
	case "function_call":
		return Object{"role": "assistant", "content": nil, "tool_calls": []any{Object{"id": item["call_id"], "type": "function", "function": Object{"name": item["name"], "arguments": item["arguments"]}}}}, nil
	case "function_call_output":
		content, err := contentToChat(Responses, item["output"])
		return Object{"role": "tool", "tool_call_id": item["call_id"], "content": content}, err
	case "", "message":
		role := Str(item["role"])
		if role == "" {
			return nil, errors.New("message role is required")
		}
		content, err := contentToChat(Responses, item["content"])
		return Object{"role": role, "content": content}, err
	default:
		return nil, fmt.Errorf("unsupported Responses input item %q", Str(item["type"]))
	}
}

func anthropicInput(item Object) ([]any, error) {
	role := Str(item["role"])
	if role != "user" && role != "assistant" {
		return nil, errors.New("Messages role must be user or assistant")
	}
	if s, ok := item["content"].(string); ok {
		return []any{Object{"role": role, "content": s}}, nil
	}
	var result, parts, tools []any
	flush := func() {
		if len(parts) > 0 || len(tools) > 0 {
			msg := Object{"role": role, "content": parts}
			if len(tools) > 0 {
				msg["tool_calls"] = tools
			}
			result = append(result, msg)
			parts = nil
			tools = nil
		}
	}
	for _, raw := range List(item["content"]) {
		part := Obj(raw)
		switch Str(part["type"]) {
		case "tool_use":
			tools = append(tools, Object{"id": part["id"], "type": "function", "function": Object{"name": part["name"], "arguments": marshal(part["input"])}})
		case "tool_result":
			flush()
			content, err := contentToChat(Messages, part["content"])
			if err != nil {
				return nil, err
			}
			result = append(result, Object{"role": "tool", "tool_call_id": part["tool_use_id"], "content": content})
		default:
			content, err := contentToChat(Messages, []any{part})
			if err != nil {
				return nil, err
			}
			parts = append(parts, List(content)...)
		}
	}
	flush()
	return result, nil
}

func contentToChat(protocol string, content any) (any, error) {
	if s, ok := content.(string); ok {
		return s, nil
	}
	var result []any
	for _, raw := range List(content) {
		p := Obj(raw)
		switch Str(p["type"]) {
		case "text", "input_text", "output_text":
			result = append(result, Object{"type": "text", "text": p["text"]})
		case "input_image":
			image := Object{"url": p["image_url"]}
			if p["detail"] != nil {
				image["detail"] = p["detail"]
			}
			result = append(result, Object{"type": "image_url", "image_url": image})
		case "image":
			src := Obj(p["source"])
			url := Str(src["url"])
			if src["type"] == "base64" {
				url = "data:" + Str(src["media_type"]) + ";base64," + Str(src["data"])
			}
			if url == "" {
				return nil, errors.New("image source must be URL or base64")
			}
			result = append(result, Object{"type": "image_url", "image_url": Object{"url": url}})
		default:
			return nil, fmt.Errorf("unsupported %s content block %q", protocol, Str(p["type"]))
		}
	}
	return result, nil
}

func toolsToChat(protocol string, b, out Object) error {
	var tools []any
	for _, raw := range List(b["tools"]) {
		tool := Obj(raw)
		if protocol == Responses && tool["type"] != "function" {
			return errors.New("only function tools are supported")
		}
		if protocol == Messages && tool["type"] != nil && tool["type"] != "custom" {
			return errors.New("only custom function tools are supported")
		}
		fn := Object{}
		copyFields(fn, tool, "name", "description", "parameters", "strict")
		if protocol == Messages {
			fn["parameters"] = tool["input_schema"]
		}
		tools = append(tools, Object{"type": "function", "function": fn})
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	choice := b["tool_choice"]
	if choice == nil {
		return nil
	}
	if s, ok := choice.(string); ok {
		out["tool_choice"] = s
		return nil
	}
	c := Obj(choice)
	switch Str(c["type"]) {
	case "auto", "none":
		out["tool_choice"] = c["type"]
	case "any":
		out["tool_choice"] = "required"
	case "tool", "function":
		out["tool_choice"] = Object{"type": "function", "function": Object{"name": c["name"]}}
	default:
		return errors.New("unsupported tool choice")
	}
	if v, ok := c["disable_parallel_tool_use"].(bool); ok {
		out["parallel_tool_calls"] = !v
	}
	return nil
}

func FromChatRequest(protocol string, b Object) (Object, error) {
	if protocol == Chat {
		return b, nil
	}
	if n, ok := number(b["n"]); ok && n > 1 {
		return nil, errors.New("multiple choices are not supported by this upstream protocol")
	}
	if err := reject(b, "audio", "logprobs", "top_logprobs", "prediction", "seed", "logit_bias", "presence_penalty", "frequency_penalty", "modalities", "functions", "function_call", "web_search_options"); err != nil {
		return nil, err
	}
	if protocol == Messages {
		if err := reject(b, "reasoning_effort", "response_format"); err != nil {
			return nil, err
		}
	}
	if protocol == Responses {
		if err := reject(b, "stop"); err != nil {
			return nil, err
		}
	}
	out := Object{}
	copyFields(out, b, "model", "stream", "temperature", "top_p", "metadata")
	var items, systems []any
	for _, raw := range List(b["messages"]) {
		msg := Obj(raw)
		role := Str(msg["role"])
		if role == "system" || role == "developer" {
			text, err := textParts(msg["content"])
			if err != nil {
				return nil, err
			}
			systems = append(systems, Object{"type": "text", "text": text})
			continue
		}
		converted, err := chatMessage(protocol, msg)
		if err != nil {
			return nil, err
		}
		items = append(items, converted...)
	}
	if protocol == Responses {
		out["input"], out["store"] = items, false
		var text []string
		for _, raw := range systems {
			text = append(text, Str(Obj(raw)["text"]))
		}
		if len(text) > 0 {
			out["instructions"] = strings.Join(text, "\n")
		}
		copyFields(out, b, "parallel_tool_calls", "user")
		if effort := b["reasoning_effort"]; effort != nil {
			out["reasoning"] = Object{"effort": effort}
		}
		if v := b["response_format"]; v != nil {
			out["text"] = Object{"format": responseFormatFromChat(Obj(v))}
		}
	} else {
		out["messages"] = items
		if len(systems) > 0 {
			out["system"] = systems
		}
		if v := b["stop"]; v != nil {
			if s, ok := v.(string); ok {
				out["stop_sequences"] = []any{s}
			} else {
				out["stop_sequences"] = v
			}
		}
	}
	limit := b["max_completion_tokens"]
	if limit == nil {
		limit = b["max_tokens"]
	}
	if protocol == Responses {
		if limit != nil {
			out["max_output_tokens"] = limit
		}
	} else {
		if limit == nil {
			limit = 1024
		}
		out["max_tokens"] = limit
	}
	if err := toolsFromChat(protocol, b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func chatMessage(protocol string, msg Object) ([]any, error) {
	role := Str(msg["role"])
	if role == "tool" {
		content := msg["content"]
		if _, text := content.(string); !text {
			converted, err := contentFromChat(protocol, content, "user")
			if err != nil {
				return nil, err
			}
			content = converted
		}
		if protocol == Responses {
			return []any{Object{"type": "function_call_output", "call_id": msg["tool_call_id"], "output": content}}, nil
		}
		return []any{Object{"role": "user", "content": []any{Object{"type": "tool_result", "tool_use_id": msg["tool_call_id"], "content": content}}}}, nil
	}
	content, err := contentFromChat(protocol, msg["content"], role)
	if err != nil {
		return nil, err
	}
	var result []any
	if len(content) > 0 {
		result = append(result, Object{"role": role, "content": content})
	}
	for _, raw := range List(msg["tool_calls"]) {
		tool := Obj(raw)
		fn := Obj(tool["function"])
		if protocol == Responses {
			result = append(result, Object{"type": "function_call", "call_id": tool["id"], "name": fn["name"], "arguments": fn["arguments"]})
		} else {
			var input any
			if json.Unmarshal([]byte(Str(fn["arguments"])), &input) != nil {
				return nil, errors.New("tool arguments must be valid JSON")
			}
			content = append(content, Object{"type": "tool_use", "id": tool["id"], "name": fn["name"], "input": input})
		}
	}
	if protocol == Messages {
		return []any{Object{"role": role, "content": content}}, nil
	}
	return result, nil
}

func contentFromChat(protocol string, v any, role string) ([]any, error) {
	if s, ok := v.(string); ok {
		v = []any{Object{"type": "text", "text": s}}
	}
	var out []any
	for _, raw := range List(v) {
		p := Obj(raw)
		switch p["type"] {
		case "text":
			kind := "text"
			if protocol == Responses {
				kind = "input_text"
				if role == "assistant" {
					kind = "output_text"
				}
			}
			out = append(out, Object{"type": kind, "text": p["text"]})
		case "image_url":
			image := Obj(p["image_url"])
			url := Str(image["url"])
			if protocol == Responses {
				part := Object{"type": "input_image", "image_url": url}
				copyFields(part, image, "detail")
				out = append(out, part)
			} else {
				source := Object{"type": "url", "url": url}
				if strings.HasPrefix(url, "data:") {
					meta, data, ok := strings.Cut(strings.TrimPrefix(url, "data:"), ";base64,")
					if !ok {
						return nil, errors.New("image data URL must use base64")
					}
					source = Object{"type": "base64", "media_type": meta, "data": data}
				}
				out = append(out, Object{"type": "image", "source": source})
			}
		default:
			return nil, errors.New("unsupported Chat content for selected upstream")
		}
	}
	return out, nil
}

func toolsFromChat(protocol string, b, out Object) error {
	var tools []any
	for _, raw := range List(b["tools"]) {
		tool := Obj(raw)
		if tool["type"] != "function" {
			return errors.New("only function tools are supported")
		}
		fn := Obj(tool["function"])
		native := Object{}
		copyFields(native, fn, "name", "description")
		if protocol == Responses {
			native["type"] = "function"
			copyFields(native, fn, "parameters", "strict")
		} else {
			native["input_schema"] = fn["parameters"]
		}
		tools = append(tools, native)
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	c := b["tool_choice"]
	if c != nil {
		if s, ok := c.(string); ok {
			if protocol == Responses {
				out["tool_choice"] = s
			} else {
				if s == "required" {
					s = "any"
				}
				out["tool_choice"] = Object{"type": s}
			}
		} else {
			fn := Obj(Obj(c)["function"])
			kind := "function"
			if protocol == Messages {
				kind = "tool"
			}
			out["tool_choice"] = Object{"type": kind, "name": fn["name"]}
		}
	}
	if protocol == Messages {
		if parallel, ok := b["parallel_tool_calls"].(bool); ok {
			choice := Obj(out["tool_choice"])
			if choice == nil {
				choice = Object{"type": "auto"}
				out["tool_choice"] = choice
			}
			choice["disable_parallel_tool_use"] = !parallel
		}
	}
	return nil
}
