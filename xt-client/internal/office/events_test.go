package office

import (
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestTaskEventsAreScopedAndUnsubscribeCloses(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	task, err := svc.CreateTask(CreateTaskRequest{ProjectPath: project, Goal: "事件", Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe, err := svc.SubscribeTask(project, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.SubscribeTask(t.TempDir(), task.ID); err == nil {
		t.Fatal("cross-project event subscription accepted")
	}
	svc.publishTask(task.ID, agent.Event{Type: "status", Status: StatusRunning})
	found := false
	for len(events) > 0 {
		if event := <-events; event.Status == StatusRunning {
			found = true
		}
	}
	if !found {
		t.Fatal("task event not delivered")
	}
	unsubscribe()
	unsubscribe()
	if _, open := <-events; open {
		t.Fatal("unsubscribe did not close event stream")
	}
	svc.closeTaskEvents()
}
