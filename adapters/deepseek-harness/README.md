# @xiaotao-ai/dsh-adapter

Checkpoint M1 的接入证据、拟定协议与后续验收见
[checkpoint contract](../../docs/architecture/checkpoint-contract.md)。可在仓库根目录运行
`node adapters/deepseek-harness/scripts/audit-checkpoint.mjs` 复核已安装 DSH 声明与 lockfile。
该检查不访问 Session、不启动宿主，也不代表功能启用。现已有默认按当前 Session 绑定项目的 snapshot checkpoint
工具；真实模型提供方与持久文件系统的端到端验收仍未完成。

DeepSeek Harness (dsh) 的 XiaoTao 适配层。它把可移植的 XiaoTao Core Skill 挂进 dsh，并在
dsh 的 `ctx.fs` 原语之上提供确定性的状态写协议。这是 [Issue #14][issue-14]「Core 可移植 +
可选 Harness Plugin / Adapter」方向的第一个宿主实现。

> **当前接线状态（如实）**：**Product A**（把 Core 注册成 dsh skill）已接线。
> `ctx.fs` 存在时，确定性的 `XiaoTaoStateStore`、`XiaoTaoSchemaValidator` 与
> `XiaoTaoTransactionStore` 会被构建并注册成 Cordis service（`xiaotao.stateStore` /
> `xiaotao.schemaValidator` / `xiaotao.transactionStore`），但**尚未暴露成
> 任意 model-facing storage tool**。启用下述 checkpoint 工具后，单目标快照保存会经过锁 / CAS /
> 校验；事务 service 只提供 create/replace 的逻辑提交、overlay 与重启恢复机械层，现有 checkpoint
> 及其他 Core 写入不会自动改道。DSH 没有 delete/rename seam，因此生命周期 move 仍不支持。
> `ctx.agents` 的实际接线状态见下文自动 checkpoint 与 Runtime Context 章节。

> **Worker Delegation Contract**：当前 Adapter 尚未实现 Worker Delegation Packet 到 dsh
> subagent prompt / tool isolation 的映射，因此不能对独立 Worker 执行宣称 `supported`。在该映射
> 完成前，Core 可以在当前 Agent 中直接执行同一份契约，但必须如实描述为本地 fallback；若要求
> 独立 Worker 隔离，则应报告 `unsupported`，不能假设子 Agent 自动继承父 Skill 或权限。

> dsh 目前是 v0.1 开发者预览版，官方 README 明确「THERE WILL BE COMPATIBILITY-BREAKING
> CHANGES」。本适配层把对 dsh API 的引用收敛到 `src/` 内的 TypeScript 类型导入，Core 永不
> import 任何 `@deepseek-ai/*` 包。

## 架构边界

| 层 | 职责 | 本仓库位置 |
| --- | --- | --- |
| **XiaoTao Core** | 角色调度、三层记忆语义、Handoff 边界、Playbook 规则（宿主无关） | `../../xiaotao/` |
| **Skill adapter（薄）** | 把 Core 注册成 dsh skill | `src/skill.ts` |
| **Capability plugin（厚）** | 锁 / 原子写 / 事务、schema 校验、session 生命周期 | `src/storage.ts`、`src/transaction.ts`、`src/validate.ts`、`src/hooks.ts` |
| **detection + fallback** | 探测 dsh 提供了哪些 seam，按能力降级 | `src/detect.ts` |

红线（来自 Issue #14 验收标准）：

1. Core 不直接依赖宿主专有 API。
2. Adapter 不复制 XiaoTao 的业务决策逻辑。
3. 移除 Adapter 后，XiaoTao Core 仍可基本运行。

## 用法

### 本地仓库一键安装（无需发布 npm）

在仓库根目录运行：

```powershell
npm run dsh:install:local -- --profile web
```

如果 PowerShell 的执行策略拦截 `npm.ps1`，把命令开头的 `npm` 换成 `npm.cmd` 即可。

安装器仅在本次 `dsh plugin add` 中传入 `--ignore-workspace-root-check`，允许向 DSH
profile 自身的 workspace 根添加插件，不修改全局 pnpm 配置。

该命令会安装 adapter 的构建依赖，生成包含当前 XiaoTao Core 的本地 `.tgz`，并把它安装进
`~/.dsh/profiles/web/`。DSH 会根据包内的 `dsh.bundle` 声明自动把它加入 profile，随后加载包内
的 `cordis.patch.yml`：

```yaml
- insert:
    - id: xiaotao-adapter
      name: '@xiaotao-ai/dsh-adapter'
      inject:
        - skills
```

安装包保存在 `~/.dsh/local-packages/<profile>/`，不会使用容易在 Windows 上生成错误 junction
的本地目录 `link:`。安装器最后会验证包入口、随包 Core、`dsh.profile.bundles` 和
`dsh --dump-config`。如果旧 profile 里存在手工添加的 XiaoTao insert，安装器会移除该重复项，
改由包的 bundle 激活；新 profile 默认使用包内 `lib/core/`。

常用选项：

```text
--profile <name>       目标 profile，默认 web
--dsh-home <path>      自定义 DSH home
--skip-dependencies    已安装依赖时跳过 npm install
--no-verify            跳过最终 DSH 配置验证
```

### 包构建

adapter 通过 `prepack` 自动构建：

```bash
cd adapters/deepseek-harness
npm install
npm pack
```

构建会输出 ESM 入口、类型声明和当前 XiaoTao Core 到 `lib/`。`lib/` 不提交 Git，但会进入
生成的 npm tarball。安装运行时不需要 TypeScript、tsx 或 esbuild。

### Cordis 调用

把 `xiaotao/` 目录作为 Core 传入，插件在 `apply()` 时读取 `SKILL.md` 的 frontmatter
（`name` / `description`）并用 `ctx.skills.register()` 注册为运行时 skill，`references/`
和 `schemas/` 通过 `resourceBase: { kind: 'directory' }` 暴露给模型按需加载。

```ts
import adapter from '@xiaotao-ai/dsh-adapter'

// 在 Cordis profile / bundle 里挂载
export const name = 'xiaotao-adapter'
export const inject = ['skills']
export function apply(ctx, config) {
  adapter.apply(ctx, config)
}
```

`Config.coreDir` 指向 XiaoTao Core 的 `xiaotao/` 目录（含 `SKILL.md`）。缺省时按
`process.cwd()/.dsh/skills/xiaotao` → `process.cwd()/xiaotao` → adapter 包内 `lib/core/` 的
顺序探测。

## Capability detection 与降级

`src/detect.ts` 用 `ctx.get()` 获取启动时能力快照。可选 fs/tools 增强通过子级 `ctx.inject()`
等待服务就绪后激活，跟随服务卸载释放，不阻塞 Core Skill：

| Seam | 探测键 | 有 → | 无 → |
| --- | --- | --- | --- |
| skill registry | `ctx.skills` | 注册 Core（必需） | 报错（无 skill 无法工作） |
| filesystem | `ctx.fs` | 启用 `storage`（锁 + CAS 写） | 降级：Core 按 `storage.md` 文字协议自行用文件工具 |
| agent registry | `ctx.agents` | 启用 `hooks`（session 生命周期） | 降级：不监听 agent 事件 |

Session 启动时，Adapter 只通过 DSH 的非唤醒 `agent.inject()` 排队一份有界运行时概览；它会随
用户下一次真实请求进入上下文，但不会因为项目存在 `.xiaotao/` 状态而自行开启模型轮次。启动 Hook
只读取并验证已有 Catalog 快照，不扫描或重建项目状态。注入内容同时要求逻辑、历史与决策类问题
先加载 XiaoTao Skill，并执行有界 `search` / `show`，再检查当前代码；具体查询会按需刷新 Catalog。

降级到纯 skill 模式时，XiaoTao Core 仍可用——只是锁 / 原子写 / 校验由模型自行按 Core 里的
文字协议执行，可靠性下降但不丢失功能。这对应 Issue #14 的「无 Plugin 也必须能运行」。

## 关键 API 映射

| XiaoTao 概念 | dsh 原语 |
| --- | --- |
| `revision`（`storage.md`） | `ctx.fs` 的 `FsVersion`（`stat()` 返回的不透明版本 token） |
| 原子替换 + 冲突检测 | `ctx.fs.writeText(target, content, { kind: 'replaceIfVersion', version })`；冲突抛 `FS_STALE_VERSION` |
| 独占锁 create-if-absent | `ctx.fs.writeText(lockTarget, owner, { kind: 'createIfAbsent' })`；已存在抛 `FS_NOT_OBSERVED` |
| 状态路径边界 | 每次解析都走 `ctx.fs.contains(.xiaotao/, target)` 做权威 containment 校验，`lockPathFor` / 状态路径再拒绝 `..` 与绝对路径 |
| schema 校验 | 复用 `xiaotao/references/schemas/*.json`（JSON Schema draft 2020-12），用 `ajv` 校验 |
| create/replace 多文件事务 | 不可变 before/staged/intent + 互斥 terminal marker + overlay + CAS materialization；`execute` 强制调用方提供 fail-closed validator seam，缺失时不写入；内部 service，尚无通用模型工具 |
| lifecycle move | 当前 filesystem 无 delete/rename，明确 unsupported，不用残留 source 文件伪装 move |
| Worker 指令与上下文注入 | 尚未接线；独立 Worker 必须报告 `unsupported`，不能静默继承父上下文 |

## 锁协议（`acquireLock`）

dsh `ctx.fs` 目前没有 delete/remove 原语，锁不能像 `storage.md` 那样删目录释放，因此用**文件 +
租约 + tombstone** 实现，语义对齐 `storage.md`：

- **独占获取**：`createIfAbsent` 写 `{ owner, acquiredAt, expiresAt, state: 'held' }`。
- **正常释放**：release 把锁写成 `state: 'released'` tombstone（guarded replace，只覆盖自己
  拿到的那一版）。下一个 writer 看到 `released` 立即回收，**不必等租约过期**。
- **过期回收**：`storage.md` 明确「仅凭时间已经过去不能证明 owner 不活动」——已过期但仍 `held` 的锁
  只有在调用方通过 `canReclaim(lease)` 确认原 owner 确已不活跃后才被回收；**不提供
  `canReclaim` 时绝不自动强抢**，超时后 surface conflict。回收走 `replaceIfVersion` CAS，
  并发回收者只有一个能赢。
- 调用方在授权回收后，仍须按 `storage.md` 在 transaction / Evidence 里记录「原 owner、过期
  时间、回收者、时间戳」。

## 与 `storage.md` 的已知偏差

- **锁是文件而非目录**：`storage.md` 规定锁是 `.xiaotao/locks/<key>.lock/` 目录，但 dsh `ctx.fs`
  没有"创建目录"原语，适配层用同名**文件**实现，排他语义等价。
- **锁靠 tombstone + 租约而非删除释放**：因无 delete 原语，release 是写 `released` tombstone
  而非删锁；回收过期锁依赖调用方 `canReclaim` 确认 owner 不活跃，符合 `storage.md` 的保守回收
  要求。

## 目录结构

```text
adapters/deepseek-harness/
├── build.mjs
├── cordis.patch.yml
├── package.json
├── tsconfig.json
├── tsconfig.build.json
├── README.md
├── scripts/
│   ├── install-local.mjs
│   ├── install-local.test.mjs
│   └── check-package.mjs
├── lib/             # 构建产物，不入 Git；发布包中包含
│   ├── index.js
│   ├── index.d.ts
│   └── core/        # 打包时从 ../../xiaotao/ 复制
└── src/
    ├── index.ts        # 唯一插件入口，唯一 import dsh API 处；注册 Core skill + 提供 storage service
    ├── types.ts        # Config 与共享类型
    ├── detect.ts       # capability detection + fallback 决策
    ├── skill.ts        # 产物 A：注册 Core 为 dsh skill
    ├── storage.ts      # 产物 B：ctx.fs 上的锁 / CAS 写协议（含 .xiaotao/ 边界强制）
    ├── transaction.ts  # 多文件 create/replace 逻辑提交、overlay、materialization 与恢复
    ├── validate.ts     # 产物 B：schema 校验
    ├── hooks.ts        # 产物 B：可等待的 turn-stopping 与上下文压力 checkpoint 触发器
    ├── storage.test.ts # storage / lock 单元测试（fake seam）
    └── hooks.test.ts   # hooks 监听原语单元测试（fake ctx）
```

## Checkpoint（自动配置项目）

安装更新后的 Adapter 并重启 DSH，即可在同一个 profile 中对多个项目使用 checkpoint，
无需填写项目路径。每次调用从当前 Agent 的 `session.header.cwd` 绑定项目，不使用启动
DSH 进程的目录，也不接受模型传入路径。缺少会话或绝对路径时拒绝操作。

默认恢复目录为 `${DSH_HOME}/xiaotao-recovery/<项目标识>/`，未设置 `DSH_HOME` 时使用
`~/.dsh/xiaotao-recovery/<项目标识>/`。项目标识是文件系统规范 target key 的 SHA-256，
同名但路径不同的项目不会共用恢复目录。默认仍是显式保存；自动触发需要单独配置。

写入使用当前会话的 sandboxPolicy，保留会话只读等限制。默认外部恢复目录若被沙箱拒绝，
自动退回项目内的 `references/checkpoints/` 恢复记录，`status` 返回 `recovery: project`；
不会自动扩权。显式配置的 recoveryRoot 写入失败仍会报错，不静默忽略。权限拒绝返回
`filesystem_permission_denied`。`save` 缺字段时返回 `missing_fields`，不要通过删除 request_id
或改用绝对 source_refs 绕过校验。

现有 profile 的空补丁 `[]` 可以保持原样。可选覆盖如下：

```yaml
- id: xiaotao-adapter
  config:
    checkpoint:
      recoveryRoot: 'E:\xiaotao-recovery' # 自动模式下是基目录，下面再按项目隔离
```

### 自动触发（M2，实验性）

DSH 当前没有可等待的 pre-compaction / Session End Hook。Adapter 不把 `turn-stopping` 冒充成
pre-compaction，而是在宿主能报告上下文压力时，把真实
`agent/turn-stopping` 作为提前量触发点：达到阈值且存在新进展后，向当前 Agent 加入一个专用步骤，
由 Agent 复用同一个 `xiaotao_checkpoint inspect/save/status/retry` 工具完成保存。

压力事实优先读取 DSH token-meter 的 `contextPressure.projectedTokens / contextWindow`，与 DSH
占用率和 compaction 使用同一口径；projection 不可用时才退回最近一次 provider prompt usage
（`inputTokens + cacheReadTokens + cacheWriteTokens`，不含输出）除以请求的 context window。

该能力默认关闭。先在可丢弃项目中启用：

```yaml
- id: xiaotao-adapter
  config:
    checkpoint:
      auto:
        pressureThreshold: 0.72 # 实验起点，不是通用安全值；允许 0.5–0.95
        cooldownTurns: 2       # 同一状态去重后，再限制提醒间隔
        timeoutMs: 1000        # Hook 最多等待 50–5000ms
```

触发条件同时满足才会增加 checkpoint 步骤：

- 最近请求用量可计算，且达到配置阈值；
- 上次成功 checkpoint 或自动提醒之后，出现新的用户输入或非 checkpoint 工具结果；
- 已经过冷却 turn 数；
- 当前 turn 未取消。

成功的 `save/retry` 从 DSH 持久 Session 日志识别，重启后仍可去重。同一自动提醒也写入带稳定来源的
Session 事件，模型忽略或保存失败时不会在同一 turn 无限循环。失败仍通过 M1 工具结果、持久 request
和 `status/retry` 暴露，不虚报成功。宿主未提供 context window、usage、agents、fs 或 tools 时明确
降级，不自动触发，裸 Core 和显式 checkpoint 继续可用。

这不是 transcript 备份，也不保证卡在宿主压缩阈值之后仍来得及保存。阈值必须通过真实模型逐步
校准。自动提醒只负责“什么时候检查”；目标选择、事实摘要和是否值得保存仍由 Core/当前 Agent 决定，
不能创建新 Task、恢复无关任务或扩大权限。能力矩阵见
[自动 checkpoint 架构说明](../../docs/architecture/automatic-checkpoint.md)。

设 `checkpoint: false` 可禁用工具。原有固定项目配置仍兼容，指定 `projectRoot` 时
`recoveryRoot` 保持原来的精确目录语义（可省略）：

```yaml
checkpoint:
  projectRoot: 'E:\projects\my-app'
  recoveryRoot: 'E:\xiaotao-recovery'
```

要从旧固定配置切换为自动模式，删除 `projectRoot`，或删除整个 checkpoint 覆盖配置。
旧恢复文件不会自动迁移；处理旧的待重试请求时保留原配置。

恢复目录及自动模式的恢复基目录必须位于项目之外，不能与项目互相包含。它保存有界快照、
目标绑定及完整提案的写前副本，不是新的 Memory 层，也不会自动备份整个 transcript。
同一磁盘/后端仍可能同时故障，路径分离不等于故障域独立。目录应由部署者配置访问权限和保留期；
工具不会自动清理、扩权或将源材料发送到外部服务。

宿主必须同时提供 `fs`、`tools`，并加载带 checkpoint schema 的 Core。满足条件后默认注册
`xiaotao_checkpoint`，保持原生 tools 审批/取消管线；缺少能力时日志说明未启用，裸 Skill 继续运行。
固定模式下，调用 Agent 的 `session.header.cwd` 必须与配置项目的 canonical root 一致。
自动模式每次从该会话重新绑定项目，写入仍受同一 canonical root 的边界检查。

操作顺序：

1. `inspect`：提供 `kind`（temporary/task）、`target_id`，获得当前内容、revision 与 base_hash。
2. `save`：额外提供稳定 `request_id`、`base_revision`、`base_hash` 和 `snapshot`。快照字段为
   objective、confirmed、rejected、in_progress、next、open_questions、source_refs；除 objective
   为字符串外，其余为字符串数组。总快照限制 16 KiB；source_refs 为存在的项目内相对路径。
3. 出错后用相同 kind/target_id/request_id 调用 `status` 或 `retry`。`failed` 可能发生在提交后，
   不要因未收到成功结果就再次生成新请求。`recovery` 表示当前已核验来源，`none` 不保证可补存。
   save/retry 还会尽力落盘失败原因，`failure_recorded` 表示该诊断是否确认保存；取消时停止新增写入。
4. 成功后按 Core 协议刷新 Memory catalog。此工具不回滚已提交状态来处理 catalog 错误。

Temporary 更新既有 current.md，Task 更新既有 progress.md；保留用户正文，仅替换 managed
checkpoint 区段并推进 revision。完整请求保存在目标 references/checkpoints/，原子状态替换是
提交点，随后发布 committed observation。不同请求之间的 pending 不会被自动清除；conflict
需要重新查看当前事实，旧 pending 保留作为证据。

Checkpoint 本身不支持 Worker 目标、宿主原生 pre-compaction 接线、transaction overlay 或 Task 晋升。
虽然 Adapter 已提供内部 transaction service，checkpoint 尚未接入它；因此 checkpoint 看到任何
transaction bundle（即使已完成）时仍保守拒绝写入，空 transactions 目录允许。不要删除事务记录来绕过限制。
进程被强制终止后若留下 held 锁，必须先由已有存储恢复流程核验 owner 已失活；工具不按过期时间
抢锁。测试验证的是机制和真实 ToolRuntime 管线，完整 DSH 模型提供方/磁盘后端的人工验收仍待完成。

## Core Guard 与 Memory Worker 宿主映射

### Core Guard 协议
在长 Session、Turn Guard 或会话恢复时，避免重新注入完整的 `SKILL.md`（数千 tokens）和 references。
适配器导出 `CANONICAL_CORE_GUARD_PROMPT`、`CORE_GUARD_MAX_CHARS`（1200 字符，约 250~300 tokens）以及校验辅助
`validateCoreGuardText`。Core Guard 固化小涛角色、显式授权、有界 Worker 提案约束和四层渐进检索四大不可逾越底线。

### Memory Worker 映射状态
当前 DSH preview 版仅通过 `ctx.agents` 提供生命周期钩子，未暴露原生可隔离的 subagent 运行时。
因此本适配器如实将原生 Memory Worker 标记为 `unsupported`。当需要执行经验审查与记忆合并时，
遵循 Core 的 **In-Session Fallback** 协议：
- 由当前 Agent 在当前会话中执行有界压缩；
- 严格遵循只读工具白名单、`memory-worker-request.schema.json` 有界输入与 `memory-worker-response.schema.json` 候选提案约束；
- 候选仅限 `UPDATE`、`MERGE`、`CREATE`、`SKIP`，严禁自我批准或直接改写正式条目；
- 在执行记录中明确标记 `execution: in-session-fallback`，严禁虚构独立派工。

## 测试命令

```sh
cd adapters/deepseek-harness
npm install
npm run typecheck   # tsc --noEmit
npm test            # tsx --test src/*.test.ts
npm run test:package
```

自动化测试用内存 Session 事件覆盖压力计算、冷却、去重、成功观察、取消和有界等待；真实 DSH
模型与持久后端仍需按人工验收清单验证。

## 落地顺序

1. ~~产物 A：skill 注册~~（本 PR）
2. ~~detect 骨架 + fallback~~（本 PR）
3. ~~storage 的锁 / CAS 写 + `.xiaotao/` 边界强制~~（本 PR，含单测）
4. ~~hooks 的 turn-stopping 监听原语（正确 await）~~
5. 已接通显式启用的 snapshot checkpoint 工具；通用 storage / validator 工具仍属后续
6. 已接通可选的上下文压力自动 checkpoint；真实宿主验收后再判断默认策略
7. ~~create/replace 事务机械层（逻辑 commit、overlay、materialization、重启恢复）~~；具体窄业务路径与 lifecycle move 后续
8. 宿主未来提供 pre-compaction / Session End 时再接原生生命周期，不使用假事件
9. 将 Worker Delegation Packet 映射到 dsh subagent 指令、上下文、工具与权限隔离，并返回真实
   `supported` / `degraded` / `unsupported` 状态——后续

[issue-14]: https://github.com/IHongTaoI/xiaotao/issues/14
