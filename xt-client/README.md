# XiaoTao 客户端

## Codex 接入（2026-11-01）

账号与配额 → 添加账号 → 选择 Codex（ChatGPT）后，在浏览器完成 OAuth 2.0 PKCE 授权，也可以通过“导入 Codex 登录态”直接粘贴本机 ~/.codex/auth.json 凭据。授权默认监听本地 http://localhost:1455/auth/callback，端口冲突或未自动跳转时支持手动粘贴回调完整 URL 或 code 参数。账号凭据经 Windows DPAPI 加密保存，刷新自动处理 refresh token 轮换。

模型支持静态底表与账号目录实时拉取（GET /models?client_version=0.159.0），默认包含 gpt-6.1-sol、gpt-6-astra、gpt-6-sol、gpt-6-luna、gpt-5.6-sol 等主力模型，调用 ID 带 codex/ 前缀（如 codex/gpt-6.1-sol）。

已实现 OpenAI Chat Completions 双向投影（Chat 请求转 Responses 协议，Responses SSE 增量折叠为 Chat 流式与非流式结果），支持 instructions / system 拼接、图片输入、工具调用、reasoning 内容折叠及真实 token usage 统计。额度查询接入真实 /backend-api/wham/usage 接口，按 5小时/周窗口展示百分比额度与重置时间。

详细记录见 [Codex 实施计划](../docs/plans/2026-11-01-codex-proxy.md)。

## Qoder 接入（2026-11-01）

账号与配额 → 添加账号 → 选择 Qoder 国内版或海外版后，在浏览器完成设备授权。授权窗口为 10 分钟，可取消、复制链接或重新打开；账号详情支持按原区域重新登录。同 UID 在两区分别保存，凭据仍由 Windows DPAPI 加密。设备 token 刷新由聊天、模型目录和额度共用按账号加锁的入口，轮换结果不会覆盖重新登录或已删除账号。

模型从该账号上游目录获取，缓存 5 分钟；“刷新列表”强制刷新。失败时显示上次目录与错误，无历史目录则显示不可用，不用静态模型冒充账号权益。外部工具使用完整的 `qoder/<模型名>`（如目录中的 `qoder/qwen3.8-flash`），未知新模型的公开 ID 带稳定 key 摘要以避免重名。

已实现 OpenAI Chat Completions 的流式/非流式、reasoning、工具参数分片、图片内容透传、真实 token usage；上游 HTTP 或带内错误会失败，未收到完成标记的断流不会伪装成功。连接上限 15 秒、聊天空闲上限 300 秒、总时长 30 分钟，不自动重放、换号或排队重试。订阅、加购、活动资源包分别显示，标题合计已知积分；独立到期日期显示为“到期”，缺失数值保留未知。

验证边界：当前通过本地 mock、Go 回归、前端和 Windows EXE 构建；CN / Global 真实账号授权、模型权益、聊天与额度尚未实测，桌面壳交互仍待验收。自动签到、PAT/本机凭据导入、Responses/Anthropic 入口不在本期范围。详细记录见 [Qoder 实施计划](../docs/plans/2026-11-01-qoder-proxy.md)。

已实现 Windows 客户端：Go 独立本地服务、嵌入式 Web 页面、系统 WebView2 窗口及浏览器入口。模型代理已接入 Trae 国内版、WorkBuddy（国内版）与 Antigravity（Google OAuth）账号反代。小涛 Agent 已具备单任务、单项目的首版执行闭环：选择项目和模型后，可以对话、查看项目文件、修改代码、运行验证并停止任务。

产品范围、分发更新规则和待定事项见[客户端总体方案](../docs/architecture/xt-client-overview.md)。

## 构建与验证

当前验证环境为 Windows amd64、Go 1.27、Node.js 22。开发构建需要 Node.js，最终用户无需 Node.js 或 Python。前端采用 React + TypeScript + Vite，源码位于 frontend，构建产物嵌入 Go EXE。

```powershell
cd xt-client
cd frontend
npm ci
npm run build
cd ..
go test ./...
go vet ./...
./build.ps1
./dist/xiaotao.exe
```

### 不重启服务调试前端

当正式版或 UAT 正在运行、又只想调整页面样式或交互时，不要让 `build.ps1` 或 `--stop` 打断当前实例。另起 Vite 开发服务器，把 `/api` 代理到正在运行的服务：

```powershell
$runtime = Get-Content -Raw "$env:LOCALAPPDATA\XiaoTao\runtime.json" | ConvertFrom-Json
cd xt-client/frontend
$env:XIAOTAO_DEV_API = $runtime.URL
npm run dev -- --host 127.0.0.1 --port 5460 --strictPort
```

