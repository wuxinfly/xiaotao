package office

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestMemberTaskIdentityDiscussionAndSerialExecution(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	ids := makeTeamForChat(t, svc, project)
	chat, err := svc.SendChat(project, ids[0], ChatInput{Content: "讨论任务", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { item, _ := svc.Chat(project, ids[0]); return item.Status == StatusCompleted })
	exec.history[chat.SessionID] = &agent.HistoryPage{Session: &agent.Session{Messages: []agent.Message{{Role: "user", Content: "讨论任务"}}}}
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "实现方案", Model: "test/model", MemberID: ids[0], ChatSessionID: chat.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if task.MemberID != ids[0] || task.Executor.Name == executorName || task.Executor.Kind != "member" {
		t.Fatalf("member identity missing: %+v", task)
	}
	if !strings.Contains(task.Discussion, "讨论任务") || len(exec.created) != 2 || exec.created[1].Member == nil || exec.created[1].ReadOnly {
		t.Fatalf("member handoff invalid: %+v %+v", task, exec.created)
	}
	if _, err = svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "重复", Model: "test/model", MemberID: ids[0]}); !errors.Is(err, ErrMemberBusy) {
		t.Fatalf("concurrent member task: %v", err)
	}
	waitFor(t, func() bool { item, _ := svc.loadTask(task.ID); return isTerminal(item.Status) })
	snapshot, err := svc.Snapshot(project, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range snapshot.Members {
		if member.ID == task.SessionID {
			t.Fatal("member task created an anonymous executor")
		}
	}
}

func TestMemberTaskRejectsInvalidMembershipAndHandoff(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	ids := makeTeamForChat(t, svc, project)
	other := t.TempDir()
	if _, err := svc.CreateTask(CreateTaskRequest{ProjectPath: other, Goal: "越界", Model: "test/model", MemberID: ids[0]}); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("cross project member accepted: %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "伪造", Model: "test/model", MemberID: ids[0], ChatSessionID: "fake"}); err == nil {
		t.Fatal("forged chat accepted")
	}
}

func TestActiveMemberTaskPreventsRemoval(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	exec.run = func(ctx context.Context, _ string, _ string, _ func(agent.Event)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	svc, project := newTestService(t, exec)
	ids := makeTeamForChat(t, svc, project)
	if _, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "执行", Model: "test/model", MemberID: ids[0]}); err != nil {
		t.Fatal(err)
	}
	<-started
	team, _ := svc.teams.Snapshot(project)
	if _, err := svc.teams.Remove(project, ids[0], revisions(team)); !errors.Is(err, ErrMemberActive) {
		t.Fatalf("active task did not protect member: %v", err)
	}
}
