package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func run(t *testing.T, server *Server, lines ...string) []Message {
	t.Helper()
	var out bytes.Buffer
	if err := server.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var messages []Message
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("bad output %q: %v", line, err)
		}
		messages = append(messages, msg)
	}
	return messages
}

func TestServeDispatchesInitializeAndHandlers(t *testing.T) {
	server := &Server{ID: "example.echo", Handlers: map[string]Handler{
		"models/refresh": func(context.Context, Operation) (Result, error) {
			return Result{Models: []Model{{Key: "m1", UpstreamID: "m1", Label: "Echo"}}}, nil
		},
		"quota/get": func(context.Context, Operation) (Result, error) {
			percent := 42.0
			return Result{Display: &Display{Summary: "ok", Items: []DisplayItem{{Label: "额度", Value: "42 / 100", Percent: &percent}}}}, nil
		},
	}}
	messages := run(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"models/refresh","params":{"settings":{"region":"cn"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"quota/get","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"chat/start","params":{}}`,
	)
	if len(messages) != 4 {
		t.Fatalf("responses: %d", len(messages))
	}
	var init map[string]any
	if err := json.Unmarshal(messages[0].Result, &init); err != nil || init["id"] != "example.echo" || init["contractVersion"] != float64(ContractVersion) {
		t.Fatalf("initialize: %s", messages[0].Result)
	}
	if !strings.Contains(string(messages[1].Result), `"upstreamId":"m1"`) {
		t.Fatalf("models: %s", messages[1].Result)
	}
	if !strings.Contains(string(messages[2].Result), `"percent":42`) {
		t.Fatalf("display: %s", messages[2].Result)
	}
	if messages[3].Error == nil || messages[3].Error.Code != CodeUnsupported {
		t.Fatalf("unsupported method: %+v", messages[3].Error)
	}
}

func TestServeMapsHandlerErrors(t *testing.T) {
	server := &Server{ID: "example.echo", Handlers: map[string]Handler{
		"quota/get": func(context.Context, Operation) (Result, error) {
			return Result{}, Errorf(CodeAuthExpired, "login expired")
		},
		"models/refresh": func(context.Context, Operation) (Result, error) {
			return Result{}, errors.New("boom")
		},
	}}
	messages := run(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"quota/get","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"models/refresh","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"initialize","params":{}}`,
	)
	if messages[0].Error == nil || messages[0].Error.Code != CodeAuthExpired {
		t.Fatalf("reserved code lost: %+v", messages[0].Error)
	}
	if messages[1].Error == nil || messages[1].Error.Code != CodePluginError {
		t.Fatalf("generic error lost: %+v", messages[1].Error)
	}
}

func TestServeRejectsMalformedInput(t *testing.T) {
	server := &Server{ID: "example.echo"}
	var out bytes.Buffer
	if err := server.Serve(strings.NewReader("not json\n"), &out); err == nil {
		t.Fatal("malformed input accepted")
	}
	if err := server.Serve(strings.NewReader(`{"jsonrpc":"2.0","method":"x"}`+"\n"), &out); err == nil {
		t.Fatal("notification accepted")
	}
}
