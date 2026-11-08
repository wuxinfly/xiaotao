# 小涛客户端模块化日志系统

## 一、背景与问题

当前客户端只有一个日志文件 `%LOCALAPPDATA%\XiaoTao\logs\client-YYYY-MM-DD.log`，由
`internal/logging/daily.go` 的 `Daily` 类型按天轮转写入。生产代码里实际只有三处写入：

| 位置 | 记录内容 |
| --- | --- |
| `cmd/xiaotao/main.go` | 顶层启动失败、窗口打开失败回退浏览器 |
| `internal/providers/workbuddy/chat.go` | WorkBuddy 11128 渠道拦截诊断 |

其余全部静默：服务启动与退出、端口占用、HTTP 请求进出、代理选了哪个账号、上游返回什么、
凭据刷新结果、Agent 每一轮运行、工具调用成败、上下文压缩触发、用量落盘、登录态变化。
实际排查时经常出现「日志是空的」或「只有 HUD 窗口那几行」，无法定位问题。

历史文件 `client-2026-11-02.log`（62 KB）曾把 WorkBuddy 完整请求正文写进日志，与 README
中「不记录消息正文或凭据」的承诺冲突，属于需要清理的历史隐患。

## 二、目标与非目标

### 目标

1. 日志按模块分目录存放，故障时可直接定位到相关文件，不必在单一大文件里翻找。
2. 补齐关键链路的埋点，覆盖「一次故障从用户操作到上游响应」的完整路径。
3. 建立 ID 串联机制，使分散在不同模块的日志能还原出同一次请求的完整时间线。
4. 日志内容只含元数据与结构化字段，不含任何正文与凭据。
5. 设定保留期与归档规则，避免日志目录无节制增长。

### 非目标（本轮不做）

- 不做前端 WebView2 `console` 收集。前端报错目前完全不可见，确实是缺口，但需要额外处理
  WebView2 消息通道，留到单独一轮，避免本轮范围失控。
- 不做日志查看界面。日志定位依靠文件系统和文本工具，不为日志去改前端。
- 不做日志上报、远程采集或告警。数据不出本机。
- 不改 `internal/app/web` 生成的静态资源，不改前端任何代码。

## 三、目录结构与保留策略

```
<data-dir>/logs/
  client/client-2026-11-08.log
  proxy/proxy-2026-11-08.log
  agent/agent-2026-11-08.log
  auth/auth-2026-11-08.log
  usage/usage-2026-11-08.log
  archive/                       旧的扁平日志归档
```

- 模块划分为 `client` / `proxy` / `agent` / `auth` / `usage` 五个。渠道差异（workbuddy、
  codex、antigravity、trae、qoder）用 `channel=` 字段区分，不单独开目录，避免文件过碎。
- 保留期 **14 天**。服务启动时扫描各模块子目录，删除文件名日期早于 14 天的日志文件。
- 启动时把 `logs/` 根目录下遗留的扁平 `*.log`（旧版 `client-YYYY-MM-DD.log`）移动到
  `logs/archive/`，避免与新目录结构混在一起。归档文件同样受保留期约束。
- 单模块单日文件体积预期在数十 KB 量级，无需压缩或分片。

## 四、`internal/logging` 包设计

### 4.1 包依赖约束

`internal/logging` 是叶子包，只依赖标准库，不得导入任何其他 `internal/*` 包。
其余包（`app`、`proxy`、`agent`、`authsessions`、`usage`、`providers`）都可以依赖它，
不存在循环依赖。

### 4.2 类型与 API

```go
package logging

type Level int

const (
    LevelDebug Level = iota
    LevelInfo
    LevelWarn
    LevelError
)

func ParseLevel(string) (Level, error)
func (l Level) String() string

type Module string

const (
    ModuleClient Module = "client"
    ModuleProxy  Module = "proxy"
    ModuleAgent  Module = "agent"
    ModuleAuth   Module = "auth"
    ModuleUsage  Module = "usage"
)

// Set 持有全部模块的 logger，按模块分文件写入。
type Set struct { /* ... */ }

// Open 创建日志目录并打开各模块当日文件。level 为全局最低记录级别。
func Open(dir string, level Level) (*Set, error)

// Close 关闭所有模块的文件句柄。
func (s *Set) Close() error

// For 返回指定模块的 logger。模块名非法时回退到 ModuleClient。
func (s *Set) For(m Module) *Logger

// Dir 返回日志根目录。
func (s *Set) Dir() string

// Logger 写入单个模块的日志文件。
type Logger struct { /* ... */ }

func (l *Logger) Debug(msg string, fields ...Field)
func (l *Logger) Info(msg string, fields ...Field)
func (l *Logger) Warn(msg string, fields ...Field)
func (l *Logger) Error(msg string, fields ...Field)

// Log 按显式级别写入，级别低于 logger 阈值时直接丢弃。
func (l *Logger) Log(level Level, msg string, fields ...Field)

// Path 返回该模块当前写入的文件路径。
func (l *Logger) Path() string
```