打开 `http://127.0.0.1:5460/` 即可用真实账号、配额和仪表盘数据预览最新前端代码，Vite 修改后热更新。UAT 将 runtime 路径换成 `$env:LOCALAPPDATA\XiaoTao-uat\runtime.json`，并另选预览端口；不要与 5455/5456 冲突。

调试期间注意：

- 只在 Vite 代理内访问运行中的服务，`XIAOTAO_DEV_API` 必须是本机 `127.0.0.1` 地址；代理会把请求的 `Host` 和 `Origin` 改写为服务自身地址，以满足服务的同源校验。
- 预览页面的增删账号、刷新配额、轮换密钥等按钮会真实修改当前实例数据。只调样式时避免点击这些写操作。
- 不把 `XIAOTAO_DEV_API` 或临时代理配置提交到构建产物。默认未设置该变量时，开发服务器不启用代理，正式构建行为不变。
- 验证完成后关闭 Vite 进程并清理环境变量；需要让桌面窗口加载新前端时，再选择合适时机构建和重启，不能中断用户正在使用的服务。

正式构建和 UAT 构建使用同一份源码，但环境身份在编译时注入，互不共用数据目录、端口和单实例。UAT 首次启动时会从正式版只读导入一次已登录的模型账号；代理密钥、用量、配额、配置、Agent 会话、日志和运行时状态仍分别保存：

| 构建 | 输出 | 默认数据目录 | 默认端口 | 桌面快捷方式 |
| --- | --- | --- | --- | --- |
| `./build.ps1` | `dist/xiaotao.exe` | `%LOCALAPPDATA%\XiaoTao` | `5455` | 小涛 |
| `./build.ps1 -Env uat` | `dist/xiaotao-uat.exe` | `%LOCALAPPDATA%\XiaoTao-uat` | `5456` | 小涛 UAT |

固定端口被占用时服务仍会回退到系统分配的空闲端口，实际地址以 `runtime.json` 或 `--status` 为准。`--data-dir` 显式传入时仍以参数为准。

UAT 的模型账号导入只执行一次，导入结果保存在 `%LOCALAPPDATA%\XiaoTao-uat\accounts\accounts.json`，完成标记为同目录的 `import.json`。正式版文件只读取、不写入；之后在 UAT 增删或重新登录账号都不会影响正式版。需要重新导入时，先关闭 UAT，再删除 UAT 的 `accounts` 和 `keys` 目录；不要删除正式版数据。会轮换 refresh token 的来源（如 Codex、Qoder）在正式版和 UAT 同时刷新时可能使另一侧登录态失效，双开测试时优先使用不会轮换的来源，或在一侧测试结束后重新登录。

双击 dist/xiaotao.exe 自动启动隐藏服务进程并打开桌面窗口；未安装 WebView2 时尝试用默认浏览器打开。不安装运行时，不主动写入注册表。当前 EXE 是运行程序，尚不是带版本检查的安装启动器。

build.ps1 构建成功后会在当前用户桌面创建或更新“小涛”快捷方式，指向本仓库 dist/xiaotao.exe；移动 EXE 后需重新构建更新快捷方式。桌面窗口新建时由原生窗口通过 GDI 帧流渲染 startup.mp4 动画作为启动封面，同时后台静默启动服务并初始化主工作区；后台流程跑完且动画播完后自动平滑切换为主界面，无外部依赖、无安全下载弹窗、无跳过按钮，彻底杜绝了 Webview 初始化阶段的白屏现象。托盘恢复、页面刷新及浏览器入口不显示启动动画。

## 使用与生命周期

- 默认数据目录：正式版为 LOCALAPPDATA/XiaoTao，UAT 版为 LOCALAPPDATA/XiaoTao-uat；使用 --data-dir 指定其他目录。UAT 仅首次启动时从正式版导入模型账号登录态，其他数据不共享。
- --browser：启动或复用服务，在默认浏览器打开页面。
- --service：只运行服务，主要供启动入口内部调用。
- --status：输出运行服务地址；GUI 构建不便查看标准输出时，可读取数据目录中的 runtime.json，或使用 go run ./cmd/xiaotao --status。
- --stop：停止该数据目录对应的服务。
- 自绘标题栏提供拖动、最小化、最大化和关闭。关闭收起到系统托盘，服务继续运行。
- 最小化左侧的“更多”按钮支持悬停或点击展开菜单，可用资源管理器打开安装目录（当前 EXE 所在目录）和当前实例的数据目录（遵循 --data-dir）；支持方向键选择及 Escape 关闭，仅桌面窗口显示。
- 托盘双击恢复窗口，右键提供打开小涛、在浏览器打开、退出小涛。--stop 同样停止服务及窗口。
- 同一数据目录仅一个服务和一个桌面窗口；正式版和 UAT 版使用不同数据目录与锁，可同时运行；已有窗口时再次打开尝试恢复窗口。
- 仅监听 127.0.0.1 回环地址，不向局域网提供服务；正式版优先使用 5455，UAT 版优先使用 5456，端口占用时自动回退到空闲端口。

