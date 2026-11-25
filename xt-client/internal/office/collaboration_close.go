package office

import (
	"context"
	"fmt"
	"time"
)

type linkedConversationTask struct {
	taskID    string
	sessionID string
}

func (s *Service) linkedConversationTasks(parent string) ([]linkedConversationTask, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT rel.task_id, COALESCE(json_extract(t.data,'$.sessionId'),'') FROM office_conversation_tasks rel LEFT JOIN office_tasks t ON t.id=rel.task_id WHERE rel.parent_id=?", parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []linkedConversationTask{}
	for rows.Next() {
		var item linkedConversationTask
		if err = rows.Scan(&item.taskID, &item.sessionID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) CloseConversations(ctx context.Context, parent string, input CloseConversationInput, finalize ...func() error) error {
	executor, err := s.collaboration()
	if err != nil {
		return err
	}
	session, err := executor.ParentSession(parent)
	if err != nil || session == nil || session.Role != "" || session.ReadOnly {
		return fmt.Errorf("小涛主会话不存在")
	}
	s.mu.Lock()
	if s.closingConversations[parent] {
		s.mu.Unlock()
		return ErrConversationBusy
	}
	s.closingConversations[parent] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.closingConversations, parent)
		s.mu.Unlock()
	}()

	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		finished, err := s.clearConversationScope(parent, input)
		if err != nil {
			return err
		}
		if finished {
			if len(finalize) > 0 {
				return finalize[0]()
			}
			return nil
		}
		select {
		case <-ticker.C:
		case <-waitCtx.Done():
			return ErrConversationBusy
		}
	}
}

func (s *Service) clearConversationScope(parent string, input CloseConversationInput) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.listConversations(parent)
	if err != nil {
		return false, err
	}
	tasks, err := s.linkedConversationTasks(parent)
	if err != nil {
		return false, err
	}
	if err = s.checkConversationSummaries(parent, entries, input.DiscardSummary); err != nil {
		return false, err
	}
	busy := false
	for _, entry := range entries {
		if cancel := s.runs[conversationKey(parent, entry.MemberID)]; cancel != nil {
			busy = true
			if input.Force {
				cancel()
			}
		}
	}
	for _, task := range tasks {
		if cancel := s.runs[task.taskID]; cancel != nil {
			busy = true
			if input.Force {
				cancel()
			}
		}
	}
	if busy {
		if input.Force {
			return false, nil
		}
		return false, ErrConversationBusy
	}
	return true, s.deleteConversationScope(parent, entries, tasks)
}

func (s *Service) checkConversationSummaries(parent string, entries []Conversation, discard bool) error {
	for index := range entries {
		entry := &entries[index]
		if err := s.reconcileConversationLocked(parent, entry); err != nil {
			return err
		}
		if entry.SummaryStatus == "pending" {
			return ErrSummaryPending
		}
		if !discard && (entry.SummaryStatus == "failed" || len(entry.DeferredSummaries) > 0) {
			return ErrSummaryFailed
		}
	}
	return nil
}

func (s *Service) deleteConversationScope(parent string, entries []Conversation, tasks []linkedConversationTask) error {
	executor, err := s.collaboration()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.SessionID != "" {
			if err = executor.DeleteSession(entry.SessionID); err != nil {
				return err
			}
		}
	}
	for _, task := range tasks {
		if task.sessionID != "" {
			if err = executor.DeleteSession(task.sessionID); err != nil {
				return err
			}
		}
	}
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		"DELETE FROM office_messages WHERE task_id IN (SELECT task_id FROM office_conversation_tasks WHERE parent_id=?)",
		"DELETE FROM office_artifacts WHERE task_id IN (SELECT task_id FROM office_conversation_tasks WHERE parent_id=?)",
		"DELETE FROM office_delivery WHERE task_id IN (SELECT task_id FROM office_conversation_tasks WHERE parent_id=?)",
		"DELETE FROM office_tasks WHERE id IN (SELECT task_id FROM office_conversation_tasks WHERE parent_id=?)",
		"DELETE FROM office_conversation_tasks WHERE parent_id=?",
		"DELETE FROM office_conversations WHERE parent_id=?",
		"DELETE FROM office_conversation_archive WHERE parent_id=?",
	} {
		if _, err = tx.Exec(query, parent); err != nil {
			return err
		}
	}
	return tx.Commit()
}
