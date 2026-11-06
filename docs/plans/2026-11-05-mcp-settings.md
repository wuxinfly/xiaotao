# MCP 设置界面 Implementation Plan

**Goal:** 在 Agent 设置管理 stdio MCP 服务，无需通过环境变量重启客户端。

**Architecture:** Agent 数据目录内保存 mcp.json；本地文件优先，否则兼容 XIAOTAO_MCP_CONFIG。每次任务读取配置快照，运行中连接不受修改影响。管理接口校验同源；测试仅初始化、发现工具并清理进程。

**Tech Stack:** Go、React、TypeScript、现有 ModalFrame。

## 实现步骤

1. `internal/agent/mcp_settings.go` 实现配置读取、校验、原子保存和连接测试；`mcp.go` 复用显式配置加载，跳过禁用服务；`service.go` 按任务读取配置。
2. `internal/app/agent_handlers.go` 添加 GET/PUT `/api/agent/mcp` 和 POST `/api/agent/mcp/test`，保留同源检查和请求大小限制。
3. `frontend/src/features/agent/MCPSettings.tsx` 提供服务名、命令、逐行参数、启停、删除、Chrome 浏览器工具和测试结果；接入 Agent 设置页签，独立保存。
4. Go 测试覆盖持久化、优先级、禁用、错误和连接测试；浏览器合成 API 回归覆盖保存、编辑失败保留、测试反馈及窄屏。
5. 同步 README 与架构文档。执行前端构建、浏览器回归、`go test ./...`、`go vet ./...`、规则检查；需要桌面验证时仅使用 `./build.ps1 -Env uat -Verify`。

## 验收边界

- 命令以当前用户身份运行，配置仅由本地管理界面或应用拥有者维护，不从项目读取。
- 保存从下一次任务生效，不修改运行中任务。测试未保存草稿，不代表已保存或真实工具调用成功。
- 本期仅 stdio command/args，不新增 HTTP、凭据环境变量或桌面操作能力。
- 未配置时显示空状态；旧环境变量配置可查看并另存到当前环境，不回写外部文件。
- 已有 VERSION 修改不属于本需求，不覆盖、不递增。

## 验证结果

- `npm ci`、`npm run build` 通过。
- `node --test tests/mcp-settings.browser.test.mjs tests/model-pricing.browser.test.mjs` 通过，覆盖新设置流程及原模型设置回归；390px 窄屏、浅海主题截图已检查。
- `go test ./...`、`go vet ./...` 通过，包含真实子进程协议 fixture、配置隔离和接口同源检查。
- `./build.ps1 -Env uat -Verify` 构建验证包成功，不递增版本，不替换正式产物。
- 启动 UAT 验证包被自动审批拒绝，未提供具体原因；因此未执行真实 Chrome MCP 连接和桌面壳验收。正式版保持运行。
- README 和 Agent 架构文档已同步。本次没有新增规则候选：已有配置隔离、同源与构建约定足够覆盖，没有新的重复经验或用户确认约定。


## 2026-11-06 内置浏览器调整

用户确认浏览器 MCP 是默认功能，默认启用、不可删除，不需要点击添加。读取和保存配置时均合并固定的 chrome-devtools 内置服务，旧同名配置的禁用和启动覆盖不再生效；空本地配置仍只覆盖环境变量中的自定义服务。界面移除添加浏览器入口，锁定内置行的启用、名称、命令和参数，保留连接测试，自定义服务继续支持增删和禁用。依赖仍为本机 Node.js / Chrome，首次使用通过 npx 下载包，不代表客户端已内嵌浏览器或 Node.js。

验收：首次进入即显示内置行；无添加浏览器或删除内置行按钮；保存空配置及禁用/覆盖旧同名配置后仍启用内置浏览器；自定义服务管理及测试连接保持可用。真实 Chrome 启动与桌面壳仍需人工验收。

验证结果：完整 `go test ./... -timeout 120s`、`go vet ./...`、MCP 设置页浏览器回归、前端构建、`./build.ps1 -Env uat -Verify` 及规则 check 均通过。内置浏览器不占原有 32 个自定义服务名额。测试使用隔离协议桩，真实 Chrome 与桌面壳仍待人工验收。本次是明确的产品需求，已记录在架构、README 和验收文档，不新增通用规则候选。
