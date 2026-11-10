package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const maxFileBytes = 512 * 1024

type Workspace struct {
	Root       string
	observedMu sync.Mutex
	observed   map[string][32]byte
}

// Coordinates our file tools across sessions, including paths outside a project.
// External editors and commands do not participate in this lock.
var fileToolMu sync.RWMutex

func fileObservationKey(target string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(target)
	}
	return target
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

func (w *Workspace) Resolve(path string) (string, error) {
	return cleanAccessPath(w.Root, path)
}

func (w *Workspace) Read(relative string, offset, limit int) (string, error) {
	fileToolMu.RLock()
	defer fileToolMu.RUnlock()
	target, err := w.Resolve(relative)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		// A fresh read confirming deletion acknowledges that change.
		if os.IsNotExist(err) {
			w.observedMu.Lock()
			delete(w.observed, fileObservationKey(target))
			w.observedMu.Unlock()
		}
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
	w.observedMu.Lock()
	defer w.observedMu.Unlock()
	if w.observed == nil {
		w.observed = make(map[string][32]byte)
	}
	w.observed[fileObservationKey(target)] = sha256.Sum256(data)
	return b.String(), nil
}

func (w *Workspace) Write(relative, content string, appendMode bool) error {
	fileToolMu.Lock()
	defer fileToolMu.Unlock()
	target, err := w.Resolve(relative)
	if err != nil {
		return err
	}
	key := fileObservationKey(target)
	current, readErr := os.ReadFile(target)
	previous, observed := w.observed[key]
	if !observed && readErr == nil {
		return fmt.Errorf("写入前请先 read_file 读取 %s 的最新内容，再保留现有修改进行写入", relative)
	}
	if observed && (readErr != nil || sha256.Sum256(current) != previous) {
		return fmt.Errorf("文件冲突：%s 自上次读取或写入后已变化，未执行写入。请重新 read_file，保留其他任务修改并合并；无法合并时向用户说明冲突", relative)
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if !appendMode && content == "" && readErr == nil && len(current) > 0 {
		return fmt.Errorf("拒绝用空内容覆盖非空文件 %s", relative)
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
	if err := f.Close(); err != nil {
		return err
	}
	if appendMode {
		current = append(current, []byte(content)...)
	} else {
		current = []byte(content)
	}
	if w.observed == nil {
		w.observed = make(map[string][32]byte)
	}
	w.observed[key] = sha256.Sum256(current)
	return nil
}
