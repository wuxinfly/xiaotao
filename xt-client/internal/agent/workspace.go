package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxFileBytes = 512 * 1024

type Workspace struct {
	Root string
}

func OpenWorkspace(raw string) (*Workspace, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("请先选择项目目录")
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return nil, fmt.Errorf("项目目录无效: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("项目目录不存在: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("请选择文件夹")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		absolute = resolved
	}
	return &Workspace{Root: filepath.Clean(absolute)}, nil
}

func (w *Workspace) Resolve(relative string) (string, error) {
	relative = strings.TrimSpace(relative)
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("只能访问当前项目内的路径")
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径超出了当前项目")
	}
	target, err := filepath.Abs(filepath.Join(w.Root, cleaned))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(w.Root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径超出了当前项目")
	}
	if existing, err := filepath.EvalSymlinks(target); err == nil {
		rel, err = filepath.Rel(w.Root, existing)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("路径超出了当前项目")
		}
		target = existing
	}
	return target, nil
}

func (w *Workspace) Read(relative string, offset, limit int) (string, error) {
	target, err := w.Resolve(relative)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s 不是普通文件", relative)
	}
	if info.Size() > maxFileBytes {
		return "", fmt.Errorf("%s 超过 %d KiB，请分段读取", relative, maxFileBytes/1024)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 || limit > 400 {
		limit = 240
	}
	start := offset - 1
	if start >= len(lines) {
		return "", fmt.Errorf("起始行超出文件范围")
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i+1, lines[i])
	}
	return b.String(), nil
}

func (w *Workspace) Write(relative, content string, appendMode bool) error {
	target, err := w.Resolve(relative)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendMode {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(target, flags, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
