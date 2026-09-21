# 项目存储

XiaoTao 状态属于目标项目，绝不能写入已安装 Skill。执行状态统一写入
`workers/<worker-id>/`；不读取、迁移或恢复旧 Role 目录。

## 最小布局

按当前工作需要延迟创建目录：

```text
.xiaotao/
  config.yaml
  locks/
  transactions/
  activity/
    index.json
  memory/
    manifest.md
    index.json
    followups/
      pending/<followup-id>.yaml
      resolved/<followup-id>.yaml
    temporary/
      active/<temporary-id>/
        meta.yaml
        current.md
        handoffs/
        references/
        worker-selections/
        workers/<worker-id>/
          spec.yaml
          current-state.md
          references/
          runs/<run-id>/
            delegation.json
      archive/
      trash/
    pending/
    sources/<source-id>.json
    long-term/
      current.md
      entries/<entry-id>.md
      history/<entry-id>.md
      candidates/
        pending/
        approved/
        rejected/
      decisions/<decision-id>.decision.json
      conflicts/
      migrations/<migration-id>/
  tasks/
    <task-id>/
      task.yaml
      context.md
      decisions.md
      progress.md
      evidence/
      artifacts/
      handoffs/
      worker-selections/
      workers/<worker-id>/
        spec.yaml
        current-state.md
        references/
        runs/<run-id>/
          delegation.json
    archive/
  workers/
    registry.yaml
  instructions/
    registry.yaml
  playbooks/
    candidates/
    decisions/
```

项目可以在首次使用 XiaoTao 前自行提供 `playbooks/` 目录。

## Git 跟踪边界

为减少 Git 冲突噪声并在分支之间同步共享知识，XiaoTao 将本地执行状态与团队共享 memory 分开：

- **本地 Runtime 状态（不纳入 Git）：**
  - `memory/manifest.md` 和 `memory/index.json`：由正式 Memory 来源重建的派生 awareness 目录；
  - `memory/temporary/`：Task 前探索、scratchpad 和活动 Worker 状态；
  - `memory/pending/`：尚未压缩或解析的 Memory Worker 原始输入；
  - `locks/` 和 `transactions/`：并发和文件系统锁标记；
  - `tasks/`：本地活动 Task 执行状态、Worker current-state 和临时选择。
  - `activity/index.json`：从本地 Task 和不可变 Decision 权威记录重建的派生时间线目录；

- **团队共享 Memory（纳入 Git）：**
  - `memory/sources/`：仅在聊天是唯一证据时保存用户明确授权持久化的、内容寻址且不可变的来源记录；
  - `memory/long-term/`：`entries/`、`history/`、轻量 `current.md`、`candidates/`、`decisions/`、
    `conflicts/` 和显式迁移审计；
  - `memory/followups/`：团队共享决策待办跟进项（`pending/` 与 `resolved/`）；
  - `playbooks/`：已批准的团队指导，以及已评审 `candidates/` 和 `decisions/`；
  - `workers/registry.yaml`：已评审、项目共享的可复用 Worker 规格；
  - `instructions/registry.yaml`：已评审的项目指令引用；不得覆盖内置 refs；
  - `config.yaml`：共享项目配置，可配置 `temporary_stale_days`（默认 7 天）等设置。

目标项目可以使用标准 `.gitignore` 规则强制此边界：

```gitignore
# 排除本地 XiaoTao runtime 状态
.xiaotao/memory/manifest.md
.xiaotao/memory/index.json
.xiaotao/memory/temporary/
.xiaotao/memory/pending/
.xiaotao/locks/
.xiaotao/transactions/
.xiaotao/tasks/
.xiaotao/activity/

# 跟踪团队共享 memory 与配置
!.xiaotao/
!.xiaotao/config.yaml
!.xiaotao/playbooks/
!.xiaotao/workers/approvals/
!.xiaotao/workers/registry.yaml
!.xiaotao/instructions/registry.yaml
!.xiaotao/memory/long-term/
!.xiaotao/memory/followups/
!.xiaotao/memory/sources/
```

