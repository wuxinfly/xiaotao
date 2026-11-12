# 客户端开发约定

- Windows Go 服务和 WebView2 壳，页面嵌入服务。
- 本目录执行 go test ./...、go vet ./...；手动打包使用 ./build.ps1 或 ./build.ps1 -Env uat。AI 自行构建验证时必须添加 -Verify，不递增版本，也不覆盖正式产物或快捷方式。
- 客户端代码质量按 ../docs/agent/client-code-quality.md 检查；新增代码不得新增超限文件或函数，修改存量超限文件时不得继续变大。前端在 frontend 运行 npm run lint、npm run check:size；Go 文件大小在 xt-client 运行 node tools/check-go-size.mjs。
- 打包用的外部 Go 工具（当前为 go-winres）固定为 go.mod 的 tool 依赖并用 `go tool` 调用，不写 `go run <module>@<version>`，避免构建时联网查询模块版本。
- 不主动写注册表，不携带 WebView2 运行时。
- 数据写用户目录；管理写接口校验同源。
- 不提交 dist、日志和本机数据。不用模拟数据冒充用量。
- 正式构建输出 dist/xiaotao.exe，使用 %LOCALAPPDATA%\XiaoTao、固定端口 5455 和“小涛”快捷方式；UAT 构建输出 dist/xiaotao-uat.exe，使用 %LOCALAPPDATA%\XiaoTao-uat、固定端口 5456 和“小涛 UAT”快捷方式。两套环境的数据、端口、单实例锁和窗口互不共用，必须可以同时运行。UAT 首次启动时只读导入正式版手动模型来源和 API Key，在 UAT 密钥环重新加密后独立维护；不导入旧平台代理账号，本地网关密钥、用量、其他配置、Agent 会话、日志和运行时状态不得共享。
- build.ps1 构建后更新当前环境对应的桌面快捷方式；主窗口和启动动画原生窗口均使用系统小圆角；启动动画由原生 GDI 帧流在原生窗口以 20fps 呈现并覆盖初始化阶段，后台启动流程跑完且动画就绪后自动平滑切换为主窗口，无下载弹窗、无跳过按钮，托盘恢复不播放。
- 前端源码在 frontend，使用 React、TypeScript、Vite。先在 frontend 运行 npm ci、npm run lint、npm run check:size、npm run build，再运行 Go 验证；internal/app/web 为生成资源，不手动修改。
- 服务正在运行时只调前端样式或交互，不停止实例或直接构建覆盖；按 README 设置 XIAOTAO_DEV_API 启动独立 Vite 预览，用真实 API 调试。预览中的写操作会真实修改当前实例数据。
- 项目 Agent 办公室在 internal/office，使用独立 office/office.db，只把任务、消息和产物记录落到办公室库，不向 agent.db 加表；执行仍走 internal/agent 的单执行者会话，不新增第二套模型循环。运行中不能向执行者投递消息，只能排队并在本轮成功后送达，界面和文档不得写成已即时送达；服务重启后运行中任务标失败，不自动恢复；产物只从文件工具写入识别，不能声称覆盖命令或外部写入。
- Agent 运行内核在 internal/agent；模型调用走本地 /v1/chat/completions。工具可访问当前用户权限范围内的路径，安全模式下常见高危命令须等待用户确认；模型工具结果必须保留调用 ID 并完整回传；新增工具需验证访问模式、错误路径和前端进度。不得把路径检查或命令黑名单描述为操作系统级沙箱。

## 模型来源与通用网关

- `internal/modelsource` 保存手动来源、稳定模型 ID 和加密 API Key，`internal/gateway` 提供 Chat Completions、Responses、Anthropic Messages 对话入口，`internal/wireprotocol` 负责协议转换；手动上游可独立选择这三种格式。API URL 是实际 API 前缀或所选协议完整端点，不猜测添加 `/v1`。Responses/Anthropic 当前实现为无状态对话兼容子集，范围和验证见 `../docs/plans/2026-11-12-gateway-protocols.md`。
- 接口页的模型调用清单通过 `GET /api/gateway/models` 读取网关同一公开目录，供复制实际调用别名与来源参数；Agent 模型库继续保存内部模型 ID 和独立执行参数。交互与验证见 `../docs/plans/2026-11-12-model-library-and-export.md`。
- 路由必须使用模型内部 ID 明确绑定来源与上游模型 ID，不拆上游 ID 的 `/` 猜测平台，不把不同接口同名模型合并。旧平台模型引用保留但不可自动重绑。
- 核心不导入或注册 `internal/authsessions`、`internal/proxy` 和 `internal/providers/*` 平台实现，不执行平台登录、续期、配额、签到或目录发现。旧源码与账号数据仅作为后续插件迁移材料保留，不自动删除。
- 读取/刷新手动模型目录不请求上游；测试连接只由显式操作触发。API Key 仅在服务端密钥环加密，编辑省略表示保留，读取只回 hasAPIKey。
- 网关保留本地独立鉴权、流式工具增量、取消、空闲 300 秒/总时长 30 分钟上限和真实 usage；不因缺用量而估算为零，断流或上游错误不可伪装成功。请求发出后不携密钥跟随重定向。
- 模型来源插件使用 `internal/sourceplugin` 的独立进程协议；Trae 独立入口在 `cmd/trae-source-plugin`，核心不得直接导入其平台实现。当前支持本地目录安装、启停、单账号令牌导入与显式旧账号迁移；完整协议与本地验证步骤见 `../docs/plugins/README.md`。

## 交互与动效设计规则

- 小涛后续所有页面都要重视交互效果，将状态反馈和动效作为设计、实现与验收的一部分。
- 按钮、导航、下拉、弹窗、页面切换及异步操作应具有清晰的悬停、按下、聚焦、加载、成功或失败反馈，按组件实际需要设计。
- 动效保持海洋深色主题的轻盈、流畅与连贯；常规反馈建议 120–220ms，优先使用 opacity 和 transform，避免布局跳动及无意义的持续动画。
- 展开与收起均需考虑过渡；支持快速反向操作，不能因播放动画阻塞点击或任务操作。
- 悬停交互必须兼顾点击、键盘和焦点操作；隐藏内容不可继续获得焦点或接收点击。
- 尊重 prefers-reduced-motion，减少或取消位移、缩放及非必要动画，保留清晰状态反馈。
- 验收检查展开/收起、快速移入移出、键盘操作、减少动态效果及桌面壳/浏览器两种入口。
