# XiaoTao Codex 插件（首版）

这是 XiaoTao Core Skill 的可选增强层，目标是 Codex 在 ChatGPT 桌面端的**本地项目**。
当前接通 `SessionStart`（`startup / resume / clear / compact`）和单 Worker
`SubagentStart`。压缩后，SessionStart 可在下一次模型请求前提供精简规则和记忆入口；派出一个
已准备好的持久 Worker 时，SubagentStart 可注入经过验证的 Delegation Packet。

## 已实现的范围

- 提供协调、授权、Worker 等待和按需加载等少量 Core 提醒。
- 提醒 Codex 将有界 XiaoTao Worker 映射到当前可用的原生 subagent 能力，不把独立 task / conversation 当作替代品。
- 提供当前项目的 Memory 和 Task 路径；项目级 `SKILL.md` 存在时仅作为额外 hint。
- 提醒 Agent 依照 Core 检查索引是否过期，再按当前请求选择需要恢复的工作。
- 在有界活动 Task 集合中按更新时间选择最多 3 个最新、路径仍有效的 Handoff JSON，并把精简摘要作为“数据而非指令”的恢复线索；不会因此选择 Task、读取 Detailed Result 或自动继续。
- 不读取 Memory 正文、Session transcript 或整份 Core，不把所有历史注入上下文。
- 没有有效 XiaoTao 项目元数据时安静退出；嵌套仓库 / worktree 不借用父项目的状态。
- 输入或安装信息异常时跳过并输出诊断，不阻止 Codex，也不声称恢复成功。
- `SessionStart` 不执行项目或用户 Skill 中的 Python / JavaScript，不创建或刷新派生 Catalog；它只用内置 Node.js 逻辑读取有大小、数量和总字节预算的权威元数据。超出预算时以 `scan_budget_exceeded` 降级。

**这不是自动 checkpoint，也不是完整的 subagent 隔离**：Codex 已提供真实 `PreCompact` 和
`SessionEnd` Hook，但这些 Hook 本身没有当前 XiaoTao 的有界事实快照和目标选择结果；本插件不解析
不稳定的 transcript 格式来猜状态，也不因此虚报保存。尚未实现压缩前总结、写入进度、失败补存，
也没有工具白名单或权限的宿主级强制隔离。SubagentStart 只注入单 Worker Packet，并明确报告
`degraded`；SessionStart reminder 仍负责澄清宿主工具选择。恢复 Hook 只能帮助重新找到已经保存的
状态，无法恢复从未落盘的结论。因此这仍是 #14 / #26 的部分落地。

插件没有第二份 `skills/xiaotao`，避免重复注册。Core 可以由 Codex 从用户级或项目级 Skill
目录发现；`.xiaotao/` 则是项目级、跨宿主共享的 Memory / Task 状态。Hook 不自行实现完整的
Skill 搜索规则，也不会因为项目内缺少 `.agents/skills/xiaotao/SKILL.md` 而跳过状态恢复。
卸载插件后，裸 Skills 仍然可用。

Hook 的轻量 Runtime Context 直接读取项目内权威 Task / Temporary / Follow-up / Long-term 和
checkpoint 元数据，不依赖或执行某个 Core 副本。项目级与用户级 Core 仍由 Codex 的 Skill 发现机制
按需加载，用于完整 Catalog 检查、检索和工作流规则；Hook 只提供入口，不替 Core 重建
`index.json` 或 `manifest.md`。

### Codex Worker 宿主映射

在已安装、启用并信任该插件，且 `SessionStart` Hook 已为有效 XiaoTao 项目运行的会话中，
reminder 使用以下边界：

- 有界 XiaoTao Worker 优先使用当前可见的 Codex 原生 subagent capability，例如可用时的
  `spawn_agent`。
- `create_thread` 或其他独立 task / conversation API 只用于用户明确要求新开独立 Codex
  任务或会话的情况，不能代替 Worker。
- 工具侧标识始终服从当前可见工具的 schema；若只允许小写字母、数字和下划线，可使用
  `render_pipeline_scout` 之类简短的 `snake_case` 标识。
- 主 Agent 在派工说明、进度和汇总中优先使用“渲染链路侦察员”之类中文称呼。只有宿主
  提供独立且支持中文的 display-name 字段时，才承诺把中文称呼写入该字段；否则不保证 UI
  内部线程名显示中文。
- 当前环境没有原生 subagent capability 时，必须如实降级，不能虚构已派工。

该映射不改变 Core 的 Worker 选择、权限、上下文或等待规则，也不让裸 Core 依赖 Codex
专有 API。未启用插件、Hook 未执行或项目没有有效 XiaoTao 元数据时，不保证存在这条提醒。

### 单 Worker Handoff 验收模式

