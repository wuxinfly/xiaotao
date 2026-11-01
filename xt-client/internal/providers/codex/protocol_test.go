package codex

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestBuildUpstreamBodyInstructionsAndSystem(t *testing.T) {
	body := map[string]any{
		"instructions": "You are a helpful coding assistant.",
		"messages": []any{
			map[string]any{"role": "system", "content": "Follow system prompt."},
			map[string]any{"role": "user", "content": "Hello"},
		},
	}
	res := BuildUpstreamBody(body, "gpt-6.1-sol")
	inst, _ := res["instructions"].(string)
	expected := "You are a helpful coding assistant.\n\nFollow system prompt."
	if inst != expected {
		t.Fatalf("expected %q, got %q", expected, inst)
	}
}

func TestBuildUpstreamBodyDeduplicateInstructions(t *testing.T) {
	body := map[string]any{
		"instructions": "Common instructions",
		"messages": []any{
			map[string]any{"role": "system", "content": "Common instructions"},
			map[string]any{"role": "user", "content": "Hi"},
		},
	}
	res := BuildUpstreamBody(body, "gpt-6.1-sol")
	inst, _ := res["instructions"].(string)
	if inst != "Common instructions" {
		t.Fatalf("expected deduplicated instructions, got %q", inst)
	}
}

