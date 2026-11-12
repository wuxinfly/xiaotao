# 模型来源插件开发与 Trae 本地验证

当前实现：本地目录安装、本机插件包发现、启停、独立进程调用、统一模型目录、流式/非流式 Chat、凭据加密、当前环境旧 Trae 账号显式导入、模型发现、配额和签到。Trae 是独立可执行程序，客户端核心不链接平台代码。真实 Trae 账号和 Windows 桌面壳需本地验收。

账号：插件可保存多个账号，授权成功后直接列出；可拖动账号手柄、聚焦手柄后按上下方向键，或点击“设为首选”调整顺序。首位账号用于聊天与模型目录刷新；每行的配额和签到使用该行账号。配额默认显示总额度摘要和已用比例，展开“明细”查看各分项；同名的重复奖励项按名称合并并标注数量。同一插件的操作串行排队，避免令牌轮换覆盖。尚无自动账号故障转移、卸载、在线市场或旧模型引用批量映射。其他平台未接入。

## Windows 本地测试

在 `xt-client` 执行：

```powershell
go test ./internal/sourceplugin ./cmd/trae-source-plugin ./internal/providers/trae
go test ./...
go vet ./...
./plugins/trae/build.ps1
./build.ps1 -Env uat -Verify
```

插件包生成在 `xt-client/dist/plugins/trae`，包含 `manifest.json` 和 `provider.exe`。客户端构建与插件构建分别执行；插件不会随客户端自动安装。客户端会读取可执行文件旁的 `plugins/` 和当前目录的 `dist/plugins/`，在“安装插件”面板列出发现的包，可一键安装或更新，不需要手填完整路径。更新会短暂停用插件，成功后自动恢复启用；失败时保持停用并提示。更新保留账号、模型目录和选择。`-Verify` 不递增版本。

1. 启动 UAT，在“模型接口”底部找到“模型来源插件”。
2. 展开“安装插件”，从“发现本机插件包”选择包并安装；也可以手动填写完整目录路径。安装只复制文件，不启动代码。核对插件名称、版本、ID 和入口后启用。已安装旧版时，卡片和安装面板会提示可更新版本。
3. 点击“添加另一个账号”，Trae 插件会打开系统默认浏览器，完成授权后回到小涛即可看到新账号。同一身份重新授权会更新该账号。当前环境里的旧 Trae 账号直接列出，点击“加入插件”后才会导入凭据，旧文件保持只读。已有插件账号会保留，刷新凭据时会补全账号身份。拖动账号行的手柄，把常用账号移到首位。
4. 点击“刷新目录”，在卡片的“模型”区域查看、搜索或展开完整目录，再到 Agent 设置中选择 Trae 插件模型。目录只在显式刷新时访问上游，刷新失败保留旧快照。现有插件默认开放全部目录模型；勾选后点击“保存模型”可限定 Agent 和网关使用的模型。保存为全不选会关闭此插件的全部模型入口。刷新目录保留按模型键保存的选择，新出现的模型需要手动勾选。也可输入上游模型 ID 后添加并保存；手动模型默认不声明图片、推理和上下文能力，能否调用由上游决定。
5. 测试普通聊天、推理、工具调用、图片、Max 变体、停止生成；测试配额、签到状态和签到。当前账号行展示签到状态，以及查询得到的配额摘要、已用比例和可展开明细；插件操作在 Windows 后台启动，不显示控制台窗口。
6. 外部工具继续使用 `/v1/models` 返回的稳定内部模型 ID 调用 `/v1/chat/completions`，使用本地网关密钥。
7. 停用后模型从可用目录消失，在途请求被取消；旧会话保留，旧 ID 不会自动绑定别的同名模型。

正式版与 UAT 的插件目录、密钥和状态分别在各自数据目录下；不自动复制正式版插件账号到 UAT。轮换 refresh token 的同一账号不能视为两份独立登录，建议两环境分别登录并避免同时续期。

## 包与传输协议

包根目录 `manifest.json` 示例见 `xt-client/plugins/trae/manifest.json`。必填 ID、名称、版本、contractVersion=1、相对可执行入口和能力列表。必需 chat，可选 login、browser-login、refresh、models、quota、checkin。入口不得越出包目录。安装拒绝符号链接、非普通文件及大于 256 MiB 的包；更新必须在停用后显式进行；新版包先复制到独立目录，再写入状态启用新版本路径。插件有当前用户权限，路径检查不构成操作系统沙箱。

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
| `login/browser` | 插件发起交互授权，成功返回 account 与 credential；取消请求会终止插件进程 |
| `login/import` | input.refreshToken；返回 account 与 credential，Trae 会交换令牌 |
| `credentials/import` | 迁移私有 JSON，Trae 仅校验并保存，不联网 |
| `credentials/refresh` | 必须先于聊天/目录/配额执行；无须续期时返回原 credential |
| `models/refresh` | 返回 models 数组；失败不覆盖目录 |
| `quota/get`、`checkin/status`、`checkin/claim` | 返回 value，含平台真实数据 |
| `chat/start` | 非流式返回 value=完整 Chat Completion；流式先通知，再返回空 result |