### 4.3 默认实例

`providers` 等深层包拿不到 `*Set`，因此包级提供默认实例：

```go
// SetDefault 设置进程级默认 Set，可为 nil（此时所有写入被丢弃）。
func SetDefault(s *Set)

// Default 返回当前默认 Set，可能为 nil。
func Default() *Set
```

`Default()` 为 nil 时，`For` 必须返回一个丢弃所有写入的 no-op logger，不允许 nil 解引用。
这样 `providers/workbuddy` 之类深层包可以直接写：

```go
logging.Default().For(logging.ModuleProxy).Warn("channel rejected", ...)
```

`cmd/xiaotao/main.go` 在 `Open` 成功后调用 `logging.SetDefault(set)`，进程退出时
`defer set.Close()` 并 `logging.SetDefault(nil)`。

### 4.4 字段与格式

```go
type Field struct{ Key, Value string }

// F 构造一个字段。value 支持 string、int、int64、float64、bool、error。
// error 取 Error()；nil 值转为空字符串；其余类型用 %v。
func F(key string, value any) Field
```

日志行格式（单行，行尾换行）：

```
<时间> <级别> <模块> <key=value>... <自由文本>
```

具体规则：

- 时间格式 `2006/01/02 15:04:05.000`，本地时间。
- 级别固定四字符：`DEBUG`、`INFO`、`WARN`、`ERROR`。
- 模块取 `Module` 常量值。
- 字段顺序即调用顺序。同名 key 重复出现时合并为一个，值用 `,` 连接。
- 值的字符串化结果若包含空格、`=`、双引号或换行，用 `strconv.Quote` 包裹；其余原样输出。
- 自由文本即 `msg` 参数，排在字段之后；若含换行，替换为 `\n` 字面量。

示例：

```text
2026/11/08 11:35:12.345 INFO proxy req=chatcmpl-3f2a session=run_9a1b channel=workbuddy model=glm-5.3-flash status=200 ttft_ms=812 upstream=ok
2026/11/08 11:35:14.001 WARN proxy req=chatcmpl-3f2a channel=workbuddy status=400 upstream_reject=11128 shape=count=2 first20=[system:text:known_identity=false,user:text:known_identity=false]
```

### 4.5 级别控制

- 默认级别 `LevelInfo`。DEBUG 默认不写。
- 通过环境变量 `XIAOTAO_LOG_LEVEL` 临时开启，取值 `debug` / `info` / `warn` / `error`，
  大小写不敏感，非法值回退到 `info` 并在 client 日志里记一条 WARN。
- 只提供环境变量这一种入口，不加配置项、不加 UI 开关。理由：本地文件成本极低，最需要避免的
  是「故障当天发现级别没开所以看不到」；真正需要深挖时用环境变量临时开即可。

### 4.6 脱敏

```go
// Redact 遮蔽字符串中疑似凭据的内容。命中 token、jwt、key、password、secret、bearer、
// authorization、cookie 中的任意一个（大小写不敏感）时，截断到命中位置并追加
// "<key>=<redacted>"；超过 maxLen 的部分截断并追加 "..."。
func Redact(s string, maxLen int) string
```

`internal/proxy` 现有的 `redact` 与 `internal/usage` 现有的 `sanitizeError` 统一改为调用
`logging.Redact`，删除各自的重复实现。`logging.Redact` 的遮蔽词表取两者并集。

### 4.7 上下文 ID

```go
// WithReqID 把请求 ID 写入 context。
func WithReqID(ctx context.Context, id string) context.Context

// ReqID 从 context 读取请求 ID，没有时返回空字符串。
func ReqID(ctx context.Context) string
```

### 4.8 保留期与归档

```go
// Cleanup 删除各模块子目录中日期早于 keepDays 的日志文件，并把 logs 根目录下的
// 扁平 *.log 移入 archive 子目录。返回被删除和被归档的文件数。keepDays <= 0 时不清理。
func Cleanup(dir string, keepDays int) (removed int, archived int, err error)
```

