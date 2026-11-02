package agent

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const maxProjectInstructionsBytes = 32 * 1024

func (w *Workspace) projectInstructions() (string, error) {
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		target, err := w.Resolve(name)
		if err != nil {
			return "", fmt.Errorf("项目指令 %s: %w", name, err)
		}
		f, err := os.Open(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("读取项目指令 %s: %w", name, err)
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return "", fmt.Errorf("读取项目指令 %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			f.Close()
			return "", fmt.Errorf("项目指令 %s 不是普通文件", name)
		}
		data, err := io.ReadAll(io.LimitReader(f, maxProjectInstructionsBytes+1))
		f.Close()
		if err != nil {
			return "", fmt.Errorf("读取项目指令 %s: %w", name, err)
		}
		if len(data) > maxProjectInstructionsBytes {
			return "", fmt.Errorf("项目指令 %s 超过 32 KiB", name)
		}
		if !utf8.Valid(data) {
			return "", fmt.Errorf("项目指令 %s 不是有效的 UTF-8", name)
		}
		if strings.TrimSpace(string(data)) == "" {
			continue
		}
		return fmt.Sprintf("项目指令（%s）：\n%s", name, string(data)), nil
	}
	return "", nil
}
