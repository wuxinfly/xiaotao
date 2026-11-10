# Agent 消息复制与回答分支

日期：2026-11-10
状态：已实现，浏览器自动化通过，桌面壳待人工验收。

## 需求

- 支持复制用户提问（query）。
- 支持从某条已完成回答创建对话分支。
- 用户确认交互调整：操作入口使用三个点“更多”菜单，不再往回答统计行直接加按钮；复制结果使用全局提示，不在按钮内计时；分支要带完整的工具执行过程和统计。

## 交互

- 用户提问在悬停或键盘聚焦时直接显示复制按钮，不放进菜单；触屏与窄窗口下入口常驻。
- 已完成回答在悬停或键盘聚焦时显示“更多”按钮，菜单项：
  - 复制：复制回答原始 Markdown，可原样粘回输入框，保留列表、代码块结构。
  - 创建分支：运行中的会话禁用并在提示中说明。
  - 查看思考过程：原先统计行右侧的独立过程按钮移入菜单，运行中的过程卡仍保留临时入口。
- 复制使用 `navigator.clipboard.writeText`，失败时回退到隐藏 textarea + `execCommand`；成功/失败通过全局 toast 提示。
- 菜单复用 `Popover`：Portal、视口避让、Escape、外部点击、方向键导航和焦点恢复沿用现有实现。

## 后端

`POST /api/agent/sessions/{id}/branch`，body `{messageId}`，同源校验。实现位于 `internal/agent/branch.go`，`Service.Branch`：

- 拒绝源会话运行中、消息不存在、非 assistant 消息、工具调用消息（`toolCalls` 非空）、回答所属轮次未结束。
- 截断到目标回答所在位置，复制全部消息；活动、思考、轮次统计按复制范围内的 `runId` 过滤，无 `runId` 的旧记录按时间不晚于回答时间保留。
- 不重新编号消息、`runId` 和工具调用 ID，保证工具调用与工具结果配对；存储按 `session_id + position` 隔离，跨会话重名无影响。
- 引用快照按引用 ID 全局保存，消息中的 references 元数据随之复制后仍可打开。
- 不复制上下文压缩摘要，避免截止点越过分支点；原始消息完整保留。
- 新会话：新 `run_` ID、标题“原标题 · 分支”（上限 100 字符）、沿用项目、模型和 `ModelSettings`、状态 idle、当前时间戳，并登记到运行表以便继续发送。

## 前端

- `messages/MessageActions.tsx`：单条消息的“更多”菜单与复制逻辑。
- `messages/MessageFeed.tsx`：从 `AgentPage.tsx` 抽出的消息展示组件（`MessageRow`、`RunTimeline`、`RunSummary`、`LiveStats`），本轮新增分支入口。
- `messages/runStats.ts`：轮次展示辅助函数与思考标签。
- `useBranchSession.ts`：调用分支接口、更新侧栏、切到新会话并提示。
- `lib/clipboard.ts`：复制与兜底。
- 新增样式 `messages/message-actions.css`，浅海主题覆盖写入 `shallow-sea.css`；不扩大 `agent.css` 和 `AgentPage.tsx`。

## 验证

- `go test ./internal/agent -run TestBranch`（新增 `branch_test.go`）：复制截断位置、工具调用配对、统计与活动、源会话不变、运行中/未完成/非法目标拒绝。
- `node --test tests/agent-message-actions.browser.test.mjs`：菜单触发、复制原始 Markdown、键盘打开与 Escape、复制成功提示、分支请求体、切换新会话、源会话保留、运行中无分支入口。
- `npm run lint`、`npm run check:size`、`npm run build`、`go test ./...`、`go vet ./...`、`node tools/check-go-size.mjs` 通过。
- 已知既有失败（与本功能无关，已在干净 HEAD 复现）：`tests/agent-avatar.browser.test.mjs`（失败态头像断言）和 `tests/agent-answer.browser.test.mjs`（思考抽屉 strict mode 多元素匹配）。

## 待验收

- WebView2 桌面壳中的剪贴板行为、菜单定位与键盘焦点。
- 在接近 80 条历史上限的长会话中分支并继续对话。