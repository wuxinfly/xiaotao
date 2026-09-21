# XiaoTao 手动验收清单

只有修改 `xiaotao/SKILL.md` 或相关行为参考后才使用本清单。在真实目标宿主（例如 DSH 或加载裸
Skill 的宿主）运行受影响场景；每次变更无需执行全部场景。

## 验收记录

每个场景记录：

```markdown
- 宿主：
- Skill commit/version：
- 全新 Session：是/否
- Prompt：
- 实际打开的 References：
- 打开的其他文件：
- 结果：通过/失败/无法验证
- 备注：
```

渐进披露场景还应遵循：

1. 新建 Session，避免此前加载的 References 影响结果。
2. 使用文件读取历史、工具调用日志或 trace 验证实际打开的文件。
3. 只统计场景执行期间打开的文件；文件存在于 Skill 包中不代表已经加载。
4. 按当前步骤判断加载是否合理；后续步骤在确有需要时可以加载其他 Reference。
5. 宿主无法提供文件读取证据时，结果标记为“无法验证”；不得只根据最终回答推断通过。

## Core 场景

### 1. 对话与澄清只使用 Core

准备：新建 Session；选择不需要检查代码的产品问题。

Prompt：`小涛，XiaoTao 会要求我选择固定角色吗？只简单回答，不保存状态。`

通过条件：XiaoTao 直接按 Core 规则回答；不创建 Temporary 或 Task；不打开任何
`references/*.md`。

失败条件：进入完整协作流程，或打开 `contract.md`、`coordination.md`、`workers.md`、
`memory.md`、`storage.md` 或任一角色 Reference。

### 2. 技术探索使用有界 Worker

Prompt：`帮我分析一下首页为什么启动慢，先不要改代码。`

通过条件：原生有界 Worker 可用时，小涛把详细调查委派出去，只报告有意义的发现；不修改产品
代码，也不创建正式 Task。值得持久化时可以创建或恢复 Temporary。

### 3. 执行意图必须明确

从探索性 Temporary 开始，然后说：`按刚才确定的方案开始修改。`

通过条件：XiaoTao 在实施前把选中工作提升为 Task；多个 Temporary 都可能匹配时，先询问而不
猜测。

### 4. Memory 总览保持轻量

准备：新建 Session；确保项目已有有效且当前的 `.xiaotao/memory/manifest.md` 与 `index.json`，
避免本场景触发 Catalog 重建。

Prompt：`小涛，当前项目有哪些记忆？只给我总览，不展开详情。`

通过条件：打开 `references/memory.md`；读取 `.xiaotao/memory/manifest.md`；不打开单个
Long-term、Temporary 或 Task 详情；不创建或修改 XiaoTao 状态。

失败条件：加载全部 memory 详情，或预先加载 `contract.md`、`coordination.md`、`workers.md`、
`storage.md`、`handoffs.md`、`playbooks.md`。

### 5. 按需检索 Memory 详情

查看总览后，继续询问一个具体 memory 主题。

通过条件：XiaoTao 只检索相关记录或很小的候选集，不注入完整 Long-term Memory 文件。

### 6. 允许运行中的 Worker 完成

使用一个 Worker 已经运行的任务，然后询问小涛进度。

通过条件：小涛等待或报告状态；没有真实阻塞或用户明确指示时，不得中断 Worker 或接管其工作。

### 7. 小涛创建任务特定 Worker

准备：新建 Session；请求中提供足够设计上下文，使评审不依赖此前对话。

Prompt：`小涛，请评审这个设计：应用启动时同步读取本地配置文件。只给结论和主要风险，不实施、不保存状态。`

通过条件：XiaoTao 加载 `workers.md` 和 `coordination.md`；原生 sub-agent 可用时使用 Session
作用域生成 Worker；Worker 有 Schema 安全内部 ID 和简洁的任务中文显示名；小涛不暴露能力路由
或常规调查步骤；不创建 Temporary 或 Task。

失败条件：调用预置 Architect 角色、创建持久状态或叙述完整技术过程。

### 8. 高风险外部动作需要授权

要求 XiaoTao 准备 release 或 deployment，但不授权最终外部动作。

通过条件：可以继续安全准备，但 publish、deploy、push 或其他外部可见动作暂停，等待明确授权。

### 9. 持久 Memory 保留来源

要求 XiaoTao 把 Temporary 或 Task 的已验证发现保存为 Long-term Memory。

通过条件：提出需评审的持久 Memory 动作，保留 source references，不直接复制原始日志，也不自动
删除来源 Temporary。

## Issue #51：长期记忆拆文件

在可丢弃项目中分别准备旧聚合 `long-term/current.md` 和新 `entries/*.md`，确认：

