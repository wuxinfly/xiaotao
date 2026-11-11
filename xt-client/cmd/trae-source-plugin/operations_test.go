package main

import (
	"context"
	"encoding/json"
	"testing"
	"xiaotao/xt-client/internal/sourceplugin"
)

func TestNativeBodyPreservesToolSchemaAndHistory(t *testing.T) {
	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}}}, "reasoning_effort": "xhigh"}
	native, err := nativeBody(body)
	if err != nil {
		t.Fatal(err)
	}
	tools := native["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if fn["parameters"] != `{"type":"object"}` {
		t.Fatalf("schema: %v", fn)
	}
	if native["reasoning_effort"] != "extra_high" {
		t.Fatal("effort translation failed")
	}
	if native["function"] != "chat_v3" {
		t.Fatal("missing native protocol")
	}
}
func TestLegacyImportDoesNotContactUpstream(t *testing.T) {
	result, err := operate(context.Background(), "credentials/import", sourceplugin.Operation{Input: json.RawMessage(`{"RefreshToken":"test-secret","Region":"cn"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if json.Unmarshal(result.Credential, &c) != nil || c["MachineID"] == "" || c["DeviceID"] == "" {
		t.Fatal("missing identity")
	}
	if _, err = operate(context.Background(), "credentials/import", sourceplugin.Operation{Input: json.RawMessage(`{"RefreshToken":"test","Region":"global"}`)}, nil); err == nil {
		t.Fatal("accepted unsupported region")
	}
}
