package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeEndpointsUseGatewayKeyOutsideManagementOrigin(t *testing.T) {
	h, state := newSourceTestApp(t, t.TempDir())
	keyResponse := appHTTP(t, h, state, "GET", "/api/proxy/key", "")
	var key struct{ Key string }
	if json.Unmarshal(keyResponse.Body.Bytes(), &key) != nil || key.Key == "" {
		t.Fatal("missing local gateway key")
	}
	for path, body := range map[string]string{
		"/v1/responses":             `{"model":"missing","input":"hi","store":false}`,
		"/v1/messages":              `{"model":"missing","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`,
		"/v1/messages/count_tokens": `{"model":"missing","messages":[{"role":"user","content":"hi"}]}`,
	} {
		for _, tc := range []struct {
			name, header, value string
			want                int
		}{
			{name: "no key", want: 401},
			{name: "management key", header: "Authorization", value: "Bearer " + state.Token, want: 401},
			{name: "gateway key", header: "x-api-key", value: key.Key, want: 400},
		} {
			r := httptest.NewRequest("POST", state.URL+path, strings.NewReader(body))
			r.Header.Set("Origin", "https://external-tool.test")
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%s with %s: %d %s", path, tc.name, w.Code, w.Body.String())
			}
		}
	}
}