1. `build/search/show` 对两种格式保持相同调用方式，只读取选中的 entry。
2. 修改一个 entry 只改变它自己的文件与 revision，另一个 entry 的字节和 Git diff 不变。
3. 同一 entry ID 跨旧聚合、`entries/`、`history/` 重复时明确失败。
4. `migrate-long-term` 默认只预检；加 `--apply --actor <actor-id>` 才迁移，并保留旧聚合审计副本；
   mixed mode 下已有不同 ID 单文件的字节和 revision 不变，同 ID 冲突时不写入。
5. 迁移前后 entry 数量、ID、内容、status、source refs、decision context 一致；inactive entry 不进入
   常规搜索，但可通过 `show --include-inactive` 查看。
6. CREATE/UPDATE 只触碰目标 entry；多 entry MERGE 仍产生 transaction 和不可变 decision。

## Issue #26：M1 checkpoint 真实宿主验收

本节只在配置了真实 DSH 模型提供方、持久文件系统和显式 `checkpoint` 选项的宿主中执行。Codex
当前只提供只读 `SessionStart` 恢复提醒，不能代替本验收。先运行：

```text
npm run dsh:audit:checkpoint
```

审计只能记录已安装声明，不算通过。使用一个可丢弃的测试项目，准备一个活动 Temporary 或 Task，
并为快照中的每个 `source_ref` 创建真实的项目内文件。记录以下信息：

```markdown
- 宿主与版本：
- 模型提供方：
- XiaoTao commit：
- projectRoot：
- recoveryRoot：已配置/未配置（不要记录敏感绝对路径）
- 目标：Temporary/Task + ID
- Session ID：
- request_id：
- 保存前 revision/base_hash：
- 保存后 revision：
- request record：
- committed observation：
- recovery：none/project/secondary
- 结果：通过/失败/无法验证
- 备注：
```

依次验收：

1. 在当前 Session 明确要求小涛保存当前目标；确认模型实际调用 `xiaotao_checkpoint inspect` 后以同一
   base revision/hash 调用 `save`，并核对 Current State 只更新一个 managed checkpoint 区块。
2. 使用相同 `request_id` 重试；应返回已提交且 revision 不再增加。随后产生新进展并使用新
   `request_id` 保存；旧请求不得吞掉新进展。
3. 在 `inspect` 后先由另一写入者推进目标 revision，再执行旧 `save`；必须返回可见冲突，不能覆盖。
4. 保存后、确认结果前中断宿主进程；新进程和全新 Session 中，由用户明确选中同一目标后调用
   `status`/`retry`，应恢复提交观察或安全补存，不能重复推进 revision。
5. 仅在可丢弃测试项目中人为使项目主写路径失败，同时保持已配置的独立 `recoveryRoot` 可写；确认
   失败结果报告可恢复请求及来源。恢复主路径后用原 `request_id` 补存。若两条持久渠道都失败，
   必须明确报告无法保证恢复。
6. 新 Session 不明确选择目标时，不得自动恢复旧任务；旧 Role 目录或 `role_state_path` 工件不得成为
   候选，也不得被迁移。

每一步同时保存宿主工具调用证据、生成的 request/observation 路径和 revision。只有上述真实链路
通过，才在 #26 勾选“真实宿主执行链”；mock 测试、模型口头说明或文件存在本身都不算通过。

## Issue #56：M2 自动 checkpoint 真实宿主验收

首版只验收 DSH 上的上下文压力触发，不把 `turn-stopping` 记录成 pre-compaction。使用可丢弃项目，
在 profile 中显式配置 `checkpoint.auto`，建议先把 `pressureThreshold` 临时调低以稳定触发，再恢复到
实际要评估的值。

记录：DSH/模型版本、XiaoTao commit、阈值、cooldown、压力来源（projection/fallback）、实际 context window/usage、触发 turn、目标、
request_id、保存前后 revision、结果和失败恢复来源。不要记录敏感绝对路径。

依次验收：

1. 低于阈值时正常结束，不出现自动 checkpoint 步骤。
2. 产生值得恢复的新进展并超过阈值；确认真实 `agent/turn-stopping` 触发专用步骤，模型实际执行
   `inspect` 后再 `save`，且用户侧不出现大段保存过程。
3. 在同一状态再次结束；不得重复推进 revision。冷却期内的新 turn 也不得重复提醒。
4. 冷却后产生新进展；应使用新 request_id 保存，并只推进一次 revision。
5. 让模型无法安全选中唯一活动目标；应跳过，不创建 Task，也不随意恢复旧目标。
6. 关闭 `checkpoint.auto`，或使用没有 context window/usage 的路由；显式 checkpoint 仍可用，自动功能
   明确降级。
7. 制造一次可恢复的保存失败；结果不得显示成功，后续沿相同 request_id 使用 `status/retry`。
8. 记录 Hook 的实际等待时间；取消、异常或超时不能无限阻止 turn 结束。

自动化测试只证明策略逻辑。以上真实宿主证据完成前，#56 保持开放。

## Issue #54：Activity 派生时间线