内置 Worker 注册表是不可变的已安装参考数据。只有需要可复用的项目特定 Worker 或能力别名时，
才创建项目注册表。选中的 Worker 规格复制到匹配的 Task 或 Temporary 中，执行时绝不通过引用
再次解析。

内置指令注册表同样是不可变已安装参考数据。经评审的项目指令注册表可以扩展它，但不能替换内置
引用。执行前，持久化每个 Task 或 Temporary 作用域运行经过校验的 `delegation.json`；其中记录
准确的注入指令摘要、上下文引用、工具、有效权限和 Host Adapter 支持状态。Session 作用域 Packet
保持 ephemeral。

解析后的最小项目注册表形态如下：

```yaml
schema_version: 1
id: project-workers
source: project
revision: 0
updated_at: 2026-08-27T14:25:00Z
updated_by: xiao-tao/session-or-run-id
aliases: {}
workers: []
```

每个可复用 Worker 都是 [workers.md](workers.md) 定义的完整规格。根据历史 Task 证据提升并经
评审的 Worker 使用 `source: learned`；它仍是普通注册表条目，不获得额外权限。使用
[worker-registry.schema.json](schemas/worker-registry.schema.json) 校验解析后的注册表。

Long-term 当前知识以 `entries/<entry-id>.md` 为权威，一条 entry 一个文件。每个文件拥有独立
revision 和按规范状态路径派生的独立 lock；措辞更新不改变 ID，只有不可变决策能将其退役。
`current.md` 新格式只保存固定说明，不是聚合权威内容。已取代或拒绝的 snapshot 放入 `history/`，
并继续由不可变 decision/conflict/source refs 审计。每个通过校验的
Memory Worker 提案（包括 `SKIP`）都持久化到 `memory/long-term/candidates/pending/`，直到评审
记录批准或拒绝。未解决合并冲突存入 `memory/long-term/conflicts/`，状态为
`pending-confirmation`。`SKIP` 决策不修改 entry 文件，但应保留，避免没有新证据时重复评审
同一 duplicate 或 low-value 声明。

新批准的 `decision` 可以包含 [memory.md](memory.md) 定义的向后兼容 `decision_context`。更新和
语义合并必须保留它。它解释理由和已记录的否定方案，但绝不授予权限或替代不可变评审记录。

新评审以 `memory/long-term/decisions/<decision-id>.decision.json` 发布不可变 Decision Record。
文件名必须与 `decision_id` 一致，并显式记录 `outcome`、`importance`、`decided_at`、评审者、理由、
目标及可达来源。`superseded` 还必须记录 `superseded_by`。旧格式无需迁移；没有可靠 `decided_at`
的历史记录不进入 Activity，也不得从更新时间或文件时间推断。

发布 Decision 时必须确认 `source_refs` 当前存在；后续读取和 Activity 重建只验证引用是安全的项目
相对路径，不再要求历史证据当前存在。这样本地 Task 归档或换机器后不会使团队共享的不可变记录
失效。需要长期下钻的重要证据应优先放在 Git 跟踪且路径稳定的位置。

Long-term 条目使用 [memory.md](memory.md) 定义的 fenced `xiaotao-memory-entry` JSON 表示，使
确定性目录构建器拥有可寻址记录边界。旧聚合 `current.md` 保持可读，但新写入只进入 entry 文件；
同一 ID 跨旧聚合、`entries/`、`history/` 重复时失败。生成的
`memory/manifest.md` 和 `memory/index.json` 是本地缓存，不通过 Git 共享，也不参与可变状态
revision 协议。先发布正式 Memory 改动，再原子重建目录。目录失败绝不回滚已提交的正式写入。

