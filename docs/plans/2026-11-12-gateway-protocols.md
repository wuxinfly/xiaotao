# 本地网关与手动上游的三协议接入

外部入口与手动来源的上游格式独立选择。小涛 Agent 继续使用 Chat Completions；现有插件进程协议不变。

| 方向 | Chat Completions | Responses | Anthropic Messages |
| --- | --- | --- | --- |
| 本地入口 | `POST /v1/chat/completions` | `POST /v1/responses` | `POST /v1/messages` |
| 手动来源 protocol | `openai-chat` | `openai-responses` | `anthropic-messages` |
| 上游端点 | `/chat/completions` | `/responses` | `/messages` |
| 上游认证 | Bearer API Key | Bearer API Key | `x-api-key` + `anthropic-version: 2023-06-01` |

`internal/wireprotocol` 负责请求、响应及 SSE 事件转换；`internal/gateway` 保留鉴权、显式模型身份、用量、取消、空闲/总超时及拒绝重定向。三种外部入口都可调用手动来源和插件；协议互转经过共同 Chat 表达，未经客户端指定不会切换模型或来源。

## 配置和调用

在“模型接口”添加或编辑来源，按服务要求选择“上游格式”，填写该服务的实际 API 前缀或所选协议完整端点。不会猜测添加 `/v1`。例如 Responses 服务填写 `https://example.com/v1/responses`，保存为前缀 `https://example.com/v1`；测试连接使用所选协议发送小请求，可能产生实际用量。原有来源继续使用 `openai-chat`，编辑协议保留来源、内部模型 ID、调用别名及密钥。

外部工具仍用小涛本地访问密钥。支持 `Authorization: Bearer <本地密钥>`，也支持 `x-api-key: <本地密钥>`；上游密钥不会透传给外部工具。`GET /v1/models` 返回可调用模型 ID。OpenAI 客户端通常填 `http://127.0.0.1:5455/v1` 为 base URL；Anthropic 客户端如果自动追加 `/v1/messages`，base URL 应填 `http://127.0.0.1:5455`，以实际工具的路径拼接规则为准。

`POST /v1/responses`：

```json
{"model":"我的接口/模型","input":"你好","store":false,"stream":true}
```

`POST /v1/messages`：

```json
{"model":"我的接口/模型","max_tokens":1024,"messages":[{"role":"user","content":"你好"}],"stream":true}
```

## 支持范围与边界

- 文本、多轮消息、system/instructions、URL/base64 图片、函数工具定义/选择/调用及结果回传；Chat 与 Responses 支持 JSON schema 格式转换。图片能力是否可用仍以所选模型和上游支持为准。
- 同时支持流式与非流式。保留工具调用 ID、参数增量、原始模型路由与调用方使用的 model ID；Responses 输出事件带连续 sequence_number，Anthropic 输出 message/content block 事件。
- Chat/Responses 保留推理文本或摘要；不伪造 Anthropic 签名 thinking block，也不启用扩展 thinking。Anthropic 上游不接受网关的 `reasoning_effort` 或 `response_format`，请求会明确报错。
- Responses 为无状态对话接口：调用方发送完整历史，使用 `store:false`（省略也按 false 处理）。不支持 `previous_response_id`、conversation、后台任务、检索/删除已存响应、内置搜索/计算机工具、文件/音频及 encrypted reasoning。签名 thinking 历史和 provider 扩展选项不在共同协议范围，明确拒绝已识别的不支持字段。
- 不模拟服务端会话，也不提供 `/v1/messages/count_tokens`。这不是完整的 Responses/Anthropic 产品 API，不能据此宣称所有第三方 Agent 客户端全部兼容。
- 仅报告真实 usage，区分输入、输出与缓存 token；未知用量保留缺失/null，不估算为零。跨协议流式的起始事件可能尚无输入 token 数，结束时补充上游提供的真实数据。
- SSE 必须收到原生终止事件；错误、截断、取消或无终止事件不发送成功结束。HTTP 错误与 SSE 错误按外部协议返回，不回显上游响应中的密钥或其他敏感原文。
- 沿用 32 MiB 请求/单事件限制、300 秒空闲与30分钟总超时。Responses 最终响应需要保留输出，额外限制累计内容为32 MiB及最多1024个工具项；不能无限缓存整条流。Anthropic 若请求与模型均未给输出上限，适配器使用1024作为必要的协议默认输出上限，并限制为小于已声明的模型窗口。

## 验证

- `go test ./internal/wireprotocol ./internal/modelsource ./internal/sourceplugin ./internal/gateway`：三种入口 × 三种上游 × 流式/非流式的18组交叉回归；另覆盖插件复用、图片、工具历史、并行工具参数、JSON schema、拒答、真实/未知用量、格式持久化、无效请求、认证、重定向、上游错误、截断和取消。
- `go vet` 对上述包及 Windows 目标全量运行；前端 lint、大小检查与构建。
- `XIAOTAO_TEST_EXECUTABLE=/usr/bin/chromium node --test tests/source-protocols.browser.test.mjs tests/model-sources-layout.browser.test.mjs tests/model-catalog.browser.test.mjs`：隔离 mock API 验证格式选择、保存/回显/修改、密钥省略保留、目录及窄屏布局。布局断言按现有插件区块核查相邻间距，不把插件区块误当空白。
- 当前环境是 Linux；全量原生运行受已有 Windows desktop/secrets 实现及 runtimeenv 的 Windows 路径断言限制。真实上游账号、Windows 桌面壳与具体外部工具还需本地验收，不将 mock 回归视作真实服务验证。

本次定向 Go 测试及 race、定向与 Windows 目标全量 vet、前端 lint/大小检查/构建、三项浏览器回归均通过。Linux 全量 test/vet 的失败项为前述既有平台限制。

现有规则已覆盖明确模型身份、真实 usage、独立密钥与异常失败边界；本次无新增规则候选。
