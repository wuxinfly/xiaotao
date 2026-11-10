//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFileGlobWindows(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.go", "two.js", "THREE.GO"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("needle"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XIAOTAO_FILE_GLOB", "*.js")
	w, err := OpenWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.searchFiles(".", "needle", "*.go")
	if err != nil || !strings.Contains(result, "one.go") || !strings.Contains(result, "THREE.GO") || strings.Contains(result, "two.js") {
		t.Fatalf("filtered search: %s, %v", result, err)
	}
	result, err = w.searchFiles(".", "needle", "")
	if err != nil || !strings.Contains(result, "two.js") || !strings.Contains(result, "one.go") {
		t.Fatalf("unfiltered search: %s, %v", result, err)
	}
	if _, err = w.searchFiles(".", "needle", "["); err == nil {
		t.Fatal("invalid pattern accepted")
	}
}
