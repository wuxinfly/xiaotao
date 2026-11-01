package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceRejectsEscapesAndRewritesInsideProject(t *testing.T) {
	root := t.TempDir()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Resolve("../outside.txt"); err == nil {
		t.Fatal("expected parent escape rejection")
	}
	if _, err := workspace.Resolve(filepath.Join(root, "absolute.txt")); err == nil {
		t.Fatal("expected absolute path rejection")
	}
	if err := workspace.Write("src/main.go", "package main\n", false); err != nil {
		t.Fatal(err)
	}
	text, err := workspace.Read("src/main.go", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "1\tpackage main") {
		t.Fatalf("unexpected read: %q", text)
	}
}

func TestWorkspaceCommandRunsInProjectAndRejectsDestructive(t *testing.T) {
	root := t.TempDir()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	command, _ := json.Marshal(map[string]any{"command": "Get-Location | Select-Object -ExpandProperty Path"})
	output, err := workspace.Execute(context.Background(), "run_command", command)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(output), strings.ToLower(root)) {
		t.Fatalf("command ran outside project: %q", output)
	}
	destructive, _ := json.Marshal(map[string]any{"command": "git reset --hard"})
	if _, err := workspace.Execute(context.Background(), "run_command", destructive); err == nil {
		t.Fatal("expected destructive command rejection")
	}
}

func TestSessionPersistsAndRejectsUnsafeID(t *testing.T) {
	data := t.TempDir()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(data, "http://127.0.0.1:9999", "", nil, nil)
	session, err := svc.Create(Options{ProjectPath: root, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.Path != root {
		t.Fatalf("project mismatch: %s", loaded.Project.Path)
	}
	if _, err := svc.Get("../accounts"); err == nil {
		t.Fatal("expected invalid session ID rejection")
	}
}
