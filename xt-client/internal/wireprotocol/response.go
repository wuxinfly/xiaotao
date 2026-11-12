package wireprotocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func ToChatResponse(protocol string, b Object) (Object, error) {
	if protocol == Chat {
		return b, nil
	}
	if b["error"] != nil {
		return nil, errors.New("upstream protocol error")
	}
	message := Object{"role": "assistant", "content": ""}
	var texts, reasoning string
	var calls []any
	finish := "stop"
	if protocol == Responses {
		if b["status"] != "completed" && b["status"] != "incomplete" {
			return nil, errors.New("incomplete Responses result")
		}
		if b["status"] == "incomplete" {
			finish = "length"
		}
		for _, raw := range List(b["output"]) {
			item := Obj(raw)
			switch item["type"] {
			case "message":
				for _, raw := range List(item["content"]) {
					p := Obj(raw)
					switch p["type"] {
					case "output_text":
						texts += Str(p["text"])
					case "refusal":
						message["refusal"] = p["refusal"]
					default:
						return nil, errors.New("unsupported Responses output content")
					}
				}
			case "function_call":
				calls = append(calls, Object{"id": item["call_id"], "type": "function", "function": Object{"name": item["name"], "arguments": item["arguments"]}})
			case "reasoning":
				for _, raw := range List(item["summary"]) {
					reasoning += Str(Obj(raw)["text"])
				}
			default:
				return nil, errors.New("unsupported Responses output item")
			}
		}
	} else {
		if b["type"] != "message" || b["stop_reason"] == nil {
			return nil, errors.New("incomplete Messages result")
		}
		finish = EndReason(Str(b["stop_reason"]))
		for _, raw := range List(b["content"]) {
			p := Obj(raw)
			switch p["type"] {
			case "text":
				texts += Str(p["text"])
			case "thinking":
				reasoning += Str(p["thinking"])
			case "tool_use":
				calls = append(calls, Object{"id": p["id"], "type": "function", "function": Object{"name": p["name"], "arguments": marshal(p["input"])}})
			default:
				return nil, fmt.Errorf("unsupported Messages output block %q", Str(p["type"]))
			}
		}
	}
	message["content"] = texts
	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	if len(calls) > 0 {
		message["tool_calls"] = calls
		if finish != "length" {
			finish = "tool_calls"
		}
	}
	out := Object{"id": b["id"], "object": "chat.completion", "model": b["model"], "choices": []any{Object{"index": 0, "message": message, "finish_reason": finish}}}
	if u := ChatUsage(protocol, Obj(b["usage"])); u != nil {
		out["usage"] = u
	}
	return out, nil
}

func FromChatResponse(protocol string, b Object) (Object, error) {
	choices := List(b["choices"])
	if len(choices) != 1 {
		return nil, errors.New("exactly one completion choice is required")
	}
	c := Obj(choices[0])
	msg := Obj(c["message"])
	reason := Str(c["finish_reason"])
	if reason == "" || reason == "error" {
		return nil, errors.New("incomplete chat completion")
	}
	id := Str(b["id"])
	model := b["model"]
	var content, output []any
	if protocol == Messages {
		if text := Str(msg["content"]); text != "" {
			content = append(content, Object{"type": "text", "text": text})
		}
		if text := Str(msg["refusal"]); text != "" {
			content = append(content, Object{"type": "text", "text": text})
		}
		for _, raw := range List(msg["tool_calls"]) {
			tool := Obj(raw)
			fn := Obj(tool["function"])
			var input any
			if json.Unmarshal([]byte(Str(fn["arguments"])), &input) != nil {
				return nil, errors.New("invalid function arguments")
			}
			content = append(content, Object{"type": "tool_use", "id": tool["id"], "name": fn["name"], "input": input})
		}
		// Reasoning from a different protocol is not a signed Anthropic thinking block.
		return Object{"id": id, "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": NativeReason(reason), "stop_sequence": nil, "usage": NativeUsage(protocol, Obj(b["usage"]))}, nil
	}
	if text := Str(msg["reasoning_content"]); text != "" {
		output = append(output, Object{"id": id + "_reasoning", "type": "reasoning", "summary": []any{Object{"type": "summary_text", "text": text}}})
	}
	if text := Str(msg["content"]); text != "" {
		output = append(output, Object{"id": id + "_message", "type": "message", "role": "assistant", "status": "completed", "content": []any{Object{"type": "output_text", "text": text, "annotations": []any{}}}})
	}
	if refusal := Str(msg["refusal"]); refusal != "" {
		output = append(output, Object{"id": id + "_refusal", "type": "message", "role": "assistant", "status": "completed", "content": []any{Object{"type": "refusal", "refusal": refusal}}})
	}
	for i, raw := range List(msg["tool_calls"]) {
		tool := Obj(raw)
		fn := Obj(tool["function"])
		output = append(output, Object{"id": fmt.Sprintf("%s_tool_%d", id, i), "type": "function_call", "status": "completed", "call_id": tool["id"], "name": fn["name"], "arguments": fn["arguments"]})
	}
	if output == nil {
		output = []any{}
	}
	out := Object{"id": id, "object": "response", "created_at": time.Now().Unix(), "status": "completed", "model": model, "output": output, "error": nil, "incomplete_details": nil, "usage": NativeUsage(protocol, Obj(b["usage"])), "store": false}
	if reason == "length" {
		out["status"] = "incomplete"
		out["incomplete_details"] = Object{"reason": "max_output_tokens"}
	}
	return out, nil
}
