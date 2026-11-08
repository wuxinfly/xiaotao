# 小涛 Agent 上下文瘦身与长会话优化

## 一、背景与问题

在小涛 Agent 的长会话中，随着任务深入、多轮工具调用（如 `run_command`、`read_file`、`search_files`）不断累积，历史工具结果（单次最高可达 80 KiB）会被无上限地在每一轮请求中原样携带，导致：
1. 模型窗口极易被大段陈旧的命令输出与文件代码塞满，甚至触碰上游上下文上限；
2. 用户无法直观了解当前上下文到底消耗在系统指令、工具定义、对话还是工具结果上；
3. 上游返回的真实 usage 及缓存命中字段在 Agent 内核未被充分利用；
4. 缺乏在不破坏原始会话记录的前提下安全精简模型视图并提供历史回查的能力。

## 二、架构设计与实施成果

### 1. 阶段 A：上下文用量分类可视化（ContextBreakdown & Usage）
- **分类统计与口径一致**：在 `internal/agent/context_stats.go` 中实现 `calculateContextBreakdown`，将每次发送给模型的请求内容精确拆解为：
  - 系统指令与项目上下文（`systemTokens`）
  - 工具定义及其 JSON Schema（`toolsTokens`）
  - 对话历史消息正文（`messagesTokens`）
  - 工具调用与工具输出（`toolResultsTokens`）
  - 引用对话快照（`referenceTokens`）
  - 图片估算（`imageTokens`，标记 `hasImages`）
  - 各分类采用统一的 UTF-8 bytes-to-tokens 算法，合计严格等于 `totalTokens`。
- **缓存命中与真实 Usage 归一化**：
  - 更新 `stream.go` 与 `service.go` 的后端请求解析，接通代理层提供的 `cached_tokens` 与 `prompt_tokens_details`；
  - `RunStats` 新增 `cachedTokens`、`cacheKnown` 与 `breakdown` 字段，严格区分“当前请求估算”与“任务累计实际用量”。
- **前端交互入口**：
  - 抽离 `ContextDetailPopover.tsx` 与独立样式 `context-detail.css`；
  - 点击底栏 `agent-context-meter` 弹出详情浮层，展示分段彩色进度条、分类占比、剩余空间及累计用量；
  - 支持键盘 Escape 关闭、焦点管理与 `prefers-reduced-motion` 动效优化；未知窗口与缺失缓存时不崩溃，以“—”和明确文字降级展示。

### 2. 阶段 B：旧工具结果瘦身（Pruning & Lookup）
- **原始历史完整持久化，仅模型请求动态裁剪**：
  - 消息在 SQLite 中的原始内容未做任何修改，用户在界面查看执行过程和历史仍可展开查看完整的 80 KiB 输出；
  - 仅在 `modelHistoryWith` 投影模型视图时触发裁剪。
- **有界裁剪与边界保留**：
  - 边界：当前正在执行的任务及最近 1 轮（`preserveRecentToolTurns = 1`）的工具结果保持完整；
  - 更早轮次且长度超过 2048 字符的工具输出执行确定性精简：
    - `run_command`：保留启动前部与尾部关键报错/退出码（如 `FAIL`, `exit status 1` 等）；
    - `read_file`：保留首尾代码行结构，注明裁剪行数与总字节；
    - `search_files` / `list_files`：保留首批匹配项与项数总计；
    - 明确标识已裁剪，并注入 `read_historical_tool_result(tool_call_id=...)` 提示；
    - 严格保证 `assistant` 的 `tool_calls` 与 `tool` 结果的 `tool_call_id` 成对匹配，绝不破坏协议。
- **受限的历史结果回查（`read_historical_tool_result`）**：
  - 模型在发现精简信息不足时可按需调用该工具回查；
  - 严格限制只能在当前会话的 `history.Messages` 中按 `tool_call_id` 读取；
  - 支持关键词 `query` 定位或行号 `offset/limit` 局部提取，单次提取上限 3000 runes；
  - 历史输出按不可信数据标注，禁止借此访问外部文件或其他会话。

### 3. 阶段 C：长会话基于阈值的自动压缩（Auto Compaction）
- **触发与边界**：
  - 窗口已知时在上下文达到 80% 阈值（`compactionThresholdRatio = 0.8`）触发；窗口未知时按保守阈值 40,000 tokens 触发；
  - 保留最近 2 轮完整用户问答，边界（`findCompactionCutoff`）严格落在完整回合之间，不切断未完成的工具调用批次。
- **派生存储与恢复**：
  - `Session` 增加 `Summary *ContextSummary` 派生记录，保存覆盖的消息边界 `upToMessageId`、摘要正文和模型信息；
  - 请求时仅在模型视图头部注入结构化摘要（用户目标、确认约束、关键文件、完成工作、未完成事项），并只发送边界之后的较新消息；
  - 摘要生成失败或超时时优雅保留原历史，不丢弃上下文或中断任务。

## 三、实测验证与数据对比

1. **单测与端到端覆盖**：
   - Go 单测：`context_stats_test.go`（分类估算与缓存归一化）、`context_prune_test.go`（各种类工具裁剪、配对有效性与回查）、`context_summary_test.go`（截断边界、阈值判定、摘要视图投影与生成降级）；
   - 前端端到端：`agent-context-detail.browser.test.mjs`（Playwright 覆盖详情浮层弹窗、分类柱状图、实际用量、未知窗口与键盘 Escape 交互）。
2. **长会话样本实测 Token 削减**：
   - 在多轮重型工具调用的长会话基准测试样本中：
     - 瘦身前总上下文估算：31,723 tokens（其中工具输出占 31,023 tokens）；
     - 瘦身后总上下文估算：14,411 tokens（其中工具输出降为 13,711 tokens）；
     - 单次直接节省 **17,312 tokens**，总上下文规模降低了 **54.6%**；
     - 关键错误堆栈、退出码以及最近 1 轮工具输出保持 100% 完整原貌。
3. **代码质量指标**：
   - `internal/agent/service.go` 通过抽取 `context_stats.go`、`context_prune.go`、`context_summary.go`，行数由 1275 降至 1196（低于基线 1207）；
   - 前端 `AgentPage.tsx` 组件行数保持在 1064（低于基线 1070），`api.ts` 保持在 598 行（低于基线 599）；
   - 全套门禁 `npm run lint`、`npm run check:size`、`node tools/check-go-size.mjs`、`go vet ./...`、`go test ./...` 均通过。
