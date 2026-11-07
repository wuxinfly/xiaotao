package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	validIdentifierRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_\-]{0,79}$`)
)

func sanitizeIdentifier(s string) string {
	low := strings.ToLower(strings.TrimSpace(s))
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, low)
	cleaned = strings.Trim(cleaned, "-_")
	if cleaned == "" {
		cleaned = "rev-issue"
	}
	if len(cleaned) > 70 {
		cleaned = cleaned[:70]
	}
	if !validIdentifierRegex.MatchString(cleaned) {
		cleaned = "rev-" + cleaned
	}
	return cleaned
}

func findPythonBinary() (string, error) {
	candidates := []string{"python", "python3"}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
	}
	return "", errors.New("未检测到系统 Python 运行环境 (python/python3)")
}

func (s *Service) getTrustedCoreRoot() (string, error) {
	return prepareCore(s.dataDir, false)
}

func (s *Service) getTrustedCoreEvolutionScript() (string, error) {
	_, err := s.getTrustedCoreRoot()
	if err != nil {
		return "", fmt.Errorf("准备可信 Core 失败: %w", err)
	}
	digest := fmt.Sprintf("%x", s.CoreVersion())
	root := filepath.Join(s.dataDir, "core", digest)
	script := filepath.Join(root, "scripts", "evolution.py")
	if _, err := os.Stat(script); err != nil {
		// If exact digest path not found, search in core dir
		coreDir := filepath.Join(s.dataDir, "core")
		entries, readErr := os.ReadDir(coreDir)
		if readErr == nil {
			for _, e := range entries {
				if e.IsDir() {
					candidate := filepath.Join(coreDir, e.Name(), "scripts", "evolution.py")
					if _, err := os.Stat(candidate); err == nil {
						return candidate, nil
					}
				}
			}
		}
		return "", fmt.Errorf("可信 Core 中缺少 scripts/evolution.py: %w", err)
	}
	return script, nil
}

// AddIssueSuggestion bridges an identified review issue into the project's evolution suggestions box.
func (s *Service) AddIssueSuggestion(reviewID, issueID string, req AddSuggestionRequest) (*ReviewSuggestionLink, error) {
	r, _, err := s.getReview(reviewID)
	if err != nil {
		return nil, err
	}
	if r.Status != ReviewStatusSucceeded || r.Result == nil {
		return nil, fmt.Errorf("只能将已成功的复盘问题加入建议箱")
	}

	// 1. Locate the issue
	var targetIssue *ReviewIssue
	for i := range r.Result.Issues {
		if r.Result.Issues[i].ID == issueID {
			targetIssue = &r.Result.Issues[i]
			break
		}
	}
	if targetIssue == nil {
		return nil, fmt.Errorf("复盘记录中未找到对应的问题: %s", issueID)
	}

	// 2. Check if already linked
	for _, link := range r.SuggestionLinks {
		if link.IssueID == issueID {
			// Already linked; return existing link (idempotent)
			return &link, nil
		}
	}

	// 3. Locate session and project
	sess, err := s.loadSession(r.SessionID)
	if err != nil {
		return nil, fmt.Errorf("加载原会话失败: %w", err)
	}
	projectRoot := sess.Project.Path
	if projectRoot == "" {
		return nil, fmt.Errorf("当前会话未绑定项目目录，无法加入建议箱")
	}

	xiaotaoDir := filepath.Join(projectRoot, ".xiaotao")
	info, err := os.Stat(xiaotaoDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("当前项目未启用小涛项目记忆 (.xiaotao)，无法加入建议箱")
	}

	// 4. Secure path validation
	cleanProj, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	safeReviewID := sanitizeIdentifier(r.ID)
	safeIssueID := sanitizeIdentifier(targetIssue.ID)
	evidenceRelPath := fmt.Sprintf(".xiaotao/review-evidence/%s/%s.json", safeReviewID, safeIssueID)
	evidenceAbsPath := filepath.Join(cleanProj, filepath.FromSlash(evidenceRelPath))

	// Guard against directory junction / path traversal
	cleanEvidenceAbs := filepath.Clean(evidenceAbsPath)
	if !strings.HasPrefix(strings.ToLower(cleanEvidenceAbs), strings.ToLower(cleanProj)+string(filepath.Separator)) {
		return nil, fmt.Errorf("目标路径存在越界风险")
	}

	if err := os.MkdirAll(filepath.Dir(cleanEvidenceAbs), 0700); err != nil {
		return nil, fmt.Errorf("创建证据目录失败: %w", err)
	}

	// 5. Build minimal sanitized fact snapshot
	evidencePayload := map[string]any{
		"reviewId":         r.ID,
		"issueId":          targetIssue.ID,
		"sessionId":        r.SessionID,
		"sessionTitle":     r.SessionTitleSnapshot,
		"reviewTime":       r.CreatedAt.Format(time.RFC3339),
		"model":            r.Model,
		"clientVersion":    r.ClientVersion,
		"coreVersion":      r.CoreVersion,
		"title":            targetIssue.Title,
		"problem":          targetIssue.Problem,
		"impact":           targetIssue.Impact,
		"speculation":      targetIssue.Speculation,
		"proposal":         targetIssue.Proposal,
		"acceptance":       targetIssue.Acceptance,
		"citedEvidenceIds": targetIssue.EvidenceIDs,
	}
	evidenceJSON, err := json.MarshalIndent(evidencePayload, "", "  ")
	if err != nil {
		return nil, err
	}

	// Atomic write evidence file
	tmpFile := fmt.Sprintf("%s.tmp.%d", cleanEvidenceAbs, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, evidenceJSON, 0600); err != nil {
		return nil, fmt.Errorf("写入证据快照文件失败: %w", err)
	}
	if err := os.Rename(tmpFile, cleanEvidenceAbs); err != nil {
		_ = os.Remove(tmpFile)
		return nil, fmt.Errorf("原子替换证据快照失败: %w", err)
	}

	// 6. Locate Python & Trusted Core Script
	pythonBin, err := findPythonBinary()
	if err != nil {
		return nil, err
	}
	scriptPath, err := s.getTrustedCoreEvolutionScript()
	if err != nil {
		return nil, err
	}

	// 7. Determine issue_key & observation_id
	issueKey := strings.TrimSpace(req.IssueKey)
	if issueKey == "" {
		issueKey = fmt.Sprintf("rev-%s-%s", safeReviewID, safeIssueID)
	}
	issueKey = sanitizeIdentifier(issueKey)
	observationID := fmt.Sprintf("obs-%s-%s", safeReviewID, safeIssueID)
	observationID = sanitizeIdentifier(observationID)

	clampString := func(s string, max int) string {
		r := []rune(s)
		if len(r) > max {
			return string(r[:max])
		}
		return s
	}

	obsRecord := map[string]any{
		"issue_key":      issueKey,
		"observation_id": observationID,
		"title":          clampString(targetIssue.Title, 150),
		"problem":        clampString(targetIssue.Problem, 1900),
		"impact":         clampString(targetIssue.Impact, 950),
		"proposal":       clampString(targetIssue.Proposal, 1900),
		"acceptance":     clampString(targetIssue.Acceptance, 1900),
		"evidence":       clampString(fmt.Sprintf("从客户端行为复盘记录导出: %s", targetIssue.Problem), 1900),
		"host":           "xiaotao-client",
		"skill_version":  r.CoreVersion,
		"source_refs":    []string{evidenceRelPath},
	}

	obsJSON, err := json.Marshal(obsRecord)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonBin, scriptPath, "--project-root", cleanProj, "record")
	cmd.Stdin = bytes.NewReader(obsJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("未加入建议: 建议箱脚本返回错误: %s", errMsg)
	}

	var cmdResp struct {
		Status      string `json:"status"`
		Path        string `json:"path"`
		IssueStatus string `json:"issue_status"`
	}
	_ = json.Unmarshal(stdout.Bytes(), &cmdResp)

	status := cmdResp.IssueStatus
	if status == "" {
		status = "pending"
	}

	newLink := ReviewSuggestionLink{
		IssueID:       targetIssue.ID,
		IssueKey:      issueKey,
		ObservationID: observationID,
		LinkedAt:      time.Now().UTC(),
		Status:        status,
		SourcePath:    evidenceRelPath,
	}

	updatedLinks := append(r.SuggestionLinks, newLink)
	if err := s.saveReviewSuggestionLinks(r.ID, updatedLinks); err != nil {
		// Log warning, return link anyway because suggestion has succeeded
		return &newLink, nil
	}

	return &newLink, nil
}

// ListProjectSuggestions reads pending evolution suggestions from the project.
func (s *Service) ListProjectSuggestions(projectPath string) ([]ExistingSuggestionItem, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("项目路径不能为空")
	}
	xiaotaoDir := filepath.Join(projectPath, ".xiaotao")
	info, err := os.Stat(xiaotaoDir)
	if err != nil || !info.IsDir() {
		return []ExistingSuggestionItem{}, nil
	}

	pythonBin, err := findPythonBinary()
	if err != nil {
		return nil, err
	}
	scriptPath, err := s.getTrustedCoreEvolutionScript()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonBin, scriptPath, "--project-root", projectPath, "list", "--limit", "50")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("读取建议列表失败: %s", stderr.String())
	}

	var resp struct {
		Suggestions []struct {
			IssueKey          string `json:"issue_key"`
			Title             string `json:"title"`
			Problem           string `json:"problem"`
			Status            string `json:"status"`
			ObservationsCount int    `json:"observations_count"`
			UpdatedAt         string `json:"updated_at"`
		} `json:"suggestions"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("解析建议列表失败: %w", err)
	}

	items := make([]ExistingSuggestionItem, 0, len(resp.Suggestions))
	for _, sg := range resp.Suggestions {
		upTime := time.Now()
		if t, err := time.Parse(time.RFC3339Nano, sg.UpdatedAt); err == nil {
			upTime = t
		}
		items = append(items, ExistingSuggestionItem{
			IssueKey:          sg.IssueKey,
			Title:             sg.Title,
			Problem:           sg.Problem,
			Status:            sg.Status,
			ObservationsCount: sg.ObservationsCount,
			UpdatedAt:         upTime,
		})
	}

	return items, nil
}
