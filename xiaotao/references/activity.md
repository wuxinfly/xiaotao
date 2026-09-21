# 项目活动时间线

Activity 回答“这个月完成了什么”“今年做过哪些事”。它是从现有权威记录生成的查询视图，
不是第四层 Memory，也不是新的权威状态源。删除 `.xiaotao/activity/index.json` 不会丢失事实；
下一次查询可以重建。

## 当前范围

当前派生 Task 完成事件、Temporary 晋升事件、里程碑 Decision 事件、里程碑 Playbook 评审事件、
Worker registry 批准事件，以及显式 checkpoint 恢复成功事件。

`task_completed`：

- 来源只包括 `.xiaotao/tasks/<task-id>/task.yaml` 和
  `.xiaotao/tasks/archive/<task-id>/task.yaml`，不会递归扫描 Task 的 artifacts 或 references；
- Task 的 `status` 必须是 `completed` 或 `archive`；
- `occurred_at` 只取显式 `completed_at`，并归一化为 UTC；
- `source_refs` 指向 Task 当前所在的 `task.yaml`。Task 移入 archive 后，重建会刷新引用路径；
- 事件 ID 由 Task ID 和 `completed_at` 确定性生成，同一 Task 不会重复出现。

旧 Task 没有 `completed_at` 时跳过。不要用 `updated_at`、Git 时间或文件 mtime 猜测事件时间。

`temporary_promoted`：

- 来源同样只包括 `.xiaotao/tasks/<task-id>/task.yaml` 和
  `.xiaotao/tasks/archive/<task-id>/task.yaml`；
- Task 必须同时具备 `source_temporary` 和显式 `promoted_at`；
- `occurred_at` 只取 `promoted_at`，并归一化为 UTC；
- `status` 为 `preparing` 的 Task 尚未提交，不投影晋升事件；
- `source_refs` 指向 Task 当前的 `task.yaml`。来源 Temporary 的规范 ID 记录在事件摘要里；
  对晋升事件而言，Temporary 目录本身不是 Activity 来源，它已归档或已丢弃都不影响该晋升事件。
  checkpoint recovery 事件另按下文规则扫描目标内的不可变 observation。

`promotion_transaction` **不是**事件时间，Activity 不解释它：该标记在事务打开时写入，早于晋升
生效，只用于事务关联、恢复与审计。晋升真正生效的边界是原子发布 `committed.yaml`，`promoted_at`
记录的才是这个登记时刻。

没有 `promoted_at` 的历史提升 Task 不产生晋升事件，也绝不用 `promotion_transaction`、
`created_at`、`updated_at`、Git 时间或文件 mtime 代替。同一个 Task 可以先产生晋升事件，之后再
产生完成事件，两者互不替代。

`decision_approved` 和 `decision_superseded`：

- 来源只包括 `.xiaotao/memory/long-term/decisions/<decision-id>.decision.json`；
- Decision Record 必须是 `importance: milestone`，结果必须是 `approved` 或 `superseded`；
- `occurred_at` 只取不可变记录的显式 `decided_at`，并归一化为 UTC；
- Activity Event 的 `source_refs` 指向当前存在的不可变 Decision Record；记录内保留发布时已验证的
  证据路径，但历史本地证据后续可能因归档或未同步而暂不可达；
- 事件 ID 由事件类型、Decision ID 和 `decided_at` 确定性生成。

`routine` 决策和 `rejected` 结果保留审计价值，但不进入面向用户的 Activity。旧格式 Decision、嵌套
记录及缺少可靠时间的历史记录保持原样，不迁移、不投影，也不得用 `updated_at`、Git 时间或文件
mtime 猜测事件时间。

`playbook_approved` 和 `playbook_superseded`：

- 来源只包括 `.xiaotao/playbooks/decisions/<decision-id>.decision.json`，不会递归扫描
  `candidates/` 或已批准 Playbook 文件；
- 记录使用 [decision-record.schema.json](schemas/decision-record.schema.json)，必须是
  `importance: milestone`，结果必须是 `approved` 或 `superseded`，`target_ids` 指向受影响的
  `playbook_id`；
- `occurred_at` 只取不可变记录的显式 `decided_at`，并归一化为 UTC；
- `source_refs` 指向当前存在的不可变 Decision Record；发布后单纯缺少历史证据文件不会阻塞重建，
  但证据路径格式无效或逃逸项目边界时构建失败；
- 事件 ID 由事件类型、Decision ID 和 `decided_at` 确定性生成。

