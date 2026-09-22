# XiaoTao Antigravity 插件与深度 Adapter

这是 XiaoTao Core Skill 针对 Google Antigravity 的深度增强层。

XiaoTao 作为 Antigravity 的命名空间插件部署在 `~/.gemini/config/plugins/xiaotao-antigravity/`。它接通 Antigravity 的 `PreInvocation` 生命周期 Hook 与原生子代理（Subagent）体系，提供启动轻量上下文恢复和真正的工具物理隔离。

## 核心能力与机制

1. **零冷启动失忆与极简 Runtime Context (`PreInvocation`)**：
   - 会话首次调用（`invocationNum: 1`）时，Hook 读取项目权威 `.xiaotao/` 状态（活跃任务、可恢复 Checkpoint、最新交接与待跟进事项）；
   - 在不超过 300 token 的预算内生成 Core Guard 与状态线索，通过 `ephemeralMessage` 注入；
   - 随后的多轮对话中 Hook 安静退出（`{}`），不污染长期对话上下文。

2. **宿主级工具物理隔离 (`Subagent Bridge`)**：
   - Antigravity 原生支持在 `define_subagent` 中声明 `enable_write_tools: boolean`；
   - XiaoTao 将调查类 Worker、Memory Worker 提案阶段的只读约束真正映射为 `enable_write_tools: false`，由宿主底层剥离写操作与命令执行工具，成为主流 AI 编码宿主中首个支持真正工具级物理隔离的实现。

3. **零额外依赖的 Skill 发现**：
   - 插件将 XiaoTao Core 放置在 `skills/xiaotao/` 下，Antigravity 自动加载其名称与描述，并在模型需要时按需渐进式展开，无需第三方额外运行时。

## 能力矩阵（基于 core-adapter-boundary）

| 能力 | 状态 | 说明 |
| :--- | :--- | :--- |
| **Core Skill 加载** | `activated` | 放置于插件 `skills/xiaotao/`，由 Antigravity 自动发现与按需加载。 |
| **Session 恢复 / 运行时锚点** | `activated` | 由 `PreInvocation` Hook 生成不超过 300 token 的 `ephemeralMessage` 运行时上下文。 |
| **原生隔离 Worker / 派工包** | `activated` | 映射至 `define_subagent` 与 `invoke_subagent`，只读 Worker 强制启用 `enable_write_tools: false` 物理隔离。 |
| **状态边界、锁与 CAS** | `degraded` | Antigravity 通过宿主文件工具操作，依赖 XiaoTao Core 的乐观锁机制。 |
| **自动 Checkpoint** | `unsupported` | Antigravity 尚未暴露 pre-compaction 拦截能力，Checkpoint 采用 Core 显式协议。 |

## 本地安装

运行以下命令将适配器安装至当前用户的 Antigravity 插件目录：

```bash
npm run antigravity:install:local
```

或者在项目中运行统一交互式 CLI 并选择 Antigravity：

```bash
npm run xiaotao:init
```
