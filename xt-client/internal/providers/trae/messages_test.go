package trae

import (
	"encoding/json"
	"testing"
)

func TestNativeMessagesWrapsStringContent(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	out := NativeMessages(msgs)
	b, _ := json.Marshal(out[0]["content"])
	want := `[{"text":"hello","type":"text"}]`
	// Field order in the map is not deterministic; compare as JSON values.
	var got, exp any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &exp); err != nil {
		t.Fatal(err)
	}
	if len(got.([]any)) != 1 {
		t.Fatalf("want one text block, got %s", b)
	}
	block := got.([]any)[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "hello" {
		t.Fatalf("bad block: %v", block)
	}
	_ = exp
}

func TestNativeMessagesToolCallsShape(t *testing.T) {
	msgs := []map[string]any{
		{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{
				"id": "call_1", "type": "function",
				"function": map[string]any{"name": "get_weather", "arguments": `{"city":"BJ"}`},
			},
		}},
		{"role": "tool", "tool_call_id": "call_1", "name": "get_weather", "content": "sunny"},
	}
	out := NativeMessages(msgs)
	first := out[0]
	if first["role"] != "assistant" {
		t.Fatalf("role = %v", first["role"])
	}
	tcs := first["tool_calls"].([]map[string]any)
	fc := tcs[0]["function_call"].(map[string]any)
	if fc["name"] != "get_weather" || fc["arguments"] != `{"city":"BJ"}` {
		t.Fatalf("bad function_call: %v", fc)
	}
	tool := out[1]
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" {
		t.Fatalf("bad tool message: %v", tool)
	}
}
