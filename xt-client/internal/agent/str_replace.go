package agent

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StrReplace replaces one non-overlapping match after verifying the last observation.
func (w *Workspace) StrReplace(relative, oldStr, newStr string) error {
	fileToolMu.Lock()
	defer fileToolMu.Unlock()
	if oldStr == "" {
		return fmt.Errorf("old_str 不能为空")
	}
	target, err := w.Resolve(relative)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s 不是普通文件，不支持替换", relative)
	}
	current, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	key := fileObservationKey(target)
	previous, observed := w.observed[key]
	if !observed {
		return fmt.Errorf("替换前请先 read_file 读取 %s 的最新内容", relative)
	}
	if sha256.Sum256(current) != previous {
		return fmt.Errorf("文件冲突：%s 自上次读取后已变化，请重新 read_file", relative)
	}
	count := strings.Count(string(current), oldStr)
	if count == 0 {
		return fmt.Errorf("未找到匹配内容，请检查原文、缩进和换行")
	}
	if count != 1 {
		return fmt.Errorf("找到 %d 处匹配，请补充上下文", count)
	}
	updated := strings.Replace(string(current), oldStr, newStr, 1)
	if updated == "" && len(current) > 0 {
		return fmt.Errorf("替换结果为空，已拒绝")
	}
	if err := replaceFile(target, []byte(updated), info.Mode().Perm()); err != nil {
		return err
	}
	w.observed[key] = sha256.Sum256([]byte(updated))
	return nil
}

// replaceFile writes in the same directory; it never truncates the original on failure.
func replaceFile(target string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(target), ".strreplace-*")
	if err != nil {
		return err
	}
	name := f.Name()
	replaced := false
	defer func() {
		_ = f.Close()
		if replaced {
			return
		}
		// Windows cannot remove a read-only file; restore writable mode before cleanup.
		_ = os.Chmod(name, 0600)
		_ = os.Remove(name)
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if err := os.Rename(name, target); err != nil {
		return fmt.Errorf("替换文件失败: %w", err)
	}
	replaced = true
	return nil
}