文件名日期从 `client-2026-11-08.log` 这类名字里解析，解析失败的旧文件按修改时间判断。
清理在服务启动时执行一次，失败只记 WARN 不阻断启动。

### 4.9 文件组织

`internal/logging` 现有 `daily.go` 与 `daily_test.go` 删除，替换为：

| 文件 | 职责 | 预估行数 |
| --- | --- | --- |
| `logging.go` | `Level`、`Module`、`Set`、`Logger`、写入与格式化 | ≤ 260 |
| `fields.go` | `Field`、`F`、值字符串化与转义 | ≤ 90 |
| `redact.go` | `Redact` 脱敏 | ≤ 70 |
| `context.go` | `WithReqID` / `ReqID` | ≤ 40 |
| `retain.go` | `Cleanup` 清理与归档 | ≤ 110 |
| `logging_test.go` | 对应单测 | ≤ 320 |

所有文件必须在 600 行预警线以下。

## 五、串联机制

三个 ID 串起整条链路：

| ID | 产生位置 | 串联范围 |
| --- | --- | --- |
| `req_id` | HTTP 层，每个请求生成 `req_` + 8 位十六进制 | HTTP 访问日志 ↔ 代理日志 |
| `chatcmpl-xxx` | 代理层 `HandleChat`（沿用现有 `randHex()` 生成方式） | 代理日志 ↔ 用量落盘 |
| `run_xxx` / `session_id` | Agent 层（沿用现有 `turn_` / `run_` ID 体系） | Agent 轮次 ↔ 代理请求 |

传递方式：

1. `internal/app` 的 HTTP 中间件为每个请求生成 `req_id`，通过
   `logging.WithReqID(r.Context(), id)` 注入 context。代理层用 `logging.ReqID(ctx)` 读取。
2. Agent 调用本地代理 `/v1/chat/completions` 时，在请求头附加：
   - `X-Xiaotao-Session: <sessionID>`
   - `X-Xiaotao-Run: <runID>`
   代理层读取这两个头并写入日志字段 `session`、`run`。
3. 代理层内部沿用现有 `chatcmpl-` 前缀的请求 ID，作为 `req` 字段写入所有相关日志行。

效果：检索 `run_9a1b` 可在 `agent/agent-*.log` 找到该轮调用了几次模型；拿到对应的
`chatcmpl-xxx` 后可在 `proxy/proxy-*.log` 找到渠道、账号、上游状态与耗时；`usage/usage-*.log`
可确认 token 是否落盘。

## 六、安全红线（不得违反）

日志只允许记录元数据与结构化字段。以下内容**一律不得写入**：

- 用户输入文本、模型输出正文、系统提示词内容。
- 完整请求体或响应体（含 `messages`、`content`、`arguments`、工具输出）。
- 任何 token、access token、refresh token、API key、cookie、Authorization 头、凭据 JSON。
- 文件绝对路径内容之外的敏感信息（路径本身可记，用于定位操作对象）。

允许记录的内容举例：模型 ID、渠道名、账号 ID（不含凭据）、工具名、HTTP 状态码、耗时、
token 计数、消息条数、工具数量、输出字节数、压缩前后 token 数、结果是否成功。

WorkBuddy 的 11128 诊断保留现有的 `requestShape` 结构化摘要（角色、内容类型、是否包含
已知身份句），**不得**改为记录消息正文。现有实现已经是合规的，只需把写入位置从全局
`log.Printf` 迁到 `proxy` 模块日志。

## 七、埋点清单

下列每一项都要求落成具体的 `logging` 调用。括号内是建议的字段。

### 7.1 client 模块

`cmd/xiaotao/main.go`：

- 进程启动（`env` 环境名、`data_dir`、`pid`、命令行参数）。
- 服务已在运行，本次直接返回（`mode=service`）。
- 服务启动成功（`url` 实际地址、`port`）。
- 服务启动失败与启动超时（`error`）。
- 桌面窗口打开失败回退浏览器（`error`）。
- 启动子进程失败（`args`）。

`internal/app/app.go`：

- 组装阶段：`storage.Open`、`secrets.Open`、`importModelAccounts` 的结果。
- `Serve`：监听端口、实际端口（端口回退到随机端口时明确记录）、`runtime.json` 写入、
  服务退出原因（正常退出 / 上下文取消 / 错误）。
