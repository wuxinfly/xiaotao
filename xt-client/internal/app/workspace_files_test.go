package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const workspaceTestOrigin = "http://127.0.0.1:5455"

func newWorkspaceTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	registerWorkspaceRoutes(mux,
		func(r *http.Request) bool { return r.Header.Get("Origin") == workspaceTestOrigin },
		func(w http.ResponseWriter, v any) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(v)
		},
		func(w http.ResponseWriter, status int, message string) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message}})
		})
	return mux
}

func workspaceGet(t *testing.T, mux *http.ServeMux, target string, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", target+"?"+query.Encode(), nil)
	req.Header.Set("Origin", workspaceTestOrigin)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func workspacePostJSON(t *testing.T, mux *http.ServeMux, target, origin string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", target, bytes.NewReader(data))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func decodeWorkspaceBody[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return value
}

func TestWorkspaceListRootAndNested(t *testing.T) {
	project := t.TempDir()
	for _, dir := range []string{"alpha", "Beta", "nested/deep"} {
		if err := os.MkdirAll(filepath.Join(project, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zebra.txt", "Apple.txt", "nested/child.txt", "nested/deep/leaf.txt"} {
		if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(name)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mux := newWorkspaceTestMux(t)

	root := decodeWorkspaceBody[workspaceListing](t, workspaceGet(t, mux, "/api/workspace/files", url.Values{"projectPath": {project}}))
	if root.Path != "" || root.Parent != "" {
		t.Fatalf("root path/parent: %q %q", root.Path, root.Parent)
	}
	if root.Root == "" || !filepath.IsAbs(root.Root) {
		t.Fatalf("root should be absolute: %q", root.Root)
	}
	wantRoot := []string{"alpha", "Beta", "nested", "Apple.txt", "zebra.txt"}
	if names := workspaceNames(root.Entries); !equalStrings(names, wantRoot) {
		t.Fatalf("root order: %v want %v", names, wantRoot)
	}
	for _, entry := range root.Entries {
		wantDir := entry.Name == "alpha" || entry.Name == "Beta" || entry.Name == "nested"
		if entry.Dir != wantDir {
			t.Fatalf("entry %+v", entry)
		}
		if _, err := time.Parse(time.RFC3339, entry.Modified); err != nil {
			t.Fatalf("entry modified %q: %v", entry.Modified, err)
		}
	}

	nested := decodeWorkspaceBody[workspaceListing](t, workspaceGet(t, mux, "/api/workspace/files", url.Values{"projectPath": {project}, "path": {"nested/deep"}}))
	if nested.Path != "nested/deep" || nested.Parent != "nested" {
		t.Fatalf("nested path/parent: %q %q", nested.Path, nested.Parent)
	}
	if names := workspaceNames(nested.Entries); !equalStrings(names, []string{"leaf.txt"}) {
		t.Fatalf("nested entries: %v", names)
	}
	if nested.Entries[0].Path != "nested/deep/leaf.txt" {
		t.Fatalf("nested entry path: %q", nested.Entries[0].Path)
	}
}

func TestWorkspaceListRejectsTraversalAndAbsolute(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(project, outside)
	if err != nil {
		t.Fatal(err)
	}
	mux := newWorkspaceTestMux(t)
	for _, bad := range []string{"../", relative, secret, "sub/../" + filepath.Base(secret)} {
		w := workspaceGet(t, mux, "/api/workspace/files", url.Values{"projectPath": {project}, "path": {bad}})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("path %q: %d %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestWorkspaceListRejectsSymlinkEscape(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(project, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	w := workspaceGet(t, newWorkspaceTestMux(t), "/api/workspace/files", url.Values{"projectPath": {project}, "path": {"escape"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("symlink escape: %d %s", w.Code, w.Body.String())
	}
}

func TestWorkspaceListTruncatesAtLimit(t *testing.T) {
	project := t.TempDir()
	for i := 0; i <= workspaceListLimit; i++ {
		name := filepath.Join(project, fmt.Sprintf("f%04d.txt", i))
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := workspaceGet(t, newWorkspaceTestMux(t), "/api/workspace/files", url.Values{"projectPath": {project}})
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	listing := decodeWorkspaceBody[workspaceListing](t, w)
	if !listing.Truncated || len(listing.Entries) != workspaceListLimit {
		t.Fatalf("truncated=%v entries=%d", listing.Truncated, len(listing.Entries))
	}
}

func TestWorkspaceFilePreviewKinds(t *testing.T) {
	project := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(name)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hello.txt", []byte("hello 世界\n"))
	write("big.txt", bytes.Repeat([]byte("a"), workspacePreviewLimit+100))
	write("nul.bin", []byte{'a', 0, 'b'})
	write("pixel.png", onePixelPNG())
	write("huge.txt", bytes.Repeat([]byte("b"), workspaceFileLimit+1))
	mux := newWorkspaceTestMux(t)
	preview := func(rel string) workspaceFilePreview {
		t.Helper()
		w := workspaceGet(t, mux, "/api/workspace/file", url.Values{"projectPath": {project}, "path": {rel}})
		if w.Code != http.StatusOK {
			t.Fatalf("preview %s: %d %s", rel, w.Code, w.Body.String())
		}
		return decodeWorkspaceBody[workspaceFilePreview](t, w)
	}

	text := preview("hello.txt")
	if text.Kind != "text" || text.Content != "hello 世界\n" || text.Truncated || text.Name != "hello.txt" {
		t.Fatalf("text preview: %+v", text)
	}
	big := preview("big.txt")
	if big.Kind != "text" || !big.Truncated || len(big.Content) != workspacePreviewLimit {
		t.Fatalf("big preview: kind=%s truncated=%v len=%d", big.Kind, big.Truncated, len(big.Content))
	}
	binary := preview("nul.bin")
	if binary.Kind != "binary" || binary.Content != "" {
		t.Fatalf("binary preview: %+v", binary)
	}
	image := preview("pixel.png")
	if image.Kind != "image" || image.Content != "" {
		t.Fatalf("image preview: %+v", image)
	}
	huge := preview("huge.txt")
	if huge.Kind != "too-large" || huge.Content != "" || !huge.Truncated {
		t.Fatalf("too-large preview: %+v", huge)
	}

	dir := workspaceGet(t, mux, "/api/workspace/file", url.Values{"projectPath": {project}, "path": {"."}})
	if dir.Code != http.StatusBadRequest {
		t.Fatalf("directory preview: %d %s", dir.Code, dir.Body.String())
	}
	missing := workspaceGet(t, mux, "/api/workspace/file", url.Values{"projectPath": {project}, "path": {"missing.txt"}})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing preview: %d %s", missing.Code, missing.Body.String())
	}
}

func TestWorkspaceFileRaw(t *testing.T) {
	project := t.TempDir()
	png := onePixelPNG()
	if err := os.WriteFile(filepath.Join(project, "pixel.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "note.txt"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := newWorkspaceTestMux(t)
	w := workspaceGet(t, mux, "/api/workspace/file/raw", url.Values{"projectPath": {project}, "path": {"pixel.png"}})
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("raw png: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"))
	}
	if !bytes.Equal(w.Body.Bytes(), png) {
		t.Fatalf("raw body mismatch: %d bytes", w.Body.Len())
	}
	notImage := workspaceGet(t, mux, "/api/workspace/file/raw", url.Values{"projectPath": {project}, "path": {"note.txt"}})
	if notImage.Code != http.StatusBadRequest {
		t.Fatalf("raw non-image: %d %s", notImage.Code, notImage.Body.String())
	}
	big := filepath.Join(project, "big.png")
	if err := os.WriteFile(big, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, workspaceImageLimit+1); err != nil {
		t.Fatal(err)
	}
	tooBig := workspaceGet(t, mux, "/api/workspace/file/raw", url.Values{"projectPath": {project}, "path": {"big.png"}})
	if tooBig.Code != http.StatusBadRequest {
		t.Fatalf("raw too big: %d %s", tooBig.Code, tooBig.Body.String())
	}
}

func TestOpenExternalURLValidation(t *testing.T) {
	original := openExternalURL
	var opened string
	openExternalURL = func(target string) error { opened = target; return nil }
	t.Cleanup(func() { openExternalURL = original })
	mux := newWorkspaceTestMux(t)

	w := workspacePostJSON(t, mux, "/api/system/open-url", workspaceTestOrigin, map[string]string{"url": "https://example.com/a?b=1"})
	if w.Code != http.StatusOK || opened != "https://example.com/a?b=1" {
		t.Fatalf("valid url: %d %s opened=%q", w.Code, w.Body.String(), opened)
	}
	for _, bad := range []string{
		"javascript:alert(1)",
		"file:///C:/Windows/win.ini",
		"https://" + "user" + ":" + "secret" + "@example.com/",
		"ftp://example.com/file",
		"https://",
		"https://example.com/" + strings.Repeat("a", workspaceURLLimit),
	} {
		w := workspacePostJSON(t, mux, "/api/system/open-url", workspaceTestOrigin, map[string]string{"url": bad})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("bad url %q: %d %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestOpenExternalURLErrorIsReported(t *testing.T) {
	original := openExternalURL
	openExternalURL = func(string) error { return errors.New("no browser") }
	t.Cleanup(func() { openExternalURL = original })
	w := workspacePostJSON(t, newWorkspaceTestMux(t), "/api/system/open-url", workspaceTestOrigin, map[string]string{"url": "https://example.com"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("browser error: %d %s", w.Code, w.Body.String())
	}
}

func TestWorkspaceRoutesRequireSameOrigin(t *testing.T) {
	mux := newWorkspaceTestMux(t)
	project := t.TempDir()
	targets := []string{
		"/api/workspace/files?" + url.Values{"projectPath": {project}}.Encode(),
		"/api/workspace/file?" + url.Values{"projectPath": {project}, "path": {"a.txt"}}.Encode(),
		"/api/workspace/file/raw?" + url.Values{"projectPath": {project}, "path": {"a.png"}}.Encode(),
	}
	for _, target := range targets {
		req := httptest.NewRequest("GET", target, nil)
		req.Header.Set("Origin", "http://evil.example")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "same-origin required") {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
	}
	w := workspacePostJSON(t, mux, "/api/system/open-url", "http://evil.example", map[string]string{"url": "https://example.com"})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "same-origin required") {
		t.Fatalf("open-url: %d %s", w.Code, w.Body.String())
	}
}

func workspaceNames(entries []workspaceEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// onePixelPNG is a valid 1x1 transparent PNG.
func onePixelPNG() []byte {
	return []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
		0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
		0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00,
		0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60,
		0x82,
	}
}
