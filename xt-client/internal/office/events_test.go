package office

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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
	svc.CloseEventSubscriptions()
}

func TestEmployeeEventsDeliverChunksBeforeCompletion(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"第一段🌊"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"第二段"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	data, project := t.TempDir(), t.TempDir()
	kernel := agent.NewService(filepath.Join(data, "agent"), server.URL, nil, server.Client(), func() []agent.ModelOption {
		return []agent.ModelOption{{Model: "test/model"}}
	})
	svc, err := New(filepath.Join(data, "office"), NewAgentExecutor(kernel))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	parent, err := kernel.Create(agent.Options{ProjectPath: project, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	member := makeTeamForChat(t, svc, project)[0]
	if _, _, err := svc.SubscribeConversation(parent.ID, t.TempDir(), member); err == nil {
		t.Fatal("接受了跨项目订阅")
	}
	events, unsubscribe, err := svc.SubscribeConversation(parent.ID, project, member)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	if _, err = svc.SendScopedChat(parent.ID, project, member, ChatInput{Content: "你好"}); err != nil {
		t.Fatal(err)
	}
	next := func(kind string) agent.Event {
		t.Helper()
		timeout := time.After(3 * time.Second)
		for {
			select {
			case event := <-events:
				if event.Type == kind {
					return event
				}
			case <-timeout:
				t.Fatalf("未收到 %s 事件", kind)
			}
		}
	}
	next("session")
	first := next("message_delta")
	if first.Message == nil || first.Message.Content != "第一段🌊" || first.StreamOffset == nil || *first.StreamOffset != 0 {
		t.Fatalf("首段未及时送达: %+v", first)
	}
	page, err := svc.ScopedHistory(parent.ID, project, member, "")
	if err != nil || len(page.StreamDrafts) != 1 || page.StreamDrafts[0].Content != "第一段🌊" {
		t.Fatalf("重连快照缺少首段: %v %v", page, err)
	}
	close(release)
	second := next("message_delta")
	if second.StreamOffset == nil || *second.StreamOffset != 5 {
		t.Fatalf("分片位置没有按 UTF-16 计数: %+v", second)
	}
	final := next("answer_end")
	if final.Message == nil || final.Message.Content != "第一段🌊第二段" {
		t.Fatalf("最终回复不完整: %+v", final)
	}
}
