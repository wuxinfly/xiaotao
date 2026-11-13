package office

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/agent"
)

var ErrChatBusy = errors.New("员工正在回复，请等待或取消")

type Chat struct {
	MemberID  string        `json:"memberId"`
	SessionID string        `json:"sessionId,omitempty"`
	Status    string        `json:"status"`
	Error     string        `json:"error,omitempty"`
	Approval  *TaskApproval `json:"approval,omitempty"`
}
type ChatInput struct {
	Content       string               `json:"content"`
	Model         string               `json:"model"`
	ProjectName   string               `json:"projectName,omitempty"`
	ModelSettings *agent.ModelSettings `json:"modelSettings,omitempty"`
}

func chatKey(path, id string) string { return "chat:" + projectKey(path) + ":" + id }

// chatLocked and saveChatLocked require s.mu, serializing creation and updates.
func (s *Service) chatLocked(path, id string) (*Chat, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRow("SELECT data FROM office_chats WHERE project_key=? AND member_id=?", projectKey(path), id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &Chat{MemberID: id, Status: "idle"}, nil
	}
	if err != nil {
		return nil, err
	}
	var chat Chat
	if err = json.Unmarshal([]byte(raw), &chat); err != nil {
		return nil, err
	}
	// No live run after restart means an interrupted turn, never an automatic retry.
	if (chat.Status == StatusRunning || chat.Status == StatusWaiting) && s.runs[chatKey(path, id)] == nil {
		chat.Status = StatusFailed
		chat.Error = "服务重启导致对话中断"
		chat.Approval = nil
		if err = s.saveChatLocked(path, &chat); err != nil {
			return nil, err
		}
	}
	return &chat, nil
}
func (s *Service) saveChatLocked(path string, chat *Chat) error {
	raw, err := json.Marshal(chat)
	if err != nil {
		return err
	}
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO office_chats(project_key,member_id,data) VALUES(?,?,?) ON CONFLICT(project_key,member_id) DO UPDATE SET data=excluded.data`, projectKey(path), chat.MemberID, string(raw))
	return err
}
func chatProject(path, id string) (string, error) {
	if !roleIDPattern.MatchString(id) && id != "xiaotao" && id != "designer" && id != "developer" {
		return "", fmt.Errorf("员工不存在")
	}
	path, _, err := resolveProject(path, "")
	return path, err
}
func (s *Service) Chat(projectPath, id string) (*Chat, error) {
	path, _, err := resolveProject(projectPath, "")
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatLocked(path, id)
}

func (s *Service) HistoricalChat(projectPath, id string) (*Chat, error) {
	return s.Chat(projectPath, id)
}
func (s *Service) ChatHistory(projectPath, id, before string) (*agent.HistoryPage, error) {
	chat, err := s.Chat(projectPath, id)
	if err != nil {
		return nil, err
	}
	if chat.SessionID == "" {
		return nil, fmt.Errorf("对话尚未创建")
	}
	return s.exec.History(chat.SessionID, before)
}
func (s *Service) SendChat(projectPath, id string, input ChatInput) (*Chat, error) {
	path, err := chatProject(projectPath, id)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(input.Content)
	if text == "" || len([]rune(text)) > maxMessageText {
		return nil, fmt.Errorf("消息不能为空且最多 %d 个字符", maxMessageText)
	}
	member, err := s.teams.Snapshot(path)
	if err != nil {
		return nil, err
	}
	role, err := member.Member(id)
	if err != nil || role.Lifecycle != RoleActive {
		return nil, ErrMemberNotFound
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = role.Model.ModelID
	}
	if model == "" || !s.modelAvailable(model) {
		return nil, fmt.Errorf("角色模型 %q 当前不可用，请编辑角色或选择可用模型", model)
	}
	if input.ModelSettings == nil {
		input.ModelSettings = modelSettingsFromPreference(role.Model)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	key := chatKey(path, id)
	if s.runs[key] != nil {
		return nil, ErrChatBusy
	}
	chat, err := s.chatLocked(path, id)
	if err != nil {
		return nil, err
	}
	if chat.SessionID == "" {
		session, err := s.exec.CreateSession(agent.Options{ProjectPath: path, ProjectName: input.ProjectName, Role: "office_chat_" + id, Model: model, ModelSettings: input.ModelSettings, ReadOnly: true})
		if err != nil {
			return nil, err
		}
		chat.SessionID = session.ID
	}
	chat.Status = StatusRunning
	chat.Error = ""
	chat.Approval = nil
	if err = s.saveChatLocked(path, chat); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(s.rootCtx)
	s.runs[key] = cancel
	s.wg.Add(1)
	snapshot := *chat
	go s.runChat(ctx, cancel, path, id, chat.SessionID, text, input.Model, input.ModelSettings)
	return &snapshot, nil
}

func modelSettingsFromPreference(preference ModelPreference) *agent.ModelSettings {
	if preference.ContextWindow == 0 && preference.MaxOutputTokens == 0 && preference.ReasoningEffort == "" {
		return nil
	}
	return &agent.ModelSettings{ContextWindow: preference.ContextWindow, MaxOutputTokens: preference.MaxOutputTokens, ReasoningEffort: preference.ReasoningEffort}
}
func (s *Service) runChat(ctx context.Context, cancel context.CancelFunc, path, id, session, text, model string, settings *agent.ModelSettings) {
	defer s.wg.Done()
	defer cancel()
	err := s.exec.RunTurn(ctx, session, text, model, settings, func(event agent.Event) {})
	s.mu.Lock()
	defer s.mu.Unlock()
	key := chatKey(path, id)
	// Keep the run registered while reading the persisted running state.
	chat, loadErr := s.chatLocked(path, id)
	if loadErr == nil {
		switch {
		case ctx.Err() != nil:
			chat.Status = StatusCanceled
			chat.Error = "对话已取消"
		case err != nil:
			chat.Status = StatusFailed
			chat.Error = err.Error()
		default:
			chat.Status = StatusCompleted
			chat.Error = ""
		}
		_ = s.saveChatLocked(path, chat)
	}
	delete(s.runs, key)
}
func (s *Service) CancelChat(projectPath, id string) error {
	path, _, err := resolveProject(projectPath, "")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel := s.runs[chatKey(path, id)]
	if cancel == nil {
		return ErrNotRunning
	}
	cancel()
	return nil
}
func (s *Service) ApproveChat(projectPath, id, approvalID string, approved bool) error {
	_, err := s.Chat(projectPath, id)
	if err != nil {
		return err
	}
	return fmt.Errorf("只读对话没有需要确认的工具操作")
}
