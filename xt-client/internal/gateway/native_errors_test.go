package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeAuthenticationAndUnsupportedRequestsNeverReachUpstream(t *testing.T) {
	var calls atomic.Int32
	svc, id := setupProtocolGateway(t, "openai-chat", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		serveProtocolFixture(w, "openai-chat", false, false)
	})
	for _, protocol := range []string{"openai-responses", "anthropic-messages"} {
		r := httptest.NewRequest("POST", "/v1/test", strings.NewReader(inboundProtocolBody(protocol, id, false)))
		w := httptest.NewRecorder()
		if protocol == "openai-responses" {
			svc.HandleResponses(w, r)
		} else {
			svc.HandleMessages(w, r)
		}
		if w.Code != 401 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, body := range []string{
		fmt.Sprintf(`{"model":%q,"input":"hi","previous_response_id":"old"}`, id),
		fmt.Sprintf(`{"model":%q,"input":"hi","tools":[{"type":"web_search"}]}`, id),
		fmt.Sprintf(`{"model":%q,"input":"hi","stream":"yes"}`, id),
		fmt.Sprintf(`{"model":%q,"input":"hi","max_output_tokens":1.5}`, id),
		fmt.Sprintf(`{"model":%q,"input":"hi","store":true}`, id),
		`{} {}`,
	} {
		w := callProtocol(svc, "openai-responses", body)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid native request reached upstream")
	}
}
func TestNativeCancelClosesUpstream(t *testing.T) {
	for _, upstream := range []string{"openai-responses", "anthropic-messages"} {
		t.Run(upstream, func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			svc, id := setupProtocolGateway(t, upstream, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, data := range protocolStreams[upstream][:2] {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				}
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
				close(stopped)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(inboundProtocolBody("openai-responses", id, true))).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer local-key")
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { svc.HandleResponses(w, r); close(done) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("upstream did not start")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("downstream cancellation blocked")
			}
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("upstream was not canceled")
			}
			if strings.Contains(w.Body.String(), "event: response.completed") {
				t.Fatal("cancellation reported success")
			}
		})
	}
}
func TestNativeRedirectNeverForwardsCredentials(t *testing.T) {
	var called atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Store(true) }))
	defer destination.Close()
	svc, id := setupProtocolGateway(t, "anthropic-messages", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	})
	w := callProtocol(svc, "openai-responses", inboundProtocolBody("openai-responses", id, false))
	if w.Code != 502 || called.Load() {
		t.Fatal(w.Code, called.Load(), w.Body.String())
	}
}
func TestNativeUpstreamErrorsDoNotLeakKeysOrEmitSuccess(t *testing.T) {
	for _, stream := range []bool{false, true} {
		svc, id := setupProtocolGateway(t, "anthropic-messages", func(w http.ResponseWriter, r *http.Request) {
			if stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"error\",\"error\":{\"message\":\"upstream-key\"}}\n\n")
			} else {
				w.WriteHeader(401)
				_, _ = io.WriteString(w, `{"error":{"message":"upstream-key"}}`)
			}
		})
		w := callProtocol(svc, "openai-responses", inboundProtocolBody("openai-responses", id, stream))
		if w.Code != 502 || strings.Contains(w.Body.String(), "upstream-key") || strings.Contains(w.Body.String(), "event: response.completed") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestNativeNonStreamingConversionFailureIsRecordedAsFailure(t *testing.T) {
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call","type":"function","function":{"name":"lookup","arguments":"invalid JSON"}}]},"finish_reason":"tool_calls"}]}`)
	})
	w := callProtocol(svc, "anthropic-messages", inboundProtocolBody("anthropic-messages", src.Models[0].ID, false))
	if w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	if rec := summary(t, u); rec.Status != "failed" {
		t.Fatal("native conversion failure reported as successful usage", rec.Status)
	}
}
