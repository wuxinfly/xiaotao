package wireprotocol

import (
	"errors"
	"fmt"
	"time"
)

type Event struct {
	Name string
	Data Object
}
type streamPart struct {
	kind, text, id, name, args string
	index                      int
	started                    bool
}

// Encoder emits native incremental events while retaining only bounded content
// needed for Responses' final response object.
type Encoder struct {
	Protocol string
	id       string
	model    any
	started  bool
	finish   string
	usage    Object
	parts    []*streamPart
	tools    map[int]*streamPart
	bytes    int
	sequence int
	created  int64
}

func (e *Encoder) event(name string, b Object) Event {
	b["type"] = name
	if e.Protocol == Responses {
		b["sequence_number"] = e.sequence
		e.sequence++
	}
	return Event{Name: name, Data: b}
}
func (e *Encoder) start() []Event {
	if e.started {
		return nil
	}
	e.started = true
	e.created = time.Now().Unix()
	if e.Protocol == Messages {
		return []Event{e.event("message_start", Object{"message": Object{"id": e.id, "type": "message", "role": "assistant", "model": e.model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": NativeUsage(Messages, e.usage)}})}
	}
	response := e.response("in_progress")
	return []Event{e.event("response.created", Object{"response": response}), e.event("response.in_progress", Object{"response": response})}
}
func (e *Encoder) Push(b Object) ([]Event, error) {
	if b["error"] != nil {
		return nil, errors.New("chat stream failed")
	}
	if e.id == "" {
		e.id = Str(b["id"])
		e.model = b["model"]
	}
	if u := Obj(b["usage"]); u != nil {
		if e.usage == nil {
			e.usage = Object{}
		}
		for k, v := range u {
			e.usage[k] = v
		}
	}
	events := e.start()
	for _, raw := range List(b["choices"]) {
		c := Obj(raw)
		if index, _ := number(c["index"]); index != 0 {
			return nil, errors.New("native protocols support one choice")
		}
		delta := Obj(c["delta"])
		for _, field := range []string{"content", "reasoning_content", "refusal"} {
			text := Str(delta[field])
			if text == "" {
				continue
			}
			if field == "reasoning_content" && e.Protocol == Messages {
				continue
			}
			kind := field
			if field == "refusal" && e.Protocol == Messages {
				kind = "content"
			}
			part := e.textPart(kind)
			part.text += text
			e.bytes += len(text)
			events = append(events, e.openPart(part)...)
			if e.Protocol == Messages {
				events = append(events, e.event("content_block_delta", Object{"index": part.index, "delta": Object{"type": "text_delta", "text": text}}))
			} else {
				name := "response.output_text.delta"
				body := Object{"item_id": part.id, "output_index": part.index, "content_index": 0, "delta": text}
				if field == "refusal" {
					name = "response.refusal.delta"
				}
				if field == "reasoning_content" {
					name = "response.reasoning_summary_text.delta"
					delete(body, "content_index")
					body["summary_index"] = 0
				}
				events = append(events, e.event(name, body))
			}
		}
		for _, raw := range List(delta["tool_calls"]) {
			tool := Obj(raw)
			index, valid := number(tool["index"])
			if !valid || index > 1023 || len(e.parts) > 1023 {
				return nil, errors.New("too many or invalid tool indices")
			}
			part := e.toolPart(int(index))
			fn := Obj(tool["function"])
			if part.started {
				if value := Str(fn["name"]); value != "" && value != part.name {
					return nil, errors.New("tool name changed after arguments started")
				}
				if value := Str(tool["id"]); value != "" && value != part.id {
					return nil, errors.New("tool ID changed after arguments started")
				}
			} else {
				part.id += Str(tool["id"])
				part.name += Str(fn["name"])
			}
			args := Str(fn["arguments"])
			part.args += args
			e.bytes += len(args) + len(Str(fn["name"])) + len(Str(tool["id"]))
			if part.id != "" && part.name != "" {
				events = append(events, e.openPart(part)...)
			}
			if args != "" && part.started {
				if e.Protocol == Messages {
					events = append(events, e.event("content_block_delta", Object{"index": part.index, "delta": Object{"type": "input_json_delta", "partial_json": args}}))
				} else {
					events = append(events, e.event("response.function_call_arguments.delta", Object{"item_id": e.itemID(part), "output_index": part.index, "delta": args}))
				}
			}
		}
		if reason := Str(c["finish_reason"]); reason != "" {
			e.finish = reason
		}
	}
	if e.bytes > MaxContent {
		return nil, errors.New("native response exceeds content limit")
	}
	return events, nil
}
func (e *Encoder) textPart(kind string) *streamPart {
	for _, part := range e.parts {
		if part.kind == kind {
			return part
		}
	}
	part := &streamPart{kind: kind, index: len(e.parts), id: fmt.Sprintf("%s_item_%d", e.id, len(e.parts))}
	e.parts = append(e.parts, part)
	return part
}
func (e *Encoder) toolPart(index int) *streamPart {
	if e.tools == nil {
		e.tools = map[int]*streamPart{}
	}
	if part := e.tools[index]; part != nil {
		return part
	}
	part := &streamPart{kind: "tool", index: len(e.parts)}
	e.parts = append(e.parts, part)
	e.tools[index] = part
	return part
}
func (e *Encoder) itemID(p *streamPart) string {
	if p.kind == "tool" {
		return fmt.Sprintf("%s_item_%d", e.id, p.index)
	}
	return p.id
}
func (e *Encoder) item(p *streamPart, status string) Object {
	switch p.kind {
	case "tool":
		return Object{"id": e.itemID(p), "type": "function_call", "call_id": p.id, "name": p.name, "arguments": p.args, "status": status}
	case "refusal":
		return Object{"id": p.id, "type": "message", "role": "assistant", "status": status, "content": []any{Object{"type": "refusal", "refusal": p.text}}}
	case "reasoning_content":
		return Object{"id": p.id, "type": "reasoning", "summary": []any{Object{"type": "summary_text", "text": p.text}}}
	default:
		return Object{"id": p.id, "type": "message", "role": "assistant", "status": status, "content": []any{Object{"type": "output_text", "text": p.text, "annotations": []any{}}}}
	}
}
func (e *Encoder) openPart(p *streamPart) []Event {
	if p.started {
		return nil
	}
	p.started = true
	if e.Protocol == Messages {
		block := Object{"type": "text", "text": ""}
		if p.kind == "tool" {
			block = Object{"type": "tool_use", "id": p.id, "name": p.name, "input": Object{}}
		}
		return []Event{e.event("content_block_start", Object{"index": p.index, "content_block": block})}
	}
	item := e.item(p, "in_progress")
	if p.kind == "tool" {
		item["arguments"] = ""
	} else if p.kind == "content" || p.kind == "refusal" {
		item["content"] = []any{}
	} else {
		item["summary"] = []any{}
	}
	events := []Event{e.event("response.output_item.added", Object{"output_index": p.index, "item": item})}
	if p.kind == "content" || p.kind == "refusal" {
		events = append(events, e.event("response.content_part.added", Object{"item_id": p.id, "output_index": p.index, "content_index": 0, "part": e.emptyTextPart(p)}))
	}
	if p.kind == "reasoning_content" {
		events = append(events, e.event("response.reasoning_summary_part.added", Object{"item_id": p.id, "output_index": p.index, "summary_index": 0, "part": Object{"type": "summary_text", "text": ""}}))
	}
	return events
}
func (e *Encoder) response(status string) Object {
	output := []any{}
	if status != "in_progress" {
		for _, part := range e.parts {
			output = append(output, e.item(part, "completed"))
		}
	}
	out := Object{"id": e.id, "object": "response", "created_at": e.created, "status": status, "model": e.model, "output": output, "usage": NativeUsage(Responses, e.usage), "error": nil, "incomplete_details": nil, "store": false}
	if status == "incomplete" {
		out["incomplete_details"] = Object{"reason": "max_output_tokens"}
	}
	return out
}
func (e *Encoder) Finish() ([]Event, error) {
	if e.finish == "" || e.finish == "error" {
		return nil, errors.New("stream ended without completion")
	}
	var events []Event
	for _, p := range e.parts {
		if p.kind == "tool" && (p.id == "" || p.name == "") {
			return nil, errors.New("incomplete tool identity")
		}
		events = append(events, e.openPart(p)...)
		if e.Protocol == Messages {
			events = append(events, e.event("content_block_stop", Object{"index": p.index}))
			continue
		}
		body := Object{"item_id": e.itemID(p), "output_index": p.index}
		switch p.kind {
		case "tool":
			body["arguments"] = p.args
			events = append(events, e.event("response.function_call_arguments.done", body))
		case "content", "refusal":
			body["content_index"], body["text"] = 0, p.text
			name := "response.output_text.done"
			if p.kind == "refusal" {
				name = "response.refusal.done"
				delete(body, "text")
				body["refusal"] = p.text
			}
			events = append(events, e.event(name, body), e.event("response.content_part.done", Object{"item_id": p.id, "output_index": p.index, "content_index": 0, "part": Obj(e.item(p, "completed")["content"].([]any)[0])}))
		case "reasoning_content":
			body["summary_index"], body["text"] = 0, p.text
			events = append(events, e.event("response.reasoning_summary_text.done", body), e.event("response.reasoning_summary_part.done", Object{"item_id": p.id, "output_index": p.index, "summary_index": 0, "part": Object{"type": "summary_text", "text": p.text}}))
		}
		events = append(events, e.event("response.output_item.done", Object{"output_index": p.index, "item": e.item(p, "completed")}))
	}
	if e.Protocol == Messages {
		events = append(events, e.event("message_delta", Object{"delta": Object{"stop_reason": NativeReason(e.finish), "stop_sequence": nil}, "usage": NativeUsage(Messages, e.usage)}), e.event("message_stop", Object{}))
	} else {
		status, name := "completed", "response.completed"
		if e.finish == "length" {
			status, name = "incomplete", "response.incomplete"
		}
		events = append(events, e.event(name, Object{"response": e.response(status)}))
	}
	return events, nil
}

func (e *Encoder) emptyTextPart(p *streamPart) Object {
	if p.kind == "refusal" {
		return Object{"type": "refusal", "refusal": ""}
	}
	return Object{"type": "output_text", "text": "", "annotations": []any{}}
}

func (e *Encoder) Error(message string) Event {
	if e.Protocol == Responses {
		return e.event("error", Object{"code": "upstream_error", "message": message, "param": nil})
	}
	return e.event("error", Object{"error": Object{"type": "api_error", "message": message}})
}
