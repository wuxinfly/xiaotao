package office

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/agent"
)

func TestTurnDeliverySeparatesExistingFilesAndCommands(t *testing.T) {
	executor := newFakeExecutor()
	svc, dir := newTestService(t, executor)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	write := func(name, text string) error { return os.WriteFile(filepath.Join(dir, name), []byte(text+"\n"), 0600) }
	git("init", "-q")
	for _, name := range []string{"mine", "shared", "source"} {
		if err := write(name, "original"); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "baseline")
	for _, name := range []string{"mine", "shared"} {
		if err := write(name, "before task"); err != nil {
			t.Fatal(err)
		}
	}
	executor.run = func(ctx context.Context, id, text string, emit func(agent.Event)) error {
		for _, name := range []string{"shared", "source", "generated"} {
			if err := write(name, "during task"); err != nil {
				return err
			}
		}
		emit(agent.Event{Type: "activity", Activity: &agent.Activity{ID: "check", Kind: "run_command", Status: "failed", Input: `{"command":"npm test"}`, Output: "one failed assertion"}})
		return nil
	}
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: dir, Goal: "test", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { current, e := svc.loadTask(task.ID); return e == nil && isTerminal(current.Status) })
	d, err := svc.TaskDetail(dir, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := d.Delivery
	if r == nil || r.FileNote != "" || len(r.Files) != 4 {
		t.Fatalf("unexpected receipt: %+v", r)
	}
	files := map[string]FileChange{}
	for _, f := range r.Files {
		files[f.Path] = f
	}
	if files["mine"].Diff != "" || !strings.Contains(files["mine"].Label, "内容未变") || !strings.Contains(files["shared"].Diff, "-before task") || !strings.Contains(files["source"].Diff, "-original") || !strings.Contains(files["generated"].Diff, "+during task") {
		t.Errorf("files: %+v", r.Files)
	}
	if len(r.Commands) != 1 || r.Commands[0].Command != "npm test" || r.Commands[0].Status != "failed" || r.Commands[0].Output != "one failed assertion" {
		t.Errorf("commands: %+v", r.Commands)
	}
}

func TestResumeInterruptedUsesEvidenceOnce(t *testing.T) {
	ex := newFakeExecutor()
	svc, project := newTestService(t, ex)
	task := &Task{ID: "crashed", ProjectPath: project, SessionID: "session", Status: StatusRunning, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	db, err := svc.openStore()
	if err != nil {
		t.Fatal(err)
	}
	err = svc.writeTask(db, task)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	svc.saveDelivery(task.ID, &Delivery{Confirmed: "修改 docs", Pending: map[string]string{"a1": "发布文档"}})
	if err = svc.recoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 1)
	ex.run = func(ctx context.Context, id, text string, emit func(agent.Event)) error {
		started <- text
		<-ctx.Done()
		return ctx.Err()
	}
	if _, err = svc.ResumeTask(project, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ResumeTask(project, task.ID); err == nil {
		t.Fatal("duplicate recovery accepted")
	}
	select {
	case prompt := <-started:
		if !strings.Contains(prompt, "最后确认完成：修改 docs") || !strings.Contains(prompt, "结果待核对：发布文档") {
			t.Fatal(prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no recovery turn")
	}
}

func TestWorkspaceReviewAcrossFirstCommitInSubdirectory(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s: %v", args, out, err)
		}
	}
	run("init", "-q")
	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	before, err := captureWorkspace(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(project, "inside"), filepath.Join(dir, "outside")} {
		if err = os.WriteFile(path, []byte("added\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "first")
	after, err := captureWorkspace(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := compareWorkspace(context.Background(), before, after)
	if err != nil || len(changes) != 1 || changes[0].Path != "inside" || !strings.Contains(changes[0].Diff, "+added") {
		t.Fatalf("changes=%+v err=%v", changes, err)
	}
}
