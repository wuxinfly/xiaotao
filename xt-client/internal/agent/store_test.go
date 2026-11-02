package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyMigrationAndDefaultWorkspace(t *testing.T) {
	data := t.TempDir()
	dir := filepath.Join(data, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := Session{ID: "run_legacy", Title: "old", Model: "test", Project: Project{Path: t.TempDir()}, Messages: []Message{{ID: "m", Role: "user", Content: "hello"}}, Activity: []Activity{{ID: "a", Title: "read"}}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	b, _ := json.Marshal(old)
	original := filepath.Join(dir, old.ID+".json")
	if err := os.WriteFile(original, b, 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(data, "http://localhost", nil, nil, nil)
	items, err := svc.List()
	if err != nil || len(items) != 1 || len(items[0].Messages) != 1 || len(items[0].Activity) != 1 {
		t.Fatalf("migration: %+v %v", items, err)
	}
	items[0].Title = "updated"
	if err = svc.saveSession(items[0]); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(data, "http://localhost", nil, nil, nil)
	items, err = restarted.List()
	if err != nil || len(items) != 1 || items[0].Title != "updated" {
		t.Fatalf("restart: %+v %v", items, err)
	}
	got, err := os.ReadFile(original)
	if err != nil || string(got) != string(b) {
		t.Fatal("legacy file changed")
	}
	session, err := restarted.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if session.Project.Path != filepath.Join(data, "workspace") {
		t.Fatalf("unexpected default path: %s", session.Project.Path)
	}
	workspace, err := OpenWorkspace(session.Project.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Resolve("../agent.db"); err == nil {
		t.Fatal("workspace must not expose database")
	}
}
func TestInvalidLegacyDoesNotPartiallyMigrate(t *testing.T) {
	data := t.TempDir()
	dir := filepath.Join(data, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	valid, _ := json.Marshal(Session{ID: "run_good"})
	if err := os.WriteFile(filepath.Join(dir, "run_good.json"), valid, 0600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "run_bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(data, "http://localhost", nil, nil, nil)
	if _, err := svc.List(); err == nil {
		t.Fatal("expected import error")
	}
	if err := os.Remove(bad); err != nil {
		t.Fatal(err)
	}
	items, err := svc.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("retry: %+v %v", items, err)
	}
}

func TestSummariesPreserveInterruptedSessionHistory(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	original := &Session{ID: "run_interrupted", Status: StatusRunning, Messages: []Message{{ID: "m", Content: "hello"}}, Activity: []Activity{{ID: "a", Title: "read"}}, UpdatedAt: time.Now()}
	if err := svc.saveSession(original); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListSummaries()
	if err != nil || len(items) != 1 || items[0].Status != StatusFailed || len(items[0].Messages) != 0 || len(items[0].Activity) != 0 {
		t.Fatalf("summary: %+v %v", items, err)
	}
	full, err := svc.Get(original.ID)
	if err != nil || full.Status != StatusFailed || len(full.Messages) != 1 || len(full.Activity) != 1 {
		t.Fatalf("history: %+v %v", full, err)
	}
}
