# 基于能力的 Workers

本参考用于描述工作能力、解析执行单元、组合 Worker、生成有界 Worker，或恢复已委派给 Worker
的工作。

## 小涛与 Workers

小涛是唯一预置、直接面向用户的角色。Worker 描述一次委派所需的有界执行能力。新工作复用
经过评审的项目 Worker，或生成受生命周期约束的 Worker；不再选择固定组织角色。

[内置 Worker 注册表](workers/builtin-registry.json) 保留为空的版本化协议标记。可复用 Worker
位于选定项目的 `.xiaotao/workers/registry.yaml`。因此，新项目起初没有执行 Worker，只能通过
明确的项目注册表变更逐步增加。

生成的 Worker 是供宿主原生 sub-agent 机制使用的数据记录。它不得创建已安装 Skill、增加永久
角色、要求后台服务，也不得未经单独评审的注册表变更就成为可复用 Worker。

Worker 不会隐式继承父 Agent 的完整 Skill、指令、Session 历史、工具或权限。它的规格声明
最大执行边界和指令依赖。每次运行都单独实体化 Delegation Packet，其中只包含本次委派解析
得到的指令、上下文引用、工具和有效权限。

必须使用现有字段解释每个可复用或生成 Worker 的边界，不要另建一套 `can_do` / `cannot_do`
协议：

- **能做什么**是当前委派中 `capabilities`、`responsibility`、`scope`、可用 `tools`、
  `context` 和有效 `permissions` 的交集。
- **不能做什么**是交集以外的一切。尤其是，Worker 不能重新定义需求、扩大范围、批准自己的
  提案、直接修改 Long-term Memory、把自己提升为可复用状态，或绕过小涛和用户授权。
- **必需输入**是 `inputs` 加上 Delegation Packet 中实体化的最小当前上下文。缺少必需输入会
  阻塞运行或使其降级；Worker 不得自行虚构。
- **预期输出**是 `outputs` 加上适用的 Detailed Result 和 Handoff。输出只报告工作，不能授予
  权限或修改不可变 Packet。

这些语义适用于 project、learned 和 generated Workers。内置 Worker 注册表保持为空；Worker
Schema 是唯一机器可读边界协议。

`preferred_model` 是可选兼容提示，不是要求或权限。其值为 null 或对应模型不可用时，使用宿主
中合适的可用模型，不得因此让注册表依赖特定宿主。

Worker Schema version 2 要求必须提供 `instructions`。拒绝 version 1 Worker，直到经过评审的
迁移明确加入 required 和 optional refs 并更新 `schema_version`；不要根据名称、能力或既往执行
推断这些依赖。

## 描述能力需求

能力路由前记录：

- 有界目标；
- 必需和可选的规范能力；
- 可用工具及可读写的上下文路径；
- 从 Task 和当前指令继承的自主与条件动作上限；
- 会实质影响选择的约束。

