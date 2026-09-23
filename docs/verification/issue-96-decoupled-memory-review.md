# Issue #96 验收与场景验证：Task 收尾与长期记忆审查解耦

## 背景与目标

本方案针对 GitHub Issue #96，解决正式 Task 业务目标与验证完成时，交付被 Memory Worker 压缩与审查耗时拖慢或卡住的痛点。方案将“任务事实落盘交付”与“长期记忆提炼审查”彻底解耦。

## 场景验证

### 场景 1：任务事实同步落盘与即时交付（主通道）

- **行为**：业务验证通过后，立即写入 `completion.md`，校验其结果、证据、限制与未决事项。先持久化 `.xiaotao/memory/pending/tasks/<task-id>.json` 待审指针，再在 `task.yaml` 中标记 `completed_at`、`status: completed` 与 `memory_pending: true`，最后归档。指针的来源路径指向归档位置。
- **效果**：小涛向用户返回简洁大白话的业务交付结果，完成任务收尾闭环。Memory Worker 慢、瞬时失败或不可用时，不影响业务任务的可靠归档与交付。
- **防虚报守卫**：若 `completion.md` 写入失败，严禁标记完成，不丢失任务状态。

### 场景 2：待审记忆有界定位与按需审查（异步通道）

- **行为**：当用户明确提出“整理记忆/处理待审任务”或后续关联场景触发时，从 `.xiaotao/memory/pending/tasks/` 有界读取待审指针（单次最多 limit 项），定位已归档 Task 的 `completion.md` 作为只读权威来源。
- **效果**：会话启动时不扫描全部已归档 Task，启动零卡顿；单次处理受批次限制，防止上下文膨胀。

### 场景 3：幂等性与审查重试

- **行为**：以 Task ID 与 `completion.md` 来源路径作为幂等键。由只读 Memory Worker 执行 Experience Review，提出 `UPDATE`、`MERGE`、`CREATE`、`SKIP` 提案。Memory Worker 严格禁止自我批准。
- **效果**：
  1. 只有全部审查结果（包括显式 `SKIP`）与不可变 Decision Record 确认持久化后，才先将归档 `task.yaml` 更新为 `memory_pending: false` 并记录 `memory_reviewed_at`，最后移除待审指针。
  2. 同一 Task 的审查持有 Task 级锁；候选、决策和新目标使用持久化的稳定 ID。若审查过程中断、失败或等待确认，保留待审现场与 `last_error` 诊断。重试核对已提交记录和目标 revision，避免重复生成 Long-term 条目、重复写入或发布另一份 Decision Record。
  3. 若任务状态已更新但指针尚未清除，重试仅核验并清理指针；若完成登记后归档前中断，则借助指针定位活动 Task 并恢复归档。

### 场景 4：用户显式记忆请求保持独立

- **行为**：用户显式要求“创建这条 Memory”或“记住这条信息”时，继续走 Issue #80 的显式轻量路径（`explicit-create`），不进入待审队列，即时处理。

## 契约与 Schema 覆盖

1. `xiaotao/references/schemas/task.schema.json` 扩展了可选属性 `memory_pending` (boolean) 与 `memory_reviewed_at` (date-time)。
2. Fixtures 覆盖：
   - `task-completed-pending-valid.json`：合法的待审任务状态；
   - `task-completed-reviewed-valid.json`：合法的审查完成状态；
   - `task-memory-pending-invalid.json`：非法类型的 `memory_pending` 校验被 Ajv 拦截。
3. 单元测试 `test/task-completion-decouple.test.js` 覆盖了协调、存储与记忆规范对收尾解耦流程的严格约束。