每个当前 Playbook 向 Experience Review 暴露稳定 `playbook_id`、规范 `file_path`、标题、触发
条件、有序步骤、检查、active 状态、revision 元数据和可达 `source_refs`。每个通过校验的
Playbook Candidate（包括 `SKIP`）都持久化在 `playbooks/candidates/`，直到 `playbooks/decisions/`
下的不可变 Decision Record 批准或拒绝。该记录同样使用
[decision-record.schema.json](schemas/decision-record.schema.json)，文件名必须是
`<decision-id>.decision.json` 并与 `decision_id` 一致，`target_ids` 指向受影响的 `playbook_id`；
`decided_at` 是批准、拒绝或取代实际发生的时间，不得用 `updated_at` 或文件时间推断。候选记录包含
可达 `source_refs` 和 `evidence_refs`；它们不是生效指导，且在用户明确批准前不能修改 Playbook。

拒绝没有正式目标的 Playbook 候选时，Decision Record 允许 `target_ids: []`，并通过
`source_refs` 引用候选；批准和取代仍必须有目标，拒绝记录仍保留为审计。

## 配置

使用简短 `config.yaml`：

```yaml
schema_version: 1
models:
  primary: null
  memory: null
```

v1 不要虚构细粒度的逐 Worker 模型设置。memory model 为 null 表示使用宿主可用模型，或由当前
Agent 执行压缩。

## Temporary 路由元数据

每个活动 Temporary 的 `meta.yaml` 包含最小、宿主无关的路由协议：

```yaml
id: 20260831-首页启动性能
topic: 首页启动性能
status: active
created_at: 2026-08-31T10:30:00Z
updated_at: 2026-08-31T11:05:00Z
updated_by: xiao-tao/session-or-run-id
revision: 12
aliases:
  - home startup performance
  - 首屏启动慢
last_session_id: optional-stable-host-session-id
```

必需字段为 `id`、`topic`、`status`、`created_at`、`updated_at`、`updated_by` 和 `revision`。
`id` 必须与 Temporary 目录名一致，`status` 必须与生命周期位置一致。`revision` 是非负整数，
并按下方可变状态协议递增。`aliases` 是标识同一主题的可选用户名称列表。宿主不暴露稳定、非
敏感 Session 标识时必须省略 `last_session_id`。它只是恢复提示，不是权威路由状态；多个候选
包含同一 ID 时，仍应用普通歧义规则。

不要向元数据加入 embeddings、模型评分或宿主特定路由对象。路由可以解释 `topic`、`aliases`
和 `current.md`，但置信度是针对当前请求作出的决定，不是持久事实。

使用 [temporary-meta.schema.json](schemas/temporary-meta.schema.json) 校验解析后的元数据。

## Task 元数据

每个 `task.yaml` 只包含执行、恢复、生命周期和冲突控制字段：

```yaml
id: 20260831-优化登录流程
objective: 优化登录流程
status: active
created_at: 2026-08-31T12:00:00Z
updated_at: 2026-08-31T12:03:00Z
updated_by: xiao-tao/session-or-run-id
revision: 1
source_temporary: 20260831-登录流程梳理
promotion_transaction: 20260831T120000Z-p7q8r9
promoted_at: 2026-08-31T12:00:15Z
```

`source_temporary` 仅在 Task 由 Temporary Memory 提升时存在。根据明确执行请求直接创建的 Task
省略它。提升后的 Task 还记录 `promotion_transaction`，其事务提交标记控制 Task 初始可见性。
使用 [task.schema.json](schemas/task.schema.json) 校验解析后的 Task 元数据。

`promotion_transaction` 是事务关联、恢复与审计标记，不是事件时间。它在事务打开时写入，因此
早于晋升生效；晋升真正生效的逻辑可见性边界是原子发布 `committed.yaml`。该标记的形状由产生
它的事务决定，Activity 不解释其中任何部分。

