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

func TestDeletingSessionPreservesImplicitProject(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	root := t.TempDir()
	session, err := svc.Create(Options{ProjectPath: root, ProjectName: "原项目", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(session.ID); err != nil {
		t.Fatal(err)
	}
	reopened := NewService(data, "http://localhost", nil, nil, nil)
	projects, err := reopened.ListProjects()
	if err != nil || len(projects) != 1 || !projects[0].Saved || projects[0].Path != root || projects[0].Name != "原项目" {
		t.Fatalf("project missing after deletion and restart: %+v %v", projects, err)
	}
	if err = reopened.DeleteProject(root); err != nil {
		t.Fatal(err)
	}
	projects, err = reopened.ListProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("deleted project returned: %+v %v", projects, err)
	}
}

func TestDeletingRemovedProjectSessionDoesNotRestoreProject(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	root := t.TempDir()
	if err := svc.SaveProject(root, "项目"); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.DeleteProject(root); err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(session.ID); err != nil {
		t.Fatal(err)
	}
	projects, err := svc.ListProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("removed project resurrected: %+v %v", projects, err)
	}
}

func TestDeleteProjectSendsSessionsToRecent(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	root := t.TempDir()
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	session.Messages = []Message{{ID: "m", Role: "user", Content: "keep history"}}
	if err = svc.saveSession(session); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveProject(root, "项目"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProject(root); err != nil {
		t.Fatal(err)
	}
	// The session keeps its working directory and history but is flagged as
	// recent so the sidebar stops showing a project group for it.
	loaded, err := svc.Get(session.ID)
	if err != nil || loaded.Project.Path != root || loaded.Project.Saved || !loaded.Project.Removed {
		t.Fatalf("session must stay under its project path but move to recent: %+v %v", loaded.Project, err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "keep history" {
		t.Fatal("history changed by project deletion")
	}
	projects, err := svc.ListProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("deleted project must disappear from the project list: %+v %v", projects, err)
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
	if !strings.EqualFold(filepath.Clean(stored.Project.Path), filepath.Clean(root)) || !stored.Project.Removed {
		t.Fatal("stored session must keep its path and the recent flag")
	}
	// Saving the project again brings its sessions back into the group.
	if err = svc.SaveProject(root, "项目"); err != nil {
		t.Fatal(err)
	}
	loaded, err = svc.Get(session.ID)
	if err != nil || loaded.Project.Removed {
		t.Fatalf("re-saving the project must reclaim its sessions: %+v %v", loaded.Project, err)
	}
	projects, err = svc.ListProjects()
	if err != nil || len(projects) != 1 || !projects[0].Saved || projects[0].Path != root {
		t.Fatalf("re-saved project missing: %+v %v", projects, err)
	}
}

// Sessions created under a deleted project stay in the recent list until the
// project is saved again.
func TestCreateAfterProjectDeleteStaysRecent(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://localhost", nil, nil, nil)
	root := t.TempDir()
	if err := svc.SaveProject(root, "项目"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProject(root); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Create(Options{ProjectPath: root, Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if session.Project.Removed || session.Project.Saved {
		t.Fatalf("a new session belongs to the project again: %+v", session.Project)
	}
	projects, err := svc.ListProjects()
	if err != nil || len(projects) != 1 || projects[0].Saved || projects[0].Path != root {
		t.Fatalf("session-backed project listed once: %+v %v", projects, err)
	}
}
