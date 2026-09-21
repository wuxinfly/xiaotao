# Playbooks

Playbook 是项目自行编写的可选指导，使用 Markdown 或 YAML 存储在 `.xiaotao/playbooks/` 下。
它们描述步骤目标、必需能力、检查、证据和典型顺序，不会把步骤绑定到固定角色或 Worker ID。

Long-term Memory 回答“我们对这个项目知道什么？”。Playbook 回答“遇到这类情况时，应考虑
哪种有证据支撑的方法？”。因此，Playbook 有明确触发条件、有序的可复用步骤和检查。项目事实、
一次性命令、临时实验、精确的 Task 时间线和未经验证的建议都不是 Playbook。

## Playbook Candidates

在 Experience Review 边界，Memory Worker 可以在 Long-term candidates 之外输出
`playbook_candidates`。这不会增加预置角色。每个候选记录：

- 稳定的 `candidate_id`、标题、触发条件、有序 `steps` 和 `checks`；
- 对照当前稳定 `playbook_id` 得到的 `action` 和匹配分类；
- 理由、结构化来源元数据、可达的 `source_refs` 和 `evidence_refs`；
- `status: candidate`。

Temporary 或 Task 材料可以作为来源，但每个非 `SKIP` 候选都必须建立在真实执行证据之上。
一段听起来合理的讨论不能证明某个方法可复用。一次成功执行的证据可以支持候选，但绝不能自动
提升。

按以下顺序分类和维护候选：

```text
UPDATE → MERGE → CREATE → SKIP
```

- `CREATE` 要求流程是全新的，且不指向当前 Playbook。
- `UPDATE` 准确指向一个重叠或冲突的 Playbook。
- `MERGE` 指向至少两个重叠或冲突的 Playbook。
- `SKIP` 记录有目标的重复项，或没有目标的低价值材料。

优先维护连贯的 Playbook，不要创建近似重复项。将经过验证的提案（包括 `SKIP`）持久化到
`.xiaotao/playbooks/candidates/`，将不可变评审记录放到 `.xiaotao/playbooks/decisions/`。
被拒绝的候选仍保持可审计，以免没有新证据时反复提出同一个薄弱流程。

### 不可变 Playbook 决策记录

`playbooks/decisions/` 下的评审记录使用 [decision-record.schema.json](schemas/decision-record.schema.json)
定义的不可变 Decision Record，文件名必须是 `<decision-id>.decision.json`，且与 `decision_id`
一致。`target_ids` 指向受影响的 `playbook_id`；`outcome` 为 `approved`、`rejected` 或
`superseded`，取代时还必须用 `superseded_by` 指向替代项。`decided_at` 是批准、拒绝或取代实际
发生的时间，发布后不可修改；不得用 `updated_at`、文件 mtime 或 Git 时间代替。Activity 只投影
`importance: milestone` 的 `approved` 与 `superseded`，`routine` 与 `rejected` 保留为审计记录。

拒绝全新的 `CREATE` 或无目标的 `SKIP` 候选时，没有受影响的正式 Playbook，允许
`outcome: rejected` 使用 `target_ids: []`，并通过 `source_refs` 引用被拒绝的候选记录。
不得为满足目标列表而虚构 `playbook_id`；批准和取代仍必须包含至少一个目标 ID。

Memory Worker 不能批准候选或写入正式 Playbook。第一版必须由用户明确批准后才能提升。重复
成功可以增加证据，但不会赋予自动提升权限。获批动作使用可变状态写入协议，保留现有证据和
决策历史，且不超出用户当前授权。

候选不是生效的指导。只有已批准且当前有效的 Playbook 才能参与选择。

## 规范 Playbook 文件

受管理的正式 Playbook 是 `.xiaotao/playbooks/` 下的单个 Markdown 或 YAML 文件，不包括保留
目录 `candidates/` 和 `decisions/`。YAML 在顶层暴露以下字段；Markdown 在 YAML front matter
中暴露相同字段，正文可用于人类说明：

```yaml
playbook_id: pb-20260829t000000z-a1b2
file_path: .xiaotao/playbooks/performance-diagnosis.md
title: 证据优先的性能诊断
trigger: 用户要求诊断运行时性能回退。
steps:
  - 先收集运行时证据，再提出原因。
checks:
  - 来源 Task 中的证据可达。
status: active
revision: 0
updated_at: 2026-08-29T00:00:00Z
updated_by: xiao-tao/session-or-run-id
source_refs:
  - .xiaotao/tasks/archive/task-id/evidence/trace.md
```

`playbook_id` 创建后永不改变。`file_path` 是指向同一文件的规范化项目相对路径。active
Playbook 被规范化到 `current_playbooks`；`status: superseded` 的文件作为历史保留并被排除。
`revision`、`updated_at` 和 `updated_by` 遵循 [storage.md](storage.md) 中的可变状态协议。

请求校验仅在 `current_playbooks.file_path` 指向 `.xiaotao/playbooks/` 下已存在的 Markdown
或 YAML 文件，且不位于保留目录 `candidates/`、`decisions/` 时接受。守卫读取文件，并要求其
`playbook_id`、`file_path`、`revision` 和 `status` 元数据与索引条目一致；任意项目文件不能
冒充正式 Playbook。

### 旧格式迁移

第一次 Experience Review 前，扫描 Playbook 树下由项目编写的 Markdown 和 YAML，同时排除
保留的候选与决策记录。如果任何正式 Playbook 缺少规范字段，应停止 Experience Review，
不得静默忽略，也不得每次运行都虚构新 ID。

提供一次迁移，并要求用户明确批准。经过评审的迁移映射现有标题、触发条件、有序步骤和检查，
添加 `status: active`，分配并持久化一个稳定 `playbook_id`，将现有路径记录为 `file_path`，
设置 `revision: 0`，并在 `source_refs` 引用的不可变迁移/事务快照中保留原始字节。所有获批
文件规范化应在同一个事务中应用。
后续读取使用已存 ID，绝不根据标题、内容哈希或模型输出重新生成。如果用户拒绝迁移，显式选择
时仍可使用旧文件，但自动 Experience Review 保持 pending，不得与不完整索引比较。

用户要求遵循某个 Playbook 时：

1. 读取指定 Playbook。未指定时，只有其用途明确匹配才能选择。
2. 说明影响范围、成本、风险或外部动作的实质含义。
3. 根据当前任务和可用证据调整其建议。
4. 如果用户当前指令与可选 Playbook 指导冲突，遵循用户指令；但不得违反安全或授权边界。

不要把 Playbook 章节变成强制 Runtime 状态。跳过无关步骤，并根据当前证据解析必需能力。
用户可随时改变路径。

除非用户明确批准评审后的动作，否则不得修改项目自行编写的 Playbook。任何 Playbook 或候选
都不能绕过安全检查、授权外部动作或强制固定 Worker 顺序。
