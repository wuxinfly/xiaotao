# 模型来源插件开发与 Trae 本地验证

当前实现：本地目录安装、启停、独立进程调用、统一模型目录、流式/非流式 Chat、凭据加密、当前环境旧 Trae 账号显式导入、模型发现、配额和签到。Trae 是独立可执行程序，客户端核心不链接平台代码。真实 Trae 账号和 Windows 桌面壳需本地验收。

限制：每个插件 ID 当前只有一个账号；同一插件的操作串行排队，避免续期覆盖。尚无浏览器 OAuth 登录、卸载/更新界面、在线市场、多账号和旧模型引用批量映射。账号更换会清空目录，需重新刷新；旧会话模型参数不改写。其他平台未接入。不要将此版本当作原计划第 3、4 部分全部完成。

## Windows 本地测试

在 `xt-client` 执行：

```powershell
go test ./internal/sourceplugin ./cmd/trae-source-plugin ./internal/providers/trae
go test ./...
go vet ./...
./plugins/trae/build.ps1
./build.ps1 -Env uat -Verify
```

插件包生成在 `xt-client/dist/plugins/trae`，包含 `manifest.json` 和 `provider.exe`。客户端构建与插件构建分别执行；插件不会随客户端自动安装。`-Verify` 不递增版本。

1. 启动 UAT，在“模型接口”底部找到“模型来源插件”。
2. 填插件包的完整目录路径，点击安装。安装只复制文件，不启动代码。核对插件名称、版本、ID 和入口后启用。
3. 导入 Trae 国内版刷新令牌；如果当前环境已有旧 Trae 账号，可点击对应导入按钮。旧账号迁移只解密当前环境 `accounts/accounts.json` 的选中账号，将凭据用当前密钥环保存到插件状态，不改旧文件，也不立即联网验证。下次刷新会验证登录状态。导入会替换插件当前账号。
4. 点击“刷新模型”，再到 Agent 设置中选择 Trae 插件模型。目录只在显式刷新时访问上游，刷新失败保留旧快照。
5. 测试普通聊天、推理、工具调用、图片、Max 变体、停止生成；测试配额、签到状态和签到。界面当前展示原始结构化配额结果。
6. 外部工具继续使用 `/v1/models` 返回的稳定内部模型 ID 调用 `/v1/chat/completions`，使用本地网关密钥。
7. 停用后模型从可用目录消失，在途请求被取消；旧会话保留，旧 ID 不会自动绑定别的同名模型。

正式版与 UAT 的插件目录、密钥和状态分别在各自数据目录下；不自动复制正式版插件账号到 UAT。轮换 refresh token 的同一账号不能视为两份独立登录，建议两环境分别登录并避免同时续期。

## 包与传输协议

包根目录 `manifest.json` 示例见 `xt-client/plugins/trae/manifest.json`。必填 ID、名称、版本、contractVersion=1、相对可执行入口和能力列表。必需 chat，可选 login、refresh、models、quota、checkin。入口不得越出包目录。安装拒绝符号链接、非普通文件及大于 256 MiB 的包；当前不支持覆盖安装。插件有当前用户权限，路径检查不构成操作系统沙箱。

每个操作启动独立进程，工作目录为安装目录；完成或取消后回收直接子进程。stdin/stdout 使用 UTF-8、每行一条 JSON-RPC 2.0，不使用 Content-Length，消息上限 32 MiB。stdout 仅用于协议；stderr 当前丢弃，不进入应用日志。插件不得在错误正文或日志输出凭据和对话正文。

初始化请求：

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"contractVersion":1}}
```

15 秒内返回：

```json
{"jsonrpc":"2.0","id":1,"result":{"id":"xiaotao.trae","contractVersion":1}}
```

身份必须匹配 manifest。后续请求的 params 为 Operation：

```json
{"credential":{"opaque":"encrypted-at-rest-only"},"input":{},"body":{"messages":[],"stream":true},"model":"exact-upstream-id"}
```

`credential` 是核心解密后通过管道传递的插件私有 JSON，不经过 HTTP 或前端；`input` 是管理动作输入；body 为标准 Chat 请求；model 是目录明确提供的上游 ID。不要解析 ID 中的斜杠推测平台。

方法及返回约定：

| 方法 | 输入 / 返回 |
| --- | --- |
| `login/import` | input.refreshToken；返回 credential，Trae 会交换令牌 |
| `credentials/import` | 迁移私有 JSON，Trae 仅校验并保存，不联网 |
| `credentials/refresh` | 必须先于聊天/目录/配额执行；无须续期时返回原 credential |
| `models/refresh` | 返回 models 数组；失败不覆盖目录 |
| `quota/get`、`checkin/status`、`checkin/claim` | 返回 value，含平台真实数据 |
| `chat/start` | 非流式返回 value=完整 Chat Completion；流式先通知，再返回空 result |

所有方法返回 Result 对象：可选 credential、models、value。credential 有更新时核心加密并持久化；models 各项包含 key、upstreamId、label、contextWindow、supportsImages、supportsReasoning、reasoningEfforts。key 必须在插件内唯一且重启稳定；核心以插件 ID+key 生成内部模型 ID。协议 DTO 位于 `internal/sourceplugin/contract.go`，不依赖平台 Credential 或 storage DTO。

流式通知方法为 `chat/delta`，params 是标准 Chat chunk 内容（choices、delta、finish_reason、usage）。文本、reasoning_content、tool_calls 增量保留原结构；usage 通知的 choices 可为空。插件只报告真实用量，未知保持未知。核心改写公开 model/id 并转换为 SSE；不重放聊天请求。

插件必须在收到真实上游完成后发送 finish_reason，最后回复对应 chat/start 请求。上游错误用 JSON-RPC error 结束请求。断流、协议损坏、超时或崩溃都进入失败路径。当前一个进程只承载一个动作，取消由核心结束子进程来中断上游，没有单独的 chat/cancel RPC。

## 管理 HTTP 接口

接口遵守客户端同源/管理鉴权，GET 仅返回公开状态；不返回 credential 或 secret。

- `GET /api/source-plugins`：插件状态和目录快照。
- `POST /api/source-plugins/install`：`{"path":"本地包完整目录"}`，只安装。
- `POST /api/source-plugins/{id}/enabled`：`{"enabled":true}`。
- `POST /api/source-plugins/{id}/action`：`{"method":"models/refresh","input":{}}`；允许登录导入、刷新目录、配额及签到动作。
- `GET /api/source-plugins/legacy-trae`：当前环境可迁移账号预览，无凭据。
- `POST /api/source-plugins/legacy-trae`：`{"id":"预览中的旧账号 ID"}`，显式替换当前 Trae 插件账号。

本机协议示例：`cmd/trae-source-plugin`。离线假插件回归：`internal/sourceplugin/manager_test.go`，覆盖安装、稳定模型 ID、凭据不回显、取消、崩溃与重启。开发第三方插件时应先用假上游验证这些异常路径，再验证真实账号。
