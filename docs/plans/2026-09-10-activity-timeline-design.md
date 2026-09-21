# Activity 时间线设计

## 背景

Long-term Memory 回答“项目现在知道什么”，不会保存完整工作流水。Issue #54 需要回答“这个月
完成了什么”，但 Activity 不能成为第四层 Memory，也不能复制一份会与 Task、Decision、Memory
或 Playbook 分叉的事实。

## 权威边界

现有 Task、Decision、Memory、Playbook 和 Worker 记录继续是唯一权威来源。Activity 只保存一个
可删除、可重建的本地查询缓存：

```text
.xiaotao/activity/
  index.json  # 从权威记录确定性派生，不纳入 Git
```

不存在 Activity 事件日志或手工 `record` 接口。这样无需解决第二套权威数据的并发追加、迁移和
一致性问题，也不会要求各生命周期调用点同时写两份状态。

## Task 完成事件

当前权威格式中，只有 Task 能形成最小且可靠的自动闭环。Task 进入 `completed` 或 `archive`
终态时，在同一次生命周期更新中写入一次 `completed_at`。目录构建器扫描活动及归档 Task，生成：

```json
{
  "event_id": "activity-20260910-<stable-hash>",
  "occurred_at": "2026-09-10T12:00:00Z",
  "event_type": "task_completed",
  "title": "完成老周单入口重构",
  "summary": "完成 Task：完成老周单入口重构",
  "source_refs": [".xiaotao/tasks/archive/20260910-老周单入口/task.yaml"],
  "status": "completed"
}
```

- `event_id` 由事件类型、Task ID 和归一化后的 `completed_at` 确定性生成；
- `occurred_at` 只来自显式 `completed_at`；
- 旧 Task 缺少 `completed_at` 时保持兼容，但不进入时间线；
- `updated_at`、文件 mtime 和 Git 时间都不能替代事件时间；
- `source_refs` 每次从 Task 当前路径生成，Task 移入 archive 后会自动刷新；
- 重复 Task ID、损坏 YAML、无效时间或越界引用必须使构建失败，不能静默忽略。

## Temporary 晋升事件

晋升生效的边界是提升事务原子发布 `committed.yaml`，因此事件时间由提交时写入的显式 `promoted_at`
承载；`promotion_transaction` 在事务打开时写入，早于晋升生效，只用于关联、恢复与审计：

```json
{
  "event_id": "activity-20260831-<stable-hash>",
  "occurred_at": "2026-08-31T12:00:15Z",
  "event_type": "temporary_promoted",
  "title": "优化登录流程",
  "summary": "由 Temporary 20260831-登录流程梳理 晋升为 Task：优化登录流程",
  "source_refs": [".xiaotao/tasks/20260831-优化登录流程/task.yaml"],
  "status": "completed"
}
```

- 只有同时具备 `source_temporary` 和显式 `promoted_at` 的 Task 参与派生；
- `occurred_at` 只取 `promoted_at`，并归一化为 UTC；
- 没有 `promoted_at` 的历史提升 Task 保持兼容但不进入时间线；`promotion_transaction`、
  `created_at`、`updated_at`、文件 mtime 和 Git 时间都不能替代晋升生效时间；
- `promoted_at` 存在但 `source_temporary` 缺失或不是合法时间时，按损坏的规范记录明确失败；
- `preparing` 状态的 Task 尚未提交，不投影晋升事件；
- 来源 Temporary 目录不是 Activity 来源：它已归档或已丢弃都不影响时间线，规范 ID 记录在事件
  摘要里；
- `event_id` 由事件类型、Task ID 和晋升生效时刻确定性生成，Task 移入 archive 后保持不变。

## Decision 里程碑事件

Decision 使用独立的不可变权威记录：

```text
.xiaotao/memory/long-term/decisions/<decision-id>.decision.json
```

记录显式保存 `decided_at`、`outcome` 和 `importance`。Activity 只投影 `importance: milestone` 的
`approved` 与 `superseded`，分别生成 `decision_approved` 和 `decision_superseded`。`routine` 与
`rejected` 保留审计价值但不进入用户时间线。`source_refs` 指向 Decision Record 本身，证据再由
Record 内的来源追溯。

