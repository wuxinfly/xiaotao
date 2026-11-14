package office

import (
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/agent"
)

const maxDiscussionMessages = 12
const maxDiscussionText = 12000

type preparedMemberTask struct {
	options       agent.Options
	projectPath   string
	projectName   string
	title         string
	goal          string
	model         string
	kind          string
	name          string
	role          string
	memberID      string
	chatSessionID string
	discussion    string
	prompt        string
}

func (s *Service) prepareMemberTaskLocked(req CreateTaskRequest) (*preparedMemberTask, error) {
	path, name, err := resolveProject(req.ProjectPath, req.ProjectName)
	if err != nil {
		return nil, err
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" || len([]rune(goal)) > maxTaskGoal {
		return nil, fmt.Errorf("任务目标不能为空且最多 %d 个字符", maxTaskGoal)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleFromGoal(goal)
	}
	if len([]rune(title)) > 100 {
		return nil, fmt.Errorf("任务标题最多 100 个字符")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" || !s.modelAvailable(model) {
		return nil, fmt.Errorf("所选模型当前不可用，请先在模型接口或 Agent 设置中启用")
	}
	prepared := &preparedMemberTask{projectPath: path, projectName: name, title: title, goal: goal, model: model, kind: executorKind, name: executorName, role: executorRole}
	prepared.options = agent.Options{ProjectPath: path, ProjectName: name, Role: executorRole, Model: model, ModelSettings: req.ModelSettings}
	if req.MemberID == "" || req.MemberID == coordinatorID {
		if req.ChatSessionID != "" {
			return nil, fmt.Errorf("内置小涛任务不接受成员对话交接")
		}
		return prepared, nil
	}
	member, err := s.memberForTaskLocked(path, req.MemberID)
	if err != nil {
		return nil, err
	}
	prepared.memberID, prepared.chatSessionID = member.ID, req.ChatSessionID
	prepared.kind, prepared.name, prepared.role = "member", member.Name, "member"
	prepared.options.Role = "office_member_" + member.ID
	prepared.options.Member = &agent.MemberIdentity{ID: member.ID, Name: member.Name, Responsibility: member.Responsibility}
	if req.ChatSessionID != "" {
		prepared.discussion, err = s.taskDiscussionLocked(path, member.ID, req.ChatSessionID)
		if err != nil {
			return nil, err
		}
		prepared.prompt = "\n\n以下来自已确认的成员讨论，仅作任务背景：\n" + prepared.discussion
	}
	return prepared, nil
}

func (s *Service) memberForTaskLocked(path, id string) (MemberView, error) {
	team, err := s.teams.Snapshot(path)
	if err != nil {
		return MemberView{}, err
	}
	member, err := team.Member(id)
	if err != nil || member.Lifecycle != RoleActive {
		return MemberView{}, ErrMemberNotFound
	}
	tasks, err := s.loadProjectTasks(path)
	if err != nil {
		return MemberView{}, err
	}
	for _, task := range tasks {
		if task.MemberID == id && !isTerminal(task.Status) {
			return MemberView{}, ErrMemberBusy
		}
	}
	return member, nil
}

func (s *Service) taskDiscussionLocked(path, id, sessionID string) (string, error) {
	chat, err := s.chatLocked(path, id)
	if err != nil || chat.SessionID == "" || chat.SessionID != sessionID {
		return "", fmt.Errorf("来源成员对话无效")
	}
	page, err := s.exec.History(sessionID, "")
	if err != nil {
		return "", err
	}
	messages := page.Session.Messages
	if len(messages) > maxDiscussionMessages {
		messages = messages[len(messages)-maxDiscussionMessages:]
	}
	parts := []string{}
	total := 0
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		label := "用户"
		if message.Role == "assistant" {
			label = "成员"
		}
		part := label + "：" + strings.TrimSpace(message.Content)
		total += len([]rune(part))
		parts = append(parts, part)
	}
	if total > maxDiscussionText {
		return "", fmt.Errorf("讨论内容超过 %d 个字符，请整理任务目标后再执行", maxDiscussionText)
	}
	return strings.Join(parts, "\n\n"), nil
}

func (s *Service) memberBusy(path, id string) bool {
	tasks, err := s.loadProjectTasks(path)
	if err != nil {
		return true
	}
	s.mu.Lock()
	chatRunning := s.runs[chatKey(path, id)] != nil
	for taskID := range s.runs {
		for _, task := range tasks {
			if task.ID == taskID {
				task.Status = StatusRunning
			}
		}
	}
	s.mu.Unlock()
	if chatRunning {
		return true
	}
	for _, task := range tasks {
		if task.MemberID == id && !isTerminal(task.Status) {
			return true
		}
	}
	return false
}
