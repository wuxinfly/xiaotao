package wireprotocol

import (
	"errors"
	"fmt"
)

// Decoder turns upstream protocol events into canonical Chat chunks. A clean
// EOF is not success: only the protocol's explicit terminal event finishes it.
type Decoder struct {
	Protocol string
	Done     bool
	finish   string
	tools    map[int]int
	usage    Object
}

func (d *Decoder) Event(b Object) ([]Object, error) {
	if len(d.tools) > 1023 {
		return nil, errors.New("too many upstream tool calls")
	}
	if d.Done {
		return nil, errors.New("event after protocol completion")
	}
	if b["type"] == "error" || b["error"] != nil {
		return nil, errors.New("upstream streaming error")
	}
	if d.Protocol == Responses {
		return d.responses(b)
	}
	return d.messages(b)
}
func (d *Decoder) messages(b Object) ([]Object, error) {
	switch b["type"] {
	case "message_start":
		d.usage = ChatUsage(Messages, Obj(Obj(b["message"])["usage"]))
		start := chunk(Object{"role": "assistant"}, nil)
		if d.usage != nil {
			start["usage"] = d.usage
		}
		return []Object{start}, nil
	case "content_block_start":
		p := Obj(b["content_block"])
		index, _ := number(b["index"])
		switch p["type"] {
		case "tool_use":
			if d.tools == nil {
				d.tools = map[int]int{}
			}
			toolIndex := len(d.tools)
			d.tools[int(index)] = toolIndex
			args := ""
			if input := Obj(p["input"]); len(input) > 0 {
				args = marshal(input)
			}
			return []Object{chunk(Object{"tool_calls": []any{Object{"index": toolIndex, "id": p["id"], "type": "function", "function": Object{"name": p["name"], "arguments": args}}}}, nil)}, nil
		case "text":
			if text := Str(p["text"]); text != "" {
				return []Object{chunk(Object{"content": text}, nil)}, nil
			}
		case "thinking":
		default:
			return nil, errors.New("unsupported upstream content block")
		}
	case "content_block_delta":
		p := Obj(b["delta"])
		switch p["type"] {
		case "text_delta":
			return []Object{chunk(Object{"content": p["text"]}, nil)}, nil
		case "thinking_delta":
			return []Object{chunk(Object{"reasoning_content": p["thinking"]}, nil)}, nil
		case "signature_delta":
			return nil, nil
		case "input_json_delta":
			index, _ := number(b["index"])
			i, ok := d.tools[int(index)]
			if !ok {
				return nil, errors.New("tool delta without start")
			}
			return []Object{chunk(Object{"tool_calls": []any{Object{"index": i, "function": Object{"arguments": p["partial_json"]}}}}, nil)}, nil
		default:
			return nil, errors.New("unsupported upstream content delta")
		}
	case "message_delta":
		if reason := Str(Obj(b["delta"])["stop_reason"]); reason != "" {
			d.finish = EndReason(reason)
		}
		if d.usage == nil {
			d.usage = Object{}
		}
		for k, v := range ChatUsage(Messages, Obj(b["usage"])) {
			d.usage[k] = v
		}
		if in, ok := number(d.usage["prompt_tokens"]); ok {
			if out, ok := number(d.usage["completion_tokens"]); ok {
				d.usage["total_tokens"] = in + out
			}
		}
		return []Object{{"choices": []any{}, "usage": d.usage}}, nil
	case "message_stop":
		if d.finish == "" {
			return nil, errors.New("message stopped without stop reason")
		}
		d.Done = true
		return []Object{chunk(Object{}, d.finish)}, nil
	case "ping", "content_block_stop":
	default:
		return nil, fmt.Errorf("unsupported Messages event %q", Str(b["type"]))
	}
	return nil, nil
}
func (d *Decoder) responses(b Object) ([]Object, error) {
	switch b["type"] {
	case "response.created", "response.in_progress":
		return nil, nil
	case "response.output_text.delta":
		return []Object{chunk(Object{"content": b["delta"]}, nil)}, nil
	case "response.refusal.delta":
		return []Object{chunk(Object{"refusal": b["delta"]}, nil)}, nil
	case "response.reasoning_summary_text.delta":
		return []Object{chunk(Object{"reasoning_content": b["delta"]}, nil)}, nil
	case "response.output_item.added":
		item := Obj(b["item"])
		if item["type"] == "function_call" {
			if d.tools == nil {
				d.tools = map[int]int{}
			}
			index, _ := number(b["output_index"])
			i := len(d.tools)
			d.tools[int(index)] = i
			return []Object{chunk(Object{"tool_calls": []any{Object{"index": i, "id": item["call_id"], "type": "function", "function": Object{"name": item["name"], "arguments": Str(item["arguments"])}}}}, nil)}, nil
		}
	case "response.function_call_arguments.delta":
		index, _ := number(b["output_index"])
		i, ok := d.tools[int(index)]
		if !ok {
			return nil, errors.New("function arguments without item")
		}
		return []Object{chunk(Object{"tool_calls": []any{Object{"index": i, "function": Object{"arguments": b["delta"]}}}}, nil)}, nil
	case "response.completed", "response.incomplete":
		response := Obj(b["response"])
		if response["status"] != "completed" && response["status"] != "incomplete" {
			return nil, errors.New("invalid Responses terminal status")
		}
		finish := "stop"
		if len(d.tools) > 0 {
			finish = "tool_calls"
		}
		if response["status"] == "incomplete" {
			finish = "length"
		}
		d.Done = true
		return []Object{{"choices": []any{}, "usage": ChatUsage(Responses, Obj(response["usage"]))}, chunk(Object{}, finish)}, nil
	case "response.failed", "error":
		return nil, errors.New("upstream Responses failed")
	case "response.refusal.done", "response.output_text.done", "response.content_part.added", "response.content_part.done", "response.output_item.done", "response.function_call_arguments.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.done":
	default:
		return nil, fmt.Errorf("unsupported Responses event %q", Str(b["type"]))
	}
	return nil, nil
}
