package office

import (
	"os"
	"path/filepath"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestArtifactListsHideMissingFilesWithoutDeletingHistory(t *testing.T) {
	svc, project := newTestService(t, newFakeExecutor())
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "整理资料", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deleted.md", "moved.md", "retained.md"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		svc.recordArtifact(task.ID, &agent.Activity{ID: name, Kind: "write_file", Input: `{"path":"` + name + `"}`})
	}
	initial, err := svc.Snapshot(project, "")
	if err != nil || len(initial.Artifacts) != 3 {
		t.Fatalf("before removing files: %+v, %v", initial, err)
	}
	if err := os.Remove(filepath.Join(project, "deleted.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(project, "moved.md"), filepath.Join(project, "renamed.md")); err != nil {
		t.Fatal(err)
	}
	assertRetained := func(items []*Artifact) {
		t.Helper()
		if len(items) != 1 || items[0].Path != "retained.md" {
			t.Fatalf("expected only retained.md, got %+v", items)
		}
	}
	snapshot, err := svc.Snapshot(project, "")
	if err != nil {
		t.Fatal(err)
	}
	assertRetained(snapshot.Artifacts)
	detail, err := svc.TaskDetail(project, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRetained(detail.Artifacts)
	items, err := svc.ProjectArtifacts(project, "")
	if err != nil {
		t.Fatal(err)
	}
	assertRetained(items)
	stale, err := svc.ArtifactDetail(project, artifactID(task.ID, "deleted.md"))
	if err != nil || stale.Available || stale.Error != "文件已被移动或删除" {
		t.Fatalf("stale reference must retain its history and explain why preview is unavailable: %+v, %v", stale, err)
	}
}