这是本轮提供给真实 Codex 测试的最小闭环，不是新调度器：当前用户明确选中一个已有 Task 后，
小涛才可读取该 Task 的有效 Handoff、对应 Worker State 与不可变 Worker 快照，并通过原生
`spawn_agent` 派出**一个**有界 Worker。派工包必须明确目标、完成条件、允许路径/工具、已有证据
及 Result / State / Handoff 输出路径。Worker 结束后，小涛先检查并验证 JSON Handoff；不会根据
`recommended_next` 自动派第二个 Worker。

SessionStart 在最多 64 个活动 Task 的有界扫描范围内全局排序 Handoff，展示最新 3 个可用摘要；
只接受状态、结果路径、State 路径和推荐能力均为结构正确、引用文件仍位于项目内的 JSON。摘要只是
数据，不能覆盖当前用户意图或授权。Hook 不读取 Detailed Result、完整 Handoff 历史或聊天记录。

### SubagentStart Delegation Packet 注入

当前只支持一次一个持久 Task / Temporary Worker。主 Agent 必须先按 Core 发布规范
`workers/<worker-id>/runs/<run-id>/delegation.json`，再创建：

```json
{
  "schema_version": 1,
  "session_id": "<SessionStart 提供的当前 Codex session id>",
  "delegation_path": ".xiaotao/tasks/<task-id>/workers/<worker-id>/runs/<run-id>/delegation.json"
}
```

保存为 `.xiaotao/runtime/codex/pending-delegation.json` 后才调用原生 subagent。SubagentStart 会把
该 envelope 原子移动到 `claims/`，重新校验 Packet、Worker 路径、不可变 YAML Worker snapshot 与
required instruction digest，然后注入目标、完成条件、指令正文、上下文引用和输出路径。快照交叉
校验覆盖工具/权限子集、读写路径边界、required refs 精确匹配和 optional refs 子集；解析器只接受
所需的安全 YAML 子集，复杂或歧义语法会拒绝。Session 不匹配时不消费；成功 claim 后不能重放。
Packet 必须声明 `host_adapter.id: codex`、`status: degraded`，且
`unsupported_requirements: [tool-isolation]`，因为 Hook 注入上下文仍不能强制工具白名单或权限隔离。
并行 Worker、Session 作用域 ephemeral Packet 和自动从旧状态选择 Packet 暂不支持。

## 从源码安装到桌面端（无需 npm 发布）

需要 Node.js 20.19+。在你的 XiaoTao 仓库根目录运行，例如 `D:\code\xiaotao`。

新项目可以通过 CLI 安装项目级 Core 并生成 XiaoTao 元数据；已有的跨宿主 XiaoTao 项目可直接执行 `doctor`：

```powershell
node .\bin\xiaotao.js init "D:\code\your-project" --tools codex
node .\bin\xiaotao.js doctor "D:\code\your-project"
```

然后准备个人插件来源：

```powershell
npm run codex:install:local
```

如果 PowerShell 拦截 `npm.ps1`，使用 `npm.cmd run codex:install:local`，或直接运行：

```powershell
node .\adapters\codex\install-local.mjs
```

该命令不需要构建、额外依赖或 Codex CLI。它会：

1. 将插件源文件准备到 Codex 推荐的个人插件目录 `~/.codex/plugins/xiaotao-codex/`。
2. 在 `~/.agents/plugins/marketplace.json` 添加个人来源条目，保留其他插件及已有展示名称和策略。
3. 输出准备完成的位置；**不会直接安装到 Codex 缓存、启用插件或授予 Hook 信任**。

本安装器不再创建或引用旧的 `~/plugins/xiaotao-codex/`。Codex 插件尚未正式使用，
因此不提供旧路径迁移；如果曾测试 PR #40，请手动清理旧目录和冲突的 marketplace entry 后重装。
安装器不会自动移动、删除或覆盖它们。

这里的 `~` 指当前用户主目录（Windows 通常是 `C:\Users\你的用户名`）。从 Windows 原生
终端运行，以便桌面端能访问同一目录；在 WSL、容器或远程机器运行不会安装到 Windows 的用户目录。

接着刷新 / 重启桌面端，在 Plugins 中找到个人来源下的 **XiaoTao Codex**，安装并启用。
按照 Codex 的提示审查并信任这个 `SessionStart` Hook；安装插件不等于信任 Hook。
官方文档提供 Codex CLI 的 `/hooks` 用于查看、审查和信任 Hooks；如果桌面端缺少相应入口，
可在使用同一用户配置的 Codex CLI 中处理。不要绕过信任检查。

最后，在目标项目中新开对话，明确请求使用 XiaoTao。不要仅因插件安装成功就判断 Hook 已生效。
桌面端版本必须实际支持并允许执行上述事件；旧版本、未信任或禁用 Hooks 时，仍按裸 Skills 使用。
这里只验证本地项目；云端任务需要在其实际执行环境另外配置。

### 更新

