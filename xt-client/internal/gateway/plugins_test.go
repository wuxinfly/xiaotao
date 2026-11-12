package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/sourceplugin"
)

func TestMain(m *testing.M) {
	if os.Getenv("XT_GATEWAY_PLUGIN_CHILD") == "1" {
		gatewayHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func gatewayHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req sourceplugin.Message
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			return
		}
		var result any = sourceplugin.Result{}
		switch req.Method {
		case "initialize":
			result = map[string]any{"id": "test.gateway", "contractVersion": 1}
		case "models/refresh":
			result = sourceplugin.Result{Models: []sourceplugin.Model{{Key: "shared", Label: "Shared", UpstreamID: "org/model"}}}
		case "chat/start":
			var op sourceplugin.Operation
			_ = json.Unmarshal(req.Params, &op)
			if op.Body["stream"] == true {
				for _, chunk := range []string{`{"choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`, `{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}]}`, `{"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}`} {
					_ = encoder.Encode(sourceplugin.Message{JSONRPC: "2.0", Method: "chat/delta", Params: json.RawMessage(chunk)})
				}
			} else {
				result = sourceplugin.Result{Value: json.RawMessage(`{"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`)}
			}
		}
		raw, _ := json.Marshal(result)
		_ = encoder.Encode(sourceplugin.Message{JSONRPC: "2.0", ID: req.ID, Result: raw})
	}
}
func TestPluginGatewayRoundTrip(t *testing.T) {
	t.Setenv("XT_GATEWAY_PLUGIN_CHILD", "1")
	pkg := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(filepath.Join(pkg, "provider.exe"), os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, in)
	out.Close()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"id":"test.gateway","version":"1","contractVersion":1,"name":"Fake","entrypoint":"provider.exe","capabilities":["chat"]}`)
	if err = os.WriteFile(filepath.Join(pkg, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	crypt := func(b []byte) ([]byte, error) { return b, nil }
	m, err := sourceplugin.Open(t.TempDir(), crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Install(pkg); err != nil {
		t.Fatal(err)
	}
	if err = m.Enable("test.gateway", true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Run(context.Background(), "test.gateway", "models/refresh", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	store, err := modelsource.Open(t.TempDir(), crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	gw := New(store, "local-key", nil)
	gw.SetPlugins(m)
	id := m.Sources()[0].Models[0].CallID
	if len(gw.Models()) != 1 || gw.Models()[0]["id"] != "test.gateway/shared" {
		t.Fatal("plugin catalog missing")
	}
	for _, callID := range []string{id, m.Sources()[0].Models[0].ID} {
		for _, stream := range []bool{false, true} {
			r := chatRequest(callID, stream)
			w := httptest.NewRecorder()
			gw.HandleChat(w, r)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "hello") || !strings.Contains(w.Body.String(), callID) {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
			if stream && (!strings.Contains(w.Body.String(), "[DONE]") || !strings.Contains(w.Body.String(), "reasoning_content")) {
				t.Fatal("lost stream semantics")
			}
		}
	}
	if err = m.Enable("test.gateway", false); err != nil {
		t.Fatal(err)
	}
	for _, callID := range []string{id, m.Sources()[0].Models[0].ID} {
		w := httptest.NewRecorder()
		gw.HandleChat(w, chatRequest(callID, false))
		if w.Code != 400 {
			t.Fatal("disabled plugin called")
		}
	}
}
