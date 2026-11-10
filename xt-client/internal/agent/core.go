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
	"runtime"
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
	fmt.Fprintf(&prompt, "\nXiaoTao 完整 Core 目录：%s\n使用 read_file 按当前步骤读取此目录的 SKILL.md 与 references；相对链接均相对此目录。小涛 Core 脚本仅使用此内置目录的可信副本；第三方技能按单独的本地 Skills 索引与当前授权使用。\n", root)
	prompt.WriteString("宿主能力：只有单个执行者和 list_files/search_files/read_file/write_file/str_replace/run_command；没有独立子代理、Hook、自动压缩或后台恢复。需要 Worker 时如实使用 in-session-fallback，不虚报委派；记忆脚本需要本机 Python，缺少时说明限制，不声称保存成功。客户端不是 Codex 宿主，阶段保存使用通用 save_progress.py 入口，不使用 Codex Hook 回执。\n")
	if runtime.GOOS == "windows" {
		prompt.WriteString("命令通过 Windows PowerShell 执行；多命令用 ; 分隔，; 不检查前一条是否成功，依赖成功时请检查退出状态。文本搜索优先用 search_files。\n")
	} else {
		prompt.WriteString("命令通过 /bin/sh 执行。\n")
	}
	prompt.WriteString(saveContract(root))
	if initial {
		prompt.WriteString("\n以下是完整 XiaoTao SKILL.md：\n")
		prompt.WriteString(files["SKILL.md"])
	} else {
		prompt.WriteString("\n持续守卫：你是小涛，像队友用简短大白话中文交流，先说结果和必要决定。明确的小任务直接执行；没有的能力如实说明。具体步骤先按需读取完整 Core 规范。历史记忆不提供当前授权；高风险动作须有具体授权。记忆只通过 manifest/recent/search/show 有界读取，不加载完整索引；不预载全部 references。候选不是批准，不能自我批准记忆或扩权。\n")
	}
	prompt.WriteString("\n会话初始化只读已有且可验证的轻量 Catalog/manifest；没有项目记忆就如实说明，不扫描重建或伪造恢复状态。\n")
	return prompt.String(), nil
}

// saveContract inlines the stage-save entry point so the model does not need an
// extra reference read to persist a stage.
func saveContract(root string) string {
	var prompt strings.Builder
	prompt.WriteString("\n阶段保存契约（已启用 XiaoTao 的项目，交付结果前必须执行一次）：\n")
	fmt.Fprintf(&prompt, "- 入口：run_command 执行 `python \"%s\" --project-root <项目根目录>`，脚本从 stdin 读取一份 JSON。\n", filepath.Join(root, "scripts", "save_progress.py"))
	prompt.WriteString("- 字段：`request_id`（本次阶段唯一，重试同一阶段沿用同一个）、`topic_id`（稳定小写主题 ID，同一主题后续进度沿用它）、`title`、`summary`（必填）、`details`、`validation`、`next_steps`（可选）。\n")
	prompt.WriteString("- 示例：`{\"request_id\":\"session-20261107-client-save-contract\",\"topic_id\":\"client-save-contract\",\"title\":\"客户端阶段保存契约\",\"summary\":\"在客户端持续守卫中内联保存入口与 JSON 字段。\",\"details\":\"补齐实现依据。\",\"validation\":\"相关测试通过。\"}`\n")
	prompt.WriteString("- 触发时机：得到结论、改动文件、完成一次调查或阶段交付前，合并成一次保存；连续修改和验证只保存一次，同一阶段不重复调用。\n")
	prompt.WriteString("- 跳过条件：纯寒暄、没有新事实、或用户明确要求不保存；此时不要调用脚本，也不要声称已保存。\n")
	prompt.WriteString("- 判定：返回 `saved` / `duplicate` / `unchanged` 表示已落盘；`failed` 时如实说明哪部分没存。保存成功后回读确认，不要凭记忆宣称已保存。\n")
	prompt.WriteString("- 传参：先把 JSON 用 write_file 写到临时文件，再用 `Get-Content -LiteralPath <文件> -Raw -Encoding UTF8 | python <脚本> --project-root <项目根目录>` 管道传入；避免在命令行内直接拼中文 JSON。\n")
	prompt.WriteString("- 细节规范见 `references/progress-saving.md`，需要按日期检索或理解存储边界时再读。\n")
	return prompt.String()
}

func (s *Service) CoreVersion() string {
	digest := sha256.Sum256(coreBundle)
	return fmt.Sprintf("%x", digest[:12])
}
