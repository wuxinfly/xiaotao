package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/modelsource"
)

func TestManualSourcesCRUDAndLocalCatalog(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	h, state := newSourceTestApp(t, t.TempDir())
	create := func(label string) modelsource.Source {
		t.Helper()
		input := fmt.Sprintf(`{"label":%q,"protocol":"openai-chat","baseURL":%q,"apiKey":"private-upstream-key","models":[{"upstreamModelId":"org/model","label":"同名模型","contextWindow":64000,"maxOutputTokens":2048,"supportsImages":true,"supportsReasoning":true,"reasoningEfforts":["low","high"]}]}`, label, upstream.URL+"/v1")
		w := appHTTP(t, h, state, "POST", "/api/model-sources", input)
		if w.Code != 200 || strings.Contains(w.Body.String(), "private-upstream-key") {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var source modelsource.Source
		if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
			t.Fatal(err)
		}
		return source
	}
	first, second := create("接口一"), create("接口二")
	if first.ID == second.ID || first.Models[0].ID == second.Models[0].ID {
		t.Fatal("same upstream model collided across sources")
	}
	for _, path := range []string{"/api/model-sources", "/api/agent/models", "/api/accounts"} {
		w := appHTTP(t, h, state, "GET", path, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "private-upstream-key") {
			t.Fatalf("read: %s %d %s", path, w.Code, w.Body.String())
		}
	}
	for range 2 {
		w := appHTTP(t, h, state, "POST", "/api/agent/models/refresh", "{}")
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("reading local catalog contacted upstream")
	}
	models := readAgentCatalog(t, h, state)
	if len(models) != 2 || models[0].ContextWindow != 64000 || models[0].MaxOutputTokens != 2048 || !models[0].SupportsImages || models[0].SourceID != first.ID {
		t.Fatalf("model metadata: %+v", models)
	}
	input := modelsource.Input{Label: "改名接口", Protocol: first.Protocol, BaseURL: first.BaseURL, Models: first.Models}
	raw, _ := json.Marshal(input)
	w := appHTTP(t, h, state, "PUT", "/api/model-sources/"+first.ID, string(raw))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"hasAPIKey":true`) || !strings.Contains(w.Body.String(), first.Models[0].ID) {
		t.Fatalf("edit with retained key: %d %s", w.Code, w.Body.String())
	}
	disabled := false
	input.Enabled = &disabled
	raw, _ = json.Marshal(input)
	w = appHTTP(t, h, state, "PUT", "/api/model-sources/"+first.ID, string(raw))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if got := readAgentCatalog(t, h, state); len(got) != 1 || got[0].SourceID != second.ID {
		t.Fatalf("disabled source advertised: %+v", got)
	}
	if w = appHTTP(t, h, state, "POST", "/api/model-sources/"+second.ID+"/test", "{}"); w.Code != 200 {
		t.Fatalf("test saved source: %d %s", w.Code, w.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("connection test calls: %d", calls.Load())
	}
	if w = appHTTP(t, h, state, "DELETE", "/api/model-sources/"+second.ID, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if got := readAgentCatalog(t, h, state); len(got) != 0 {
		t.Fatalf("deleted source advertised: %+v", got)
	}
	// Stable old IDs remain unmapped; they are never resolved by matching names.
	r := httptest.NewRequest("POST", state.URL+"/v1/chat/completions", strings.NewReader(`{"model":"trae/org/model","messages":[{"role":"user","content":"hi"}]}`))
	keyRes := appHTTP(t, h, state, "GET", "/api/proxy/key", "")
	var key struct{ Key string }
	_ = json.Unmarshal(keyRes.Body.Bytes(), &key)
	r.Header.Set("Authorization", "Bearer "+key.Key)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, r)
	if recorder.Code < 400 {
		t.Fatalf("legacy model unexpectedly routed: %d", recorder.Code)
	}
	if calls.Load() != 1 {
		t.Fatal("unknown model reached upstream")
	}
}

func readAgentCatalog(t *testing.T, h http.Handler, state State) []agent.ModelOption {
	t.Helper()
	w := appHTTP(t, h, state, "GET", "/api/agent/models", "")
	var result struct{ Models []agent.ModelOption }
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Models
}

func TestManualSourcesRejectCrossOriginAndInvalidUpdates(t *testing.T) {
	h, state := newSourceTestApp(t, t.TempDir())
	for _, path := range []string{"/api/model-sources", "/api/agent/models/refresh", "/api/model-sources/unknown/test"} {
		r := httptest.NewRequest("POST", state.URL+path, strings.NewReader("{}"))
		r.Header.Set("Origin", "http://evil.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("cross-origin %s: %d", path, w.Code)
		}
	}
	for _, body := range []string{"{}", `{"apiKey":"secret","unexpected":true}`, `{} {}`} {
		if w := appHTTP(t, h, state, "POST", "/api/model-sources", body); w.Code != 400 {
			t.Fatalf("invalid input: %d %s", w.Code, w.Body.String())
		}
	}
	if w := appHTTP(t, h, state, "DELETE", "/api/model-sources/unknown", ""); w.Code != 404 {
		t.Fatalf("missing source: %d", w.Code)
	}
}