- HTTP 访问日志（`method`、`path`、`status`、`duration_ms`、`bytes`、`req`）：
  覆盖全部路由，包括 `/v1/chat/completions`、`/v1/models`、`/api/**` 与静态资源。
  静态资源刷屏，按路径前缀聚合计数即可，不必逐条记录。
- 同源校验拒绝（`path`、`reason`）。
- `/api/shutdown` 处理。

### 7.2 proxy 模块

`internal/proxy/service.go`（`HandleChat`）：

- 请求入口（`req`、`model`、`stream`、`messages` 条数、`tools` 数量、`session`、`run`）。
- 鉴权失败（`status=401`）。
- 请求体或参数解析失败（`status=400`、`reason`）。
- 缺少 model。
- 账号选择结果（`provider`、`account`、候选数量）。
- 凭据读取失败、凭据 JSON 损坏。
- 凭据即将过期触发刷新：刷新前后结果（`refresh=ok|failed`）。
- 上游调用开始与结束（`status`、`duration_ms`、`ttft_ms` 首 token 延迟）。
- 上游错误（`status`、`error`，经 `logging.Redact` 处理）。

`internal/proxy/routing.go`：

- 账号被跳过的原因，逐条记录（`model`、`provider`、`account`、`reason`）。原因取值：
  `disabled`（账号禁用）、`model_unsupported`（不支持该模型）、`quota_exhausted`（额度用尽）、
  `cooling`（限流冷却中）。
- 改为新增 `accountCandidatesWithReasons(model) ([]storage.Account, []SkipReason)` 返回跳过原因，
  原有 `accountCandidates` 保留签名并包一层，避免影响 `hasCoolingAccount` 等调用点。
- 冷却设置与到期（`account`、`until`、`quota`）。

`internal/proxy/antigravity_failover.go`：

- 主备降级决策（`attempt`、`account`、`error`、`retryable`、`action`）。`action` 取
  `switch`（换下一个账号）、`cool`（进入冷却）、`abort`（不再重试）。

`internal/proxy/catalog.go`：

- 模型目录刷新结果（`provider`、`account`、`source`、`count`、`duration_ms`、失败原因）。

`internal/proxy/qoder.go`：

- Qoder 分支的模型解析失败、区域无效、上游调用结果与错误。

`internal/providers/workbuddy/chat.go`：

- `logChannelRejection` 从全局 `log.Printf` 改为写入 `proxy` 模块（`upstream_reject=11128`、
  `shape`）。删除该文件的 `log` 导入。

### 7.3 agent 模块

`internal/agent/service.go`：

- `send`：会话开始（`session`、`run`、`model`、`history` 消息数、`context_window`）。
- `send`：会话结束（`session`、`run`、`status`、`duration_ms`、输出 token、工具调用数）。
- 每轮模型请求开始（`session`、`run`、`attempt`、`context_tokens`、`tools` 数量）。
- 每轮模型请求结束（`ttft_ms`、`duration_ms`、输出 token、是否有工具调用）。
- 工具调用（`session`、`run`、`tool` 名字、`status` 成败、`duration_ms`、`output_bytes`）。
  **不记录工具参数与工具输出正文**，只记名字、成败、耗时与字节数。
- 用户取消与任务中断（`session`、`run`、`reason`）。

`internal/agent/context_summary.go`：

- 上下文压缩触发（`session`、`before_tokens`、`after_tokens`、`threshold`、保留轮数）。
- 压缩摘要生成失败降级（`session`、`error`）。
- 埋点位置：`shouldAutoCompact` 判定为真时、以及 `tryAutoCompact` 返回错误时。

`internal/agent/context_prune.go`：

- 工具结果裁剪发生的次数与裁剪掉的字符总量（按会话汇总一条，不逐条记录）。
- 埋点位置：`pruneToolResult` 发生实际裁剪时累计计数。

### 7.4 auth 模块

`internal/authsessions/sessions.go`：

- 登录开始（`provider`、`region`、`session`）。
- 登录成功（`provider`、`session`、`duration_ms`）。
- 登录失败、超时、取消（`provider`、`session`、`error`）。
- 同渠道同区域的旧等待会话被取消（`provider`、`old_session`）。
- Codex 回调校验失败、state 不匹配。

`internal/proxy/account.go`：

- 凭据解密失败、凭据内容非法（`account`、`error`）。
- 配额刷新开始与结果（`account`、`provider`、`status`、耗时）。
- 签到状态查询与领取结果（`account`、`provider`、`checked`、`streak`）。
- 对应函数：`decodeCred`、`RefreshQuota`、`FetchCheckin`、`ClaimCheckin`。

