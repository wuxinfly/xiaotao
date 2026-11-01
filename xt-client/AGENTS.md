# 客户端开发约定

- Windows Go 服务和 WebView2 壳，页面嵌入服务。
- 本目录执行 go test ./...、go vet ./...；正式构建运行 ./build.ps1，UAT 构建运行 ./build.ps1 -Env uat。
- 不主动写注册表，不携带 WebView2 运行时。
- 数据写用户目录；管理写接口校验同源。
- 不提交 dist、日志和本机数据。不用模拟数据冒充用量。
- 正式构建输出 dist/xiaotao.exe，使用 %LOCALAPPDATA%\XiaoTao、固定端口 5455 和“小涛”快捷方式；UAT 构建输出 dist/xiaotao-uat.exe，使用 %LOCALAPPDATA%\XiaoTao-uat、固定端口 5456 和“小涛 UAT”快捷方式。两套环境的数据、端口、单实例锁和窗口互不共用，必须可以同时运行。UAT 首次启动时只读导入正式版已登录的模型账号，导入后在 UAT 独立维护；代理密钥、用量、配额、配置、Agent 会话、日志和运行时状态不得共享。
- build.ps1 构建后更新当前环境对应的桌面快捷方式；主窗口和启动动画原生窗口均使用系统小圆角；启动动画由原生 GDI 帧流在原生窗口以 20fps 呈现并覆盖初始化阶段，后台启动流程跑完且动画就绪后自动平滑切换为主窗口，无下载弹窗、无跳过按钮，托盘恢复不播放。
- 前端源码在 frontend，使用 React、TypeScript、Vite。先在 frontend 运行 npm ci、npm run build，再运行 Go 验证；internal/app/web 为生成资源，不手动修改。
- 服务正在运行时只调前端样式或交互，不停止实例或直接构建覆盖；按 README 设置 XIAOTAO_DEV_API 启动独立 Vite 预览，用真实 API 调试。预览中的写操作会真实修改当前实例数据。
- Agent 运行内核在 internal/agent；模型调用走本地 /v1/chat/completions。工具只能访问任务所选项目根目录，模型工具结果必须保留调用 ID 并完整回传；新增工具需同时补目录边界、错误路径和前端进度验证。不得把路径检查或命令黑名单描述为操作系统级沙箱。

## 反代 Provider 约定

- WorkBuddy 上游使用标准 OpenAI 消息格式（原始 messages、JSON Schema tools、字符串或函数名 tool_choice）；不要复用 Trae 的 `function: chat_v3` 信封或 NativeMessages 转换。
- 聊天流必须带空闲 300 秒和总时长 30 分钟超时；上游非 2xx 统一返回 `providers.UpstreamError`。
- 代理层按模型前缀确定来源，再按启用账号的当前顺序路由；已知额度用完的账号跳过，额度缺失或错误不阻塞。Qoder 有免费模型，额度不消耗，零额度不作为跳过依据。`/v1/models` 聚合启用账号目录并按完整模型 ID 去重；WorkBuddy 模型 ID 必须带 `workbuddy/` 前缀。不使用默认账号路由，也不要新增默认账号状态。
- Antigravity 上游使用 Gemini 原生协议的 Antigravity 包装（`/v1internal:streamGenerateContent`），内部请求含 project/model/request envelope、身份声明注入和 `[ignore]` 包裹副本；模型 ID 必须带 `antigravity/` 前缀，使用内置底表与实时目录合并后的名称及档位映射。
- Antigravity 流式空闲 90 秒、非流式 120 秒；thoughtSignature 使用按账号隔离的内存 LRU 缓存，签名必须放在包含 functionCall 的 part 外层，不能嵌进 functionCall；签名缺失时 Gemini 模型注入哨兵。请求内 429/403 主备降级只能在首个语义事件前进行，统一短冷却 1 分钟，已提交的流绝不重放。
- Codex 上游使用 Responses SSE 协议（POST /backend-api/codex/responses），请求固定 store: false、stream: true 并携带 Originator: codex-tui、Session-Id 及 Chatgpt-Account-Id 请求头；模型 ID 必须带 codex/ 前缀。
- Codex OAuth 固定使用 localhost:1455/auth/callback 并校验 state；支持粘贴 ~/.codex/auth.json 导入，refresh token 轮换时写盘需比对内存版本避免覆盖。
- 修改 WorkBuddy 或 Codex 请求格式或超时行为时同步更新本节、README 和 docs/plans 中的接入计划。

## 交互与动效设计规则

- 小涛后续所有页面都要重视交互效果，将状态反馈和动效作为设计、实现与验收的一部分。
- 按钮、导航、下拉、弹窗、页面切换及异步操作应具有清晰的悬停、按下、聚焦、加载、成功或失败反馈，按组件实际需要设计。
- 动效保持海洋深色主题的轻盈、流畅与连贯；常规反馈建议 120–220ms，优先使用 opacity 和 transform，避免布局跳动及无意义的持续动画。
- 展开与收起均需考虑过渡；支持快速反向操作，不能因播放动画阻塞点击或任务操作。
- 悬停交互必须兼顾点击、键盘和焦点操作；隐藏内容不可继续获得焦点或接收点击。
- 尊重 prefers-reduced-motion，减少或取消位移、缩放及非必要动画，保留清晰状态反馈。
- 验收检查展开/收起、快速移入移出、键盘操作、减少动态效果及桌面壳/浏览器两种入口。
