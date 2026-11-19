package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverManualSourceDraft(t *testing.T) {
	h, state := newSourceTestApp(t, t.TempDir())
	const path = "/api/model-sources/discover"
	const key = "draft-upstream-key"
	for _, protocol := range []string{"openai-chat", "openai-responses", "anthropic-messages"} {
		t.Run(protocol, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/custom/models" {
					t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL)
					w.WriteHeader(400)
					return
				}
				if protocol == "anthropic-messages" {
					if r.Header.Get("x-api-key") != key || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Anthropic authentication")
					}
					if r.URL.Query().Get("after_id") == "" {
						fmt.Fprint(w, `{"data":[{"id":"org/model","display_name":"Model"}],"has_more":true,"last_id":"org/model"}`)
						return
					}
					if r.URL.Query().Get("after_id") != "org/model" {
						t.Error("unexpected pagination cursor")
					}
					fmt.Fprint(w, `{"data":[{"id":"org/model"},{"id":"second"}],"has_more":false}`)
					return
				}
				if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("x-api-key") != "" {
					t.Error("wrong OpenAI authentication")
				}
				fmt.Fprint(w, `{"data":[{"id":"org/model"},{"id":"org/model"},{"id":""}]}`)
			}))
			defer upstream.Close()
			suffix := map[string]string{"openai-chat": "/chat/completions", "openai-responses": "/responses", "anthropic-messages": "/messages"}[protocol]
			body := fmt.Sprintf(`{"baseURL":%q,"protocol":%q,"apiKey":%q}`, upstream.URL+"/custom"+suffix, protocol, key)
			w := appHTTP(t, h, state, "POST", path, body)
			if w.Code != http.StatusOK || strings.Contains(w.Body.String(), key) {
				t.Fatalf("discovery: %d %s", w.Code, w.Body.String())
			}
			var result struct {
				Models []struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
				} `json:"models"`
			}
			expectedCount := 1
			if protocol == "anthropic-messages" {
				expectedCount = 2
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Models) != expectedCount || result.Models[0].ID != "org/model" {
				t.Fatalf("unexpected discovery results: %s", w.Body.String())
			}
			if protocol == "anthropic-messages" && (result.Models[0].DisplayName != "Model" || result.Models[1].ID != "second") {
				t.Fatalf("unexpected Anthropic discovery results: %s", w.Body.String())
			}
		})
	}
	w := appHTTP(t, h, state, "GET", "/api/model-sources", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sources":[]`) {
		t.Fatalf("discovery wrote to the source store: %d %s", w.Code, w.Body.String())
	}
}

func TestDiscoverManualSourceRejectsRedirect(t *testing.T) {
	h, state := newSourceTestApp(t, t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Error("catalog request followed the redirect")
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer upstream.Close()
	body := fmt.Sprintf(`{"baseURL":%q,"protocol":"openai-chat","apiKey":"draft-key"}`, upstream.URL)
	w := appHTTP(t, h, state, "POST", "/api/model-sources/discover", body)
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "draft-key") {
		t.Fatalf("redirect: %d %s", w.Code, w.Body.String())
	}
}
