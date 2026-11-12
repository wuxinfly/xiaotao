package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/modelsource"
)

var protocolFixtures = map[string]string{
	"openai-chat":        `{"id":"up","model":"actual","choices":[{"index":0,"message":{"role":"assistant","content":"hello","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"x\":1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":2}}}`,
	"openai-responses":   `{"id":"up","object":"response","status":"completed","model":"actual","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"x\":1}"}],"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"input_tokens_details":{"cached_tokens":2}}}`,
	"anthropic-messages": `{"id":"up","type":"message","role":"assistant","model":"actual","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"call_1","name":"lookup","input":{"x":1}}],"stop_reason":"tool_use","usage":{"input_tokens":6,"cache_read_input_tokens":2,"output_tokens":3}}`,
}
var protocolStreams = map[string][]string{
	"openai-chat": {
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"x\":"}}]},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":2}}}`,
		`[DONE]`,
	},
	"openai-responses": {
		`{"type":"response.created","response":{"status":"in_progress"}}`,
		`{"type":"response.output_text.delta","output_index":0,"delta":"hello"}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"x\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"1}"}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"input_tokens_details":{"cached_tokens":2}}}}`,
	},
	"anthropic-messages": {
		`{"type":"message_start","message":{"usage":{"input_tokens":6,"cache_read_input_tokens":2}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"x\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"1}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`,
		`{"type":"message_stop"}`,
	},
}

