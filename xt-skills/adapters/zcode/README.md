# ZCode 宿主适配

复用 XiaoTao Core 与项目 `.xiaotao`，通过原生 Agent 工具派发一层有界 Worker；不支持嵌套派工。Explore 只读。使用 Core 的 Delegation Packet 交接目标、证据、权限和输出契约；工具不存在时如实降级，不能声称完成隔离。

## 安装与更新

运行 `npm run xiaotao:init`，选择 ZCode。CLI 在用户目录 `~/.xiaotao/hosts/zcode/` 准备本地插件市场（测试/自定义位置可使用 XIAOTAO_HOME）。它不修改 ZCode 私有安装数据库，也不自动启用代码。

在 ZCode **设置 → 插件 → 创建 → 添加插件市场** 中选择上述目录，安装并启用 `xiaotao-zcode`。新建会话，确认设置中的三个插件 Hooks 已加载。

运行 `npm run xiaotao:update` 后，回 ZCode 刷新市场、检查更新并更新插件，再新建会话。CLI doctor 检查本地市场的文件与 JSON 可读性；`zcode_activation_unverified` 明确表示客户端启用状态没有被验证，不代表运行时验收通过。

## 进度与记忆

SessionStart / UserPromptSubmit 注入 Core 入口、Worker 边界和阶段保存规范。仅对当前 cwd 下安装元数据明确选择 zcode 的项目生效；请打开初始化项目根目录。普通目录不自动初始化。

Stop 使用官方 `decision: block` 要求 Agent 做一次收尾检查；`stop_hook_active` 防重复。Agent 按 Core 的 save_progress.py 保存有价值的新进展，关联任务同步更新；重复内容、闲聊不制造记录，长期知识走独立评审。Hook 不调用模型、不总结、不写记忆、也不伪造保存回执。目前不是基于落盘回执的强制完成校验：保存是否成功仍需 Agent 核对脚本结果。强退、崩溃、压缩前保存不保证。

仅注册 SessionStart、UserPromptSubmit、Stop，不模拟 PreCompact 或 SubagentStart/Stop。Node 与 Core 所需 Python 必须在本机可用。

## 验证

`node --test test/zcode-adapter.test.js` 覆盖安装、更新、doctor、真实 stdin/stdout 协议、收尾循环保护、普通项目跳过与损坏输入。真实 ZCode 客户端安装、原生 Agent 派工和端到端保存仍待人工验收。

官方协议依据（2026-10-21）：
- https://zcode.z.ai/cn/docs/plugin
- https://zcode.z.ai/cn/docs/hooks
- https://zcode.z.ai/cn/docs/subagents
- https://zcode.z.ai/cn/docs/skill
