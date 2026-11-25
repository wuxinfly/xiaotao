package office

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xiaotao/xt-client/internal/agent"
)

type collaborationFixture struct {
	svc                  *Service
	kernel               *agent.Service
	project              string
	parent               string
	member               string
	parentModelAvailable *atomic.Bool
	imageRequests        *atomic.Int32
}

func newCollaborationFixture(t *testing.T, respond func(system, text string) string) collaborationFixture {
	t.Helper()
	imageRequests := &atomic.Int32{}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []struct{ Content json.RawMessage }
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) < 2 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var system, text string
		_ = json.Unmarshal(input.Messages[0].Content, &system)
		last := input.Messages[len(input.Messages)-1].Content
		if json.Unmarshal(last, &text) != nil {
			var parts []struct{ Type, Text string }
			if json.Unmarshal(last, &parts) != nil {
				http.Error(w, "invalid message content", http.StatusBadRequest)
				return
			}
			for _, part := range parts {
				switch part.Type {
				case "text":
					text += part.Text
				case "image_url":
					imageRequests.Add(1)
				}
			}
		}
		response := respond(system, text)
		if response == "" {
			http.Error(w, "synthetic model failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": response}}}})
	}))
	t.Cleanup(model.Close)
	data, project := t.TempDir(), t.TempDir()
	parentModelAvailable := &atomic.Bool{}
	kernel := agent.NewService(filepath.Join(data, "agent"), model.URL, nil, model.Client(), func() []agent.ModelOption {
		options := []agent.ModelOption{{Model: "test/model", SupportsImages: true}}
		if parentModelAvailable.Load() {
			options = append(options, agent.ModelOption{Model: "test/old-parent"})
		}
		return options
	})
	officeService, err := New(filepath.Join(data, "office"), NewAgentExecutor(kernel))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(officeService.Close)
	parent, err := kernel.Create(agent.Options{ProjectPath: project, Model: "test/model"})
	if err != nil {
		t.Fatal(err)
	}
	member := makeTeamForChat(t, officeService, project)[0]
	return collaborationFixture{svc: officeService, kernel: kernel, project: project, parent: parent.ID, member: member, parentModelAvailable: parentModelAvailable, imageRequests: imageRequests}
}

func employeeTestImage(t *testing.T) []agent.ImageInput {
	t.Helper()
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return []agent.ImageInput{{Data: base64.StdEncoding.EncodeToString(raw.Bytes()), MimeType: "image/png"}}
}

func (f collaborationFixture) wait(t *testing.T, accept func(*Conversation) bool) *Conversation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var entry *Conversation
	for time.Now().Before(deadline) {
		var err error
		entry, err = f.svc.ScopedChat(f.parent, f.project, f.member)
		if err != nil {
			t.Fatal(err)
		}
		if accept(entry) {
			return entry
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("协作未达到预期状态: %+v", entry)
	return nil
}

func TestScopedChatMemoryAndFailureSensitiveCleanup(t *testing.T) {
	var summaryFails atomic.Bool
	summaryFails.Store(true)
	var laterMemory atomic.Bool
	f := newCollaborationFixture(t, func(system, text string) string {
		switch {
		case strings.Contains(system, "只输出 JSON，对象包含 memory"):
			if summaryFails.Load() {
				return ""
			}
			if strings.Contains(text, "记住按钮统一主色") {
				return "```json\n{\"memory\":\"按钮统一主色\",\"reply\":\"已确认配色规则\",\"question\":\"旧问题不应恢复\"}\n```"
			}
			return `{"memory":"","reply":"只聊了临时内容","question":""}`
		default:
			if strings.Contains(system, "长期记忆") && strings.Contains(system, "按钮统一主色") {
				laterMemory.Store(true)
			}
			return "按钮统一主色"
		}
	})
	if _, err := f.svc.SendScopedChat(f.parent, t.TempDir(), f.member, ChatInput{Content: "跨项目请求"}); err == nil {
		t.Fatal("接受了与主会话不匹配的项目")
	}
	if _, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Content: "记住按钮统一主色"}); err != nil {
		t.Fatal(err)
	}
	turn := f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "failed" })
	if err := f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{}); !errors.Is(err, ErrSummaryFailed) {
		t.Fatalf("错误关闭结果 %v", err)
	}
	summaryFails.Store(false)
	if _, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Content: "继续下一轮"}); err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "failed" })
	if err := f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{}); !errors.Is(err, ErrSummaryFailed) {
		t.Fatalf("后续轮次覆盖了待补存的记忆: %v", err)
	}
	if _, err := f.svc.RetryConversationSummary(f.parent, f.member); err != nil {
		t.Fatal(err)
	}
	recovered := f.wait(t, func(c *Conversation) bool { return c.SummaryStatus == "saved" })
	if recovered.Question != "" || recovered.SessionID != turn.SessionID {
		t.Fatalf("补存覆盖了当前对话状态: %+v", recovered)
	}
	if err := f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.kernel.History(turn.SessionID, ""); err == nil {
		t.Fatal("临时对话原文没有被清理")
	}
	if _, err := f.kernel.History(f.parent, ""); err != nil {
		t.Fatalf("关闭协作不应删除小涛主会话: %v", err)
	}
	memory, err := f.svc.memoryForEmployee(f.project, f.member)
	if err != nil || len(memory) != 1 || memory[0] != "按钮统一主色" {
		t.Fatalf("角色记忆未保留: %v %v", memory, err)
	}
	if _, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Content: "再次使用已确认信息"}); err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "saved" })
	if !laterMemory.Load() {
		t.Fatal("新会话没有获得角色长期记忆")
	}
}

