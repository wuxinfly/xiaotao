package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func normalizeProjectPath(path string) string {
	return strings.ToLower(strings.ReplaceAll(filepath.Clean(path), `\\`, "/"))
}

// RenameProject changes display metadata only, preserving history and ordering.
func (s *Service) RenameProject(path, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return fmt.Errorf("项目名称需为 1–100 个字符")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("项目目录不能为空")
	}
	normalized := normalizeProjectPath(path)
	items, err := s.listSessionSummaries()
	if err != nil {
		return err
	}
	var selected []*Session
	for _, item := range items {
		s.markDefault(item)
		if !item.Project.IsDefault && strings.EqualFold(filepath.Clean(item.Project.Path), filepath.Clean(path)) {
			if r := s.runs[item.ID]; r != nil && r.cancel != nil {
				return fmt.Errorf("请先停止项目中正在运行的会话")
			}
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		db, checkErr := s.openStore()
		if checkErr != nil {
			return checkErr
		}
		var raw string
		checkErr = db.QueryRow("SELECT metadata FROM saved_projects WHERE path=?", normalized).Scan(&raw)
		db.Close()
		if checkErr != nil {
			return fmt.Errorf("项目不存在")
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
	var savedRaw string
	savedErr := tx.QueryRow("SELECT metadata FROM saved_projects WHERE path=?", normalized).Scan(&savedRaw)
	if savedErr == nil {
		var saved SavedProject
		if err = json.Unmarshal([]byte(savedRaw), &saved); err != nil {
			return err
		}
		saved.Name = name
		saved.UpdatedAt = time.Now()
		raw, marshalErr := json.Marshal(saved)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec("INSERT INTO saved_projects(path,metadata) VALUES(?,?) ON CONFLICT(path) DO UPDATE SET metadata=excluded.metadata", normalized, string(raw)); err != nil {
			return err
		}
	} else if savedErr != sql.ErrNoRows {
		return savedErr
	}
	for _, item := range selected {
		item.Project.Name = name
		item.Messages = nil
		item.Activity = nil
		raw, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE sessions SET metadata=? WHERE id=?", string(raw), item.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) listSavedProjects() ([]SavedProject, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT metadata FROM saved_projects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedProject{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item SavedProject
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) savedProject(path string) (*SavedProject, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRow("SELECT metadata FROM saved_projects WHERE path=?", normalizeProjectPath(path)).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var item SavedProject
	if err = json.Unmarshal([]byte(raw), &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) saveSavedProject(item SavedProject) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO saved_projects(path,metadata) VALUES(?,?) ON CONFLICT(path) DO UPDATE SET metadata=excluded.metadata", normalizeProjectPath(item.Path), string(raw))
	return err
}

func (s *Service) SaveProject(path, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("默认工作目录不需要单独保存")
	}
	workspace, err := OpenWorkspace(path)
	if err != nil {
		return err
	}
	path = workspace.Root
	if name == "" {
		name = filepath.Base(path)
	}
	if len([]rune(name)) > 100 {
		return fmt.Errorf("项目名称最多 100 个字符")
	}
	now := time.Now()
	existing, err := s.savedProject(path)
	if err != nil {
		return err
	}
	createdAt := now
	if existing != nil {
		createdAt = existing.CreatedAt
	}
	if err = s.saveSavedProject(SavedProject{Name: name, Path: path, CreatedAt: createdAt, UpdatedAt: now}); err != nil {
		return err
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
	if err = setProjectSessionsRemoved(tx, normalizeProjectPath(path), false); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ListProjects() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.listSavedProjects()
	if err != nil {
		return nil, err
	}
	out := []Project{}
	seen := map[string]bool{}
	for _, item := range saved {
		out = append(out, Project{Path: item.Path, Name: item.Name, IsDefault: false, Saved: true})
		seen[normalizeProjectPath(item.Path)] = true
	}
	summaries, err := s.listSessionSummaries()
	if err != nil {
		return nil, err
	}
	for _, item := range summaries {
		s.markDefault(item)
		if item.Project.IsDefault || item.Project.Path == "" || item.Project.Removed || seen[normalizeProjectPath(item.Project.Path)] {
			continue
		}
		out = append(out, Project{Path: item.Project.Path, Name: item.Project.Name, IsDefault: false})
		seen[normalizeProjectPath(item.Project.Path)] = true
	}
	return out, nil
}

func (s *Service) DeleteProject(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("项目目录不能为空")
	}
	normalized := normalizeProjectPath(path)
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
	result, err := tx.Exec("DELETE FROM saved_projects WHERE path=?", normalized)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("项目未保存或不存在")
	}
	if err = setProjectSessionsRemoved(tx, normalized, true); err != nil {
		return err
	}
	return tx.Commit()
}

// setProjectSessionsRemoved flags every session of a project so the sidebar
// keeps it under the recent list instead of a project group. The working
// directory, messages and run history are never touched.
func setProjectSessionsRemoved(tx *sql.Tx, normalized string, removed bool) error {
	rows, err := tx.Query("SELECT id, metadata FROM sessions")
	if err != nil {
		return err
	}
	var pending []struct{ id, raw string }
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var item Session
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			rows.Close()
			return err
		}
		if item.Project.IsDefault || item.Project.Path == "" || item.Project.Removed == removed || normalizeProjectPath(item.Project.Path) != normalized {
			continue
		}
		item.Project.Removed = removed
		item.Messages, item.Activity, item.Thoughts, item.RunStats = nil, nil, nil, nil
		updated, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			rows.Close()
			return marshalErr
		}
		pending = append(pending, struct{ id, raw string }{id, string(updated)})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, entry := range pending {
		if _, err = tx.Exec("UPDATE sessions SET metadata=? WHERE id=?", entry.raw, entry.id); err != nil {
			return err
		}
	}
	return nil
}
