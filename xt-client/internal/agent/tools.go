package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const maxCommandOutput = 80 * 1024

var toolDefinitions = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "list_files",
			"description": "List files and directories under the current project. Paths must be relative to the project root.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "Relative directory path, default '.'"},
					"recursive": map[string]any{"type": "boolean", "description": "Whether to recurse into subdirectories"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search_files",
			"description": "Search text inside current project files. Prefer this over reading many files.",
			"parameters": map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Literal text to find"},
					"path":  map[string]any{"type": "string", "description": "Relative directory path, default '.'"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "read_file",
			"description": "Read a project file. Returns numbered lines. Use offset and limit for large files.",
			"parameters": map[string]any{
				"type":     "object",
				"required": []string{"path"},
				"properties": map[string]any{
					"path":   map[string]any{"type": "string"},
					"offset": map[string]any{"type": "integer", "description": "First line number, starting at 1"},
					"limit":  map[string]any{"type": "integer", "description": "Maximum lines, default 240, max 400"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "write_file",
			"description": "Create or replace a UTF-8 text file within the project. Use append=true to add content.",
			"parameters": map[string]any{
				"type":     "object",
				"required": []string{"path", "content"},
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
					"append":  map[string]any{"type": "boolean"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "run_command",
			"description": "Run a non-interactive command in the project root to build, test, inspect, or edit files. Destructive system commands are rejected.",
			"parameters": map[string]any{
				"type":     "object",
				"required": []string{"command"},
				"properties": map[string]any{
					"command":     map[string]any{"type": "string"},
					"timeout_sec": map[string]any{"type": "integer", "description": "Timeout in seconds, default 120, max 600"},
				},
			},
		},
	},
}

func (w *Workspace) Execute(ctx context.Context, name string, raw json.RawMessage) (string, error) {
	switch name {
	case "list_files":
		var args struct {
			Path      string `json:"path"`
			Recursive bool   `json:"recursive"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("参数无效")
		}
		return w.listFiles(args.Path, args.Recursive)
	case "search_files":
		var args struct {
			Path  string `json:"path"`
			Query string `json:"query"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("参数无效")
		}
		return w.searchFiles(args.Path, args.Query)
	case "read_file":
		var args struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("参数无效")
		}
		return w.Read(args.Path, args.Offset, args.Limit)
	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
			Append  bool   `json:"append"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("参数无效")
		}
		if err := w.Write(args.Path, args.Content, args.Append); err != nil {
			return "", err
		}
		return fmt.Sprintf("已写入 %s", args.Path), nil
	case "run_command":
		var args struct {
			Command    string `json:"command"`
			TimeoutSec int    `json:"timeout_sec"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("参数无效")
		}
		return w.runCommand(ctx, args.Command, args.TimeoutSec)
	default:
		return "", fmt.Errorf("未知工具: %s", name)
	}
}

func (w *Workspace) listFiles(relative string, recursive bool) (string, error) {
	if relative == "" {
		relative = "."
	}
	target, err := w.Resolve(relative)
	if err != nil {
		return "", err
	}
	var lines []string
	err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(w.Root, path)
		if err != nil || rel == "." {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist") {
			return filepath.SkipDir
		}
		if len(rel) > 180 || len(lines) >= 500 {
			return nil
		}
		if entry.IsDir() {
			rel += "/"
		}
		lines = append(lines, filepath.ToSlash(rel))
		if entry.IsDir() && !recursive {
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(lines)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "目录为空", nil
	}
	return strings.Join(lines, "\n"), nil
}

func (w *Workspace) searchFiles(relative, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("搜索内容不能为空")
	}
	if relative == "" {
		relative = "."
	}
	if runtime.GOOS == "windows" {
		return w.searchWindows(relative, query)
	}
	return "", fmt.Errorf("当前系统暂不支持内置搜索，请使用 run_command 调用 rg")
}

func (w *Workspace) searchWindows(relative, query string) (string, error) {
	target, err := w.Resolve(relative)
	if err != nil {
		return "", err
	}
	script := `$ErrorActionPreference='SilentlyContinue'; $q=$env:XIAOTAO_QUERY; ` +
		`Get-ChildItem -LiteralPath $env:XIAOTAO_SEARCH_ROOT -Recurse -File | ` +
		`Where-Object { $_.FullName -notmatch '[\\/](node_modules|vendor|dist|\\.git)[\\/]' -and $_.Length -le 1048576 } | ` +
		`Select-String -SimpleMatch $q -List | Select-Object -First 80 | ForEach-Object { $_.Path + ':' + $_.LineNumber + ': ' + $_.Line.Trim() }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Dir = w.Root
	cmd.Env = append(cmd.Environ(), "XIAOTAO_SEARCH_ROOT="+target, "XIAOTAO_QUERY="+query)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("搜索失败: %w", err)
	}
	text := strings.TrimSpace(strings.ReplaceAll(string(output), w.Root+string(filepath.Separator), ""))
	if text == "" {
		return "未找到匹配内容", nil
	}
	return truncate(text, maxCommandOutput), nil
}

func (w *Workspace) runCommand(parent context.Context, command string, timeoutSec int) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("命令不能为空")
	}
	lower := strings.ToLower(command)
	for _, blocked := range []string{
		"rm -rf /", "rm -rf c:", "format ", "shutdown", "reboot",
		"reg add", "reg delete", "takeown", "icacls ", "diskpart",
		"remove-item -recurse", "rd /s", "del /s", "git reset --hard",
		"git clean -fdx", "git checkout --",
	} {
		if strings.Contains(lower, blocked) {
			return "", fmt.Errorf("该命令包含高风险操作，已拒绝执行")
		}
	}
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	if timeoutSec > 600 {
		timeoutSec = 600
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}
	cmd.Dir = w.Root
	var output cappedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return truncate(output.String(), maxCommandOutput), fmt.Errorf("命令超过 %d 秒，已停止", timeoutSec)
	}
	if ctx.Err() != nil {
		return truncate(output.String(), maxCommandOutput), ctx.Err()
	}
	text := truncate(strings.TrimSpace(output.String()), maxCommandOutput)
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("命令失败: %w", err)
	}
	if text == "" {
		return "命令执行成功，无输出", nil
	}
	return text, nil
}

type cappedBuffer struct {
	buf bytes.Buffer
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len() < maxCommandOutput {
		remain := maxCommandOutput - b.buf.Len()
		if len(p) > remain {
			b.buf.Write(p[:remain])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string { return b.buf.String() }

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n...[输出已截断]"
}