提升事务提交时，必须在同一次生命周期更新中写入一次 `promoted_at`，并在后续移动到
`tasks/archive/` 时保持不变。`promoted_at` 是 Activity 中“Temporary 晋升为 Task”的可靠发生
时间，记录晋升生效的登记时刻。为兼容旧项目，它不是 schema 必填字段；旧提升 Task 缺失时仍可
读取，但不产生晋升事件，也绝不用 `promotion_transaction`、`created_at`、`updated_at` 或文件
mtime 代替真实时间。已提交的提升会把 Task 实体化为 `active`，所以 `preparing` 状态不投影
晋升事件。

新 Task 进入 `completed` 或 `archive` 终态时，必须在同一次生命周期更新中写入一次
`completed_at`，并在后续移动到 `tasks/archive/` 时保持不变。`completed_at` 是 Activity 中
“Task 已完成”的可靠发生时间。为兼容旧项目，它不是 schema 必填字段；旧 Task 缺失时仍可读取，
但不会出现在 Activity 中。不得使用 `updated_at` 或文件 mtime 猜测完成时间。

Activity 还会从 `memory/long-term/decisions/` 与 `playbooks/decisions/` 直属的规范
`*.decision.json` 文件分别派生 `decision_approved` / `decision_superseded` 与
`playbook_approved` / `playbook_superseded`。只有 `importance: milestone` 的批准或取代记录进入
时间线；`routine` 和 `rejected` 仅供审计。事件时间只取不可变的 `decided_at`，事件引用指向该
Decision Record。

## Temporary 与 Task ID 命名

Temporary 和 Task 目录使用从 `topic`（`meta.yaml.topic`）或 `objective`（`task.yaml.objective`）
派生的可读、文件系统安全 ID，不使用不透明的时间戳加随机后缀。ID 必须与目录名和
`meta.yaml.id` / `task.yaml.id` 值一致。

### 格式

```text
<yyyymmdd>-<slug>
```

- `<yyyymmdd>` 是目录创建时的 UTC 日期。
- `<slug>` 是简短、可读的主题名：
  - 保留字母（包括 CJK）、数字、`-`、`_` 和 `.`；
  - 删除空白、路径分隔符（`/`、`\`）、文件系统不安全字符（`: * ? " < > |`）、控制字符，
    以及开头或结尾的点；
  - 合并重复分隔符。slug 不得以 `.` 开头或结尾，以避免 `.`、`..` 及宿主文件系统静默删除
    尾点的名称；
  - 保持简短；主题过长时截断，不要带入整个句子；
  - 生成和比较 slug 时使用 Unicode **NFC** 规范化：相等检查前规范化双方（目录名等于
    `meta.yaml.id` / `task.yaml.id`）。macOS HFS+ 等文件系统以 **NFD** 存储文件名，所以分解
    slug（例如假名后跟组合浊音符 U+3099）会以 NFD 显示；该组合符是 `Mn`，不在 ID 校验器
    allow-list 中，因此应拒绝而不是静默错配。生成时保持 NFC 即可避免该问题。

示例：

```text
20260831-首页启动性能
20260831-优化登录流程
```

### 重名处理

目标路径已存在时，从 `-2` 开始追加递增数字后缀：

```text
20260831-首页启动性能
20260831-首页启动性能-2
20260831-首页启动性能-3
```

绝不能覆盖现有目录；陈旧读取不得重复使用已经占用的名称。

### 稳定性与兼容性

- ID 创建后保持稳定。之后改写 topic 或 objective 时只更新 `meta.yaml` / `task.yaml`，不重命名
  目录。
- Temporary 在 `active`、`archive`、`trash` 间移动时保留原始 `<yyyymmdd>-<slug>`。
- 旧的 `<utc-timestamp>-<random-suffix>` 格式（例如 `20260827T103000Z-a1b2c3`）继续可读、
  可恢复。不要批量迁移现有目录，也不要仅因 ID 早于本规则就拒绝。
