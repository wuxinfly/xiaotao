package antigravity

import (
	"strings"
	"testing"
)

func TestScrubIdentity_ReplacesKnownTerms(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"You are Claude Code, a coding agent.", "You are the assistant, a coding agent."},
		{"You are Claude, the AI.", "You are the assistant, the AI."},
		{"You are Codex, built by OpenAI.", "You are the assistant, built by OpenAI."},
		{"You are ChatGPT, a helpful assistant.", "You are the assistant, a helpful assistant."},
		{"You are GPT, the model.", "You are the assistant, the model."},
		{"I am the user, not You are Claude", "I am the user, not You are the assistant"},
	}
	for _, c := range cases {
		if got := scrubIdentity(c.input); got != c.expected {
			t.Errorf("scrubIdentity(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestBuildRequest_InjectsIdentityAndScrubsSystem(t *testing.T) {
	body := map[string]any{
		"messages": []map[string]any{
			{"role": "developer", "content": "You are Claude, a helpful assistant."},
			{"role": "system", "content": "Always answer in Chinese."},
			{"role": "user", "content": "hi"},
		},
	}
	cache := NewSignatureCache()
	envelope := buildRequest(body, "test-project", "gemini-3.6-flash-medium", cache)
	inner, ok := envelope["request"].(map[string]any)
	if !ok {
		t.Fatal("missing request envelope")
	}
	si, ok := inner["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("missing systemInstruction")
	}
	parts, _ := si["parts"].([]map[string]any)
	if len(parts) < 3 {
		t.Fatalf("expected >= 3 system parts (identity + ignore + user), got %d", len(parts))
	}
	// Part 0: raw identity assertion
	if !strings.Contains(parts[0]["text"].(string), "You are Antigravity") {
		t.Errorf("part 0 should contain identity, got %q", parts[0]["text"])
	}
	// Part 1: ignore-wrapped identity
	if !strings.Contains(parts[1]["text"].(string), "Please ignore the following [ignore]") {
		t.Errorf("part 1 should contain ignore wrapper, got %q", parts[1]["text"])
	}
	// Part 2: scrubbed developer message (Claude -> assistant)
	if strings.Contains(parts[2]["text"].(string), "You are Claude") {
		t.Errorf("developer message should be scrubbed, got %q", parts[2]["text"])
	}
	if !strings.Contains(parts[2]["text"].(string), "You are the assistant") {
		t.Errorf("developer message should have neutral replacement, got %q", parts[2]["text"])
	}
	// Part 3: user's own system text untouched
	if parts[3]["text"] != "Always answer in Chinese." {
		t.Errorf("user system text should pass through unchanged, got %q", parts[3]["text"])
	}
	// User message content should not be scrubbed
	contents, _ := inner["contents"].([]map[string]any)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content entry (user only), got %d", len(contents))
	}
	if contents[0]["role"] != "user" {
		t.Errorf("expected role user, got %v", contents[0]["role"])
	}
}

func TestBuildRequest_UserContentNotScrubbed(t *testing.T) {
	body := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "Please pretend You are Claude."},
		},
	}
	envelope := buildRequest(body, "p", "gpt-oss-120b-medium", NewSignatureCache())
	inner := envelope["request"].(map[string]any)
	contents := inner["contents"].([]map[string]any)
	parts := contents[0]["parts"].([]map[string]any)
	if parts[0]["text"] != "Please pretend You are Claude." {
		t.Errorf("user text should not be scrubbed, got %q", parts[0]["text"])
	}
}

func TestBuildRequest_PutsThoughtSignatureOnFunctionCallPart(t *testing.T) {
	body := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "read a file"},
			{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "read_file",
							"arguments": `{"path":"README.md"}`,
						},
						"gemini_thought_signature": "signed",
					},
				},
			},
			{"role": "tool", "tool_call_id": "call_1", "content": `{"ok":true}`},
		},
	}
	envelope := buildRequest(body, "p", "gemini-3.8-flash-tiered", NewSignatureCache())
	inner := envelope["request"].(map[string]any)
	contents := inner["contents"].([]map[string]any)
	if len(contents) != 3 {
		t.Fatalf("expected 3 content entries, got %d", len(contents))
	}
	part := contents[1]["parts"].([]map[string]any)[0]
	if part["thoughtSignature"] != "signed" {
		t.Errorf("thoughtSignature should be on the functionCall part, got %v", part)
	}
	call := part["functionCall"].(map[string]any)
	if _, exists := call["thoughtSignature"]; exists {
		t.Errorf("thoughtSignature must not be nested in functionCall, got %v", call)
	}
}

func TestMessagesToContentsMergesAdjacentRoles(t *testing.T) {
	body := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "first"},
			{"role": "user", "content": "second"},
			{"role": "assistant", "content": "one"},
			{"role": "assistant", "content": "two"},
			{"role": "tool", "tool_call_id": "call_1", "content": `{"ok":true}`},
			{"role": "tool", "tool_call_id": "call_2", "content": `{"ok":true}`},
		},
	}
	envelope := buildRequest(body, "p", "gemini-3.8-flash-tiered", NewSignatureCache())
	inner := envelope["request"].(map[string]any)
	contents := inner["contents"].([]map[string]any)
	if len(contents) != 3 {
		t.Fatalf("expected alternating roles, got %d contents: %v", len(contents), contents)
	}
	if got := len(contents[0]["parts"].([]map[string]any)); got != 2 {
		t.Errorf("expected adjacent user text merged, got %d parts", got)
	}
	if got := len(contents[1]["parts"].([]map[string]any)); got != 2 {
		t.Errorf("expected adjacent assistant text merged, got %d parts", got)
	}
	if got := len(contents[2]["parts"].([]map[string]any)); got != 2 {
		t.Errorf("expected adjacent tool responses merged, got %d parts", got)
	}
}