func TestBuildUpstreamBodyToolPairing(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Check weather"},
			map[string]any{
				"role":    "assistant",
				"content": "Checking...",
				"tool_calls": []any{
					map[string]any{
						"id": "call_123",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"city":"Hangzhou"}`,
						},
					},
				},
			},
			map[string]any{
				"role":         "tool",
				"tool_call_id": "call_123",
				"content":      "Sunny, 25C",
			},
		},
	}
	res := BuildUpstreamBody(body, "gpt-6.1-sol")
	input, ok := res["input"].([]map[string]any)
	if !ok || len(input) != 4 {
		t.Fatalf("expected 4 input items, got %d", len(input))
	}
	// 0: user message
	if input[0]["type"] != "message" || input[0]["role"] != "user" {
		t.Fatalf("unexpected input[0]: %v", input[0])
	}
	// 1: assistant message
	if input[1]["type"] != "message" || input[1]["role"] != "assistant" {
		t.Fatalf("unexpected input[1]: %v", input[1])
	}
	// 2: function_call
	if input[2]["type"] != "function_call" || input[2]["call_id"] != "call_123" || input[2]["name"] != "get_weather" {
		t.Fatalf("unexpected function_call: %v", input[2])
	}
	// 3: function_call_output
	if input[3]["type"] != "function_call_output" || input[3]["call_id"] != "call_123" || input[3]["output"] != "Sunny, 25C" {
		t.Fatalf("unexpected function_call_output: %v", input[3])
	}
}

func TestBuildUpstreamBodyImagesAndTools(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "What is this?"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/pic.png"}},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "describe_image",
					"description": "desc",
					"parameters":  map[string]any{"type": "object"},
				},
			},
		},
		"tool_choice":            "required",
		"reasoning_effort":       "high",
		"max_completion_tokens": float64(5), // Should clamp to minMaxOutputTokens = 16
	}
	res := BuildUpstreamBody(body, "gpt-6.1-sol")
	if res["max_output_tokens"] != 16 {
		t.Fatalf("expected max_output_tokens 16, got %v", res["max_output_tokens"])
	}
	r, ok := res["reasoning"].(map[string]any)
	if !ok || r["effort"] != "high" {
		t.Fatalf("expected reasoning effort high, got %v", res["reasoning"])
	}
	if res["tool_choice"] != "required" {
		t.Fatalf("expected tool_choice required, got %v", res["tool_choice"])
	}
	tools, ok := res["tools"].([]map[string]any)
	if !ok || len(tools) != 1 || tools[0]["name"] != "describe_image" {
		t.Fatalf("expected converted tools, got %v", res["tools"])
	}
	input, _ := res["input"].([]map[string]any)
	if len(input) != 1 {
		t.Fatalf("expected 1 input, got %d", len(input))
	}
	contentParts, ok := input[0]["content"].([]any)
	if !ok || len(contentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %v", input[0]["content"])
	}
}

func TestResponsesToChatCompletion(t *testing.T) {
	resp := map[string]any{
		"id":     "resp_123",
		"status": "completed",
		"output": []any{
			map[string]any{
				"type": "reasoning",
				"summary": []any{
					map[string]any{"type": "summary_text", "text": "Step 1: calculate. "},
				},
			},
			map[string]any{
				"type": "message",
				"content": []any{
					map[string]any{"type": "output_text", "text": "The answer is 42."},
				},
			},
		},
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 20,
			"total_tokens":  30,
		},
	}
	chat := ResponsesToChatCompletion(resp, "gpt-6.1-sol")
	if chat["id"] != "resp_123" {
		t.Fatalf("expected id resp_123, got %v", chat["id"])
	}
	choices, _ := chat["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if msg["content"] != "The answer is 42." {
		t.Fatalf("expected content, got %v", msg["content"])
	}
	if msg["reasoning_content"] != "Step 1: calculate. " {
		t.Fatalf("expected reasoning_content, got %v", msg["reasoning_content"])
	}
	if choice["finish_reason"] != "stop" {
		t.Fatalf("expected finish_reason stop, got %v", choice["finish_reason"])
	}
	usage := chat["usage"].(map[string]any)
	if usage["prompt_tokens"] != int64(10) || usage["completion_tokens"] != int64(20) {
		t.Fatalf("unexpected usage: %v", usage)
	}
}

func TestStreamStateFolding(t *testing.T) {
	state := NewStreamState("gpt-6.1-sol")

	// Reasoning delta
	deltas := state.Feed("response.reasoning_summary_text.delta", map[string]any{"delta": "Thinking..."})
	if len(deltas) != 1 || deltas[0]["reasoning_content"] != "Thinking..." {
		t.Fatalf("unexpected reasoning delta: %v", deltas)
	}

	// Output text delta
	deltas = state.Feed("response.output_text.delta", map[string]any{"delta": "Hello"})
	if len(deltas) != 1 || deltas[0]["content"] != "Hello" {
		t.Fatalf("unexpected output delta: %v", deltas)
	}

	// Function call item added
	deltas = state.Feed("response.output_item.added", map[string]any{
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_1",
			"name":    "calc",
		},
	})
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta for tool opening: %v", deltas)
	}

	// Arguments delta
	deltas = state.Feed("response.function_call_arguments.delta", map[string]any{
		"item_id": "call_1",
		"delta":   `{"x":1}`,
	})
	if len(deltas) != 0 {
		t.Fatalf("arguments delta should be buffered, got %v", deltas)
	}

	// Function call item done
	deltas = state.Feed("response.output_item.done", map[string]any{
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_1",
		},
	})
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta on tool done: %v", deltas)
	}

	// Completed
	state.Feed("response.completed", map[string]any{
		"response": map[string]any{
			"usage": map[string]any{"input_tokens": 5, "output_tokens": 10},
		},
	})
	if state.FinishReason() != "tool_calls" {
		t.Fatalf("expected finish reason tool_calls, got %s", state.FinishReason())
	}
	if state.FinalUsage()["prompt_tokens"] != int64(5) {
		t.Fatalf("expected usage prompt_tokens 5, got %v", state.FinalUsage())
	}
}

func TestErrorEventMapping(t *testing.T) {
	code, msg := isErrorEvent("error", map[string]any{"code": "rate_limit_exceeded", "message": "Too many requests"})
	if code != 429 || msg != "Too many requests" {
		t.Fatalf("expected 429 rate limit, got %d %s", code, msg)
	}

	code, msg = isErrorEvent("response.failed", map[string]any{
		"response": map[string]any{
			"error": map[string]any{"code": 401, "message": "Invalid token"},
		},
	})
	if code != 401 || msg != "Invalid token" {
		t.Fatalf("expected 401, got %d %s", code, msg)
	}

	code, _ = isErrorEvent("response.incomplete", map[string]any{
		"response": map[string]any{
			"incomplete_details": map[string]any{"reason": "max_output_tokens"},
		},
	})
	if code != 0 {
		t.Fatalf("expected 0 for normal truncation, got %d", code)
	}
}

func TestSSEReaderAndGate(t *testing.T) {
	sseData := `: keepalive
event: keepalive
data: {"type":"keepalive"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hi"}

data: [DONE]
`
	reader := NewSSEReader(strings.NewReader(sseData))
	gate, err := GateFirstEvent(context.Background(), reader)
	if err != nil {
		t.Fatalf("gate failed: %v", err)
	}
	if !gate.Committed {
		t.Fatalf("expected gate committed")
	}
	if len(gate.Buffered) != 1 || gate.Buffered[0].Name != "response.output_text.delta" {
		t.Fatalf("unexpected buffered event: %v", gate.Buffered)
	}

	stream := NewEventStream(gate.Buffered, reader)
	ev1, err := stream.Next()
	if err != nil || ev1.Name != "response.output_text.delta" {
		t.Fatalf("unexpected ev1: %v", ev1)
	}
	ev2, err := stream.Next()
	if err != nil || ev2.Name != "[DONE]" {
		t.Fatalf("unexpected ev2: %v", ev2)
	}
	_, err = stream.Next()
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestGateEmptyStreamAndAccountError(t *testing.T) {
	// Empty stream
	reader := NewSSEReader(bytes.NewReader(nil))
	gate, err := GateFirstEvent(context.Background(), reader)
	if err != nil || !gate.EOF {
		t.Fatalf("expected EOF for empty stream, got %v, gate: %v", err, gate)
	}

	// Account error in-band (429)
	sseError := `event: error
data: {"code":"rate_limit_exceeded","message":"out of quota"}
`
	reader = NewSSEReader(strings.NewReader(sseError))
	gate, err = GateFirstEvent(context.Background(), reader)
	if err != nil || !gate.AccountError || gate.StatusCode != 429 {
		t.Fatalf("expected account error 429, got %v gate: %v", err, gate)
	}
}