- Schema 和校验器接受两种格式；只要求 ID 非空、文件系统安全，并与目录名一致。
- 存储 Schema 以 `^(?!\.)(?!.*[.]$)[\p{L}\p{N}._-]+$` 编码 allow-list。`\p{...}` property
  escapes 依赖 ajv 默认的 `unicodeRegExp: true`（`u` flag）；否则字符类会静默退化并拒绝全部
  CJK ID，因此必须启用 `unicodeRegExp`。`validate.py` 使用 `str.isalnum()` 镜像同一集合。
- JS 与 Python 搭载的 Unicode 版本不同，接受范围只等于两张表的交集：较新 Unicode 才分配的
  字符（例如 U+105C0 Todhri）会被 Schema 接受、被 `validate.py` 拒绝。CJK 与拉丁 slug 不受
  影响。

## 可变状态写入协议

可变状态包括 Temporary `meta.yaml` 和 `current.md`，Task `task.yaml`、`context.md`、
`decisions.md`、`progress.md`，Worker `current-state.md`，项目 Worker `registry.yaml`，Long-term
`entries/<entry-id>.md` 与 `history/<entry-id>.md`，以及每个规范正式 Playbook Markdown 或 YAML 文件。列出的可变 YAML 文件都携带
`revision`、`updated_at`、`updated_by`；Markdown 在 YAML front matter 携带相同字段。新状态从
revision `0` 开始，每次成功替换只递增一次。Handoff、Detailed Result、source References、
Evidence、决策记录、Task 或 Temporary `worker-selections/` 中的 Worker 选择，以及事务事件一经
发布即不可变；需要变更时新增链接记录，而不是覆盖。

Task 或 Temporary 下的 Worker `spec.yaml` 是不可变快照。首次运行前，以原子方式发布完整、
通过校验的快照。项目注册表更新不能替换快照，恢复时也不能用当前注册表条目代替缺失快照。
Session 作用域 Worker 不是项目状态，不留下快照。

原子替换能防止读取半截文件，但不能防止陈旧写入。每次替换都使用完整协议：

1. 读取状态，将其 revision 保存为 `base_revision`。
2. 从规范化项目相对状态路径派生稳定 lock key。以原子方式创建对应的
   `.xiaotao/locks/<state-key>.lock/` 目录，在其中写入持有者、获取时间和 lease 到期时间。
   create-if-absent 必须互斥。
3. 取得锁后重新读取状态。revision 与 `base_revision` 不同就不要写入；释放锁并加载较新状态，
   将非冲突事实连同双方 source paths 一并协调，或向指定 owner 返回可见冲突。
4. revision 仍匹配时，校验完整替换内容，设置 revision 为 `base_revision + 1`，更新
   `updated_at`、`updated_by`，并原子替换文件。
5. 重新读取足够的元数据，确认已提交 revision，再释放锁。

遇到锁竞争或 revision 不匹配时绝不强制写入。只在宿主合适的有界时间内重试竞争。只有已知所
记录 owner 不再活动时才可回收过期锁；在事务或 Evidence 记录先前 owner、到期时间、回收者和
时间戳。仅凭时间已经过去不能证明 owner 不活动。

协调会从新加载的 revision 生成候选替换，然后重新执行完整协议。它不是在锁外写入或跳过另一次
revision 检查的许可。

宿主不能保证原子互斥锁创建时，把该项目的所有写入交给一个 writer。互斥锁和单 writer 串行化
都不可用时，报告不支持并发修改，不要执行写入。

一项操作修改多个可变文件时，按规范化状态路径的字典序取得所有锁，在锁下重新检查全部 base
revision，并在改变规范状态前准备以下不可变事务包：

```text
.xiaotao/transactions/<transaction-id>/
  intent.yaml
  before/<state-key>
  staged/<state-key>
  applied/<sequence>.yaml
  committed.yaml | failed.yaml
```

`intent.yaml` 记录操作和 actor，并为每个创建、替换或生命周期移动记录：规范 source/target
路径、base/intended revisions、before/staged 快照路径和 SHA-256 hashes。完整字节存入
`before/` 与 `staged/`；只有引用指向不可变、可达且记录 hash 相同的内容时，才可替代复制字节。
不存在的路径使用明确 `before: absent`。发布终态事件前，校验每个 staged 文件并使整个事务包
持久化。

