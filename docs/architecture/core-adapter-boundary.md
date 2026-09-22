# XiaoTao Core / Adapter 架构边界与能力矩阵

Status: **架构边界稳定；宿主增强按能力独立激活，真实宿主验收仍需单独记录**。
Scope: [Issue #14](https://github.com/IHongTaoI/xiaotao/issues/14)。Updated 2026-09-12。

## 1. 固定边界

XiaoTao 采用一份可移植 Core，加零个或多个可选宿主 Adapter：

- **Core 决定语义**：是否创建或恢复 Temporary / Task，选择哪个 Worker，保存哪些事实，是否需要
  Handoff、Memory 或 Playbook，以及何时请求校验或持久化。
- **Adapter 负责机械执行**：把 Core 注册到宿主，探测能力，映射宿主工具与事件，并在宿主允许时
  提供路径边界、schema 校验、锁、CAS、可靠写入和确定性触发。
- **依赖保持单向**：Core 不导入 Codex、DSH 或其他宿主 API；Adapter 可以依赖宿主 API，但不能复制
  Core 的业务判断。新宿主优先新增 Adapter，不 fork XiaoTao Core。
- **没有 Adapter 仍可运行**：Bare Core 可以依赖当前宿主已有的文件与 Agent 能力按文字协议工作；
  缺少确定性增强时必须报告降级，不能虚构锁、原子性、生命周期 Hook 或隔离能力。

判断口诀：**“该不该做、找谁、保存什么”属于 Core；“宿主如何可靠执行”属于 Adapter。**

## 2. 状态词必须分开使用

| 状态 | 含义 |
| --- | --- |
| `available` | 宿主公开了所需 API / seam；只证明能力存在。 |
| `activated` | Adapter 已把能力接入当前功能路径，并有代码或配置证据。 |
| `degraded` | Core 仍可完成基本语义，但缺少某项确定性或隔离保证。 |
| `unsupported` | 当前宿主或 Adapter 没有可诚实实现该能力的接口。 |
| `unverified` | 机制或自动化测试存在，但尚无真实宿主执行证据。 |

`available != activated`，`activated != live-verified`。类型声明、配置扫描、fake filesystem 测试和模型
口头说明都不能代替真实执行链证据。

## 3. 当前宿主能力矩阵

| 能力 | Bare Core | DeepSeek Harness Adapter | Codex Adapter | Antigravity Adapter |
| --- | --- | --- | --- | --- |
| Core Skill 加载 | 宿主支持 Agent Skills 时可用；否则需要薄 Adapter | `ctx.skills` 注册已 `activated` | Core 由用户级或项目级 Skill 发现；插件不捆绑第二份 Core | 插件内 `skills/xiaotao/` 自动发现；已 `activated` |
| Core schema / 协议校验 | `validate.py` 可按需运行 | validator Cordis service 已 `activated`；只在有界 checkpoint 路径实际使用，不是任意写入工具 | 没有专用 validator 接线；按 Core / 宿主工具执行 | 没有专用 validator 接线；按 Core / Python 脚本执行 |
| 状态边界、锁与 CAS | 依赖宿主文件工具，确定性保证为 `degraded` | `ctx.fs` 上的 `.xiaotao/` containment、锁、租约、tombstone release 与 CAS 已 `activated` | 插件只读状态入口，不接管写入；写入保证为 `degraded` | 依赖宿主文件工具，确定性保证为 `degraded` |
| 有界模型 checkpoint | 由当前 Agent 按 Core 协议显式保存 | `fs + tools + schema` 就绪且 checkpoint 未禁用时，`xiaotao_checkpoint` 已进入 ToolRuntime；真实模型/后端链仍 `unverified` | `unsupported`；`SessionStart` 只能恢复已落盘入口 | 由当前 Agent 按 Core 协议显式保存，保证为 `degraded` |
| 自动 checkpoint | `unsupported` | 上下文压力触发为 opt-in `activated`；使用 awaited `turn-stopping` fallback，不是 pre-compaction / Session End；真实宿主验收 `unverified` | `PreCompact` 能力 `available`，但缺少稳定的有界事实与目标生成链，功能未激活 | 宿主暂无 pre-compaction 拦截 seam，为 `unsupported` |
| Session 恢复入口 / Runtime Context | 由新 Session 显式发现已保存状态 | `agents` 就绪时 SessionStart 有界 Runtime Context 已 `activated` | `SessionStart` 的 startup / resume / clear / compact reminder 已实现；以只读 Node.js 扫描权威元数据，不执行项目 Core、不刷新 Catalog；只有安装、启用、信任并真实运行后才算激活 | `PreInvocation` Hook 首轮注入不超过 300 token 的 `ephemeralMessage` 运行时上下文与 Guard 已 `activated` |
| 多文件 transaction / commit | Core 定义协议，执行依赖宿主 | create/replace 事务 service 已 `available`；model-facing 业务路径未激活，checkpoint 仍保守拒绝 overlay；delete/rename lifecycle move `unsupported` | `unsupported` | `unsupported` |
| 原生隔离 Worker / Delegation Packet | 取决于宿主能力；不能假设继承 | DSH preview 未提供可验证的原生隔离 subagent 运行时，使用 in-session fallback | 单持久 Worker 的 Session 绑定 Packet 注入已 `activated`；required instruction digest 与不可变 Worker 快照边界会复核，但宿主级工具/权限隔离仍为 `degraded`，并行与 ephemeral Packet 未接线 | 映射至 `define_subagent` 与 `invoke_subagent`，只读 Worker 物理隔离（`enable_write_tools: false`）已 `activated` |
| 完整分层验收 | 需要在具体宿主记录 | 自动化机制覆盖较多；真实模型、持久后端、重启恢复和故障路径仍 `unverified` | 有人工清单；真实桌面 Hook 与降级场景证据仍需记录 | 自动化机制测试完备；真实插件环境证据待记录 `unverified` |

代码与证据入口：

- DSH 激活入口：[adapters/deepseek-harness/src/index.ts](../../adapters/deepseek-harness/src/index.ts)
- DSH 当前能力与降级：[adapters/deepseek-harness/README.md](../../adapters/deepseek-harness/README.md)
- Checkpoint 契约：[checkpoint-contract.md](checkpoint-contract.md)
- 自动 checkpoint 能力矩阵：[automatic-checkpoint.md](automatic-checkpoint.md)
- Codex 当前范围：[adapters/codex/README.md](../../adapters/codex/README.md)
- Antigravity 插件与适配器：[adapters/antigravity/README.md](../../adapters/antigravity/README.md)
- 真实宿主证据格式：[manual-acceptance.md](../manual-acceptance.md)

## 4. Model-facing 工具的边界

Adapter 不应仅为了“让模型能访问 service”而暴露无限制 raw storage 或 validator 工具。优先提供
围绕一个明确 Core 流程的有界操作，例如现有 `xiaotao_checkpoint inspect/save/status/retry`：

1. Core 选择目标并提供事实；
2. 工具参数不允许模型扩大项目根、恢复根或权限；
3. Adapter 负责 schema、路径、锁、CAS、幂等和失败证据；
4. 工具结果只报告机械事实，不替 Core 决定是否创建 Task、恢复旧工作或写入 Long-term Memory。

只有出现新的具体写入流程和验收需求时，才新增对应的窄工具。内部 service 已注册不等于所有 Core
读写已经自动经过增强路径。

## 5. 生命周期接线规则

- 只能使用宿主真实提供、语义匹配且具有正确等待/取消行为的事件。
- `turn-stopping` 不得描述成 pre-compaction；`SessionStart` 不得描述成自动 checkpoint。
- Hook 发现 transcript 路径不等于获得稳定的结构化事实来源，不解析未承诺稳定的内部 transcript
  格式来猜测状态。
- 宿主缺少目标选择或模型步骤时，保留显式保存和恢复入口，并将自动能力标记为未激活或不支持。

## 6. 剩余独立工作包

### [DSH 多文件事务](https://github.com/IHongTaoI/xiaotao/issues/68)

首版实现 Core `storage.md` 的 create/replace commit 可见性机械层，包括不可变 bundle、按路径有序加锁、
提交前统一校验、CAS 冲突、部分 materialization、commit marker 和重启恢复。具体 model-facing
业务路径尚未激活；当前宿主缺少 delete/rename，生命周期 move 保持 unsupported。多个独立原子写
不能宣称为整体事务。

### [真实宿主分层验收](https://github.com/IHongTaoI/xiaotao/issues/69)

在真实 DSH 模型与持久文件系统中验证 checkpoint、进程退出、新 Session 恢复和 recovery 失败路径；
同时验证移除 Adapter 后 Bare Core 可用，以及缺少 `fs/tools/agents` 时的诚实降级。Codex 按其桌面
人工清单分别记录，不用一个宿主的测试代替另一个宿主。

### 未来宿主能力

原生 pre-compaction / Session End、完整 Worker Packet 隔离和新宿主 Adapter 只在存在真实需求及可用
宿主能力时推进，不作为当前架构 Issue 无限保持开放的理由。

## 7. Issue #14 的关闭边界

满足以下条件后可关闭架构总 Issue：

1. 本文档与能力矩阵已合并，并与实现状态一致；
2. 未完成的实现和真实宿主验收均有独立、已链接的 Issue；
3. 已完成项有代码、自动化或真实宿主证据，部分实现没有被写成完整支持；
4. Core 无宿主专有依赖，Adapter 没有复制语义决策，移除 Adapter 后 Core 仍可基本运行。

后续能力由对应 Issue 跟踪；未来新增宿主本身不重新打开本架构总 Issue。
