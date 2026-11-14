package office

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xiaotao/xt-client/internal/agent"
)

type fakeExecutor struct {
	mu       sync.Mutex
	sessions map[string]*agent.Session
	models   []agent.ModelOption
	events   []agent.Event
	run      func(ctx context.Context, sessionID, text string, emit func(agent.Event)) error
	history  map[string]*agent.HistoryPage
	created  []agent.Options
	canceled []string
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{
		sessions: map[string]*agent.Session{},
		models:   []agent.ModelOption{{Model: "test/model", Label: "测试模型"}},
		history:  map[string]*agent.HistoryPage{},
	}
}

func (f *fakeExecutor) CreateSession(options agent.Options) (*agent.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, options)
	session := &agent.Session{ID: "run_fake", Project: agent.Project{Path: options.ProjectPath, Name: options.ProjectName}, Role: options.Role, Model: options.Model, Status: agent.StatusIdle}
	f.sessions[session.ID] = session
	return session, nil
}

func (f *fakeExecutor) SetMember(sessionID string, member *agent.MemberIdentity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if session := f.sessions[sessionID]; session != nil {
		session.Member = member
	}
	return nil
}

func (f *fakeExecutor) RunTurn(ctx context.Context, sessionID, text, model string, _ *agent.ModelSettings, emit func(agent.Event)) error {
	f.mu.Lock()
	run := f.run
	f.mu.Unlock()
	if run == nil {
		emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_1"})
		emit(agent.Event{Type: "answer_end", SessionID: sessionID, RunID: "turn_1"})
		return nil
	}
	return run(ctx, sessionID, text, emit)
}

func (f *fakeExecutor) Cancel(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.canceled = append(f.canceled, sessionID)
	return true
}

func (f *fakeExecutor) History(sessionID, _ string) (*agent.HistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if page, ok := f.history[sessionID]; ok {
		return page, nil
	}
	return &agent.HistoryPage{Session: &agent.Session{ID: sessionID, Messages: []agent.Message{}, Activity: []agent.Activity{}}}, nil
}

func (f *fakeExecutor) ResolveApproval(string, string, bool) error { return nil }

func (f *fakeExecutor) ListSummaries() ([]*agent.Session, error) { return nil, nil }

func (f *fakeExecutor) AvailableModels() []agent.ModelOption {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.models
}

func (f *fakeExecutor) AccessMode() string { return agent.ModeSafe }