终态事件是逻辑可见性边界：

- 只有 `intent.yaml` 时，before 视图仍为权威。staged creates 和准备中的 Task 不可运行，且对
  常规路由或恢复不可见。
- 原子创建 `committed.yaml` 会一次性把完整逻辑视图切换到全部 staged 内容。此时生命周期移动
  即已生效，即便规范目录尚未重新排列。
- `failed.yaml` 只能在 commit 前发布，并保持 before 视图权威。一旦 committed，必须完成操作，
  不能回滚。

终态标记必须互斥创建。若两个标记同时存在，视事务已损坏并停止，等待明确恢复；不得按时间戳
任选其一。

提交后，使用常规原子替换规则把 staged 内容实体化到规范路径。每次文件替换或移动后，在
`applied/` 追加不可变事件，包含路径、staged hash、actor 和 timestamp。保持锁直到常规实体化
完成，再按反向顺序释放。中断后，恢复 writer 重新获取相同锁，并将每个规范路径与 intent 比较：

- before hash 或预期 absence 表示操作仍 pending，可以应用 staged 内容；
- staged hash 表示已经应用；缺失的 applied 事件可以重建；
- 两种快照 hash 都不匹配表示并发冲突；停止并报告，不得猜测。

正常路由、恢复或修改前，reader 和 writer 必须先解析影响目标状态的未完成事务。它们可以直接
读取事务 overlay，或完成实体化。因此，已提交的提升即使仍在清理，也会暴露 Task 并排除来源
Temporary；未提交的提升相反。不需要维护组不变量时，优先使用单文件更新。

旧聚合 Long-term 的拆分迁移是显式维护操作，不在普通 Session 自动运行。迁移前取得
`.xiaotao/locks/memory-long-term-migration.lock`；所有 Long-term writer 看到该锁时停止。工具先
完整校验旧文件和全部目标 entry，在隐藏 staging 目录准备单文件，保存旧聚合原文与 intent 审计，
再发布目录和轻量 `current.md`。发布后逐项验证 ID、内容、status、source refs 与 decision context
一致；失败则恢复旧聚合，不能报告成功。已有的不同 ID entry 保持字节和 revision 不变；同 ID
冲突在发布前失败。迁移审计不可替代正常 UPDATE/MERGE 的多文件事务。

## 文件规则

### 快照 checkpoint 记录

用户选择启用的 Adapter 可以把单个不可变请求保存到所选目标的
`references/checkpoints/<request-id>.json`。其中嵌入有界来源事实、来源 Session 标识、项目/目标
绑定、base revision/hash，以及准确替换 bytes/hash。任何替换前，先使用
[checkpoint.schema.json](schemas/checkpoint.schema.json) 和 Adapter 语义检查校验。Request ID
由小写 ASCII 字母、数字、下划线、连字符组成，长度 1–64，并以字母或数字开头。不同 payload
不能复用同一个 ID。

在当前状态变化前，以互斥方式完整发布请求。对 `current.md`（Temporary）或 `progress.md`
（Task）的单次原子替换同时提交摘要和 `checkpoint_receipt`；receipt 包含 `request_id`、
`source_hash`、`revision`。这不能替代多文件事务协议。通过验证的不可变
`<request-id>.committed.json` 记录 request hash、proposal hash 和 committed revision。新记录还按
[checkpoint-observation.schema.json](schemas/checkpoint-observation.schema.json) 保存
`completion`（`save` 或 `recovery`）以及首次发布时的 `committed_at`；旧四字段记录继续有效。
`completion` 由调用入口决定：只有显式 `retry` 写 `recovery`，重复 `save` 仍写 `save`。该标记
只是该请求完成的证据，不覆盖之后的编辑；已经存在的合法 observation 不得重写时间或 completion。

