# Agent 过程轨迹与结束摘要

**目标：** 首次提问不立即撑出空的思考面板；思考过程只设最大高度；结束后用紧凑图标摘要展示真实非零统计，并补充请求次数与输出速度。

**前端：** `RunTimeline` 在尚未收到摘要或工具活动时收起过程区，首个事件到达后再展开。轨迹采用内容高度和 96px 最大高度，最多显示最近四条。抽屉入口改为过程卡右上角图标，结束后摘要使用耗时、读取、写入、工具、请求和速度图标，零值项隐藏，完整说明通过 Portal tooltip 在悬停或键盘聚焦时显示。

**统计：** `RunStats` 新增 `modelRequests`、`outputTokens`、`outputSpeed`。主任务循环和展示摘要请求都计入请求次数；流式与非流式模型响应读取上游 `completion_tokens`，以首输出到请求结束的耗时累计速度，未提供 token 或耗时不足时不生成速度。旧历史缺失新字段时保持兼容，不显示猜测值。

**验证：** Go 覆盖请求次数、输出 token、速度累计和流式 usage；浏览器覆盖首轮紧凑高度、轨迹最大高度、右上角入口、零值隐藏、图标摘要、tooltip、浅海主题和 390/540px 窄屏。回归命令：在 `xt-client` 执行 `go test ./...`、`go vet ./...`；在 `xt-client/frontend` 执行 `npm run build` 与 `node --test --test-concurrency=1 tests/agent-summary-visual.browser.test.mjs tests/agent-timeline.browser.test.mjs tests/agent-answer.browser.test.mjs tests/agent-stream.browser.test.mjs`。

**验证结果（2026-11-07）：** 前端构建、`go test ./...`、`go vet ./...`、6 个隔离浏览器用例和 `./build.ps1 -Env uat -Verify` 均通过，验证包输出为 `xt-client/dist/verify/xiaotao-uat.exe`。规则检查通过，未修改规则源。

**上下文窗口补充（2026-11-07）：** Agent 模型选项增加 `contextWindow`，来自统一目录的 `MaxInputTokens`；发送时写入 `RunStats.ContextWindow`，前端对历史轮次或缺失统计回退当前模型窗口。这样 Codex、Qoder 等目录提供输入窗口的模型能显示百分比，其他来源仍按未知处理。Go 回归覆盖目录透传和发送统计。

**边界：** 摘要不展示无法确认的速度或 token；请求次数是客户端真实发出的模型调用次数，不等同于上游计费请求。桌面 WebView2 实际交互仍需新包人工复核。
