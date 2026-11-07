package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameProjectPersistsAllSessions(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	root := t.TempDir()
	a, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(Options{ProjectPath: t.TempDir(), Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	a.Messages = []Message{{ID: "m", Role: "user", Content: "keep history"}}
	if err = svc.saveSession(a); err != nil {
		t.Fatal(err)
	}
	if err = svc.RenameProject(root, "新项目"); err != nil {
		t.Fatal(err)
	}
	reopened := NewService(data, "http://localhost", nil, nil, nil)
	for _, id := range []string{a.ID, b.ID} {
		item, err := reopened.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if item.Project.Name != "新项目" || item.Project.Path != root {
			t.Fatalf("unexpected project: %+v", item.Project)
		}
	}
	loaded, _ := reopened.Get(a.ID)
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "keep history" || !loaded.UpdatedAt.Equal(a.UpdatedAt) {
		t.Fatal("history or ordering changed")
	}
	untouched, _ := reopened.Get(other.ID)
	if untouched.Project.Name != other.Project.Name {
		t.Fatal("other project changed")
	}
	if err = svc.RenameProject(root, " "); err == nil {
		t.Fatal("accepted empty name")
	}
	svc.runs[b.ID].cancel = func() {}
	if err = svc.RenameProject(root, "blocked"); err == nil {
		t.Fatal("accepted running project")
	}
	loaded, _ = reopened.Get(a.ID)
	if loaded.Project.Name != "新项目" {
		t.Fatal("partial rename")
	}
}

func TestSavedProjectsRemainAfterSessionsAndRenamePersist(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	root := t.TempDir()
	if err := svc.SaveProject(root, "保留项目"); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if session.Project.Name != "保留项目" {
		t.Fatalf("saved display name missing: %+v", session.Project)
	}
	projects, err := svc.ListProjects()
	if err != nil || len(projects) != 1 || !projects[0].Saved || projects[0].Name != "保留项目" {
		t.Fatalf("saved project list: %+v %v", projects, err)
	}
	if err := svc.Delete(session.ID); err != nil {
		t.Fatal(err)
	}
	projects, err = svc.ListProjects()
	if err != nil || len(projects) != 1 || !projects[0].Saved || projects[0].Name != "保留项目" || projects[0].Path != root {
		t.Fatalf("saved project must survive empty state: %+v %v", projects, err)
	}
	if err := svc.RenameProject(root, "新名字"); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.savedProject(root)
	if err != nil || saved == nil || saved.Name != "新名字" {
		t.Fatalf("rename missing saved project: %+v %v", saved, err)
	}
}

func TestDeleteProjectOnlyRemovesSavedCategory(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	root := t.TempDir()
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveProject(root, "项目"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProject(root); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(session.ID)
	if err != nil || loaded.Project.Path != root || loaded.Project.Saved {
		t.Fatalf("session must remain under its project path: %+v %v", loaded, err)
	}
	if loaded.Project.Name != filepath.Base(root) {
		t.Fatalf("deleted saved category must not change session display name: %+v", loaded.Project)
	}
	projects, err := svc.ListProjects()
	if err != nil || len(projects) != 1 || projects[0].Path != root || projects[0].Saved {
		t.Fatalf("remaining sessions keep project visible: %+v %v", projects, err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("project folder must remain")
	}
	db, err := svc.openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw string
	if err = db.QueryRow("SELECT metadata FROM sessions WHERE id=?", session.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored Session
	if err = json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal("session project path changed unexpectedly")
	}
	if !strings.EqualFold(filepath.Clean(stored.Project.Path), filepath.Clean(root)) {
		t.Fatal("session project path changed unexpectedly")
	}
}
