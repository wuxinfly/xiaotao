# Codex 反代接入计划（2026-11-01）

## 范围

在 `xt-client` 中新增 Codex（ChatGPT 订阅）反代。认证与上游协议参考
`E:\code_my\utils\buddy-proxy`（MIT License, Copyright (c) 2026 Mayer）。首期完成：
浏览器 OAuth 添加账号、导入 `~/.codex/auth.json`、token 刷新、OpenAI Chat 兼容聊天
（流式与非流式）、工具调用、模型目录、真实配额展示、账号页管理。首期不含多账号
failover、自动冷却换号、原生 Responses API、Anthropic Messages、图片生成。

## 目标边界

| 项 | 决策 | 说明 |
| --- | --- | --- |
| 客户端协议 | OpenAI Chat Completions | 现有 `/v1/chat/completions`、用量、账号页复用 |
| 上游协议 | Codex Responses SSE | Chat 请求先转 Responses；SSE 聚合成 Chat 结果 |
| 认证 | OAuth PKCE + 导入登录态 | 固定回调端口 1455，导入作备用入口 |
| 账号数 | 单账号 | 先不实现多账号索引、顺位与 failover |
| 目录 | 静态表 + 实时刷新 | 失败保留旧目录，无旧目录回退静态表 |
| 配额 | `/backend-api/wham/usage` | 不用本地估算伪装真实额度 |

## 模块

| 模块 | 位置 | 说明 |
| --- | --- | --- |
| Provider | `xt-client/internal/providers/codex/` | OAuth、刷新、模型目录、Chat ↔ Responses、Collector/Streamer、Refresher、Quoter |
| 会话 | `xt-client/internal/authsessions/sessions.go` | Codex 登录分支：先监听回调，再返回授权链接 |
| 路由 | `xt-client/internal/app/app.go` | `POST/GET/DELETE /api/auth/codex[/{id}]` 与导入接口 |
| 代理 | `xt-client/internal/proxy/service.go` | 注册模型、聊天与配额分派；模型 ID 带 `codex/` 前缀 |
| 前端 | `xt-client/frontend/src/features/accounts/` | 登录入口、导入粘贴框、等待状态与配额展示 |

## 关键技术约束

- 上游基址 `https://chatgpt.com/backend-api/codex`；对话请求 `POST /responses`，目录
  `GET /models?client_version=<ver>`，配额 `GET /backend-api/wham/usage`。
- 必须发送 `Originator: codex-tui`、Codex CLI 同代 `User-Agent` 和每次请求生成的
  `Session-Id`。
- 请求体固定 `store: false`、`stream: true`；`instructions` 缺失时补空字符串。客户端
  要非流式时，上游仍收 SSE，聚合 `response.completed` 或 `response.incomplete`。
- 请求头带 `Chatgpt-Account-Id`。ID token 里能取到 `chatgpt_account_id`、email、
  plan 和 user ID，登录时随凭据保存。
- OAuth 固定 `localhost:1455/auth/callback`。监听失败则改用手动粘贴回调 URL 或导入
  登录态；授权链接先固定 `prompt=login`，避免复用浏览器会话登录另一个账号。
- OpenAI 刷新会轮换 refresh token。所有刷新必须走账号级锁，写盘前先比对内存版本，
  旧 refresh token 不得覆盖新值。
- 流式请求按上游 200 后首帧分类：HTTP 401 先强制刷新重试一次；HTTP 或带内 403/429
  返回明确错误。首帧心跳不算成功；首个语义事件到达后不再刷新或重试。
- SSE 空闲超时 90 秒，总时长 30 分钟。连接回收必须覆盖首帧前失败、聚合失败和客户端
  取消路径。

## 请求投影

### OpenAI Chat 转 Responses

`system` / `developer` 消息合并为 `instructions`；请求里的已有 `instructions` 与 system
合并，不互相覆盖。`user` / `assistant` / `tool` 转 `input`；assistant 的 `tool_calls`
转 `function_call`，工具结果转 `function_call_output`，保持 `call_id` 配对。文本分片用
`input_text` / `output_text`；图片 URL 转 `input_image`。嵌套 function tools 转扁平
tools；`tool_choice` 做字符串和函数名映射。`temperature`、`top_p`、`max_tokens`、
`max_completion_tokens` 和 `reasoning_effort` 做受控映射；`max_output_tokens` 下限 16。
不回灌 assistant reasoning 内容；`stop`、`previous_response_id`、`stream_options` 等
上游不接受或无意义的字段丢弃。

### Responses SSE 转 Chat

文本增量转为 `delta.content`；`summary_text` / `reasoning_text` 转
`delta.reasoning_content`；函数参数增量聚合成 OpenAI `tool_calls`，补 `id`、`index`、
`name` 与 `arguments`。终态读取 `usage.input_tokens`、`output_tokens` 和缓存/推理明细，
映射成 Chat usage。`response.failed` 映射 HTTP 语义；`response.incomplete` 不误判为
账号错误。终态后停止读取并关闭连接。

## 实施任务

### 1. Codex 协议底座

**改动**

- 新建 `xt-client/internal/providers/codex/protocol.go`、`response.go`、`sse.go`。
- `protocol.go` 定义 Chat → Responses 投影和字段过滤。
- `response.go` 定义 SSE 事件解析、文本/推理/工具增量折叠、非流式聚合、usage 映射。
- `sse.go` 定义带错误传播的行扫描、心跳识别、首帧语义事件判断和连接关闭。

**测试**

