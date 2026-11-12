package office

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xiaotao/xt-client/internal/agent"
)

func (s *Service) modelAvailable(model string) bool {
	for _, option := range s.exec.AvailableModels() {
		if option.Model == model {
			return true
		}
	}
	return false
}

func modelLabel(models []agent.ModelOption, model string) string {
	for _, option := range models {
		if option.Model == model {
			if option.Label != "" {
				return option.Label
			}
			return model
		}
	}
	return model
}

func resolveProject(rawPath, rawName string) (string, string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", "", fmt.Errorf("请选择项目目录")
	}
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", "", fmt.Errorf("项目目录无效: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("项目目录不存在: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("请选择文件夹")
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	name := strings.TrimSpace(rawName)
	if name == "" {
		name = filepath.Base(abs)
	}
	if runes := []rune(name); len(runes) > 100 {
		name = string(runes[:100])
	}
	return filepath.Clean(abs), name, nil
}

// taskPrompt frames the goal as one office task. The executor still runs the
// normal XiaoTao kernel; this only tells it why it was started.
func taskPrompt(title, goal string) string {
	return "项目办公室任务：" + title + "\n\n" + goal + "\n\n请在当前项目里完成这件事，做完给出简短结果；写入的文件会出现在项目资料架。"
}

func titleFromGoal(goal string) string {
	text := strings.Join(strings.Fields(goal), " ")
	runes := []rune(text)
	if len(runes) > 28 {
		return string(runes[:28]) + "..."
	}
	return text
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func artifactID(taskID, path string) string {
	sum := sha256.Sum256([]byte(taskID + "\x00" + path))
	return "art_" + hex.EncodeToString(sum[:6])
}