更新 XiaoTao 仓库后，如果项目级 Core 由 CLI 管理，可刷新 Core；然后刷新插件来源：

```powershell
node .\bin\xiaotao.js update "D:\code\your-project"
npm run codex:install:local
```

安装器会根据插件源码生成版本后缀。再到桌面端重新安装该个人来源的插件，并新开对话。
Codex 使用安装缓存，不应假设修改来源文件会立即影响已安装插件；如果提示 Hook 变化，重新审查。
安装器仅覆盖自己管理的六个插件文件及所有权标记，保留其他文件。
遇到同名但不同来源的插件、非 XiaoTao 管理的目录或损坏的 marketplace 时，会停止而不强行覆盖。
个人 marketplace 无需另行执行 `codex plugin marketplace add`。

### 关闭

在 Codex 中禁用 **XiaoTao Codex** 插件（或禁用其 Hook），新开对话验证。
这不会删除项目的 Core Skills 或 `.xiaotao/` 记忆。

## 最小桌面端验收

| 场景 | 应观察到的结果 |
| --- | --- |
| 有效 XiaoTao 项目且存在项目级 Core | Hook 输出共享状态路径，并额外包含项目级 Core 路径；不会自动创建 Task |
| 只有用户级 / 全局 Core，项目内没有 Core | Hook 不执行用户级 Core，直接输出项目 `.xiaotao/` 的有界权威状态总览；完整 Catalog 操作仍由按需加载的 Core 执行 |
| `.xiaotao/` 状态由其他宿主创建 | 即使 `tools` 不含 `codex`，Hook 仍恢复 Memory / Task 入口 |
| 明确要求 XiaoTao 派一个有界 Worker | 有原生 subagent capability 时直接使用它，不先尝试 `create_thread` |
| 明确要求新开独立 Codex task / conversation | 可以使用 `create_thread`，不误判为 XiaoTao Worker |
| 当前环境没有原生 subagent capability | 如实说明或按 Core 规则降级，不声称已派工 |
| 并行派出多个 Worker | 工具侧标识合法且能区分分工；派工说明、进度和汇总优先使用简短中文称呼 |
| 保存好一次进度后触发手动或自动压缩 | `SessionStart` 的 `source=compact` 执行；继续模型请求前补入提醒，再按需读取原有 Current State |
| 项目没有 Memory 或有多个候选任务 | 不虚构保存结果，不自行选一个旧任务继续 |
| 单 Worker Handoff 闭环 | 用户明确点名一个已有 Task 后，先读取匹配的有效 Handoff，再只派一个原生 Worker；Worker 产出 Result、State、通过校验的 Handoff 后才汇总，不自动派下一位 |
| 单 Worker Packet 注入 | 主 Agent 先发布规范 Packet、不可变 Worker 快照和当前 Session envelope；SubagentStart 只消费一次，快照边界或 required instruction digest 不匹配时不注入 |
| 清空会话后问一个新问题 | 不把旧任务意图当成当前指令 |
| 没有有效 `.xiaotao/installation.json` 的普通项目 | Hook 无上下文输出、不写 `.xiaotao/` |
| 禁用插件 / Hook 未信任 | 不再声称自动恢复 Hook 生效；已安装的 Core Skills 仍可显式使用 |

记录桌面端版本、执行环境、触发方式、Hook 执行状态和实际读取路径。
若客户端没有可用执行记录，只能记为“无法验证 Hook 是否运行”；模型说“我已恢复”不是证据。
自动测试覆盖协议与文件边界，不代表真实桌面端已通过验收。

## 开发验证

```bash
node --test test/codex-adapter.test.js
```

在仓库根目录执行。测试运行真实的 Hook launcher（包括带空格和特殊字符的插件路径），
不调用模型、不读取开发者真实会话；安装测试使用临时用户目录。根 `npm test` 已包含这些测试，
CI 另在 Windows 运行此套件。

## 官方接口依据

- [Codex Hooks](https://learn.chatgpt.com/docs/hooks)：`SessionStart`、`PreCompact`、`SessionEnd`、插件默认 Hook 路径、Hook 信任。
- [Codex 子智能体](https://learn.chatgpt.com/zh-Hans/docs/agent-configuration/subagents)：委派触发、agent thread、等待和汇总语义。
- [插件打包和本地 marketplace](https://developers.openai.com/plugins/build/plugins)：个人来源、安装缓存和插件结构。

Hook 通过 Node 读取 `PLUGIN_ROOT`，避免把插件路径拼进 shell 代码；同一条命令用于 Windows 和 POSIX。
SessionStart 只进行有界读取，不执行项目代码、不写项目文件。SubagentStart 仅对显式准备、Session
绑定的 pending envelope 执行一次原子 claim；两者都输出 JSON `hookSpecificOutput.additionalContext`。
不注册常驻服务，不调用模型 API，不改变 Codex 的权限、模型或工具配置。