1. system 合并、instructions 去重；assistant 工具历史与 tool 结果配对。
2. 图片消息、tools、tool_choice、reasoning effort、max tokens 的映射。
3. 文本、推理摘要、工具调用、usage 的流式折叠。
4. response.failed 与 incomplete 的错误边界。
5. 心跳、空流、超时和 `data: [DONE]` 不误判成功。

### 2. OAuth 与导入登录

**改动**

- 新建 `xt-client/internal/providers/codex/oauth.go`、`login.go`。
- `oauth.go` 生成 PKCE S256、state、授权 URL，并解析 `id_token` claims。
- `login.go` 启动固定端口回调监听、换取 token、解析身份和套餐。
- `sessions.go` 增加 Codex 分支，先监听后返回 URL；15 分钟超时，取消时关闭监听。
- 监听失败降级为手动粘贴回调 URL；粘贴和导入接口都必须校验 state，缺失或不同
  直接拒绝。
- 新增 `POST /api/auth/codex/import`，接受 Codex `auth.json` 结构，必填
  `access_token` 和 `refresh_token`。导入后刷新一次，确认 refresh token 可用；为空时
  拒绝。导入成功后从 ID token 或接口补全身份，账号 ID 优先
  `chatgpt_account_id`，退 user ID，最后用 refresh token 哈希。

**测试**

1. 授权 URL、PKCE 和 ID token claims 解析。
2. 回调 state 校验、超时和取消关闭监听。
3. 回调服务不可用时流程状态和错误提示。
4. 导入解析、身份派生和缺 refresh token 拒绝。

### 3. Token 刷新与目录配额

**改动**

- 新建 `xt-client/internal/providers/codex/refresh.go`、`models.go`、`quota.go`。
- `refresh.go` 提前 5 分钟刷新，HTTP 401 后强制刷新一次。refresh token 轮换时，
  用账号级锁协调，写盘前比对内存 refresh token。
- `models.go` 维护默认模型静态表，支持 `models?client_version=` 实时刷新；结果带
  `UpstreamID` 和显示名。只收录 `visibility=list` 且 `supported_in_api != false`。
- `quota.go` 请求 usage 接口，兼容 primary/secondary 与
  `primary_window` / `secondary_window`。已用百分比转为剩余百分比，`reset_at` 或
  `reset_after_seconds` 转 `expireAt`；credits 按实际返回映射，不伪造数字。
- 代理目录缓存沿用五分钟复用、显式刷新、失败保留旧目录。

**测试**

1. token 到期、401 强刷和 refresh token 轮换回写。
2. 并发刷新只发一次 token 请求。
3. 目录筛选、显示名合并、失败回退。
4. usage 两种形状的剩余百分比和重置时间映射；非法数字不展示。

### 4. 代理和前端

**改动**

- `app.go` 注册 Codex provider 和登录、导入路由。
- `bridge.go` 增加 Codex 刷新、配额和 Collector/Streamer 分派。
- `service.go` 允许 Codex 目录动态刷新；Chat 请求先转 Responses 后转发。
- `service.go` 流式发送调整：先由 provider 确定首帧分类，再输出成功帧；首帧前
  认证错误改为 HTTP 错误，语义已开始后才走现有流内错误展示。
- 前端新增 Codex 图标、登录入口与等待文案；导入弹窗粘贴 `auth.json` 或回调 URL；
  配额按 percent 展示，无数字显示未知。
- 登录成功后默认启用账号，首个 Codex 账号自动设为默认。

**测试**

1. 登录 API、导入 API、取消和保存账号链路。
2. 模型列表、聊天非流式、流式和工具调用分派。
3. 首帧前 HTTP 401/403/429 不再伪装成成功流。
4. 前端交互覆盖悬停、键盘、Esc、加载与失败反馈。

### 5. 文档与仓库同步

**改动**

- 更新 `xt-client/README.md`、`xt-client/AGENTS.md` 反代约定。
- 更新 `docs/architecture/xt-client-overview.md` 的接入状态与后续限制。
- 提交 `docs/plans/2026-11-01-codex-proxy.md`；若补充设计先写
  `2026-11-01-codex-proxy-design.md`。

**检查**

1. 请求格式、超时、登录和配额约定与代码一致。
2. 首期范围外能力明确标为限制，不写成已实现。
3. 复用 buddy-proxy 的实现时保留 MIT 版权与许可声明；若整段复用，放
  `providers/codex/LICENSE`。

## 验收

先跑自动化验证，再做真实账号验收。

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

`git diff --check` 和 `node tools/agent-rules.mjs check` 也必须通过。真实账号验收
覆盖：

1. 浏览器 OAuth 登录、账号身份、套餐和配额数字。
2. 导入登录态、首次刷新和身份回填。
3. 流式文本、非流式文本、连续工具调用和 usage 统计。
4. 目录刷新失败保留旧目录。
5. token 到期刷新、401 强刷重试和 refresh token 轮换。
6. 429 或 403 展示明确错误，不伪装成功。
7. 请求取消、断流、空闲超时和总时长超时。
8. 桌面窗口与浏览器入口的一致性。

## 已知限制

- 多账号 failover、冷却和自动换号不做；额度耗尽或风控直接报错。
- 原生 Responses API 和 Anthropic Messages 不做；外部 Codex CLI 需等待后续适配。
- 图片生成不做。
- state 校验、refresh token 轮换和首帧错误边界是接入必须项，不因赶进度绕过。

## 规则候选

暂无。实施中若确认可复用规则，写入 `docs/agent/candidates/`，并记录理由、观察次数
和 source_refs；只有事实类可凭明确证据应用，约定或安全规则必须待用户确认。
