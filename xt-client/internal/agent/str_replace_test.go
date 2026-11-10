package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStrReplace(t *testing.T) {
	for _, tc := range []struct {
		name, input, old, replacement, want string
		fail                                bool
	}{
		{"unique", "hello world", "world", "Go", "hello Go", false},
		{"multiple", "hello hello", "hello", "x", "hello hello", true},
		{"missing", "hello", "world", "x", "hello", true},
		{"empty old", "hello", "", "x", "hello", true},
		{"empty result", "hello", "hello", "", "hello", true},
		{"overlap", "aaa", "aa", "X", "Xa", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file.txt")
			if err := os.WriteFile(path, []byte(tc.input), 0640); err != nil {
				t.Fatal(err)
			}
			w, err := OpenWorkspace(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Read("file.txt", 0, 0); err != nil {
				t.Fatal(err)
			}
			err = w.StrReplace("file.txt", tc.old, tc.replacement)
			if (err != nil) != tc.fail {
				t.Fatalf("error: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.want {
				t.Fatalf("content %q: %v", data, err)
			}
			if !tc.fail {
				if err = w.Write("file.txt", "next", false); err != nil {
					t.Fatalf("observation not updated: %v", err)
				}
			}
			temps, _ := filepath.Glob(filepath.Join(dir, ".strreplace-*"))
			if len(temps) != 0 {
				t.Fatalf("temporary files remain: %v", temps)
			}
		})
	}
}

func TestStrReplaceConflictsAndLinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	w, _ := OpenWorkspace(dir)
	if err := w.StrReplace("file.txt", "original", "new"); err == nil {
		t.Fatal("expected read requirement")
	}
	if _, err := w.Read("file.txt", 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.StrReplace("file.txt", "changed", "new"); err == nil {
		t.Fatal("expected conflict")
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(path, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	if err := w.StrReplace("link.txt", "changed", "new"); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestStrReplaceExecuteRequiresReplacement(t *testing.T) {
	w, _ := OpenWorkspace(t.TempDir())
	_, err := w.Execute(context.Background(), "str_replace", json.RawMessage(`{"path":"x","old_str":"a"}`))
	if err == nil {
		t.Fatal("missing new_str accepted")
	}
}

func TestWriteEmptyProtection(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWorkspace(dir)
	if err := w.Write("new", "", false); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("file", "content", false); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("file", "", false); err == nil {
		t.Fatal("empty overwrite accepted")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "file"))
	if string(data) != "content" {
		t.Fatalf("content changed: %q", data)
	}
	if err := w.Write("file", "", true); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("file", "updated", false); err != nil {
		t.Fatal(err)
	}
}

func TestStrReplaceApprovalAndActivity(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWorkspace(dir)
	if err := os.WriteFile(filepath.Join(dir, "large"), []byte(strings.Repeat("a", 128*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"path":"large","old_str":"secret","new_str":"new secret","append":true}`)
	if _, ok := needsApproval(w, "str_replace", args); !ok {
		t.Fatal("large replacement must require approval")
	}
	input := localActivityInput("str_replace", args)
	if strings.Contains(input, "secret") || !strings.Contains(input, "old_str_bytes") {
		t.Fatalf("unexpected activity input: %s", input)
	}
}

func TestReplaceFileFailureKeepsTargetAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing-directory")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(target, []byte("new"), 0400); err == nil {
		t.Fatal("expected rename failure")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("target changed: %v", err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".strreplace-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary files: %v; %v", temps, err)
	}
}