## 模型用量与仪表盘

在“账号与配额”的各接入来源旁点击“查看模型”，可切换账号查看模型名称与完整调用 ID、复制 ID 或刷新目录。列表区分实时、内置、合并与上次目录。Trae 兼容对象/字符串元数据并过滤内部条目，保留上游原始名称。Antigravity 实时发现公开对话模型并合并已验证底表，动态模型携带档位路由；暂不收录生图模型。刷新失败保留上次账号目录，无历史快照时回退内置表。WorkBuddy 使用内置目录。目录不保证当前账号已获模型权限，实际调用仍取决于来源匹配、权限和额度。页面通过同源 `POST /api/proxy/models?accountId=<id>` 获取指定账号目录，不影响代理路由。

模型目录：页面刷新与 `/v1/models` 按启用账号聚合，同一来源模型 ID 去重。Trae 优先读取实时目录，过滤关闭、隐藏、自定义和 builder 条目，成功后不再混入静态表；请求最多 10 秒，失败或空目录优先沿用账号上次成功目录，无快照时回退静态表。WorkBuddy 静态表已按 buddy-proxy 的 2026-11-01 配置同步为 23 项，含 `workbuddy/mimo-v2.6-pro`，其真实账号调用仍待验收。

客户端内置基于真实代理流量的用量统计模块（`internal/usage`），采用纯 Go `modernc.org/sqlite` 实现无 CGO 依赖的本地持久化。

- **数据库位置**：位于数据目录下的 `data/usage.db`（跟随 `--data-dir`，默认 `%LOCALAPPDATA%\XiaoTao\data\usage.db`）。开启 WAL 模式、串行写入及有界批量提交。
- **保留策略**：
  - 请求明细表（`request_records`）：保留最近 7 天滚动周期内已结束的请求记录，启动及每日自动清理一次；不记录提示词、回复正文与用户凭据。
  - 每日用量汇总表（`daily_usage`）：长期保存，每日一条记录，永不滚动清理。
  - 重启恢复：启动时自动扫描此前未终结（`pending`/`streaming`）的遗留记录并标记为“中断”，保留已确认的用量数据。
- **统计口径**：
  - 统计开始日期：首次启动时在 `usage_meta` 表记录 `tracking_started_date`，用于在年度热力图中区分“未开启统计的历史”与“当天零用量”。
  - Token 计算：总 Token = 输入 Token + 输出 Token。缓存命中包含在输入 Token 中，不重复累加。
  - 缺失与未知：上游未提供或无法解析的指标以空值（—）展示，不伪装或估算为零；缺失情况在每日汇总中单独计数。
  - 日界线：固定采用 `Asia/Shanghai` 时区。请求数归入发起时的开始日期，用量增量归入收到上游确认数据时的日期（跨午夜请求按此规则精确切分）。
  - 输出速率：已确认输出 token ÷ 首输出后经过时间的平均值（tok/s），流结束后冻结；不足 50ms 或无输出 token 时显示“—”。
- **管理接口与推送**：
  - `GET /api/dashboard`：返回版本信息、启动用量、今日与累计汇总、最近三条请求、进行中列表及年度热力图。
  - `GET /api/dashboard/events`：SSE 增量实时推送，建立连接时发送完整快照，后续最多每秒 4 次合并推送，提供 15 秒心跳。断线重连重新获取完整状态，通过版本号避免旧数据覆盖。离开页面自动断开。
- **已知限制**：
  - 第一版不包含全量请求历史查询页，仅展示最近三条请求及超过三条时的进行中展开列表。
  - 依赖上游模型接口返回的真实 token 数据，若上游仅在流结束时返回 usage，则本次启动和今日用量在流结束时更新。

config.json 保留底层基础配置接口，当前页面不提供配置表单；runtime.json 保存临时地址、进程号和管理令牌（不要分享该文件），正常退出会删除；运行日志按本地日期写入 `logs` 目录下的 `client-YYYY-MM-DD.log`，跨天自动换新文件，同一天多次启动继续追加，目前只记录启动错误和窗口回退浏览器；logs 目录跟随 `--data-dir`，旧版数据目录下的 `client.log` 已不再写入。webview 目录保存窗口浏览器数据。账号元数据保存在 accounts/accounts.json，上游凭据经 Windows DPAPI 保护后存放在 accounts/keys 下。

