package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OfficeComplete runs a bounded, tool-free model request for coordination.
func (s *Service) OfficeComplete(ctx context.Context, model, instruction, material string, settings *ModelSettings) (string, error) {
	normalized, err := normalizeModelSettings(settings, s.modelDefaults(model))
	if err != nil {
		return "", err
	}
	options := derefModelSettings(normalized)
	if options.MaxOutputTokens == 0 || options.MaxOutputTokens > 1400 {
		options.MaxOutputTokens = 1400
	}
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result, err := s.getModelClient().Complete(callCtx, model, []map[string]any{
		{"role": "system", "content": instruction},
		{"role": "user", "content": material},
	}, nil, options, nil)
	if err != nil {
		return "", err
	}
	if result == nil || len(result.ToolCalls) > 0 || strings.TrimSpace(result.Content) == "" {
		return "", fmt.Errorf("模型没有返回有效的协调结果")
	}
	return strings.TrimSpace(result.Content), nil
}

func (s *Service) AppendOfficeRequest(id, content, existingID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.runs[id]; r != nil && r.cancel != nil {
		return "", fmt.Errorf("请先等待小涛完成当前回复")
	}
	parent, err := s.loadSession(id)
	if err != nil {
		return "", err
	}
	if parent.Role != "" {
		return "", fmt.Errorf("仅小涛主会话支持成员协作")
	}
	if existingID != "" {
		for _, msg := range parent.Messages {
			if msg.ID == existingID && msg.Role == "user" && msg.Content == content {
				return msg.ID, nil
			}
		}
		return "", fmt.Errorf("协作来源消息无效")
	}
	now := time.Now()
	message := Message{ID: "msg_" + randomID(6), Role: "user", Content: content, CreatedAt: now}
	parent.Messages = append(parent.Messages, message)
	parent.UpdatedAt = now
	if len(parent.Messages) == 1 && parent.Title == "新任务" {
		parent.Title = titleFromPrompt(content, false)
	}
	return message.ID, s.saveSession(parent)
}

// SetOfficeContext records private, bounded instructions in an employee
// session. Long-term memory stays in the office store.
func (s *Service) SetOfficeContext(id, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.runs[id]; r != nil && r.cancel != nil {
		return fmt.Errorf("员工正在回复")
	}
	session, err := s.loadSession(id)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(session.Role, "office_") {
		return fmt.Errorf("不是员工会话")
	}
	session.OfficeContext = value
	return s.saveSession(session)
}

var errOfficeSessionMissing = errors.New("任务不存在")

func (s *Service) DeleteOfficeChild(id string) error {
	err := s.Delete(id)
	if errors.Is(err, errOfficeSessionMissing) {
		return nil
	}
	return err
}

func (s *Service) DeleteOfficeRoot(id string, cleanup func(commit func() error) error) error {
	s.mu.Lock()
	prior := s.runs[id]
	if prior != nil && prior.cancel != nil {
		s.mu.Unlock()
		return fmt.Errorf("请先停止正在运行的会话")
	}
	reservation := &run{cancel: func() {}}
	s.runs[id] = reservation
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.runs[id] != reservation {
			return
		}
		if prior == nil {
			delete(s.runs, id)
		} else {
			s.runs[id] = prior
		}
	}()
	return cleanup(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.deleteSession(id); err != nil {
			return err
		}
		delete(s.runs, id)
		return nil
	})
}

func (s *Service) OfficeSessionMetadata(id string) (*Session, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var raw string
	if err = db.QueryRow("SELECT metadata FROM sessions WHERE id=?", id).Scan(&raw); err != nil {
		return nil, err
	}
	var session Session
	if err = json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}
