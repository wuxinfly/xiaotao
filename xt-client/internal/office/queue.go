package office

import (
	"encoding/json"
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/logging"
)

// PostMessage queues a user message while the task runs, or starts a new turn
// on a finished task. Delivery is recorded only after the kernel appends the
// user message to the session.
func (s *Service) PostMessage(projectPath, taskID, text string) (*TaskMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("消息不能为空")
	}
	if len([]rune(text)) > maxMessageText {
		return nil, fmt.Errorf("消息最多 %d 个字符", maxMessageText)
	}
	task, err := s.projectTask(projectPath, taskID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	message := &TaskMessage{ID: "msg_" + randomID(8), TaskID: taskID, Content: text, Status: MessageQueued, CreatedAt: now}
	// Persist and decide under the same lock the run loop uses to exit, so the
	// message is either queued for a live loop or starts a turn here — never
	// left behind by a loop that just finished.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	db, err := s.openStore()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if err = s.writeMessage(db, message); err != nil {
		db.Close()
		s.mu.Unlock()
		return nil, err
	}
	db.Close()
	status := task.Status
	if live, ok := s.active[taskID]; ok {
		status = live.Status
	}
	_, running := s.runs[taskID]
	if running || !isTerminal(status) || task.SessionID == "" {
		s.mu.Unlock()
		return message, nil
	}
	s.runs[taskID] = func() {}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runLoop(taskID, task.SessionID, text, message.ID)
	}()
	s.mu.Unlock()
	return message, nil
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
