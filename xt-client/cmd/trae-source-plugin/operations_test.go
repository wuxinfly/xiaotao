package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"xiaotao/xt-client/internal/providers/trae"
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

func TestRPCErrorForClassifiesUpstreamFailures(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{&trae.HTTPError{Code: 401}, sourceplugin.RPCAuthExpired},
		{&trae.HTTPError{Code: 403}, sourceplugin.RPCAuthExpired},
		{&trae.HTTPError{Code: 429}, sourceplugin.RPCRateLimited},
		{&trae.HTTPError{Code: 500}, sourceplugin.RPCUpstream},
		{context.DeadlineExceeded, sourceplugin.RPCTimeout},
		{errors.New("other"), sourceplugin.RPCPluginError},
	}
	for _, tc := range cases {
		if got := rpcErrorFor(tc.err).Code; got != tc.want {
			t.Fatalf("rpcErrorFor(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestUnsupportedMethodUsesReservedCode(t *testing.T) {
	_, err := operate(context.Background(), "conformance/unknown", sourceplugin.Operation{}, nil)
	if err == nil || rpcErrorFor(err).Code != sourceplugin.RPCUnsupported {
		t.Fatalf("unknown method not rejected: %v", err)
	}
}