Playbook 候选本身（`playbooks/candidates/`）和已批准 Playbook 文件遵循可变状态协议的
`updated_at` 都不是事件时间：前者只是提案，后者是可变现状。只有不可变评审记录能进入时间线，
也不得用它们回推批准时刻。

`worker_approved`：

- 来源只包括 `.xiaotao/workers/approvals/<approval-id>.approval.json`；
- `occurred_at` 只取不可变记录的 `approved_at` 并归一化为 UTC；
- 记录保存批准时的 Worker 显示名称、registry revision 和 Worker 规格摘要，因此后续 registry 编辑、
  禁用或移除 Worker 不会改写历史事件；
- `source_refs` 指向批准记录，事件 ID 由事件类型、approval ID 和批准时间确定性生成；
- 没有批准记录的旧 registry 保持有效但不投影，绝不用 registry `updated_at`、mtime 或 Git 时间猜测。

`checkpoint_recovered`：

- 只投影显式 `xiaotao_checkpoint retry` 成功发布的新版不可变
  `references/checkpoints/<request-id>.committed.json`；
- `occurred_at` 只取 observation 的 `committed_at` 并截断到 UTC 秒，`completion` 必须为
  `recovery`；
- 普通/自动 `save`、重复 save、`already_committed`、inspect、status、失败事件、CAS 冲突和锁操作
  都不进入 Activity；
- 同目录请求文件必须存在，且目标、request ID、revision、record hash 与 proposal hash 必须和
  observation 严格绑定；非法权威链会使构建失败；
- 旧四字段 observation 和新版 `completion: save` observation 保持兼容但不投影，也不从
  `updated_at`、failure `recorded_at`、mtime 或 Git 时间猜测恢复时间；
- `source_refs` 指向证明恢复已经完成的 `.committed.json`。事件 ID 由事件类型、目标种类、目标 ID、
  request ID 和归一化时间确定性生成。

Task metadata 仍只扫描规范 `task.yaml`。checkpoint 恢复使用独立的受限来源模式，只扫描规范活动或
归档 Task/Temporary 的直属 `references/checkpoints/*.committed.json`，不递归读取其他 reference、
artifact 或 failure event。相关请求与 observation 参与 `source_digest`，所以恢复提交或权威链变化会
使 Activity 缓存失效。

## 写入规则

Activity 没有 `record` 操作，也不维护 `events/*.jsonl`。完成新 Task 时，在 Task 生命周期更新中
写入并保留 `completed_at`；提升 Temporary 时，由提升事务在提交时写入并保留 `promoted_at`，
并保留 `source_temporary` 与 `promotion_transaction`；作出新的重要决策时，发布带 `decided_at`
的不可变 Decision Record。
批准或取代 Playbook 时，把评审记录发布到 `playbooks/decisions/`。
checkpoint recovery 事件不需要额外 Activity 写入；显式 retry 成功发布带 `completion: recovery` 和
`committed_at` 的 observation 后即可派生。
Worker 批准事件不需要额外 Activity 写入；在既有评审权限边界内随 registry 逻辑提交原子发布
不可变批准记录后即可派生。
Activity 只负责读取和派生。规范来源损坏、重复 ID、文件名不匹配或时间格式无效时，构建必须
明确失败，不能静默跳过有问题的权威记录。`promoted_at` 存在但 `source_temporary` 缺失、或时间
格式无效时，同样属于损坏记录并明确失败；缺少 `promoted_at` 的旧提升记录则保持原样、不投影。
Decision 的证据路径格式无效或逃逸项目边界时同样失败，但发布后单纯缺少历史证据文件不会阻塞
重建。

## 查询协议

用户询问时间范围内做过什么时，运行受限查询，不要直接读取完整 `activity/index.json`：

```text
python xiaotao/scripts/activity_catalog.py --project-root <root> search --month 2026-09
python xiaotao/scripts/activity_catalog.py --project-root <root> search --year 2026
python xiaotao/scripts/activity_catalog.py --project-root <root> search --from 2026-09-01 --to 2026-09-30
```

查询默认最多返回最近 50 条，`--limit` 范围为 1–200。只在用户需要细节时，再读取结果中少量
`source_refs`。Index 缺失、损坏或来源变化时，查询会自动重建；`--no-refresh` 只用于检查调用方
是否错误依赖旧缓存。

维护命令：

```text
python xiaotao/scripts/activity_catalog.py --project-root <root> build
python xiaotao/scripts/activity_catalog.py --project-root <root> check
```

`build` 原子写入派生 Index。`check` 只判断现有 Index 是否有效且与当前 Task 和 Decision 来源一致，
不修改文件。
