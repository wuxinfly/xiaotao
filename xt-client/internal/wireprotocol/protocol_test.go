package wireprotocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageRetainsUnknownAndAccountsForCachedInput(t *testing.T) {
	if got := ChatUsage(Messages, nil); got != nil {
		t.Fatal(got)
	}
	usage := ChatUsage(Messages, Object{"input_tokens": float64(10), "cache_read_input_tokens": float64(5), "cache_creation_input_tokens": float64(3), "output_tokens": float64(2)})
	if usage["prompt_tokens"] != int64(18) || usage["total_tokens"] != int64(20) {
		t.Fatal(usage)
	}
	native := NativeUsage(Messages, usage)
	if native["input_tokens"] != int64(10) || native["cache_read_input_tokens"] != int64(5) {
		t.Fatal(native)
	}
	partial := ChatUsage(Responses, Object{"output_tokens": float64(7)})
	if _, known := partial["prompt_tokens"]; known {
		t.Fatal("unknown input was fabricated", partial)
	}
	if _, known := partial["total_tokens"]; known {
		t.Fatal("unknown total was fabricated", partial)
	}
}
func TestUnsupportedStatefulFeaturesFailExplicitly(t *testing.T) {
	for _, extra := range []Object{{"previous_response_id": "resp_old"}, {"store": true}, {"tools": []any{Object{"type": "web_search"}}}, {"background": true}, {"input": []any{Object{"type": "input_file", "file_id": "file_x"}}}} {
		body := Object{"model": "test", "input": "hello"}
		for k, v := range extra {
			body[k] = v
		}
		if _, err := ToChatRequest(Responses, body); err == nil {
			t.Fatalf("accepted unsupported feature: %+v", extra)
		}
	}
	if _, err := ToChatRequest(Messages, Object{"messages": []any{Object{"role": "user", "content": "hi"}}, "thinking": Object{"type": "enabled", "budget_tokens": 1024}}); err == nil {
		t.Fatal("signed extended thinking silently accepted")
	}
}
func TestStructuredFormatAndToolHistory(t *testing.T) {
	body := Object{"model": "m", "input": []any{Object{"type": "function_call", "call_id": "call_a", "name": "lookup", "arguments": "{}"}, Object{"type": "function_call_output", "call_id": "call_a", "output": "ok"}}, "text": Object{"format": Object{"type": "json_schema", "name": "answer", "schema": Object{"type": "object"}, "strict": true}}}
	chat, err := ToChatRequest(Responses, body)
	if err != nil {
		t.Fatal(err)
	}
	schema := Obj(Obj(chat["response_format"])["json_schema"])
	if schema["name"] != "answer" || schema["strict"] != true {
		t.Fatal(chat)
	}
	native, err := FromChatRequest(Responses, chat)
	if err != nil {
		t.Fatal(err)
	}
	if Obj(Obj(native["text"])["format"])["name"] != "answer" {
		t.Fatal(native)
	}
	input := List(native["input"])
	if Obj(input[0])["call_id"] != "call_a" || Obj(input[1])["call_id"] != "call_a" {
		t.Fatal(input)
	}
}
func TestParallelToolStreamPreservesIndicesAndArguments(t *testing.T) {
	encoder := Encoder{Protocol: Responses}
	var events []Event
	rawChunks := []string{
		`{"id":"resp","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"one","arguments":"{\"a\":"}},{"index":1,"id":"call_b","function":{"name":"two","arguments":"{\"b\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}},{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}]}`,
	}
	for _, raw := range rawChunks {
		var chunk Object
		if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
			t.Fatal(err)
		}
		out, err := encoder.Push(chunk)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, out...)
	}
	final, err := encoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	events = append(events, final...)
	args := map[int]string{}
	for _, event := range events {
		if event.Name == "response.function_call_arguments.delta" {
			index, _ := number(event.Data["output_index"])
			args[int(index)] += Str(event.Data["delta"])
		}
	}
	if args[0] != "{\"a\":1}" || args[1] != "{\"b\":2}" {
		t.Fatal(args)
	}
	output := List(Obj(events[len(events)-1].Data["response"])["output"])
	if Obj(output[0])["call_id"] != "call_a" || Obj(output[1])["arguments"] != "{\"b\":2}" {
		t.Fatal(output)
	}
}
func TestDecoderRequiresTerminalAndRejectsErrors(t *testing.T) {
	decoder := Decoder{Protocol: Messages}
	if _, err := decoder.Event(Object{"type": "message_delta", "delta": Object{}, "usage": Object{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.Event(Object{"type": "message_stop"}); err == nil {
		t.Fatal("stop without reason accepted")
	}
	decoder = Decoder{Protocol: Responses}
	if _, err := decoder.Event(Object{"type": "response.failed", "response": Object{"status": "failed"}}); err == nil {
		t.Fatal("failed response accepted")
	}
	encoder := Encoder{Protocol: Responses}
	if _, err := encoder.Push(Object{"choices": []any{Object{"index": 0, "delta": Object{"content": "partial"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Finish(); err == nil {
		t.Fatal("partial stream reported completed")
	}
}
func TestStreamEncoderBoundsFinalResponseMemory(t *testing.T) {
	encoder := Encoder{Protocol: Responses}
	text := strings.Repeat("x", MaxContent+1)
	if _, err := encoder.Push(Object{"choices": []any{Object{"index": 0, "delta": Object{"content": text}}}}); err == nil {
		t.Fatal("unbounded stream retention")
	}
}
func TestJSONToolArgumentsAreNotDoubleEncoded(t *testing.T) {
	raw := `{"id":"m","type":"message","model":"x","content":[{"type":"tool_use","id":"call","name":"x","input":{"text":"你好"}}],"stop_reason":"tool_use"}`
	var input Object
	_ = json.Unmarshal([]byte(raw), &input)
	chat, err := ToChatResponse(Messages, input)
	if err != nil {
		t.Fatal(err)
	}
	native, err := FromChatResponse(Messages, chat)
	if err != nil {
		t.Fatal(err)
	}
	if Obj(Obj(List(native["content"])[0])["input"])["text"] != "你好" {
		t.Fatal(native)
	}
}

func TestRefusalEventsAreNotDropped(t *testing.T) {
	for _, protocol := range []string{Messages, Responses} {
		encoder := Encoder{Protocol: protocol}
		out, err := encoder.Push(Object{"id": "x", "model": "m", "choices": []any{Object{"index": 0, "delta": Object{"refusal": "cannot comply"}, "finish_reason": "stop"}}})
		if err != nil {
			t.Fatal(err)
		}
		final, err := encoder.Finish()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, final...)
		raw, _ := json.Marshal(out)
		if !strings.Contains(string(raw), "cannot comply") {
			t.Fatal("refusal lost", string(raw))
		}
	}
}

func TestToolResultImagesUseNativeContentTypes(t *testing.T) {
	chat := Object{"model": "m", "messages": []any{Object{"role": "tool", "tool_call_id": "call", "content": []any{Object{"type": "text", "text": "result"}, Object{"type": "image_url", "image_url": Object{"url": "data:image/png;base64,aGVsbG8="}}}}}}
	for _, protocol := range []string{Responses, Messages} {
		native, err := FromChatRequest(protocol, chat)
		if err != nil {
			t.Fatal(err)
		}
		var parts []any
		if protocol == Responses {
			parts = List(Obj(List(native["input"])[0])["output"])
		} else {
			parts = List(Obj(List(Obj(List(native["messages"])[0])["content"])[0])["content"])
		}
		expected := "input_image"
		if protocol == Messages {
			expected = "image"
		}
		if len(parts) != 2 || Obj(parts[1])["type"] != expected {
			t.Fatal(native)
		}
	}
}