func inboundProtocolBody(protocol, model string, stream bool) string {
	switch protocol {
	case "openai-chat":
		return fmt.Sprintf(`{"model":%q,"stream":%t,"stream_options":{"include_usage":true},"messages":[{"role":"system","content":"be helpful"},{"role":"user","content":[{"type":"text","text":"question"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}]},{"role":"assistant","tool_calls":[{"id":"history_call","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"history_call","content":"result"},{"role":"user","content":"continue"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`, model, stream)
	case "openai-responses":
		return fmt.Sprintf(`{"model":%q,"stream":%t,"store":false,"instructions":"be helpful","input":[{"role":"user","content":[{"type":"input_text","text":"question"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]},{"type":"function_call","call_id":"history_call","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"history_call","output":"result"},{"role":"user","content":"continue"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`, model, stream)
	default:
		return fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":64,"system":"be helpful","messages":[{"role":"user","content":[{"type":"text","text":"question"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]},{"role":"assistant","content":[{"type":"tool_use","id":"history_call","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"history_call","content":"result"},{"type":"text","text":"continue"}]}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`, model, stream)
	}
}
func serveProtocolFixture(w http.ResponseWriter, protocol string, stream, truncate bool) {
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, protocolFixtures[protocol])
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	events := protocolStreams[protocol]
	if truncate {
		events = events[:2]
	}
	for _, event := range events {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
		w.(http.Flusher).Flush()
	}
}
func setupProtocolGateway(t *testing.T, protocol string, handler http.HandlerFunc) (*Service, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	crypt := func(b []byte) ([]byte, error) { return b, nil }
	store, err := modelsource.Open(t.TempDir(), crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	key := "upstream-key"
	src, err := store.Save("", modelsource.Input{Label: "Protocol", Protocol: protocol, BaseURL: server.URL + "/v1" + map[string]string{"openai-chat": "/chat/completions", "openai-responses": "/responses", "anthropic-messages": "/messages"}[protocol], APIKey: &key, Models: []modelsource.Model{{UpstreamModelID: "actual", MaxOutputTokens: 64, SupportsImages: true}}})
	if err != nil {
		t.Fatal(err)
	}
	return New(store, "local-key", nil), src.Models[0].CallID
}
func callProtocol(svc *Service, protocol, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/endpoint", strings.NewReader(body))
	if protocol == "anthropic-messages" {
		r.Header.Set("x-api-key", "local-key")
	} else {
		r.Header.Set("Authorization", "Bearer local-key")
	}
	w := httptest.NewRecorder()
	switch protocol {
	case "openai-chat":
		svc.HandleChat(w, r)
	case "openai-responses":
		svc.HandleResponses(w, r)
	default:
		svc.HandleMessages(w, r)
	}
	return w
}
func TestProtocolMatrix(t *testing.T) {
	for upstream := range protocolFixtures {
		for inbound := range protocolFixtures {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-to-%s/stream=%t", inbound, upstream, stream), func(t *testing.T) {
					svc, id := setupProtocolGateway(t, upstream, func(w http.ResponseWriter, r *http.Request) {
						var body map[string]any
						if json.NewDecoder(r.Body).Decode(&body) != nil {
							t.Error("invalid upstream JSON")
						}
						if body["model"] != "actual" || body["stream"] != stream {
							t.Errorf("lost model/stream: %+v", body)
						}
						if upstream == "anthropic-messages" {
							if r.Header.Get("x-api-key") != "upstream-key" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
								t.Error("wrong Anthropic authentication")
							}
						} else if r.Header.Get("Authorization") != "Bearer upstream-key" {
							t.Error("wrong upstream auth")
						}
						expected := map[string]string{"openai-chat": "/v1/chat/completions", "openai-responses": "/v1/responses", "anthropic-messages": "/v1/messages"}[upstream]
						if r.URL.Path != expected {
							t.Errorf("wrong endpoint %s", r.URL.Path)
						}
						raw, _ := json.Marshal(body)
						for _, value := range []string{"history_call", "result", "be helpful", "aGVsbG8=", "lookup"} {
							if !strings.Contains(string(raw), value) {
								t.Errorf("lost %s: %s", value, raw)
							}
						}
						serveProtocolFixture(w, upstream, stream, false)
					})
					w := callProtocol(svc, inbound, inboundProtocolBody(inbound, id, stream))
					raw := w.Body.String()
					if w.Code != 200 {
						t.Fatalf("%d %s", w.Code, raw)
					}
					for _, value := range []string{"hello", "call_1", "lookup", id} {
						if !strings.Contains(raw, value) {
							t.Fatalf("missing %s: %s", value, raw)
						}
					}
					if stream {
						terminal := map[string]string{"openai-chat": "data: [DONE]", "openai-responses": "event: response.completed", "anthropic-messages": "event: message_stop"}[inbound]
						if !strings.Contains(raw, terminal) {
							t.Fatalf("missing terminal: %s", raw)
						}
						if strings.Count(raw, terminal) != 1 {
							t.Fatal("duplicate terminal")
						}
					} else {
						var body map[string]any
						if json.Unmarshal(w.Body.Bytes(), &body) != nil {
							t.Fatal("invalid JSON")
						}
						usage := body["usage"].(map[string]any)
						output := "output_tokens"
						if inbound == "openai-chat" {
							output = "completion_tokens"
						}
						if usage[output] != float64(3) {
							t.Fatalf("wrong usage: %+v", usage)
						}
					}
				})
			}
		}
	}
}
func TestNativeTruncatedStreamsNeverSucceed(t *testing.T) {
	for upstream := range protocolFixtures {
		for _, inbound := range []string{"openai-responses", "anthropic-messages"} {
			t.Run(upstream+"/"+inbound, func(t *testing.T) {
				svc, id := setupProtocolGateway(t, upstream, func(w http.ResponseWriter, r *http.Request) { serveProtocolFixture(w, upstream, true, true) })
				w := callProtocol(svc, inbound, inboundProtocolBody(inbound, id, true))
				raw := w.Body.String()
				if strings.Contains(raw, "event: message_stop") || strings.Contains(raw, "event: response.completed") {
					t.Fatal("truncation reported success", raw)
				}
				if !strings.Contains(raw, "error") {
					t.Fatal("missing error", raw)
				}
			})
		}
	}
}
