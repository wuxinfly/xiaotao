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

登录、刷新、账号和额度等短请求使用 30 秒超时客户端。聊天请求使用独立的无固定 `Client.Timeout` 客户端，等待响应头 300 秒、流式空闲 300 秒、总时长 30 分钟；收到新数据时重置空闲计时，超时后取消请求并中断读取。上游非 2xx 返回 `providers.UpstreamError`，代理按其状态码透传。

## 已完成

- 模型目录继续采用静态表（参考实现亦如此，尚无已确认目录接口），2026-11-01 补入 mimo-v2.6-pro；真实账号可用性待验收。

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

### 2026-11-01：11128 对照排查

- 使用本机 buddy-proxy 两份凭据及小涛当前 WorkBuddy 凭据，以相同最小消息直连 `/v2/chat/completions`；Python httpx 与 Go net/http 均收到 HTTP 200 和含 choices 的流式响应。未输出凭据、未替换已保存账号、未调用刷新接口。
- 使用小涛当前源码的 Collector、Streamer 及代理 HandleChat 入口验证：`workbuddy/glm-5.3-flash` 路由到 WorkBuddy，上游模型为 `glm-5.3-flash`；流式与非流式入口均返回正常结果，流式包含 `[DONE]`。buddy-proxy 的 `codebuddy/` 是其对外来源前缀，不应直接传给小涛。
- 本轮未复现历史 HTTP 400/code 11128；没有证据证明 MachineID 格式是根因，也未证明历史请求的账号、消息、工具及参数均与本轮相同。当前账号 ExpiresAt 为 0，正常聊天跳过自动刷新。
- 诊断时磁盘 EXE 构建时间为 15:29:58，源码调用成功不等于已验证该 EXE 或原调用端；需以最新构建继续验收原调用场景。临时诊断源码已清理，不保留含凭据的诊断文件。

### DSH Harness 的 11128 触发条件

用户进一步确认：DSH 经 buddy-proxy 同样失败，而 Codex 可调用。以相同 CN 凭据、模型 `glm-5.3-flash`、用户消息 `Reply OK.` 和固定请求头对照，仅变化首条消息：

| 首条附加消息 | 结果 |
| --- | --- |
| 无 | HTTP 200，返回 choices |
| system：You are a helpful coding assistant. | HTTP 200，返回 choices |
| developer：You are a helpful coding assistant. | HTTP 400 / code 11128 |
| system：You are an AI agent powered by DeepSeek Harness. | HTTP 400 / code 11128 |
| developer：You are an AI agent powered by DeepSeek Harness. | HTTP 400 / code 11128 |

本机已安装 DSH 的 dsh-system-prompt 含上述 Harness 身份文本；其 pi-ai OpenAI 转换器在 reasoning 且 supportsDeveloperRole 时生成 developer 角色。对照证明消息角色与内容均可触发渠道校验，不证明完整 DSH 请求只有这两个触发因素。无需继续将 MachineID 格式视为已确认根因。未修改 DSH 身份提示词或伪装官方客户端，未改变已保存凭据。后续兼容方案应使用上游支持的调用渠道或接入允许通用 Harness 的 API。
