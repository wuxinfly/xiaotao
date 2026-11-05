package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ModeSafe = "safe"
	ModeFull = "full"
)

type Approval struct {
	ID     string `json:"id"`
	Tool   string `json:"tool"`
	Detail string `json:"detail"`
}
type pendingApproval struct {
	session string
	answer  chan bool
}

func (s *Service) AccessMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accessModeLocked()
}
func (s *Service) accessModeLocked() string {
	b, err := os.ReadFile(filepath.Join(s.dataDir, "access-mode.json"))
	var setting struct {
		Mode string `json:"mode"`
	}
	if err == nil && json.Unmarshal(b, &setting) == nil && setting.Mode == ModeFull {
		return ModeFull
	}
	return ModeSafe
}
func (s *Service) SetAccessMode(mode string) error {
	if mode != ModeSafe && mode != ModeFull {
		return fmt.Errorf("无效的访问模式")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]string{"mode": mode})
	return os.WriteFile(filepath.Join(s.dataDir, "access-mode.json"), b, 0600)
}

var riskyCommand = regexp.MustCompile(`(?i)(\b(rm|rmdir|del|erase|remove-item|rd|format|diskpart|shutdown|reboot|restart-computer|stop-computer|takeown|icacls|reg|set-content|clear-content|out-file|invoke-expression|iex|invoke-webrequest|iwr|curl|wget|powershell|pwsh|cmd|bash|sh|python|python3|node|npx|pip|pip3|winget|choco|scoop|msiexec)\b|\bgit\s+(reset|clean|checkout|restore|push|rebase|branch\s+-[dD])\b|(?:^|\s)[^\s]*\.(?:ps1|bat|cmd|exe)\b|>{1,2})`)

var packageInstall = regexp.MustCompile(`(?i)\b(npm|pnpm|yarn)\s+(install|i|add|remove|uninstall|publish|update|upgrade|ci)\b`)

func needsApproval(w *Workspace, name string, raw json.RawMessage) (string, bool) {
	if strings.HasPrefix(name, "mcp_") {
		return "外部 MCP 工具：" + name + "\n" + string(raw), true
	}
	if name == "write_file" && w != nil {
		var args struct {
			Path   string `json:"path"`
			Append bool   `json:"append"`
		}
		if json.Unmarshal(raw, &args) == nil && !args.Append {
			path, err := w.Resolve(args.Path)
			if err == nil {
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() >= 128*1024 {
					return fmt.Sprintf("覆盖文件：%s（%d 字节）", path, info.Size()), true
				}
			}
		}
		return "", false
	}
	if name != "run_command" {
		return "", false
	}
	var args struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(raw, &args) != nil {
		return "", false
	}
	return args.Command, riskyCommand.MatchString(args.Command) || packageInstall.MatchString(args.Command)
}
func (s *Service) waitApproval(ctx context.Context, session string, workspace *Workspace, call toolCall, emit func(Event)) error {
	detail, risky := needsApproval(workspace, call.Name, call.Arguments)
	if !risky || s.AccessMode() == ModeFull {
		return nil
	}
	a := Approval{ID: "approval_" + randomID(12), Tool: call.Name, Detail: detail}
	p := &pendingApproval{session: session, answer: make(chan bool, 1)}
	s.mu.Lock()
	s.approvals[a.ID] = p
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.approvals, a.ID); s.mu.Unlock() }()
	emit(Event{Type: "approval", SessionID: session, Approval: &a})
	select {
	case ok := <-p.answer:
		if !ok {
			return fmt.Errorf("用户拒绝了此操作")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) ResolveApproval(session, id string, approved bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.approvals[id]
	if p == nil || p.session != session {
		return fmt.Errorf("确认请求不存在或已失效")
	}
	select {
	case p.answer <- approved:
		delete(s.approvals, id)
		return nil
	default:
		return fmt.Errorf("确认请求已处理")
	}
}
func cleanAccessPath(root, path string) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	return filepath.Abs(path)
}