func TestEmployeeMemoryWithRemovedParentModel(t *testing.T) {
	var summaryCalls atomic.Int32
	f := newCollaborationFixture(t, func(system, _ string) string {
		if strings.Contains(system, "只输出 JSON，对象包含 memory") {
			if summaryCalls.Add(1) == 1 {
				return "模拟不合法的模型响应"
			}
			return `{"memory":"按钮统一主色","reply":"","question":""}`
		}
		return "收到"
	})
	f.parentModelAvailable.Store(true)
	parent, err := f.kernel.Create(agent.Options{ProjectPath: f.project, Model: "test/old-parent"})
	if err != nil {
		t.Fatal(err)
	}
	f.parent = parent.ID
	f.parentModelAvailable.Store(false)
	send := func(text string) {
		t.Helper()
		if _, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Content: text, Model: "test/model"}); err != nil {
			t.Fatal(err)
		}
	}
	for i, text := range []string{"你好！", "你好呀"} {
		send(text)
		f.wait(t, func(c *Conversation) bool {
			return c.Status == StatusCompleted && c.SummaryStatus == []string{"saved", "failed"}[i]
		})
		// Reproduce a failed greeting saved by the previous version.
		entry, err := f.svc.readConversation(f.parent, f.member)
		if err != nil {
			t.Fatal(err)
		}
		entry.SummaryStatus = "failed"
		if err = f.svc.saveConversation(f.parent, entry); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.RetryConversationSummary(f.parent, f.member); err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.SummaryStatus == "saved" })
	if summaryCalls.Load() != 0 {
		t.Fatal("寒暄不应调用总结模型")
	}
	send("以后按钮统一主色")
	failed := f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "failed" })
	if failed.SummaryError != "模型返回的记忆摘要格式有误" {
		t.Fatalf("缺少可理解的失败原因: %q", failed.SummaryError)
	}
	if _, err := f.svc.RetryConversationSummary(f.parent, f.member); err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.SummaryStatus == "saved" })
	memory, err := f.svc.memoryForEmployee(f.project, f.member)
	if err != nil || len(memory) != 1 || memory[0] != "按钮统一主色" {
		t.Fatalf("用员工模型补存失败: %v %v", memory, err)
	}
}

func TestEmployeeChatAcceptsImageWithoutText(t *testing.T) {
	f := newCollaborationFixture(t, func(system, _ string) string {
		if strings.Contains(system, "只输出 JSON，对象包含 memory") {
			return `{"memory":"","reply":"","question":""}`
		}
		return "已查看图片"
	})
	images := employeeTestImage(t)
	f.parentModelAvailable.Store(true)
	if _, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Model: "test/old-parent", Images: images}); err == nil || !strings.Contains(err.Error(), "不支持图片") {
		t.Fatalf("应拒绝不支持图片的模型: %v", err)
	}
	if _, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Images: images}); err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "saved" })
	page, err := f.svc.ScopedHistory(f.parent, f.project, f.member, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Session.Messages) != 2 || len(page.Session.Messages[0].Images) != 1 || f.imageRequests.Load() != 1 {
		t.Fatal("纯图片消息未持久化或未传给模型")
	}
}