删除或替换 receipt 前，校验对应 request 和 committed observation。若确认过程中断，准确
proposal bytes 加匹配 receipt 可以修复 observation，而无需重复状态写入。当前状态与原始 base、
proposal 都不匹配时，报告冲突；不得推断成功或覆盖新工作。没有已验证 commit 的请求保持
pending。恢复不得静默取代或删除它们。

宿主配置可以明确选择外部 write-ahead 恢复目录。它保存同一份有界请求以供故障恢复，不是
Long-term Memory，也不能由模型选择。没有独立可达副本时，项目存储故障可能导致无法恢复；
所有渠道失败时报告该限制。恢复总要重新检查选中目标的当前生命周期、权限和来源可达性。仅凭
时间已经过去不得回收锁。

### 通用文件规则

项目 Worker registry 批准记录位于 `.xiaotao/workers/approvals/`。写入方先校验目标 Worker、当前
registry revision 和规范 Worker 摘要，再以互斥创建发布 `<approval-id>.approval.json`；目标已存在
即停止，不得原子替换。批准时间记录逻辑提交实际生效边界。批准记录一旦发布即独立保留历史事实，
后续 registry revision 不参与其读取时校验。

- 所有写入都必须解析到选定项目的 `.xiaotao/` 目录内。
- 拒绝 `../` 等路径穿越，不把传入的绝对路径用作状态目标。
- 按“Temporary 与 Task ID 命名”使用从 topic 或 objective 派生的稳定、文件系统安全 ID；即使
  存储 ID 使用旧时间戳加随机后缀格式，也必须接受。
- 人工维护状态优先用 Markdown，小型元数据/配置使用 YAML。
- 可变状态只能按上述 revision 与 lock 协议写入，并在冲突检查通过后使用宿主最安全的原子替换
  机制。
- 保留现有无关内容和用户编写的 Playbooks。
- 绝不能因为已经压缩就删除活动 Task、Temporary Memory、Evidence 或 Artifact。按用户意图移入
  Archive 或 Trash。

## 状态转换

存储转换不定义业务工作流。允许的生命周期移动为：

- Temporary `active` → `archive` 或 `trash`。
- 用户明确确认后，Temporary `active` → 正式 Task，然后 Temporary → `archive`。
- Task active → 完成后的 `archive`。
- Long-term candidate `pending` → 评审后的 `approved` 或 `rejected`。
- Playbook Candidate `candidate` → 用户明确评审后的 `approved`、`rejected` 或 `superseded`。

批准的 Long-term `CREATE` 创建 `entries/<entry-id>.md`，从 revision `0` 开始；`UPDATE` 只锁定
并替换一个目标 entry，保留 ID 且 revision 准确递增一次。`MERGE` 更新一个获批 survivor，并把
其他目标以各自递增的 revision 移入 `history/`，同时发布不可变 superseded 决策；它属于多文件
事务，必须按路径排序获取所有锁并通过同一 transaction 发布。`SKIP` 只记录决策，绝不创建 entry。
不同 entry 的独立 UPDATE 不共享 revision 或 lock。

批准的 Playbook `UPDATE`、`MERGE` 或 `CREATE` 对受影响 Playbook 文件使用同一套 lock、revision
和 transaction 规则。`CREATE` 分配一个稳定 `playbook_id`，从 revision `0` 开始。`UPDATE`
保留目标 ID 和路径，并准确递增该文件 revision 一次。`MERGE` 指定一个获批 survivor，保留其
ID 并递增 revision；其他目标文件以各自递增的 revision 标为 `superseded`，并在不可变决策中
以 `superseded_by` 记录 survivor。按路径字典序获取并重新检查所有目标锁，通过同一事务发布
多文件变化。`SKIP` 和 rejected 候选只记录决策。重复成功的 Task 可以通过经评审更新追加证据，
但不能批准候选。

移动目录或文件前，记录转换、时间戳、actor/reviewer、适用时的理由和来源路径。