所有方法返回 Result 对象：可选 credential、models、value。credential 有更新时核心加密并持久化；models 各项包含 key、upstreamId、label、contextWindow、supportsImages、supportsReasoning、reasoningEfforts，可选 priceFactor、isFree 表示上游消耗倍率和免费标记，仅用于展示，不参与计费。key 必须在插件内唯一且重启稳定；核心以插件 ID+key 生成内部模型 ID；网关 `/v1/models` 的公开调用别名为 `<插件调用标识>/<key>`，不带 `plugin/` 前缀。内置 `xiaotao.trae` 默认缩短为 `trae`，其他插件保留完整 ID；短名称与手动来源共用持久化保留记录，若已被手动来源占用（含删除记录）则保留完整插件 ID。中文等可见 Unicode 直接保留，仅分隔符、百分号、空白等字符独立转义。插件内手动模型的 key 仍是 `manual:<上游 ID>`。网关同时接受可读别名、旧完整插件名、中文百分号编码与旧哈希内部 ID，按完整 ID 精确匹配，仅已启用、已勾选的模型可调用。协议 DTO 位于 `internal/sourceplugin/contract.go`，不依赖平台 Credential 或 storage DTO。

流式通知方法为 `chat/delta`，params 是标准 Chat chunk 内容（choices、delta、finish_reason、usage）。文本、reasoning_content、tool_calls 增量保留原结构；usage 通知的 choices 可为空。插件只报告真实用量，未知保持未知。核心改写公开 model/id 并转换为 SSE；不重放聊天请求。

插件必须在收到真实上游完成后发送 finish_reason，最后回复对应 chat/start 请求。上游错误用 JSON-RPC error 结束请求。断流、协议损坏、超时或崩溃都进入失败路径。带凭据的结果可附带 account（稳定平台身份 ID 与显示名称），客户端按身份归并并加密保存。当前一个进程只承载一个动作，取消由核心结束子进程来中断上游，没有单独的 chat/cancel RPC。

## 管理 HTTP 接口

接口遵守客户端同源/管理鉴权，GET 仅返回公开状态；不返回 credential 或 secret。

- `GET /api/source-plugins`：插件状态和目录快照。
- `GET /api/source-plugins/packages`：读取可执行文件旁 `plugins/` 和当前目录 `dist/plugins/` 下有效的本地插件包，返回清单、安装版本和是否可更新；只读 manifest，不执行插件。
- `POST /api/source-plugins/install`：`{"path":"本地包完整目录"}`，只安装。
- `POST /api/source-plugins/update`：选择与已停用插件同 ID 的其他版本，本地更新并保留配置。
- `POST /api/source-plugins/{id}/account-order`：`{"ids":["账号内部 ID"]}`，保存完整顺序。
- `POST /api/source-plugins/{id}/enabled`：`{"enabled":true}`。
- `POST /api/source-plugins/{id}/models`：保存 `selectedKeys` 和 `customIds`，仅向 Agent 和网关开放所选模型；手动条目的键为 `manual:<上游模型 ID>`。
- `POST /api/source-plugins/{id}/action`：`{"method":"models/refresh","input":{}}`；允许浏览器授权、令牌导入、刷新目录、配额及签到动作。可用 accountId 指定账号，省略时使用首位。
- `GET /api/source-plugins/legacy-trae`：当前环境可迁移账号预览，无凭据。
- `POST /api/source-plugins/legacy-trae`：`{"id":"预览中的旧账号 ID"}`，显式将当前环境的旧账号加入 Trae 插件。

本机协议示例：`cmd/trae-source-plugin`。离线假插件回归：`internal/sourceplugin/manager_test.go`，覆盖安装、稳定模型 ID、凭据不回显、取消、崩溃与重启。开发第三方插件时应先用假上游验证这些异常路径，再验证真实账号。

## 可读调用 ID 回归验收（2026-11-12）

- `/v1/models` 对手动来源和插件均返回可读 `id`；同名来源、同上游模型不串路由，中文来源名及模型名直接展示，含斜杠、空格、加号与百分号的 key 独立转义。
- 内置插件默认去掉 `xiaotao.`，覆盖先创建插件/先创建手动来源的同名冲突、删除及重启；短名称不接管旧调用。
- 使用目录 ID 执行流式与非流式 Chat，上游收到原始模型 ID，响应保留调用方使用的 ID；旧内部 ID 继续接受。
- 修改手动来源名称或上游 ID、重启、旧文件升级后别名稳定；删除后重建不会接管旧别名，停用或取消勾选后新旧 ID 均不可用。
- UAT 只读导入正式版旧文件与别名，重新加密后独立保存；损坏或重复别名应拒绝加载且不改写原文件。

自动回归入口：`go test ./internal/modelsource ./internal/sourceplugin ./internal/gateway`。真实账号与 Windows 桌面壳仍需本地验收。

本次自动验证：相关三个包的测试与 `go vet`、Windows 目标全量 `GOOS=windows GOARCH=amd64 go vet ./...`、前端 lint/大小检查/构建、Go 文件大小及规则一致性检查通过。Linux 全量 `go test ./...` 受现有 Windows 专用 desktop/secrets 实现及 runtimeenv 的 Windows 绝对路径断言限制，不能视为全量通过；未执行真实账号或 Windows 桌面验收。现有模型身份规则候选已覆盖不猜来源、不重绑旧引用，无新增候选。

2026-11-12 中文与短插件名修正：中文旧别名映射继续保存，仅规范化公开展示与匹配；新旧形式共用保留记录，不能通过编码差异复用已删除模型的别名。网关先把目录别名转换为内部身份，再精确路由；内置插件短名称保存在 `routes` 的 `plugin:<完整插件 ID>` 项中。UAT 只读导入并合并别名保留记录，冲突时拒绝导入，不覆盖已有路由。