func TestScopedDispatchTaskWaitsAndArchivesPriorRequest(t *testing.T) {
	f := newCollaborationFixture(t, func(system, text string) string {
		switch {
		case strings.Contains(system, "mode 只允许 chat 或 task"):
			if strings.Contains(text, "只问个问题") {
				return `{"mode":"chat","title":"","content":"说明结果"}`
			}
			return `{"mode":"task","title":"修复页面","content":"修复页面"}`
		case strings.Contains(system, "只输出 JSON，对象包含 memory"):
			if strings.Contains(text, "小涛协作提问") {
				return `{"memory":"","reply":"员工正在等范围确认","question":"移动端也做吗？"}`
			}
			return `{"memory":"只处理桌面端","reply":"已带回员工回复","question":""}`
		case strings.Contains(system, "只有 answer 和 question"):
			return `{"answer":"","question":"移动端也需要改吗？"}`
		default:
			if strings.Contains(text, "只做桌面端") || strings.Contains(text, "说明结果") {
				return "已完成本轮回复"
			}
			return "目前还未改动。\n小涛协作提问：移动端也做吗？"
		}
	})
	request := DispatchInput{ChatInput: ChatInput{Content: "@开发 明确请你修复页面"}, MemberID: f.member}
	first, err := f.svc.DispatchConversation(f.parent, request)
	if err != nil || first.SourceMessageID == "" {
		t.Fatalf("没有登记派单来源: %+v %v", first, err)
	}
	waiting := f.wait(t, func(c *Conversation) bool { return c.Status == StatusWaiting && c.SummaryStatus == "saved" })
	if waiting.TaskID == "" || waiting.Question == "" {
		t.Fatalf("派单未进入待回复状态: %+v", waiting)
	}
	if answer, err := f.svc.ReplyConversation(context.Background(), f.parent, ReplyInput{MemberID: f.member, Delegate: true}); err != nil || answer.CoordinatorQuestion == "" {
		t.Fatalf("不知道时小涛必须回来问用户: %+v %v", answer, err)
	}
	if _, err := f.svc.ReplyConversation(context.Background(), f.parent, ReplyInput{MemberID: f.member, Content: "只做桌面端", Images: employeeTestImage(t)}); err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "saved" })
	messages, err := f.svc.loadMessages(waiting.TaskID)
	if err != nil || len(messages) != 1 || len(messages[0].Images) != 1 || f.imageRequests.Load() != 1 {
		t.Fatalf("任务回复的图片未排队或送达模型: %v %v", messages, err)
	}
	next, err := f.svc.DispatchConversation(f.parent, DispatchInput{ChatInput: ChatInput{Content: "@开发 只问个问题"}, MemberID: f.member})
	if err != nil {
		t.Fatal(err)
	}
	f.wait(t, func(c *Conversation) bool { return c.Status == StatusCompleted && c.SummaryStatus == "saved" })
	entries, err := f.svc.Conversations(f.parent)
	if err != nil || len(entries) != 2 || entries[0].SourceMessageID != next.SourceMessageID || !entries[1].Historical || entries[1].SourceMessageID != first.SourceMessageID {
		t.Fatalf("历史派单卡丢失: %+v %v", entries, err)
	}
	if err = f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.loadTask(waiting.TaskID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("关联任务记录未清理: %v", err)
	}
	entries, err = f.svc.Conversations(f.parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("关闭没有清理状态列表: %v %v", entries, err)
	}
}

func TestScopedCloseRequiresConfirmationAndWaitsForCancellation(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	f := newCollaborationFixture(t, func(system, text string) string {
		close(started)
		<-unblock
		return "不应保存已取消的回复"
	})
	defer close(unblock)
	chat, err := f.svc.SendScopedChat(f.parent, f.project, f.member, ChatInput{Content: "会被取消的对话"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("员工回复未开始")
	}
	if err = f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{}); !errors.Is(err, ErrConversationBusy) {
		t.Fatalf("未确认就关闭了对话: %v", err)
	}
	if err = f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.kernel.History(chat.SessionID, ""); err == nil {
		t.Fatal("强制关闭后仍能读取临时对话")
	}
}

func TestScopedChatRejectsStaleParentAtMutation(t *testing.T) {
	f := newCollaborationFixture(t, func(_, _ string) string { return "未使用" })
	scope, err := f.svc.conversationScope(f.parent, f.project)
	if err != nil {
		t.Fatal(err)
	}
	input := ChatInput{Content: "发给过期会话的请求"}
	member, err := f.svc.conversationMember(scope, f.member, &input)
	if err != nil {
		t.Fatal(err)
	}
	err = f.kernel.DeleteOfficeRoot(f.parent, func(commit func() error) error {
		return f.svc.CloseConversations(context.Background(), f.parent, CloseConversationInput{}, commit)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	if _, err := f.svc.prepareConversationLocked(scope, member); err == nil {
		t.Fatal("删除主会话后，旧的请求仍能创建临时对话")
	}
}
