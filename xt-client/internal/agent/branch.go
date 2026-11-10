package agent

import (
	"fmt"
	"strings"
	"time"
)

// Branch copies a conversation up to one finished assistant answer into a new
// session. The source session stays untouched; the copy keeps the original
// message, run and tool-call IDs so that tool results stay paired and the new
// session renders its own process records without guessing.
func (s *Service) Branch(id, messageID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.runs[id]; r != nil && r.cancel != nil {
		return nil, fmt.Errorf("请先停止正在运行的会话")
	}
	source, err := s.loadSession(id)
	if err != nil {
		return nil, err
	}

	cutoff := -1
	for index, message := range source.Messages {
		if message.ID == messageID {
			cutoff = index
			break
		}
	}
	if cutoff < 0 {
		return nil, fmt.Errorf("没有找到这条回答")
	}
	answer := source.Messages[cutoff]
	if answer.Role != "assistant" {
		return nil, fmt.Errorf("只能从一条回答创建分支")
	}
	if len(answer.ToolCalls) > 0 {
		return nil, fmt.Errorf("这条记录是工具调用过程，不是正式回答")
	}
	if !branchAnswerFinished(source, answer) {
		return nil, fmt.Errorf("回答尚未完成，暂不能创建分支")
	}

	messages := append([]Message(nil), source.Messages[:cutoff+1]...)
	runs := map[string]bool{}
	for _, message := range messages {
		if message.RunID != "" {
			runs[message.RunID] = true
		}
	}
	now := time.Now()
	branch := &Session{
		ID:            "run_" + randomID(8),
		Project:       source.Project,
		Title:         branchTitle(source.Title),
		Model:         source.Model,
		ModelSettings: cloneModelSettings(source.ModelSettings),
		Status:        StatusIdle,
		Messages:      messages,
		Activity:      branchRecords(source.Activity, runs, answer.CreatedAt, func(item Activity) (string, time.Time) { return item.RunID, item.StartedAt }),
		Thoughts:      branchRecords(source.Thoughts, runs, answer.CreatedAt, func(item ThoughtStep) (string, time.Time) { return item.RunID, item.CreatedAt }),
		RunStats:      branchRecords(source.RunStats, runs, answer.CreatedAt, func(item RunStats) (string, time.Time) { return item.RunID, item.StartedAt }),
		HasImages:     historyHasImages(messages),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.saveSession(branch); err != nil {
		return nil, err
	}
	s.runs[branch.ID] = &run{}
	s.markDefault(branch)
	return branch, nil
}

// branchAnswerFinished rejects a branch point whose run is still streaming.
// Legacy answers without run statistics count as finished.
func branchAnswerFinished(source *Session, answer Message) bool {
	if answer.RunID == "" {
		return true
	}
	for _, stats := range source.RunStats {
		if stats.RunID == answer.RunID {
			return stats.EndedAt != nil
		}
	}
	return true
}

// branchRecords keeps the records of the copied turns. Records without a run
// only follow when they happened at or before the branch point.
func branchRecords[T any](items []T, runs map[string]bool, cutoff time.Time, key func(T) (string, time.Time)) []T {
	out := []T{}
	for _, item := range items {
		runID, at := key(item)
		if runID != "" {
			if runs[runID] {
				out = append(out, item)
			}
			continue
		}
		if !at.After(cutoff) {
			out = append(out, item)
		}
	}
	return out
}

func branchTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" || title == "新任务" {
		title = "未命名会话"
	}
	name := []rune(title + " · 分支")
	if len(name) > 100 {
		name = append(name[:99], '…')
	}
	return string(name)
}

func cloneModelSettings(settings *ModelSettings) *ModelSettings {
	if settings == nil {
		return nil
	}
	copied := *settings
	return &copied
}