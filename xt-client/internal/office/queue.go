package office

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/logging"
)

// PostMessage queues a user message while the task runs, or starts a new turn
// on a finished task. Delivery is recorded only after the kernel appends the
// user message to the session.
func (s *Service) PostMessage(projectPath, taskID, text string) (*TaskMessage, error) {
	return s.postMessage(projectPath, taskID, text, false)
}

func (s *Service) ResumeTask(projectPath, taskID string) (*TaskMessage, error) {
	return s.postMessage(projectPath, taskID, "检查现场并继续", true)
}

func (s *Service) postMessage(projectPath, taskID, text string, resume bool) (*TaskMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("消息不能为空")
	}
	if len([]rune(text)) > maxMessageText {
		return nil, fmt.Errorf("消息最多 %d 个字符", maxMessageText)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.postMessageLocked(projectPath, taskID, text, resume, nil)
}

func (s *Service) postMessageLocked(projectPath, taskID, text string, resume bool, images []agent.ImageAttachment) (*TaskMessage, error) {
	task, err := s.loadTask(taskID)
	if err != nil {
		return nil, err
	}
	if projectKey(task.ProjectPath) != projectKey(projectPath) {
		return nil, ErrTaskNotFound
	}
	if err := s.checkLinkedTaskMessageLocked(taskID); err != nil {
		return nil, err
	}
	now := s.now()
	message := &TaskMessage{ID: "msg_" + randomID(8), TaskID: taskID, Content: text, Images: images, Status: MessageQueued, CreatedAt: now}
	// Persist and decide under the same lock the run loop uses to exit, so the
	// message is either queued for a live loop or starts a turn here — never
	// left behind by a loop that just finished.
	if s.closed {
		return nil, ErrClosed
	}
	if resume {
		latest, loadErr := s.loadTask(taskID)
		if loadErr != nil {
			return nil, loadErr
		}
		if !canRecover(latest) || s.runs[taskID] != nil || latest.SessionID == "" {
			return nil, fmt.Errorf("任务已变化，请刷新后重试")
		}
		if latest.MemberID != "" && latest.MemberID != coordinatorID {
			if _, err = s.memberForTaskLocked(latest.ProjectPath, latest.MemberID); err != nil {
				return nil, err
			}
		}
		task = latest
	}
	if canRecover(task) {
		record, loadErr := s.loadDelivery(taskID)
		if loadErr != nil {
			return nil, loadErr
		}
		text = recoveryPrompt(task, record, text)
		message.Content = text
	}
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	if err = s.writeMessage(db, message); err != nil {
		db.Close()
		return nil, err
	}
	db.Close()
	status := task.Status
	if live, ok := s.active[taskID]; ok {
		status = live.Status
	}
	_, running := s.runs[taskID]
	if running || !isTerminal(status) || task.SessionID == "" {
		return message, nil
	}
	s.runs[taskID] = func() {}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runLoop(taskID, task.SessionID, text, message.ID, images)
	}()
	return message, nil
}

func (s *Service) checkLinkedTaskMessageLocked(taskID string) error {
	parent, member, err := s.officeTaskConversation(taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.closingConversations[parent] {
		return ErrConversationBusy
	}
	entry, err := s.readConversation(parent, member)
	if err != nil {
		return err
	}
	if entry.TaskID != taskID {
		return fmt.Errorf("请在小涛会话中继续当前员工对话")
	}
	return nil
}

func (s *Service) nextQueuedMessage(taskID string) (*TaskMessage, error) {
	messages, err := s.loadQueuedMessages(taskID)
	if err != nil || len(messages) == 0 {
		return nil, err
	}
	return messages[0], nil
}

func (s *Service) markMessage(taskID, messageID string, fn func(*TaskMessage)) {
	db, err := s.openStore()
	if err != nil {
		logging.Default().For(logging.ModuleAgent).Warn("office message update failed", logging.F("task", taskID), logging.F("error", err))
		return
	}
	defer db.Close()
	var raw string
	if err = db.QueryRow("SELECT data FROM office_messages WHERE id=? AND task_id=?", messageID, taskID).Scan(&raw); err != nil {
		return
	}
	var message TaskMessage
	if err = json.Unmarshal([]byte(raw), &message); err != nil {
		return
	}
	fn(&message)
	if err = s.writeMessage(db, &message); err != nil {
		logging.Default().For(logging.ModuleAgent).Warn("office message save failed", logging.F("task", taskID), logging.F("error", err))
	}
}
