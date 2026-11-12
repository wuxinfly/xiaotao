// Package wireprotocol adapts stateless conversational requests and SSE events
// between Chat Completions, Responses, and Anthropic Messages.
package wireprotocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Object = map[string]any

const Chat = "openai-chat"
const Responses = "openai-responses"
const Messages = "anthropic-messages"
const MaxContent = 32 << 20

func Obj(v any) Object     { m, _ := v.(map[string]any); return m }
func List(v any) []any     { a, _ := v.([]any); return a }
func Str(v any) string     { s, _ := v.(string); return s }
func marshal(v any) string { b, _ := json.Marshal(v); return string(b) }
func copyFields(to, from Object, fields ...string) {
	for _, k := range fields {
		if v, ok := from[k]; ok {
			to[k] = v
		}
	}
}
func reject(body Object, fields ...string) error {
	for _, name := range fields {
		if v, ok := body[name]; ok && v != nil && v != false && v != "" {
			return fmt.Errorf("%s is not supported by the conversational gateway", name)
		}
	}
	return nil
}
func EndReason(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
	}
}
func NativeReason(reason string) string {
	switch reason {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}
func chunk(delta Object, finish any) Object {
	return Object{"choices": []any{Object{"index": 0, "delta": delta, "finish_reason": finish}}}
}
func number(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), n >= 0 && n == float64(int64(n))
	case json.Number:
		i, e := n.Int64()
		return i, e == nil && i >= 0
	case int:
		return int64(n), n >= 0
	case int64:
		return n, n >= 0
	}
	return 0, false
}
func ChatUsage(protocol string, u Object) Object {
	if u == nil {
		return nil
	}
	result := Object{}
	in, inOK := number(u["input_tokens"])
	out, outOK := number(u["output_tokens"])
	if protocol == Messages {
		for _, name := range []string{"cache_read_input_tokens", "cache_creation_input_tokens"} {
			if n, ok := number(u[name]); ok && inOK {
				in += n
			}
		}
		if v, ok := u["cache_read_input_tokens"]; ok {
			result["prompt_tokens_details"] = Object{"cached_tokens": v}
		}
		copyFields(result, u, "cache_creation_input_tokens")
	} else if details := Obj(u["input_tokens_details"]); details != nil {
		result["prompt_tokens_details"] = details
	}
	if inOK {
		result["prompt_tokens"] = in
	}
	if outOK {
		result["completion_tokens"] = out
	}
	if inOK && outOK {
		result["total_tokens"] = in + out
	}
	return result
}
func NativeUsage(protocol string, u Object) Object {
	if u == nil {
		if protocol == Messages {
			return Object{"input_tokens": nil, "output_tokens": nil}
		}
		return nil
	}
	result := Object{}
	copyFields(result, u, "cache_creation_input_tokens")
	in, known := number(u["prompt_tokens"])
	if protocol == Messages {
		cached, ok := number(Obj(u["prompt_tokens_details"])["cached_tokens"])
		if ok {
			result["cache_read_input_tokens"] = cached
			if known {
				in -= cached
			}
		}
		if n, ok := number(u["cache_creation_input_tokens"]); ok && known {
			in -= n
		}
	} else if v := u["prompt_tokens_details"]; v != nil {
		result["input_tokens_details"] = v
	}
	if known && in >= 0 {
		result["input_tokens"] = in
	}
	if v, ok := u["completion_tokens"]; ok {
		result["output_tokens"] = v
	}
	if protocol == Responses {
		if v, ok := u["total_tokens"]; ok {
			result["total_tokens"] = v
		}
	}
	return result
}
func textParts(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	var b strings.Builder
	for _, raw := range List(v) {
		part := Obj(raw)
		if Str(part["type"]) != "text" && Str(part["type"]) != "input_text" && Str(part["type"]) != "output_text" {
			return "", fmt.Errorf("unsupported text block %q", Str(part["type"]))
		}
		b.WriteString(Str(part["text"]))
	}
	return b.String(), nil
}