### 7.5 usage 模块

`internal/usage/service.go`：

- 请求开始（`req`、`channel`、`model`、`account`）。
- 请求结束（`req`、`status`、`duration_ms`、prompt / completion / cached token）。
- 用量落盘成功与失败（`req`、`error`）。
- 数据库降级状态变化（`status`、`message`）。
- 每日清理与快照失败。
- 对应函数：`StartRequest`、`ActiveRequest.finish`、`enqueueWrite`、`writeWorker`。

## 八、代码组织与改动清单

### 新增文件

```text
xt-client/internal/logging/logging.go
xt-client/internal/logging/logging_test.go
xt-client/internal/logging/fields.go
xt-client/internal/logging/redact.go
xt-client/internal/logging/context.go
xt-client/internal/logging/retain.go
xt-client/internal/proxy/logging.go        代理埋点助手与跳过原因类型
xt-client/internal/agent/logging.go        Agent 埋点助手
```

### 删除文件

```text
xt-client/internal/logging/daily.go
xt-client/internal/logging/daily_test.go
```

### 修改文件

| 文件 | 改动 |
| --- | --- |
| `cmd/xiaotao/main.go` | 用 `logging.Open` + `SetDefault` 替换 `logging.Daily`；解析 `XIAOTAO_LOG_LEVEL`；启动时调用 `logging.Cleanup`；补启动类埋点 |
| `internal/app/app.go` | HTTP 访问日志中间件与 `req_id` 注入；服务生命周期埋点 |
| `internal/proxy/service.go` | 代理链路埋点（尽量把逻辑放进新建的 `internal/proxy/logging.go`） |
| `internal/proxy/routing.go` | 新增跳过原因收集 |
| `internal/proxy/antigravity_failover.go` | 降级决策埋点 |
| `internal/proxy/catalog.go` | 目录刷新埋点 |
| `internal/proxy/qoder.go` | Qoder 分支埋点 |
| `internal/proxy/account.go` | 配额、签到、凭据埋点 |
| `internal/proxy/service.go` | `redact` 改为调用 `logging.Redact`，删除本文件内的重复实现 |
| `internal/providers/workbuddy/chat.go` | 11128 诊断迁入 proxy 模块日志 |
| `internal/agent/service.go` | Agent 生命周期与工具调用埋点 |
| `internal/agent/context_summary.go` | 压缩埋点 |
| `internal/agent/context_prune.go` | 裁剪统计埋点 |
| `internal/authsessions/sessions.go` | 登录埋点 |
| `internal/usage/service.go` | 用量埋点，`sanitizeError` 改为调用 `logging.Redact` |

### 代码规模约束

以下文件是超限存量文件，按 `docs/agent/client-code-quality.md` 的要求，**修改后不得继续变大**：

| 文件 | 当前行数 | 基线上限 |
| --- | ---: | ---: |
| `xt-client/internal/app/app.go` | 944 | 961 |
| `xt-client/internal/proxy/service.go` | 1024 | 1079 |
| `xt-client/internal/agent/service.go` | 1144 | 1207 |
| `xt-client/internal/usage/service.go` | 744 | 850 |

约束：埋点的组装逻辑放进对应包新建的 `logging.go` 助手文件，存量文件里只保留少量调用行。
所有新增文件不超过 600 行，新增函数不超过 80 行。

## 九、测试要求

`xt-client/internal/logging/logging_test.go` 至少覆盖：

1. 按天切换文件：跨天后写入落到新的 `client-YYYY-MM-DD.log`，前一天的完整保留。
2. 模块分目录：五个模块写入各自的子目录，文件名带正确模块前缀。
3. 级别过滤：默认不写 DEBUG；`LevelDebug` 下写入；低于阈值的级别被丢弃。
4. `ParseLevel`：合法值、大小写、非法值回退。
5. 字段格式化：含空格 / `=` / 引号 / 换行的值被正确引用；同名 key 合并。
6. `Redact`：命中各敏感词时截断并遮蔽；超长文本截断；未命中时原样返回（受 maxLen 限制）。
7. `Cleanup`：删除过期文件、保留未过期文件、扁平日志被归档到 `archive/`、解析不出日期的
   文件按修改时间处理。
