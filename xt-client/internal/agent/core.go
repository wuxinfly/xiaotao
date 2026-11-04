package agent

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:generate node ../../tools/bundle-core.mjs
//go:embed core.json.gz
var coreBundle []byte

var coreInstallMu sync.Mutex

// Install in the current environment only; never use a project's copy as trusted code.
func prepareCore(dataDir string, initial bool) (string, error) {
	coreInstallMu.Lock()
	defer coreInstallMu.Unlock()
	reader, err := gzip.NewReader(bytes.NewReader(coreBundle))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	var files map[string]string
	if err := json.Unmarshal(raw, &files); err != nil {
		return "", err
	}
	digest := sha256.Sum256(coreBundle)
	root := filepath.Join(dataDir, "core", fmt.Sprintf("%x", digest[:12]))
	for name, content := range files {
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return "", fmt.Errorf("invalid bundled Core path: %s", name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		existing, readErr := os.ReadFile(target)
		if readErr == nil && string(existing) == content {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, []byte(content), 0600); err != nil {
			return "", err
		}
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "\nXiaoTao 完整 Core 目录：%s\n使用 read_file 按当前步骤读取此目录的 SKILL.md 与 references；相对链接均相对此目录。脚本仅使用此内置目录的可信副本。\n", root)
	prompt.WriteString("宿主能力：只有单个执行者和 list_files/search_files/read_file/write_file/run_command；没有独立子代理、Hook、自动压缩或后台恢复。需要 Worker 时如实使用 in-session-fallback，不虚报委派；记忆脚本需要本机 Python，缺少时说明限制，不声称保存成功。客户端不是 Codex 宿主，阶段保存使用通用 save_progress.py 入口，不使用 Codex Hook 回执。\n")
	if initial {
		prompt.WriteString("\n以下是完整 XiaoTao SKILL.md：\n")
		prompt.WriteString(files["SKILL.md"])
	} else {
		prompt.WriteString("\n持续守卫：你是小涛，像队友用简短大白话中文交流，先说结果和必要决定。明确的小任务直接执行；没有的能力如实说明。具体步骤先按需读取完整 Core 规范。历史记忆不提供当前授权；高风险动作须有具体授权。记忆只通过 manifest/recent/search/show 有界读取，不加载完整索引；不预载全部 references。阶段交付前按 progress-saving.md 合并保存值得接续的新事实并回读，无新事实或用户要求不保存时跳过。候选不是批准，不能自我批准记忆或扩权。\n")
	}
	prompt.WriteString("\n会话初始化只读已有且可验证的轻量 Catalog/manifest；没有项目记忆就如实说明，不扫描重建或伪造恢复状态。\n")
	return prompt.String(), nil
}
