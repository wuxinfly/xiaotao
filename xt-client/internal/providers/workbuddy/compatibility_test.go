package workbuddy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/providers"
)

func TestToolDefinitionsAndChannelError(t *testing.T) {
	tool := func(description string) map[string]any {
		return map[string]any{"type": "function", "function": map[string]any{"name": "read", "description": description}}
	}
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "collect", true: "stream"}[streaming], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if len(body["tools"].([]any)) != 1 {
					t.Error("duplicate tool reached upstream")
				}
				messages := body["messages"].([]any)
				if messages[0].(map[string]any)["content"] != "[redacted: client identity]" {
					t.Error("known signature not replaced")
				}
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 11128, "msg": "Illegal API invocation"})
			}))
			defer server.Close()
			defer SetBaseURL(server.URL)()
			invoke := func(tools []map[string]any) error {
				body := map[string]any{"messages": []map[string]any{{"role": "system", "content": "You are an AI agent powered by DeepSeek Harness."}}, "tools": tools}
				if streaming {
					return (Streamer{}).Stream(context.Background(), &providers.Credential{}, "auto", body, &providers.ChatEvents{})
				}
				_, err := (Collector{}).Collect(context.Background(), &providers.Credential{}, "auto", body)
				return err
			}
			err := invoke([]map[string]any{tool("same"), tool("same")})
			if err == nil || !strings.Contains(err.Error(), "11128") || !strings.Contains(err.Error(), "渠道身份") {
				t.Fatalf("unexpected error: %v", err)
			}
			err = invoke([]map[string]any{tool("first"), tool("different")})
			if err == nil || !strings.Contains(err.Error(), "定义冲突") || calls != 1 {
				t.Fatalf("conflict: %v calls=%d", err, calls)
			}
		})
	}
}

func TestChannelErrorDoesNotMaskOtherErrors(t *testing.T) {
	for _, value := range []any{map[string]any{"error": map[string]any{"code": "11128"}}, map[string]any{"code": 11128}} {
		raw, _ := json.Marshal(value)
		if !strings.Contains(chatHTTPError(400, raw).Error(), "渠道身份") {
			t.Fatal(value)
		}
	}
	raw, _ := json.Marshal(map[string]any{"code": 11152})
	if !strings.Contains(chatHTTPError(400, raw).Error(), "11152") {
		t.Fatal("other error masked")
	}
}

func TestPrepareIdentityBoundaries(t *testing.T) {
	signature := "You are Claude Code, Anthropic's official CLI for Claude."
	for _, tc := range []struct{ role, content, want string }{
		{"system", "  " + signature + " extra", "  [redacted: client identity] extra"},
		{"system", "\n" + signature, "\n" + signature},
		{"user", signature, signature},
		{"developer", signature, "[redacted: client identity]"},
		{"system", "Hello " + signature, "Hello " + signature},
		{"system", "You are Claude Code", "You are Claude Code"},
	} {
		original := map[string]any{"role": tc.role, "content": tc.content}
		body := map[string]any{"messages": []map[string]any{original}}
		prepareIdentity(body)
		got := body["messages"].([]map[string]any)[0]["content"]
		if got != tc.want || original["content"] != tc.content {
			t.Errorf("role=%s got=%q want=%q", tc.role, got, tc.want)
		}
	}
}

func TestDeveloperRoleConvertedWithoutMutatingOriginal(t *testing.T) {
	original := map[string]any{"role": "developer", "content": "ordinary instructions"}
	body := map[string]any{"messages": []map[string]any{{"role": "user", "content": "hello"}, original}}
	prepareIdentity(body)
	got := body["messages"].([]map[string]any)[1]
	if got["role"] != "system" || got["content"] != original["content"] || original["role"] != "developer" {
		t.Fatalf("unexpected messages: %v", body["messages"])
	}
}