8. `WithReqID` / `ReqID`：context 往返；无值时返回空字符串。
9. 默认实例为 nil 时 `For` 返回的 logger 不 panic 且丢弃写入。
10. 同一天多次打开同一日志目录时追加而非覆盖（沿用现有 `daily_test.go` 的行为预期）。

其余包按现有惯例补最小回归：

- `internal/proxy`：`accountCandidatesWithReasons` 的跳过原因判定（禁用、模型不支持、
  额度用尽、冷却中各一例）。
- `internal/agent`：工具调用埋点不泄露参数与输出正文（断言日志内容不含参数正文）。

## 十、验证命令

在 `xt-client` 目录：

```powershell
gofmt -l ./cmd ./internal ./tools
go vet ./...
go test ./...
node tools/check-go-size.mjs
```

在 `xt-client/frontend` 目录（本轮不改前端，仅确认未破坏）：

```powershell
npm run lint
npm run check:size
```

在仓库根目录：

```powershell
node tools/agent-rules.mjs check
```

## 十一、文档同步

文档同步属于需求完成条件，必须与代码同步落地：

1. `xt-client/README.md` 第 201 行附近描述日志的段落：改写为新的目录结构、五个模块、
   保留 14 天、级别环境变量、字段串联方式、以及「只记元数据不记正文」的约定。
2. `docs/architecture/xt-client-overview.md` 中涉及日志与数据目录的描述同步更新。
3. 本文件即设计文档，已落在 `docs/plans/`。
4. `docs/agent/rules.json` 中是否新增「客户端日志按模块分目录、只记元数据不记正文」的规则：
   属于「约定」类规则，按 `docs/agent/rules.json` 的策略需人工确认，**本轮不擅自写入**。
   如需纳入规则库，先写候选到 `docs/agent/candidates/`，再由用户确认后应用。

## 十二、验收标准

1. `logs/` 下出现 `client/`、`proxy/`、`agent/`、`auth/`、`usage/` 五个子目录，各自写入当日文件。
2. 一次完整的 Agent 对话（发起模型请求、返回工具调用、执行工具、再请求模型）结束后，
   能在 `agent/agent-*.log` 找到该 `run` 的完整轮次记录，在 `proxy/proxy-*.log` 找到对应
   `chatcmpl-` 请求的渠道、账号与上游状态。
3. 模拟一次上游失败（如渠道拒绝），能在 `proxy/proxy-*.log` 定位到状态码与错误摘要，
   且日志中不含任何请求正文。
4. 服务启动后，访问任一 API，HTTP 访问日志中能查到方法、路径、状态码、耗时与 `req_id`。
5. `XIAOTAO_LOG_LEVEL=debug` 启动时出现 DEBUG 行；不带该变量时不出现。
6. 手工把某个模块日志文件的日期改到 20 天前，重启后该文件被删除，未过期文件保留。
7. 旧的扁平 `client-YYYY-MM-DD.log` 在首次启动后出现在 `logs/archive/` 下。
8. `go vet ./...`、`go test ./...`、`node tools/check-go-size.mjs`、
   `node tools/agent-rules.mjs check` 全部通过。

## 十三、风险与注意事项

- **工作区已有未提交改动**：`internal/agent/service.go`、`internal/agent/types.go`、
  `internal/app/agent_models.go`、`internal/providers/registry.go`、
  `internal/providers/trae/models.go`、`internal/proxy/service.go` 存在 reasoning effort
  相关的未提交改动，另有未跟踪目录 `internal/effort/`。实施时只叠加日志改动，不修改或
  回退这些逻辑，也不要用 `git checkout` / `git reset` 之类命令覆盖它们。
- **日志写入不能拖慢主链路**：写日志是同步 IO，字段组装要轻量，禁止在日志调用里做
  JSON 序列化大对象或字符串拼接大文本。
- **文件句柄**：每个模块一个当前文件句柄，跨天轮转时关闭旧句柄。进程退出前必须关闭，
  否则日志尾部可能丢失。
- **并发安全**：同一模块可能被多 goroutine 写入，logger 内部必须加锁。
- **不改变现有协议与产品行为**：日志改造不得改变 HTTP 响应体、代理路由、账号选择逻辑、
  用量统计口径与 Agent 行为。

## 十四、后续可做（不在本轮范围）

- 前端 WebView2 `console.error` / `console.warn` 收集到 `client` 模块日志。
- 日志内容检索工具或「导出诊断包」按钮，一次打包五个模块日志与脱敏后的配置。
- 结构化 JSON 日志格式，便于后续做日志分析。
