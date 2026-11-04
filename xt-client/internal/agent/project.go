package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

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
		return fmt.Errorf("项目不存在")
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