Decision 发布时严格验证记录内的证据引用可达；Activity 重建只复核其路径格式和项目边界，不要求
历史证据在当前机器仍然存在。这样不可变的团队共享记录不会因本地 Task 归档或未同步而失效。

旧 Decision 不要求迁移。只有直属目录、扩展名为 `.decision.json` 且通过 schema 校验的新记录才
参与派生；不得从更新时间、文件 mtime 或 Git 时间推断 `decided_at`。

## Playbook 评审事件

Playbook 的批准、拒绝和取代同样发布不可变评审记录：

```text
.xiaotao/playbooks/decisions/<decision-id>.decision.json
```

记录复用同一份 `decision-record.schema.json`，`target_ids` 指向受影响的 `playbook_id`。Activity
只投影 `importance: milestone` 的 `approved` 与 `superseded`，分别生成 `playbook_approved` 和
`playbook_superseded`，`occurred_at` 只取显式 `decided_at`。`routine` 与 `rejected` 仍只作审计。

候选记录（`playbooks/candidates/`）和已批准 Playbook 文件遵循可变状态协议的 `updated_at` 都不是
事件时间：前者只是提案，后者是会被后续更新改写的现状。这一来源与 Long-term Memory 的 Decision
记录共享同一套记录格式、去重规则与投影规则，只是归属不同目录并生成不同事件类型。

### 无目标候选的拒绝

共享 Decision Record schema 仅对 `rejected` 允许空 `target_ids`，以支持拒绝全新 Playbook
候选；`source_refs` 引用候选，批准和取代仍要求非空目标。这样无需新增候选 ID 字段或虚构
Playbook ID。回归测试覆盖缓存失效后自动重建、历史候选缺失后重建，以及批准/取代的非空目标
约束；schema 与原生 validator 用三个 fixture 验证一致性。

## 构建和失效判断

构建器对所有规范 Task、Decision 与 Playbook 评审记录来源的相对路径及文件内容计算 SHA-256
`source_digest`。因此 Task 状态变化、移入 archive，以及 Decision 或 Playbook 评审记录新增或变更
都会使旧 Index 失效。构建结果原子写入 `.xiaotao/activity/index.json`；缺失或损坏时查询自动重建。

## 查询协议

```text
activity_catalog.py --project-root <root> search --month 2026-09
activity_catalog.py --project-root <root> search --year 2026
activity_catalog.py --project-root <root> search --from 2026-09-01 --to 2026-09-30
```

查询只返回所选窗口，默认最多 50 条、最多允许 200 条。老周不直接把完整 Index 读入上下文；只有
用户需要详情时才读取少量 `source_refs`。

## 完成标准

1. 新完成 Task 无需额外记录命令即可出现在查询中；
2. 旧 Task 不会被猜测时间；
3. Task 归档后事件不丢失，引用指向当前文件；
4. Index 缺失、损坏或陈旧时可安全重建；
5. 不生成 `events/*.jsonl`，Activity 不成为权威源；
6. 里程碑级批准和取代 Decision 使用显式 `decided_at` 进入时间线；
7. routine、rejected 和旧格式 Decision 不进入时间线，也不猜测时间；
8. 历史证据缺失不阻塞重建，但不安全或越界的证据路径仍明确失败；
9. 月、年和任意日期范围查询有边界且结果数量受限；
10. Playbook 里程碑级批准与取代使用 `playbooks/decisions/` 中的显式 `decided_at` 进入时间线；
11. Playbook 候选与已批准 Playbook 文件的 `updated_at` 不被当作事件时间；
12. Playbook 评审记录的重复 ID、文件名与 `decision_id` 不一致、无效时间或越界证据路径使构建失败。
13. 提升后的 Task 使用提交时写入的显式 `promoted_at` 进入时间线，无需额外记录命令；
14. 没有 `promoted_at` 的历史提升 Task 不投影晋升事件，也不猜测时间，`promotion_transaction`
    不被解释为事件时间；
15. `preparing` 状态的 Task 不投影晋升事件；同一个 Task 的晋升事件与完成事件互不替代。