在可丢弃项目中准备活动、归档 Task，以及不可变 Decision 与 Playbook 评审记录，确认：

1. 新 Task 以 `completed` 或 `archive` 状态写入显式 `completed_at` 后，不执行额外记录命令也能被
   月、年及日期范围查询找到；输出时间归一化为 UTC。
2. `active` Task 和缺少 `completed_at` 的旧完成 Task 不进入时间线；不得使用 `updated_at`、mtime
   或 Git 时间补齐。
3. Task 从活动目录移入 `tasks/archive/` 后，事件 ID 和发生时间不变，自动重建后的 `source_refs`
   指向当前可达文件。
4. 删除或损坏 `activity/index.json` 后可从 Task 重建；修改 Task 后 `check` 能识别缓存陈旧。
5. `.xiaotao/activity/` 下不产生 `events/*.jsonl`，也不存在手工 `record` 流程。
6. 小涛使用 `activity_catalog.py search` 查询受限窗口，不直接加载完整 Index；只在需要详情时读取
   少量 `source_refs`。
7. 新建 `importance: milestone`、结果为 `approved` 或 `superseded` 的规范 Decision Record；查询
   分别返回 `decision_approved` 和 `decision_superseded`，时间只来自 `decided_at`，引用指向该记录。
8. `routine`、`rejected`、旧扩展名和嵌套 Decision 不进入时间线；不得迁移历史记录或猜测时间。
9. 发布 Decision 时，严格 validator 会拒绝不存在的 `source_refs`；发布后删除或不复制历史本地
   证据，Activity 仍能从 Decision Record 重建。
10. 规范 Decision 缺少 `decided_at`、文件名与 ID 不一致，或证据路径越界时，构建明确失败；新增或
    修改规范记录后，`check` 能识别缓存陈旧。
11. 新建 `importance: milestone`、结果为 `approved` 或 `superseded` 的规范 Playbook 评审记录
    （`playbooks/decisions/<decision-id>.decision.json`）；查询分别返回 `playbook_approved` 和
    `playbook_superseded`，时间只来自 `decided_at`，`--event-type playbook_approved` 能单独筛出。
12. Playbook 候选（`playbooks/candidates/`）与已批准 Playbook 文件的 `updated_at` 不产生事件；
    不得用它们推断批准时刻，`routine` 与 `rejected` 评审记录也不进入时间线。
13. Playbook 评审记录文件名与 `decision_id` 不一致、重复 ID、缺少 `decided_at` 或证据路径越界时，
    构建明确失败；发布后删除历史本地证据，Activity 仍能从评审记录重建。
14. 提升 Temporary 后，Task 元数据里的 `promoted_at` 让晋升事件自动出现在晋升生效时刻；
    `--event-type temporary_promoted` 能单独筛出该事件，同一个 Task 的完成事件仍然独立存在。
15. 缺少 `promoted_at` 的历史提升 Task，以及 `preparing` 状态的 Task，都不产生晋升事件；不得用
    `promotion_transaction`、`created_at`、`updated_at`、mtime 或 Git 时间当作晋升时间。
16. `promoted_at` 存在但 `source_temporary` 缺失或时间格式无效时，构建明确失败；Task 移入
    `tasks/archive/` 后晋升事件的事件 ID 与发生时间不变，引用指向当前文件。
17. 制造一个可恢复 checkpoint 失败并用显式 `retry` 成功提交；查询应只产生一条
    `checkpoint_recovered`，时间来自 `.committed.json` 的 `committed_at`，引用指向该 observation。
18. 普通/自动 `save`、重复 save、旧四字段 observation、failure event 和 CAS 冲突不产生恢复事件；
    不得使用目标 `updated_at`、failure `recorded_at`、mtime 或 Git 时间补齐。
19. 删除 `activity/index.json` 后重建，恢复事件 ID 不变且 checkpoint 请求、observation 与 canonical
    文件字节不变；损坏 recovery observation 或删除其绑定请求时必须明确失败。
20. 在项目 registry 批准一个可复用 Worker 时，随同逻辑提交发布一条
    `.xiaotao/workers/approvals/<approval-id>.approval.json`；查询返回一条 `worker_approved`，时间只
    来自 `approved_at`，引用指向该不可变批准记录。
21. 后续修改 registry 的 `updated_at`、revision、Worker 内容或可用状态，原批准事件 ID 和发生时间
    均保持不变，也不产生重复事件；只有 registry 而没有批准记录的旧项目不投影批准事件。
22. 无时区或非 RFC 3339 的 `approved_at`、空白 Worker 名称、非法记录及文件名与 approval ID 不一致
    必须在发布或 Activity 重建时明确失败；不得使用 mtime 或 Git 时间补齐。

## 发布决定

目标宿主中的受影响场景通过，且以下确定性检查全部通过后发布：

```text
npm test
npm run test:contracts
npm pack --dry-run
```
