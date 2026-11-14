package office

import (
	"sort"

	"xiaotao/xt-client/internal/agent"
)

// Snapshot builds the office board for one project. It reads task summaries,
// artifacts and live members; it never loads transcript history.
func (s *Service) Snapshot(projectPath, projectName string) (*Snapshot, error) {
	path, name, err := projectIdentity(projectPath, projectName)
	if err != nil {
		return nil, err
	}
	tasks, err := s.loadProjectTasks(path)
	if err != nil {
		return nil, err
	}
	queued, err := s.queuedCounts(path)
	if err != nil {
		return nil, err
	}
	for i, task := range tasks {
		if live, err := s.overlay(task); err == nil && live != nil {
			tasks[i] = live
		}
		tasks[i].QueuedMessages = queued[tasks[i].ID]
	}
	artifacts, err := s.projectArtifacts(path)
	if err != nil {
		return nil, err
	}
	team, err := s.teams.Snapshot(path)
	if err != nil {
		return nil, err
	}
	members := buildMembers(tasks, team)
	for _, task := range tasks {
		if task.MemberID == "" {
			continue
		}
		for index := range members {
			if members[index].ID == task.MemberID {
				members[index].TodayTasks++
				members[index].TaskID = task.ID
				members[index].TaskTitle = task.Title
				members[index].TaskStatus = task.Status
			}
		}
	}
	for i := range members {
		if members[i].ID != coordinatorID && !roleIDPattern.MatchString(members[i].ID) {
			continue
		}
		chat, err := s.Chat(path, members[i].ID)
		if err != nil {
			return nil, err
		}
		switch chat.Status {
		case StatusRunning, StatusWaiting, StatusFailed:
			members[i].Status = chat.Status
		}
	}
	return &Snapshot{
		Project:      ProjectInfo{Path: path, Name: name},
		AccessMode:   s.exec.AccessMode(),
		Tasks:        tasks,
		Members:      members,
		Artifacts:    artifacts,
		Capabilities: defaultCapabilities(),
	}, nil
}

func projectIdentity(projectPath, projectName string) (string, string, error) {
	path, name, err := resolveProject(projectPath, projectName)
	if err != nil {
		return "", "", err
	}
	return path, name, nil
}

func (s *Service) overlay(task *Task) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if live, ok := s.active[task.ID]; ok {
		return snapshotTask(live), nil
	}
	return task, nil
}

// buildMembers shows one desk per unfinished task plus a coordinator desk.
// Everything is derived from real tasks, never invented.
func buildMembers(tasks []*Task, team TeamSnapshot) []Member {
	members := []Member{{
		ID:     coordinatorID,
		Name:   executorName,
		Role:   "coordinator",
		Status: "idle",
	}}
	for _, teammate := range team.Members {
		appearance, preference := teammate.Appearance, teammate.Model
		members = append(members, Member{ID: teammate.ID, Name: teammate.Name, Role: teammate.Scope, Responsibility: teammate.Responsibility, Capabilities: teammate.Capabilities, Status: "idle", Model: teammate.Model.ModelID, Appearance: &appearance, ModelPreference: &preference})
	}
	for _, task := range tasks {
		// An executor leaves the room when its task ends; the task record and
		// its artifacts stay available in the task list and shelf.
		if isTerminal(task.Status) {
			continue
		}
		if task.MemberID != "" {
			for index := range members {
				if members[index].ID == task.MemberID {
					members[index].Status = task.Status
					members[index].TaskID = task.ID
					members[index].TaskTitle = task.Title
					members[index].TaskStatus = task.Status
					members[index].LatestActivity = task.LatestActivity
				}
			}
			continue
		}
		status := task.Status
		if status == StatusQueued {
			status = "idle"
		}
		member := Member{
			ID:             task.Executor.SessionID,
			Name:           task.Executor.Name,
			Role:           "executor",
			Status:         status,
			Model:          task.Executor.Model,
			TaskID:         task.ID,
			TaskTitle:      task.Title,
			TaskStatus:     task.Status,
			TodayTasks:     1,
			LatestActivity: task.LatestActivity,
		}
		if member.ID == "" {
			member.ID = task.ID
		}
		members = append(members, member)
	}
	return members
}

// TaskDetail returns the task plus a bounded transcript page. Artifacts and
// queued messages belong to the same task only.
func (s *Service) TaskDetail(projectPath, taskID string) (*TaskDetail, error) {
	task, err := s.projectTask(projectPath, taskID)
	if err != nil {
		return nil, err
	}
	detail := &TaskDetail{Task: task, Artifacts: []*Artifact{}, Messages: []*TaskMessage{}}
	if artifacts, err := s.loadTaskArtifacts(taskID); err == nil {
		detail.Artifacts = visibleArtifacts(artifacts)
	}
	if messages, err := s.loadMessages(taskID); err == nil {
		detail.Messages = messages
		for _, message := range messages {
			if message.Status == MessageQueued {
				task.QueuedMessages++
			}
		}
	}
	if task.SessionID == "" {
		return detail, nil
	}
	page, err := s.exec.History(task.SessionID, "")
	if err != nil {
		detail.HistoryError = err.Error()
		return detail, nil
	}
	detail.History = page
	return detail, nil
}

func (s *Service) TaskHistory(projectPath, taskID, before string) (*agent.HistoryPage, error) {
	task, err := s.projectTask(projectPath, taskID)
	if err != nil {
		return nil, err
	}
	if task.SessionID == "" {
		return nil, ErrTaskNotFound
	}
	return s.exec.History(task.SessionID, before)
}

func (s *Service) ProjectArtifacts(projectPath, projectName string) ([]*Artifact, error) {
	path, _, err := projectIdentity(projectPath, projectName)
	if err != nil {
		return nil, err
	}
	return s.projectArtifacts(path)
}

func (s *Service) projectArtifacts(path string) ([]*Artifact, error) {
	items, err := s.loadProjectArtifacts(path)
	if err != nil {
		return nil, err
	}
	items = visibleArtifacts(items)
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}
