package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/wireprotocol"
)

const countTokensPath = "/v1/messages/count_tokens"

func countTokensGateway(t *testing.T, handler http.HandlerFunc) (*Service, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	passthrough := func(data []byte) ([]byte, error) { return data, nil }
	store, err := modelsource.Open(t.TempDir(), passthrough, passthrough)
	if err != nil {
		t.Fatal(err)
	}
	key := "upstream-key"
	source, err := store.Save("", modelsource.Input{
		Label: "Anthropic test", Protocol: wireprotocol.Messages,
		BaseURL: server.URL + "/v1/messages", APIKey: &key,
		Models: []modelsource.Model{{UpstreamModelID: "upstream/model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(store, "local-key", nil), source.Models[0].CallID
}

func TestCountTokensForwardsAnthropicPayload(t *testing.T) {
	const fields = `"messages":[{"role":"user","content":"hi"}],"system":"system instructions","tools":[{"name":"example","input_schema":{"type":"object"}}],"provider_extension":{"number":9007199254740993}`
	svc, id := countTokensGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != countTokensPath {
			t.Errorf("upstream request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "upstream-key" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect upstream authentication")
		}
		var got, want map[string]any
		gotDecoder := json.NewDecoder(r.Body)
		gotDecoder.UseNumber()
		wantDecoder := json.NewDecoder(strings.NewReader(`{"model":"upstream/model",` + fields + "}"))
		wantDecoder.UseNumber()
		if gotDecoder.Decode(&got) != nil || wantDecoder.Decode(&want) != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("upstream payload = %#v, want %#v", got, want)
		}
		_, _ = io.WriteString(w, `{"input_tokens":42,"internal":"private upstream metadata"}`)
	})
	for name, value := range map[string]string{"x-api-key": "local-key", "Authorization": "Bearer local-key"} {
		r := httptest.NewRequest(http.MethodPost, countTokensPath, strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":false,%s}`, id, fields)))
		r.Header.Set(name, value)
		w := httptest.NewRecorder()
		svc.HandleCountTokens(w, r)
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"input_tokens":42}` {
			t.Fatalf("%s response: %d %s", name, w.Code, w.Body.String())
		}
	}
}

func TestCountTokensRejectsNonAnthropicUpstream(t *testing.T) {
	var called atomic.Bool
	svc, src, _ := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { called.Store(true) })
	r := httptest.NewRequest(http.MethodPost, countTokensPath, strings.NewReader(fmt.Sprintf(`{"model":%q}`, src.Models[0].ID)))
	r.Header.Set("Authorization", "Bearer local-key")
	w := httptest.NewRecorder()
	svc.HandleCountTokens(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Anthropic Messages") || called.Load() {
		t.Fatalf("response: %d %s, upstream called: %t", w.Code, w.Body.String(), called.Load())
	}
}

func TestCountTokensRejectsPlugin(t *testing.T) {
	svc, _ := setupPluginGateway(t)
	model := svc.Models()[0]["id"].(string)
	r := httptest.NewRequest(http.MethodPost, countTokensPath, strings.NewReader(fmt.Sprintf(`{"model":%q}`, model)))
	r.Header.Set("x-api-key", "local-key")
	w := httptest.NewRecorder()
	svc.HandleCountTokens(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Anthropic Messages") {
		t.Fatalf("response: %d %s", w.Code, w.Body.String())
	}
}

func TestCountTokensRejectsInvalidRequestsBeforeCallingUpstream(t *testing.T) {
	var called atomic.Bool
	svc, model := countTokensGateway(t, func(w http.ResponseWriter, r *http.Request) { called.Store(true) })
	for _, body := range []string{
		"null",
		fmt.Sprintf(`{"model":%q} {}`, model),
		fmt.Sprintf(`{"model":%q,"stream":true}`, model),
	} {
		r := httptest.NewRequest(http.MethodPost, countTokensPath, strings.NewReader(body))
		r.Header.Set("x-api-key", "local-key")
		w := httptest.NewRecorder()
		svc.HandleCountTokens(w, r)
		if w.Code != http.StatusBadRequest || called.Load() {
			t.Fatalf("request %s: %d %s, upstream called: %t", body, w.Code, w.Body.String(), called.Load())
		}
	}
}

func TestCountTokensMasksUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		errMsg string
	}{
		{"upstream rejection", http.StatusForbidden, `{"error":"private upstream error"}`, "Upstream rejected count_tokens request"},
		{"missing count", http.StatusOK, `{"internal":"private upstream metadata"}`, "Invalid count_tokens response"},
		{"non-numeric count", http.StatusOK, `{"input_tokens":"private upstream metadata"}`, "Invalid count_tokens response"},
		{"negative count", http.StatusOK, `{"input_tokens":-1}`, "Invalid count_tokens response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, model := countTokensGateway(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			r := httptest.NewRequest(http.MethodPost, countTokensPath, strings.NewReader(fmt.Sprintf(`{"model":%q}`, model)))
			r.Header.Set("x-api-key", "local-key")
			w := httptest.NewRecorder()
			svc.HandleCountTokens(w, r)
			want := fmt.Sprintf(`{"error":{"message":%q,"type":"api_error"},"type":"error"}`, tc.errMsg)
			if w.Code != http.StatusBadGateway || strings.TrimSpace(w.Body.String()) != want {
				t.Fatalf("response: %d %s, want 502 %s", w.Code, w.Body.String(), want)
			}
		})
	}
}
