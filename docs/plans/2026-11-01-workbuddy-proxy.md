# WorkBuddy 反代接入计划（2026-11-01）

## 范围

接入 WorkBuddy 国内版反代，协议参考 `E:\code_my\utils\buddy-proxy`（CodeBuddy 插件协议）。本期包含：浏览器授权登录、token 刷新、OpenAI 兼容聊天（流式与非流式）、工具调用、真实额度展示、账号页管理。不含海外版、多账号 failover、自动签到。

## 模块

| 模块 | 位置 | 说明 |
| --- | --- | --- |
| Provider | `xt-client/internal/providers/workbuddy/` | 登录、刷新、聊天（SSE）、DSML 工具调用解析、额度查询 |
| 账号会话 | `xt-client/internal/authsessions/sessions.go` | 同步返回 authUrl 供前端打开浏览器，后台轮询 token |
| 路由 | `xt-client/internal/app/app.go` | `POST/GET/DELETE /api/auth/workbuddy[/{id}]` |
| 代理 | `xt-client/internal/proxy/service.go` | 按 provider 分派模型列表、聊天、额度；WorkBuddy 使用原始 OpenAI 请求格式 |
| 前端 | `xt-client/frontend/src/features/accounts/` | 登录流程、授权链接复制、额度优先展示 remaining |

## 请求格式约定

WorkBuddy 上游接受标准 OpenAI 格式：`model`、`stream=true`、`messages`（原始结构）、`tools`（JSON Schema 对象）、`tool_choice`（字符串或 function name）。与 Trae 不同：不使用 `function: chat_v3` 信封、不做 NativeMessages 转换、不将 tools 参数字符串化。

## 超时与稳定性

聊天流空闲超时 300 秒（`idleTimeoutReader`），总时长超时 30 分钟（`context.WithTimeout`）。上游非 2xx 返回 `providers.UpstreamError`，代理按其状态码透传。

## 已完成

- 登录链路：state → authUrl → 轮询 token → login/account，带 `Authorization`/`X-Domain`/`X-Product-Code` 及 `X-No-*` 请求头。
- 刷新链路：refresh token 接口，`expiresAt` 毫秒转秒。
- 聊天链路：强制 `stream=true`，SSE 解析 content/reasoning/usage/tool_calls，DSML 文本工具调用兜底解析（按模型缓冲）。
- 额度链路：`get-user-resource-summary`（Remaining 优先）+ `get-user-resource`（到期时间）。
- 代理分派：WorkBuddy 分支使用原始 OpenAI 请求体，`tool_choice` 结构体归一化为函数名；模型 ID 必须带 `workbuddy/` 前缀。
- 前端账号页：登录弹窗、授权链接复制、手动打开授权页按钮、额度 remaining 优先展示、官方 Trae 图标。

## 验证

```powershell
cd xt-client
go test ./...
go vet ./...
cd frontend
npm ci
npm run build
cd ..
./build.ps1
```

以上全部通过。`git diff --check` 与 `node tools/agent-rules.mjs check` 均通过。真实账号的登录闭环、额度数字、DSML 输出、usage 统计仍需人工验收。
