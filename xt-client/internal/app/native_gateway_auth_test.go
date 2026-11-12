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
		"/v1/responses": `{"model":"missing","input":"hi","store":false}`,
		"/v1/messages":  `{"model":"missing","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`,
	} {
		for _, authorized := range []bool{false, true} {
			r := httptest.NewRequest("POST", state.URL+path, strings.NewReader(body))
			r.Header.Set("Origin", "https://external-tool.test")
			if authorized {
				r.Header.Set("x-api-key", key.Key)
			} else {
				r.Header.Set("Authorization", "Bearer "+state.Token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expected := 401
			if authorized {
				expected = 400
			}
			if w.Code != expected {
				t.Fatalf("%s authorized=%t: %d %s", path, authorized, w.Code, w.Body.String())
			}
		}
	}
}
