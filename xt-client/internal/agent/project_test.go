package agent

import "testing"

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