func newTestService(t *testing.T, exec *fakeExecutor) (*Service, string) {
	t.Helper()
	svc, err := New(t.TempDir(), exec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	project := t.TempDir()
	return svc, project
}

func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestTaskLifecycleAndQueuedMessage(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	release := make(chan struct{})
	exec.run = func(ctx context.Context, sessionID, text string, emit func(agent.Event)) error {
		emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_1"})
		emit(agent.Event{Type: "activity", SessionID: sessionID, Activity: &agent.Activity{ID: "act_1", RunID: "turn_1", Kind: "write_file", Title: "更新文件", Input: `{"path":"notes.md","content_bytes":5}`, Status: "completed", StartedAt: time.Now()}})
		emit(agent.Event{Type: "run_stats", SessionID: sessionID, RunStats: &agent.RunStats{RunID: "turn_1", ToolCalls: 1, FilesWritten: 1, ContextTokens: 1200}})
		if strings.Contains(text, "第一轮") {
			close(started)
			<-release
		}
		emit(agent.Event{Type: "status", SessionID: sessionID, RunID: "turn_1", Status: agent.StatusCompleted})
		return nil
	}
	svc, project := newTestService(t, exec)
	if err := os.WriteFile(filepath.Join(project, "notes.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, ProjectName: "演示项目", Goal: "第一轮", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	queued, err := svc.PostMessage(project, task.ID, "第二轮")
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != MessageQueued {
		t.Fatalf("message delivered while busy: %s", queued.Status)
	}
	close(release)
	waitFor(t, func() bool {
		detail, err := svc.TaskDetail(project, task.ID)
		return err == nil && detail.Task.Status == StatusCompleted && detail.Task.RunID == "turn_1"
	})
	waitFor(t, func() bool {
		detail, _ := svc.TaskDetail(project, task.ID)
		for _, message := range detail.Messages {
			if message.ID == queued.ID && message.Status == MessageDelivered && message.DeliveredAt != nil {
				return true
			}
		}
		return false
	})
	detail, err := svc.TaskDetail(project, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Artifacts) != 1 || detail.Artifacts[0].Path != "notes.md" || detail.Artifacts[0].TaskID != task.ID {
		t.Fatalf("artifact not recorded: %+v", detail.Artifacts)
	}
	if detail.Artifacts[0].SHA256 == "" || detail.Artifacts[0].Tool != "write_file" {
		t.Fatalf("artifact provenance incomplete: %+v", detail.Artifacts[0])
	}
	if detail.Task.Stats.ContextTokens != 1200 {
		t.Fatalf("stats lost: %+v", detail.Task.Stats)
	}
}

func TestProjectIsolationAndTaskLookup(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	other := t.TempDir()
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "做点事", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		loaded, err := svc.loadTask(task.ID)
		return err == nil && isTerminal(loaded.Status)
	})
	if _, err = svc.TaskDetail(other, task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-project task lookup: %v", err)
	}
	snapshot, err := svc.Snapshot(other, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != 0 || len(snapshot.Artifacts) != 0 {
		t.Fatalf("other project saw tasks: %+v", snapshot)
	}
	own, err := svc.Snapshot(project, "演示")
	if err != nil {
		t.Fatal(err)
	}
	if len(own.Tasks) != 1 || own.Tasks[0].ID != task.ID {
		t.Fatalf("own project lost task: %+v", own.Tasks)
	}
}

func TestRestartMarksRunningTasksInterrupted(t *testing.T) {
	exec := newFakeExecutor()
	dir := t.TempDir()
	first, err := New(dir, exec)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	// Persist a task exactly as a crash would leave it: running, no goroutine.
	now := time.Now()
	crashed := &Task{ID: "task_crash", ProjectPath: project, ProjectName: "项目", Title: "运行中任务", Goal: "运行中任务", Status: StatusRunning, SessionID: "run_crash", CreatedAt: now, UpdatedAt: now}
	db, err := first.openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err = first.writeTask(db, crashed); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	second, err := New(dir, exec)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	reloaded, err := second.loadTask(crashed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusFailed || !strings.Contains(reloaded.Error, "重启") {
		t.Fatalf("interrupted task not reconciled: %+v", reloaded)
	}
}

func TestCancelRunningTask(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	exec.run = func(ctx context.Context, sessionID, text string, emit func(agent.Event)) error {
		emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_1"})
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	svc, project := newTestService(t, exec)
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "长任务", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err = svc.CancelTask(project, task.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		detail, err := svc.TaskDetail(project, task.ID)
		return err == nil && detail.Task.Status == StatusCanceled
	})
	exec.mu.Lock()
	canceled := append([]string{}, exec.canceled...)
	exec.mu.Unlock()
	if len(canceled) != 1 {
		t.Fatalf("kernel cancel not called: %v", canceled)
	}
}

func TestApprovalEventSetsWaiting(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	release := make(chan struct{})
	exec.run = func(ctx context.Context, sessionID, text string, emit func(agent.Event)) error {
		emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_1"})
		emit(agent.Event{Type: "approval", SessionID: sessionID, Approval: &agent.Approval{ID: "approval_1", Tool: "run_command", Detail: "rm -rf x"}})
		close(started)
		<-release
		emit(agent.Event{Type: "status", SessionID: sessionID, RunID: "turn_1", Status: agent.StatusCompleted})
		return nil
	}
	svc, project := newTestService(t, exec)
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "确认任务", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	waitFor(t, func() bool {
		detail, err := svc.TaskDetail(project, task.ID)
		return err == nil && detail.Task.Status == StatusWaiting && detail.Task.Approval != nil
	})
	if err = svc.ResolveApproval(project, task.ID, "approval_1", true); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitFor(t, func() bool {
		detail, err := svc.TaskDetail(project, task.ID)
		return err == nil && detail.Task.Status == StatusCompleted && detail.Task.Approval == nil
	})
}

func TestQueuedMessageDuringTurnFinishIsDelivered(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	turns := 0
	exec.run = func(ctx context.Context, sessionID, text string, emit func(agent.Event)) error {
		mu.Lock()
		turns++
		turn := turns
		mu.Unlock()
		emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_" + string(rune('0'+turn))})
		if turn == 1 {
			close(started)
			<-release
		}
		emit(agent.Event{Type: "status", SessionID: sessionID, RunID: "turn_1", Status: agent.StatusCompleted})
		return nil
	}
	svc, project := newTestService(t, exec)
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "第一轮", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	queued, err := svc.PostMessage(project, task.ID, "第二轮")
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return turns >= 2
	})
	waitFor(t, func() bool {
		detail, err := svc.TaskDetail(project, task.ID)
		if err != nil || detail.Task.Status != StatusCompleted {
			return false
		}
		for _, message := range detail.Messages {
			if message.ID == queued.ID {
				return message.Status == MessageDelivered
			}
		}
		return false
	})
}

func TestLateStatsDoNotReviveCompletedTask(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	release := make(chan struct{})
	exec.run = func(ctx context.Context, sessionID, text string, emit func(agent.Event)) error {
		emit(agent.Event{Type: "session", SessionID: sessionID, RunID: "turn_1"})
		emit(agent.Event{Type: "status", SessionID: sessionID, RunID: "turn_1", Status: agent.StatusCompleted})
		close(started)
		<-release
		ended := time.Now()
		emit(agent.Event{Type: "run_stats", SessionID: sessionID, RunID: "turn_1", RunStats: &agent.RunStats{RunID: "turn_1", EndedAt: &ended}})
		return nil
	}
	svc, project := newTestService(t, exec)
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "收尾统计", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	close(release)
	waitFor(t, func() bool {
		detail, err := svc.TaskDetail(project, task.ID)
		return err == nil && detail.Task.Status == StatusCompleted
	})
	// Give the late stats event time to land, then confirm the task stayed done.
	time.Sleep(50 * time.Millisecond)
	detail, err := svc.TaskDetail(project, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != StatusCompleted {
		t.Fatalf("late stats revived task: %s", detail.Task.Status)
	}
}

func TestArtifactOutsideProjectRejected(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "越界写入", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.recordArtifact(task.ID, &agent.Activity{ID: "act_out", Kind: "write_file", Input: `{"path":"` + strings.ReplaceAll(outside, `\`, `\\`) + `"}`})
	detail, err := svc.TaskDetail(project, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Artifacts) != 0 {
		t.Fatalf("outside file recorded: %+v", detail.Artifacts)
	}
}

func TestRejectsUnknownModelAndMissingProject(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	if _, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "x", Model: "missing/model"}); err == nil {
		t.Fatal("unknown model accepted")
	}
	if _, err := svc.CreateTask(CreateTaskRequest{Goal: "x", Model: "test/model"}); err == nil {
		t.Fatal("missing project accepted")
	}
}
