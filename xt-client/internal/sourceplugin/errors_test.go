package sourceplugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRPCFailureMapsStableCodes(t *testing.T) {
	cases := map[int]string{
		RPCAuthExpired:  CodeAuthExpired,
		RPCRateLimited:  CodeRateLimited,
		RPCUnsupported:  CodeUnsupported,
		RPCInvalidInput: CodeInvalid,
		RPCUpstream:     CodeUpstream,
		RPCTimeout:      CodeTimeout,
	}
	for code, want := range cases {
		if got := ErrorCode(rpcFailure(&RPCError{Code: code, Message: "detail"})); got != want {
			t.Fatalf("code %d mapped to %q, want %q", code, got, want)
		}
	}
	raw, _ := json.Marshal(map[string]string{"code": CodeRateLimited})
	if got := ErrorCode(rpcFailure(&RPCError{Code: RPCAuthExpired, Message: "detail", Data: raw})); got != CodeRateLimited {
		t.Fatalf("data code ignored: %q", got)
	}
	raw, _ = json.Marshal(map[string]string{"code": "unknown"})
	if got := ErrorCode(rpcFailure(&RPCError{Code: -1, Message: "detail", Data: raw})); got != CodePlugin {
		t.Fatalf("unknown code accepted: %q", got)
	}
	if got := ErrorCode(rpcFailure(&RPCError{Code: RPCAuthExpired, Message: strings.Repeat("x", 500)})); got != CodeAuthExpired {
		t.Fatal(got)
	}
	if got := rpcFailure(&RPCError{Code: RPCAuthExpired, Message: strings.Repeat("x", 500)}).Error(); len(got) != 300 {
		t.Fatalf("message not truncated: %d", len(got))
	}
}
