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
			result = map[string]any{"id": "xiaotao.trae", "contractVersion": 1}
		case "models/refresh":
			result = sourceplugin.Result{Models: []sourceplugin.Model{{Key: "中文模型", Label: "Shared", UpstreamID: "org/model"}}}
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
func setupPluginGateway(t *testing.T) (*Service, *sourceplugin.Manager) {
	t.Helper()
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
	raw := []byte(`{"id":"xiaotao.trae","version":"1","contractVersion":1,"name":"Fake","entrypoint":"provider.exe","capabilities":["chat"]}`)
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
	if err = m.Enable("xiaotao.trae", true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Run(context.Background(), "xiaotao.trae", "models/refresh", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	store, err := modelsource.Open(t.TempDir(), crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	gw := New(store, "local-key", nil)
	gw.SetPlugins(m)
	return gw, m
}

func TestPluginGatewayRoundTrip(t *testing.T) {
	gw, m := setupPluginGateway(t)
	id := gw.Models()[0]["id"].(string)
	if len(gw.Models()) != 1 || id != "trae/中文模型" {
		t.Fatal("plugin catalog missing")
	}
	for _, callID := range []string{id, m.Sources()[0].Models[0].ID, m.Sources()[0].Models[0].CallID, "xiaotao.trae/%E4%B8%AD%E6%96%87%E6%A8%A1%E5%9E%8B"} {
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
	if err := m.Enable("xiaotao.trae", false); err != nil {
		t.Fatal(err)
	}
	for _, callID := range []string{id, m.Sources()[0].Models[0].ID, m.Sources()[0].Models[0].CallID, "xiaotao.trae/%E4%B8%AD%E6%96%87%E6%A8%A1%E5%9E%8B"} {
		w := httptest.NewRecorder()
		gw.HandleChat(w, chatRequest(callID, false))
		if w.Code != 400 {
			t.Fatal("disabled plugin called")
		}
	}
}

func TestShortPluginPrefixCannotTakeOverManualRoutes(t *testing.T) {
	for _, manualFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "plugin-first", true: "manual-first"}[manualFirst], func(t *testing.T) {
			gw, _ := setupPluginGateway(t)
			if !manualFirst {
				_ = gw.Models()
			}
			key := "manual-key"
			manual, err := gw.store.Save("", modelsource.Input{Label: "Trae", Protocol: "openai-chat", BaseURL: "https://example.com/v1", APIKey: &key, Models: []modelsource.Model{{UpstreamModelID: "中文模型"}}})
			if err != nil {
				t.Fatal(err)
			}
			expectedManual, expectedPlugin := "trae~2/中文模型", "trae/中文模型"
			if manualFirst {
				expectedManual, expectedPlugin = "trae/中文模型", "xiaotao.trae/中文模型"
			}
			catalog := gw.Models()
			if len(catalog) != 2 || catalog[0]["id"] != expectedManual || catalog[1]["id"] != expectedPlugin {
				t.Fatal(catalog)
			}
			for route, sourceID := range map[string]string{expectedManual: manual.ID, expectedPlugin: "xiaotao.trae"} {
				src, _, _, err := gw.resolveCall(route)
				if err != nil || src.ID != sourceID {
					t.Fatalf("%s resolved to %s: %v", route, src.ID, err)
				}
			}
			if err := gw.store.Delete(manual.ID); err != nil {
				t.Fatal(err)
			}
			src, _, _, err := gw.resolveCall(expectedManual)
			if err == nil {
				t.Fatalf("deleted manual alias rebound to %s", src.ID)
			}
			if gw.Models()[0]["id"] != expectedPlugin {
				t.Fatal("deletion changed plugin alias")
			}
		})
	}
}

func TestNativeProtocolsReusePluginRoutes(t *testing.T) {
	svc, _ := setupPluginGateway(t)
	id := svc.Models()[0]["id"].(string)
	for _, protocol := range []string{"openai-responses", "anthropic-messages"} {
		for _, stream := range []bool{false, true} {
			w := callProtocol(svc, protocol, inboundProtocolBody(protocol, id, stream))
			if w.Code != 200 || !strings.Contains(w.Body.String(), "hello") {
				t.Fatal(protocol, stream, w.Code, w.Body.String())
			}
			if stream {
				terminal := "event: response.completed"
				if protocol == "anthropic-messages" {
					terminal = "event: message_stop"
				}
				if !strings.Contains(w.Body.String(), terminal) {
					t.Fatal("plugin completion not converted", w.Body.String())
				}
			}
		}
	}
}