## 已验证与限制

接口测试覆盖配置保存与重载、损坏配置保留、跨源写入与非法 Host 拒绝、页面资源及未带代理密钥的模型接口拒绝。密钥环测试覆盖 DPAPI 保护密钥的跨实例恢复。

Windows 实际进程验证通过：服务启动、重复服务拒绝、配置更新、运行文件清理、WebView2 窗口打开、关窗后服务保留、重新打开复用服务，以及退出后窗口进程结束。

当前压缩构建约 8.2 MiB，不含 Chromium 和 WebView2 运行时。使用海洋深色页面，水滴精灵图片用于页面、窗口、托盘及 EXE 图标；图标从原图缩放至 256px，构建脚本使用固定版本 go-winres 生成 Windows 资源。系统缺少 WebView2 的浏览器回退分支尚未在缺少运行时的机器上实测。当前页面已清理为设计基础版，保留顶部导航和空内容区，临时仪表盘、介绍、调试信息及配置表单已移除。

/v1/models 与 /v1/chat/completions 已接入 Trae 国内版账号反代；WorkBuddy 账号接入后模型 ID 需带 `workbuddy/` 前缀（如 `workbuddy/auto`），上游使用标准 OpenAI 消息格式，支持流式输出、工具调用（含 DSML 文本工具调用解析）、reasoning 与 usage 统计。Antigravity 账号接入后模型 ID 需带 `antigravity/` 前缀（如 `antigravity/gemini-3.6-flash`），上游为 Gemini 原生协议，支持流式输出、工具调用（含 thoughtSignature 缓存回传）、reasoning_effort 档位映射与 usage 统计；同来源账号在首个语义事件前遇 429/403 会短冷却并切换下一个，已提交的流不会重放。聊天路由按模型前缀确定来源，再按启用账号的添加顺序选择；已知额度用完的账号会被跳过，额度数据缺失或刷新失败不阻塞调用。Qoder 有免费模型，额度不消耗，零额度不作为跳过依据。裸模型名兼容按 Trae 处理。聊天流空闲 300 秒或总时长 30 分钟未完成会自动断开；Antigravity 流式空闲 90 秒。需要页面“账号与配额”中生成的代理访问密钥；模型没有可用的已启用账号时返回明确错误。真实 Trae 和 Antigravity 账号的登录、配额与调用链路仍需人工验收。管理接口仅面向本机原型，同源校验不构成对其他本机进程的隔离。尚未实现安装更新、崩溃自动恢复、伙伴与工作流、自定义 API 接入、手动账号排序及完整历史查询。浏览器单独运行模式不创建桌面托盘。

导航位于顶部中央：小涛 Agent、模型。模型支持悬停和点击展开仪表盘及账号与配额，键盘 ArrowDown 展开并进入选项，Escape 关闭。浏览器中同样显示导航，仅桌面环境显示窗口控制按钮。

Agent 工作台使用 `frontend/public/agent/` 中的透明水滴精灵素材，布局和响应式样式位于 `frontend/src/features/agent/`。所有页面共用延伸至半透明导航栏的海底背景，仪表盘和账号卡片采用深色半透明底以保证可读性；导航指示条滑动切换，下拉菜单展开和收起均有过渡，动效遵循减少动态效果设置。

## 小涛 Agent

Agent 页首屏是可直接使用的工作台，不再展示占位页。桌面窗口已接线系统文件夹选择框，真实点击验收仍待完成；浏览器中可直接填写项目绝对路径。任务优先使用已启用账号中聚合出的完整模型 ID，不默认伪造可用模型。没有模型时页面会引导到“账号与配额”。

首版支持：

- 单会话读写项目文件、字面搜索和运行非交互式命令。
- 项目内路径校验、明显破坏性命令拦截、命令超时及输出上限。
- 模型工具循环、最多 40 轮、工具结果完整回传、任务停止和错误状态。
- 会话持久化在 `agent/sessions/`，模型调用通过本地 `/v1/chat/completions`，继续使用账号路由和用量统计。

当前限制：

- 只有单个小涛执行者，没有伙伴、工作流、并行 Worker 或自动恢复。
- 执行过程只展示工具活动，不逐 token 展示模型文本。
- 路径校验和命令拦截不是操作系统级沙箱；工具权限细化、Skill 发现与按需加载尚未接入。
- 真实账号的 Agent 代码修改任务仍需人工验收。

实现和验收边界见[单任务运行内核](../docs/architecture/xt-client-agent.md)。
