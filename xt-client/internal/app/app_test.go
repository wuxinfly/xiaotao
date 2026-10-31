package app

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigAndOrigin(t *testing.T) {
	dir := t.TempDir()
	s := State{URL: "http://127.0.0.1:12345", Token: "secret", PID: 42}
	h, e := Handler(s, dir, func() {})
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path, origin, host, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, s.URL+path, strings.NewReader(body))
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	host := "127.0.0.1:12345"
	if w := call("PUT", "/api/config", "http://evil.example", host, "{}"); w.Code != 403 {
		t.Fatalf("cross origin: %d", w.Code)
	}
	if w := call("GET", "/api/status", "", "evil.example", ""); w.Code != 403 {
		t.Fatalf("host: %d", w.Code)
	}
	if w := call("PUT", "/api/config", s.URL, host, `{"name":"测试空间"}`); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	// Second write exercises replacement of an existing file on Windows.
	if w := call("PUT", "/api/config", s.URL, host, `{"name":"再次保存"}`); w.Code != 200 {
		t.Fatalf("replace: %d", w.Code)
	}
	h, e = Handler(s, dir, func() {})
	if e != nil {
		t.Fatal(e)
	}
	if w := call("GET", "/api/config", "", host, ""); !strings.Contains(w.Body.String(), "再次保存") {
		t.Fatal(w.Body.String())
	}
	if w := call("GET", "/", "", host, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "小涛") {
		t.Fatal("page missing")
	}
	// WebView video playback needs byte ranges for seeking and metadata reads.
	r := httptest.NewRequest("GET", s.URL+"/startup.mp4", nil)
	r.Header.Set("Range", "bytes=0-31")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.Len() != 32 || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("startup video range: %d %s (%d bytes)", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
	if w := call("GET", "/v1/models", "", host, ""); w.Code != 401 {
		t.Fatalf("model endpoint without key: %d", w.Code)
	}
}
func TestCorruptConfigPreserved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte("broken"), 0600)
	if _, e := Handler(State{}, dir, func() {}); e == nil {
		t.Fatal("expected error")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "broken" {
		t.Fatal("overwrote invalid data")
	}
}
