# MCP 工具按需注入 Implementation Plan

**Goal:** 让不碰浏览器的任务不再每轮支付 Chrome DevTools MCP 的全部工具 schema 开销。

**Architecture:** 每轮对话只向模型暴露一个 `enable_mcp_tools` 激活工具，MCP 服务保持关闭；模型请求浏览器能力时才启动服务并把真实工具 schema 加入下一轮请求。工具执行、审批、活动记录沿用现有链路。

## 背景

当时实测 `chrome-devtools-mcp@latest` 暴露 32 个工具，`tools/list` 完整定义 29661 字节，约 8.5k token。原实现每轮都追加工具定义，40 轮约产生 350k token 的定义开销；现行循环没有固定轮数上限，MCP 工具按需加载。

官方 `--slim`（3 个工具，约 343 token）过于激进，`--no-category-*` 分类开关能降到约 5.9k token 但仍然每次全量携带。因此选择在客户端侧做按需注入，不改变 MCP 服务本身的行为。

## 实施

1. `internal/agent/mcp_lazy.go` 新增 `lazyMCP` 状态、`enable_mcp_tools` 激活工具定义、`enableMCP`、`enableMCPTools`、`resolveMCPTool`、`runMCPTool` 与 `historyUsesMCPTools`。
2. `internal/agent/service.go` 的 `loopWithRun` 改为持有 `lazyMCP` 而不是预连接；每轮用 `mcpState.definitions()` 决定暴露的工具；工具分发新增激活分支与按需解析分支；历史里用过 MCP 工具时预激活。
3. `internal/agent/access.go` 的 `needsApproval` 为激活工具返回确认项，沿用 MCP 工具的审批流程。
4. `internal/agent/service.go` 的 `systemPrompt` 追加浏览器说明，让模型知道要先激活；`activityTitle` 增加激活工具的中文动作名。
5. `frontend/src/features/agent/AgentPage.tsx` 的审批确认文案对激活工具显示“启用浏览器与 MCP 工具”。

## 行为

- 未使用浏览器：每轮请求的工具定义约 383 字节（109 token），MCP 服务不启动。
- 模型调用激活工具：需要用户确认（安全模式下），确认后启动服务，下一轮出现真实工具。
- 模型直接调用 `mcp_N` 工具：同样按需启动服务。
- 历史含 MCP 工具调用：本轮预激活，保证存档工具名可解析。
- 激活动作以“启用浏览器工具”记入活动与思考过程面板。

## 验证

- `go test ./...`、`go vet ./...` 通过。
- 新增 `internal/agent/mcp_lazy_test.go`：未激活时请求不含 MCP 工具、激活后下一轮出现 MCP 工具、直接调用按需启动、激活需要审批、历史预激活、系统提示包含浏览器说明。
- 新增 `internal/agent/mcp_real_browser_test.go`（需 `XT_REAL_BROWSER_MCP=1`）：对真实 chrome-devtools-mcp 发现 32 个工具并成功打开 example.com。
- 新增 `frontend/tests/mcp-lazy-activity.browser.test.mjs`：思考过程面板展示激活动作。

## 已知边界

- 启用浏览器后，后续请求仍携带全部浏览器工具 schema，这部分未裁剪。如需进一步压缩，可叠加官方 `--slim` 或 `--no-category-performance --no-category-network`，但会减少可用工具。
- `frontend/tests/mcp-activity.browser.test.mjs` 使用的 `.agent-process-summary` 选择器在当前前端已不存在，该测试在本次改动之前即已失效，属于并行前端改动的遗留问题。
