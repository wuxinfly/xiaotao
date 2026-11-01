package qoder

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEncodeBodyVectors(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"a":       "$p$#",
		"hello":   "q$FruHPH",
		`{"a":1}`: "ep$$BMn%mYKi",
	}
	for plain, want := range cases {
		got := EncodeBody(plain)
		if got != want {
			t.Fatalf("EncodeBody(%q) = %q, want %q", plain, got, want)
		}
		decoded, err := DecodeBody(got)
		if err != nil || decoded != plain {
			t.Fatalf("DecodeBody(%q) = %q, %v; want %q", got, decoded, err, plain)
		}
	}
}

func TestEncodeBodyLongPlainText(t *testing.T) {
	plain := `{"messages":[{"role":"user","content":"hi"}],"stream":true}`
	got := EncodeBody(plain)
	want := "z*eFwrBEjwNOFiu,ByJZK.Dxw$ByBEF^DxB*GoKMu(QwDSQwBMn*PHmYKtDxj^#SJLNYBySkr*NOWrD,"
	if got != want {
		t.Fatalf("EncodeBody(long) = %q, want %q", got, want)
	}
	if decoded, err := DecodeBody(got); err != nil || decoded != plain {
		t.Fatalf("DecodeBody roundtrip failed: %q, %v", decoded, err)
	}
}

func TestSignaturePath(t *testing.T) {
	cases := map[string]string{
		"https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=x": "/api/v2/service/pro/sse/agent_chat_generation",
		"https://h/algo/a/b?c=1&d=2": "/a/b",
		"https://h/plain":            "/plain",
	}
	for raw, want := range cases {
		if got := SignaturePath(raw); got != want {
			t.Fatalf("SignaturePath(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSignProducesRequiredHeaders(t *testing.T) {
	key := []byte("0123456789abcdef")
	opts := SignOptions{
		ModelKey: "qmodel_38max",
		NewKey:   func() ([]byte, error) { return key, nil },
		NewUUID:  func() string { return "fixed-uuid" },
		Now:      func() time.Time { return time.Unix(1700000000, 0) },
	}
	signed, err := Sign("https://h/algo/a?Encode=1", `{"a":1}`, "uid-1", "dt-token", "machine-1", opts)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if signed.Body == "" {
		t.Fatal("expected encoded body")
	}
	auth := signed.Headers["Authorization"]
	if !strings.HasPrefix(auth, "Bearer COSY.") || strings.Count(auth, ".") != 2 {
		t.Fatalf("unexpected Authorization header: %s", auth)
	}
	if signed.Headers["Cosy-User"] != "uid-1" ||
		signed.Headers["Cosy-MachineId"] != "machine-1" ||
		signed.Headers["Cosy-MachineToken"] != "machine-1" ||
		signed.Headers["Cosy-ClientType"] != "5" ||
		signed.Headers["Cosy-Data-Policy"] != "agree" ||
		signed.Headers["Login-Version"] != "v2" ||
		signed.Headers["X-Model-Key"] != "qmodel_38max" ||
		signed.Headers["X-Model-Source"] != "system" {
		t.Fatalf("missing or wrong required COSY headers: %v", signed.Headers)
	}
}

func TestSignPayloadAndUniqueness(t *testing.T) {
	opts := SignOptions{Now: func() time.Time { return time.Unix(1700000000, 0) }}
	first, err := Sign("https://h/algo/a", "x", "u", "t", "m", opts)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	second, err := Sign("https://h/algo/a", "x", "u", "t", "m", opts)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if first.Headers["Cosy-Key"] == second.Headers["Cosy-Key"] || first.Headers["Authorization"] == second.Headers["Authorization"] {
		t.Fatal("Sign must use a fresh AES key and requestId per call")
	}
	parts := strings.Split(first.Headers["Authorization"], ".")
	payloadB64 := parts[1]
	raw, err := base64.StdEncoding.WithPadding(base64.NoPadding).Strict().DecodeString(strings.TrimRight(payloadB64, "="))
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if payload["version"] != "v1" || payload["cosyVersion"] != COSYVersion || payload["info"] == "" || payload["requestId"] == "" {
		t.Fatalf("payload fields missing: %v", payload)
	}
}

func TestSignEncodeFalseGivesEmptyBody(t *testing.T) {
	signed, err := Sign("https://h/algo/y", "", "u", "t", "m", SignOptions{Encode: boolRef(false)})
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if signed.Body != "" {
		t.Fatalf("expected empty body, got %q", signed.Body)
	}
	if _, ok := signed.Headers["X-Model-Key"]; ok {
		t.Fatal("model headers should be omitted without a model key")
	}
}

func TestPKCEPair(t *testing.T) {
	verifier, challenge, err := PKCEPair()
	if err != nil {
		t.Fatalf("PKCEPair failed: %v", err)
	}
	if len(verifier) != 107 {
		t.Fatalf("verifier length = %d, want 107", len(verifier))
	}
	if challenge == "" || strings.Contains(challenge, "=") {
		t.Fatalf("invalid challenge: %q", challenge)
	}
}

func TestIdentityASCIIKeyAndSignature(t *testing.T) {
	_, key, err := buildIdentity("u", "t", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 16 {
		t.Fatal("expected 16-byte key")
	}
	for _, c := range key {
		if !strings.ContainsRune("0123456789abcdef", rune(c)) {
			t.Fatal("key must use ASCII hex characters")
		}
	}
	signed, err := Sign("https://h/algo/chat?Encode=1", "中文", "u", "t", "m", SignOptions{})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(signed.Headers["Authorization"], ".")
	input := strings.Join([]string{parts[1], signed.Headers["Cosy-Key"], signed.Headers["Cosy-Date"], signed.Body, "/chat"}, "\n")
	if want := fmt.Sprintf("%x", md5.Sum([]byte(input))); parts[2] != want {
		t.Fatal("signature does not match wire request")
	}
}