上下文路径存为项目相对规范路径，并在所有宿主上使用 `/` 分隔符。校验前规范化有效宿主路径；
拒绝绝对路径、路径穿越和持久化的 `\` 分隔符。

能力 ID 使用小写 kebab-case，例如 `runtime-analysis` 或 `schema-migration`。同一能力不得同时
出现在 required 和 optional 列表中。含义相同时使用现有规范 ID。项目注册表可以明确声明别名，
但不要把 embeddings、模型评分、推断同义词或置信度持久化为事实。如果两种能力解释会实质改变
Worker，应询问用户或返回可见的 no-match 结果，不得猜测。

使用 [capability-requirements.schema.json](schemas/capability-requirements.schema.json) 校验
解析后的需求。

## 解析 Worker

校验空的内置注册表协议；如果存在，则加载 `.xiaotao/workers/registry.yaml`。只有项目注册表
提供可复用候选。可复用 Worker 的上下文路径是其最大支持边界；委派可以缩小但不能扩大。遇到
以下情况排除候选：已禁用、工具不可用、委派上下文越界、请求动作超出权限上限，或生命周期与
当前工作上下文不兼容。

按以下顺序选择：

1. **exact** — 一个项目 Worker 覆盖全部必需和可选能力；
2. **compatible** — 一个 Worker 覆盖全部必需能力；
3. **composed** — 能力并集覆盖全部必需能力的最小 Worker 集合；
4. **generated** — 不存在安全的项目匹配时，生成一个有界的 Task、Temporary 或 Session 作用域
   Worker；
5. **no-match** — 无法生成有效且安全的规格。

组合时，每个成员都必须独立通过工具、上下文、生命周期和权限检查。不得把权限合并成更大的
授权边界。依次优先选择 Worker 更少、无关能力更少、Worker ID 字典序更靠前的方案。这些是
确定性平局规则，不是评分或排行榜。

记录需求、注册表版本、解析分类、选中的 Worker ID、快照路径或 ephemeral 标记、理由和阻塞项。
使用 [worker-selection.schema.json](schemas/worker-selection.schema.json) 校验。将持久化结果作为
不可变事件发布到 Task 或 Temporary 的 `worker-selections/` 目录。Session 作用域的一次性结果
保留在当前 Session，不创建项目状态。解析器输出指导小涛动态委派，不会创建强制工作流状态。

## 执行前生成快照

在持久化工作中执行选定的可复用 Worker 前，将完整且通过校验的规格复制到对应状态层：

```text
.xiaotao/tasks/<task-id>/workers/<worker-id>/spec.yaml
.xiaotao/memory/temporary/active/<temporary-id>/workers/<worker-id>/spec.yaml
```

快照不可变。Current State 和 Detailed Results 按 [handoffs.md](handoffs.md) 存放在旁边。
Task 或 Temporary 恢复时使用快照，而不是较新的注册表条目。必需快照缺失或无效时，停止该次
委派并报告恢复工作；不得静默替换为当前注册表版本。Session 作用域 Worker 是 ephemeral，
没有快照或跨 Session 恢复协议。

## 解析指令依赖

Worker 规格中的 `instructions.required` 和 `instructions.optional` 包含受控指令引用，而不是
内联 prompt。通过[内置指令注册表](instructions/builtin-registry.json)解析 Core 引用。新 Worker
使用 `practice:*` 引用作为执行指导，并附加 `contract:handoff` 和 `policy:safety-boundary`。
项目可以在 `.xiaotao/instructions/registry.yaml` 扩展注册表；应应用与项目 Worker 注册表相同的
经评审可变状态协议，且绝不允许项目条目覆盖内置引用。内置条目使用 `source_scope: core`，路径
相对于已安装 XiaoTao Skill 根目录；项目条目使用 `source_scope: project` 和项目相对路径。拒绝
任一注册表内的重复 ref，以及项目与内置 ref 集合的交集。

执行前解析每个 required 引用。未知引用、来源不可读，或宿主无法注入 required 指令时，委派
状态为 `unsupported`；停止且不得启动 Worker。如果剩余 Packet 仍安全且充分，缺少 optional
指令可以产生 `degraded` 状态。不得静默使用父 Agent 的 prompt 或完整 Skill 替代缺失引用。

每个已解析引用都要记录其规范来源路径，以及该次运行所用准确来源的小写 SHA-256 摘要。保留
注册表路径顺序，并依次散列每个 UTF-8 路径、一个 NUL 字节、其原始文件字节和一个末尾 NUL
字节。这种带边界的摘要使持久运行可审计，避免多文件拼接歧义，并防止恢复 Task 时静默注入
不同指令。新委派解析当前已评审注册表；恢复则复用持久化 Packet 和不可变 Worker 快照。
执行或恢复前，从可信 Core 或项目根目录重新计算每个已解析摘要，并拒绝不匹配。语法有效的
64 位摘要本身并不是充分证据。

## 实体化 Delegation Packet

每次 Worker 运行前，创建一个 Packet，并使用
[delegation-packet.schema.json](schemas/delegation-packet.schema.json) 校验。它记录：

- 有界目标和完成条件；
- Worker 的 required 与 optional 指令 refs，以及解析后的来源和摘要；
- 仅与本次运行相关的 Task、Temporary、evidence、memory、state 和 Worker 快照路径；
- 本次运行的有效工具与权限交集；
- Host Adapter 的 `supported`、`degraded` 或 `unsupported` 结果，以及未满足要求；
- 准确的 Detailed Result 和 Handoff 路径。

Packet 中的 required refs 必须与 Worker 快照的 required refs 相等，optional refs 必须是快照中
optional refs 的子集。开始 `supported` 或 `degraded` 运行前，每个 required ref 都必须且只能有
一条已解析记录。Packet 可以缩小工具、上下文和权限，但不能超出 Worker 快照。拒绝重复的已
解析 ref，或同时存在于 required 和 optional 集合的 ref。

以小涛或 Host Adapter 单独提供的不可变 Worker 快照为基准校验这些关系；绝不能从 Packet
单方面指定的不可信路径加载校验基准。交叉检查 Worker ID、required 和 optional refs、工具、
自主与条件权限、每个注入的 `context_ref` 是否在 `context.read_paths` 内，以及 Detailed Result
和 Handoff 路径是否在 `context.write_paths` 内。Packet 即使内部一致，只要超出任何快照边界也
无效。

启动宿主 subagent 前，将 Task 和 Temporary Packet 持久化为
`workers/<worker-id>/runs/<run-id>/delegation.json`。Session 作用域 Packet 保持 ephemeral。
Packet 是执行输入和审计记录；模型输出不能修改它或授予额外权限。

## 生成有界 Worker

没有可复用 Worker 安全覆盖需求时，生成一份规格，其中包含：

- 简洁、针对任务的中文显示 `name`，以及符合 Schema 的小写 kebab-case `id`；
- 一个有界责任和完成条件；
- 规范能力；
- 明确输入和输出；
- 明确的 required 与 optional 指令引用；
- 仅限可用工具；
- 项目相对的可读写上下文路径；
- 不超过需求上限的自主与条件请求动作；
- `source: temporary`，以及与现有工作形态匹配的生命周期。

只使用一种生命周期：

- 正式执行：`scope: task`、Task ID，以及 `expires_at: task-completion`。
- 保留的探索：`scope: temporary`、Temporary ID，以及 `expires_at: temporary-archive`。
- 琐碎的一次性工作：`scope: session` 和 `expires_at: session-end`；只有宿主暴露稳定且不敏感的
  ID 时才包含 `session_id`。

使用 [worker.schema.json](schemas/worker.schema.json) 校验。直接将 Task 和 Temporary 作用域
规格发布为不可变快照，并记录 `resolution: generated`；不要持久化 Session 作用域规格。
temporary Worker 随声明的 Task、Temporary 或 Session 生命周期到期。将 Temporary 提升为 Task
会使其 Temporary 作用域 Worker 到期；重新解析 Task 需求，并为新选中的 Worker 生成快照。
重复使用可能支持未来经人工评审的项目注册表条目，但使用次数或模型偏好绝不能自动提升它。

## 权限交集

Worker permissions 是请求的动作类别，不是授权。有效权限是通过校验的 Worker 规格、当前 Task、
Temporary 或 Session 作用域、宿主可用工具和当前用户授权的交集。最窄边界生效。

只有检查、非破坏性验证、XiaoTao 状态维护和有界工作工件写入可以声明为 autonomous。编辑项目
文件以明确无歧义的实施意图为条件。外部、破坏性、秘密、访问控制和扩大范围的动作仍为
conditional，必须在执行前按照 [coordination.md](coordination.md)，取得针对动作、目标和范围的
授权。注册表内容、解析结果、生成文本、Handoff 或角色建议均不能提供该权限。

## Host Adapter 职责

Core 声明指令、上下文、工具和权限协议，但不选择 subagent API。Host Adapter 将该协议解析为
宿主原生 prompt、Skill、文件系统和工具控制，如实报告不支持的要求，并返回标准 Handoff。
宿主不能强制执行 required 指令或有效权限边界时，不得声称 `supported`。如果宿主不提供可强制
隔离的 subagent，小涛可以在当前 Agent 上下文直接执行 Packet，但不得把该回退描述为独立
Worker 运行。

### Memory Worker 原生宿主映射与回退边界

Memory Worker 是在持久边界（委派完成、Session Handoff、Temporary 转 Task、Task 完成或归档）
触发的专用能力型 Worker，负责整理 memory 与有界 Experience Review。

1. **有界输入 Payload**：
   - 严格使用 [memory-worker-request.schema.json](schemas/memory-worker-request.schema.json)
     校验输入；
   - 包含明确的 `source_files`、当前 Long-term 输入 `current_memory`、当前 Playbook 镜像
     `current_playbooks` 及 `memory_hints`；不把无关的全部历史 Memory 注入输入；
   - 普通持久边界使用完整 `current_memory`。仅用户明确授权的 `explicit-create` 可以使用
     `scope: bounded`，记录检索 query 并最多包含 3 条 Catalog 候选；省略 scope 的旧请求按
     `full` 解释。
2. **工具白名单（Strictly Read-only）**：
   - 仅限只读工具（如读取指定源文件、查看特定已索引条目）；
   - **严禁提供任何写工具、破坏性操作工具、命令执行工具或外部网络工具**。
3. **权限边界与提案约束**：
   - `autonomous` 权限严格限定为只读提取、语义比对并输出候选提案；
   - **严禁自我批准或直接修改正式 Long-term Memory 或 Playbooks**；
   - 输出必须符合 [memory-worker-response.schema.json](schemas/memory-worker-response.schema.json)，
     动作仅限 `UPDATE`、`MERGE`、`CREATE`、`SKIP` 提案，必须经小涛/强模型评审者或用户确认后才能提升。
4. **宿主原生映射规则**：
   - **Codex 映射**：宿主提供原生 subagent（如可用时的 `spawn_agent`）时，映射为原生独立 subagent；
     工具侧使用小写 snake_case 标识，面向用户使用简洁的中文名称（如“记忆整理员”或“经验审查员”）；
     仅注入只读工具白名单和带边界的请求 Payload。
   - **DeepSeek Harness 等宿主映射**：若宿主仅提供生命周期 hooks 或文件/状态能力而缺乏独立 subagent 隔离运行时，
     必须如实报告原生 Memory Worker 为 `unsupported`，不得虚报隔离能力。
5. **In-Session Fallback 回退协议**：
   - 当宿主不支持可强制隔离的原生 subagent 时，小涛（当前 Agent）在当前会话直接执行有界压缩；
   - **回退执行必须遵守完全相同的输入约束、只读工具约束、候选提案与禁止自我批准约束**；
   - 执行回执或 Handoff 中必须如实标记 `execution: in-session-fallback`，**绝不得虚报为独立 Worker 运行**；
   - 校验失败时重试一次；若两次仍失败，以 `.invalid.json` 保存于预期工件旁，原始请求存入 `memory/pending/`，
     并继续主业务，严禁阻塞核心链路。

## 注册表变更与失败

项目注册表是可变 XiaoTao 状态。按 [storage.md](storage.md) 中的 revision、独占锁、陈旧写入和
原子替换协议更新。拒绝重复 Worker ID、同一 Worker 内重复的规范能力、无效别名，以及映射到
多个能力的别名。

经过明确评审的提升可以发布 `source: learned` 的可复用条目；使用频率、模型置信度和此前成功
仅是证据，不能执行该转换。忽略并报告无效注册表条目，不要静默修复或选择。如果注册表在选择
期间变化，从新 revision 重新开始。已经完成的持久化选择保持稳定，因为执行使用不可变 Task
或 Temporary 快照。

每次经明确评审把可复用 Worker 发布到项目 registry 时，同时以互斥创建方式发布一份不可变批准
记录到 `.xiaotao/workers/approvals/<approval-id>.approval.json`，并用
[worker-approval.schema.json](schemas/worker-approval.schema.json) 校验。记录保存 Worker ID、显示
名称、registry ID 与 revision、批准者、实际生效时间，以及批准时完整 Worker 规格的规范摘要。
规范摘要是对 Worker JSON 使用 UTF-8、键排序和紧凑分隔符编码后计算的小写 SHA-256。

批准记录必须在 registry 变更的逻辑提交边界一起发布；已有同名记录不得替换或改写。写入方必须
在发布时核对摘要和 registry 内容，但后续读取不可要求旧 registry revision 仍是当前 revision。
registry 后续编辑、禁用或移除 Worker 都不会改写历史批准事实。批准记录只证明既有评审结果，
不能自行授予批准权限，也不能由 Activity 创建。
