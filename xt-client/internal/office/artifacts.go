package office

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"xiaotao/xt-client/internal/agent"
)

const (
	maxArtifactRead = 256 * 1024
	artifactToolA   = "write_file"
	artifactToolB   = "str_replace"
)

// recordArtifact derives an artifact from a completed write activity. Only
// files inside the task's project root are recorded; absolute paths elsewhere
// are ignored rather than leaked into another project's shelf.
func (s *Service) recordArtifact(taskID string, activity *agent.Activity) {
	if activity.Kind != artifactToolA && activity.Kind != artifactToolB {
		return
	}
	rawPath := activityPath(activity)
	if rawPath == "" {
		return
	}
	task, err := s.loadTask(taskID)
	if err != nil {
		return
	}
	relative, absolute, err := resolveArtifactPath(task.ProjectPath, rawPath)
	if err != nil {
		return
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	artifact := &Artifact{
		ID:          artifactID(taskID, relative),
		TaskID:      taskID,
		TaskTitle:   task.Title,
		ProjectPath: task.ProjectPath,
		Path:        relative,
		Size:        info.Size(),
		Tool:        activity.Kind,
		RunID:       activity.RunID,
		ActivityID:  activity.ID,
		CreatedAt:   activity.StartedAt,
		UpdatedAt:   s.now(),
	}
	if info.Size() <= maxArtifactRead {
		if data, err := os.ReadFile(absolute); err == nil {
			sum := sha256.Sum256(data)
			artifact.SHA256 = hex.EncodeToString(sum[:])
		}
	}
	db, err := s.openStore()
	if err != nil {
		return
	}
	defer db.Close()
	_ = s.writeArtifact(db, artifact)
}

func activityPath(activity *agent.Activity) string {
	if strings.TrimSpace(activity.Input) == "" {
		return ""
	}
	var input struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(activity.Input), &input) != nil {
		return ""
	}
	return strings.TrimSpace(input.Path)
}

// resolveArtifactPath maps a tool path to a project-relative slash path. It
// rejects anything outside the root, including symlink escapes.
func resolveArtifactPath(root, rawPath string) (string, string, error) {
	if rawPath == "" {
		return "", "", fmt.Errorf("路径为空")
	}
	target := rawPath
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, filepath.FromSlash(target))
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = resolved
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("文件不在项目目录内")
	}
	return filepath.ToSlash(rel), abs, nil
}

// visibleArtifacts keeps write history intact while exposing only files that
// still exist at their recorded, project-relative location.
func visibleArtifacts(items []*Artifact) []*Artifact {
	visible := make([]*Artifact, 0, len(items))
	for _, item := range items {
		if _, _, err := currentArtifactFile(item); err == nil {
			visible = append(visible, item)
		}
	}
	return visible
}

func currentArtifactFile(artifact *Artifact) (string, os.FileInfo, error) {
	relative, absolute, err := resolveArtifactPath(artifact.ProjectPath, artifact.Path)
	if err != nil || relative != artifact.Path {
		return "", nil, fmt.Errorf("路径不在项目目录内，已拒绝读取")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("文件已被移动或删除")
	}
	return absolute, info, nil
}

func (s *Service) ArtifactDetail(projectPath, id string) (*ArtifactDetail, error) {
	artifact, err := s.artifactByID(projectPath, id)
	if err != nil {
		return nil, err
	}
	detail := &ArtifactDetail{Artifact: artifact}
	absolute, info, err := currentArtifactFile(artifact)
	if err != nil {
		detail.Error = err.Error()
		return detail, nil
	}
	detail.Available = true
	file, err := os.Open(absolute)
	if err != nil {
		detail.Error = "文件无法读取"
		detail.Available = false
		return detail, nil
	}
	defer file.Close()
	limit := int64(maxArtifactRead)
	if info.Size() > limit {
		detail.Truncated = true
	}
	buf := make([]byte, limit)
	n, _ := file.Read(buf)
	data := buf[:n]
	if !utf8.Valid(data) {
		detail.Binary = true
		return detail, nil
	}
	detail.Content = string(data)
	return detail, nil
}
