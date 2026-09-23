# 三层 Memory

Memory 用于延续工作，不用于复刻对话或执行日志。

## 可选快照 Checkpoint

用户明确要求保存或交接，且宿主提供 checkpoint 工具时，先选择一个活动 Temporary 或 Task，
检查其当前 revision/hash，再提交有界事实：目标、已确认发现、已否定方向、进行中的工作、下一步、
Open questions 和可达的项目相对 source refs。这只是覆盖已提交的事实，不代表完整 Session 对话
已经备份。不得包含凭据或无关工作。

写入器可以在 Temporary `current.md` 或 Task `progress.md` 中维护 `Saved checkpoint` JSON 区块和
`checkpoint_receipt`。恢复所选工作时，将快照与用户编写的上下文一同读取；陈旧的旧标题不得
覆盖较新且已验证的进展。Receipt 只是恢复提示，不是授权。替换或删除前遵循
[storage.md](storage.md) 的 checkpoint 规则。正式状态写入成功后，按现有 freshness 协议重建
目录；目录构建失败不会撤销 checkpoint。

结果不确定时，查询状态或使用相同 request ID 重试。不得为了绕过冲突而制造新 ID，也不得声称
未保存工作已经恢复。即使较新请求成功，旧的 pending 请求仍是 pending；应明确报告陈旧冲突。
没有 checkpoint 工具的宿主继续遵循普通 Memory 和存储协议。

## Temporary Memory

Temporary Memory 表示正式 Task 之前值得保留的讨论。保持 `current.md` 简短：

```markdown
---
revision: 12
updated_at: 2026-08-27T11:05:00Z
updated_by: xiao-tao/session-or-run-id
---

# 主题

## 当前目标
...

## 已确认
- ...

## 已否定
- ... — 原因与来源

## 待确认问题
- ...

## 历史引用
- references/<name>.md
```

用户明确要求 Session Handoff 时，将当前讨论压缩为上述格式。讨论有长期价值但已暂停时归档；
只有用户明确否定且不再有延续价值时才移入 Trash。

### Temporary 路由上下文

选择候选时，只加载每个活动 Temporary 的 `meta.yaml` 和 `current.md` 中的主题、当前目标、已确认、
待确认问题。选择完成前，不加载候选 `references/` 树、否定历史细节或完整对话材料。

Temporary 被显式选择、唯一解析或新建后，维护当前 Session 绑定。绑定是对话状态，不要求特定
宿主 API。宿主提供稳定 Session 标识时，可以通过 `last_session_id` 持久化关联；否则只保留在
当前 Session 上下文。持久化 `last_session_id` 只是恢复提示，不是覆盖规则：只有恰好一个活动
候选声明该 ID 时，才能建立绑定。

用户明确切换主题、绑定的 Temporary 不再是 `active`，或从它创建正式 Task 时，应替换或清除
绑定。明确请求唯一标识另一个活动 Temporary 时可以替换绑定；含糊的“继续”不能切换。

切换绑定只改变加载的上下文，不得合并、归档、重命名或修改上一个 Temporary。两个主题看起来
相似也绝不能自动合并。

持久化探索 Worker 在选定 Temporary 中保存不可变规格和简短 Current State。`current.md` 应链接
恢复所需的活动 Worker 状态，而不是复制其完整结果。Temporary 被归档、丢弃或提升时，其作用域
Worker 随之到期。

### Temporary 晋升为正式任务（Promote to Task）

当阶段性讨论和探索成熟，用户明确决定立项或落地为正式 Task 时，支持通过命令原子化转正：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> promote-temporary <temporary-id> [--task-id <task-id>] [--actor <actor>]
```

- **目标与事实提炼：** 从 Temporary `current.md` 中提取“当前目标（Goal）”作为 Task 的 `objective`，将“已确认（Confirmed）”作为新 Task `progress.md` 的 `Key findings`，“待确认（Open questions/items）”作为 `Open items`；
- **元数据与溯源契约：** 生成的 `.xiaotao/tasks/<task-id>/task.yaml` 严格遵循 Schema 校验，自动注入 `source_temporary`、`promotion_transaction` 和 `promoted_at`；
- **状态流转与自动归档：** 原 Temporary 的 `meta.yaml` 状态更新为 `archive`、版本（revision）自增，并在 `current.md` 末尾追加转正审计记录，随后整个目录原子移动至 `.xiaotao/memory/temporary/archived/<temporary-id>`；
- **目录自动刷新：** 转正完成后自动重新推导并落盘 Catalog，移出归档的 Temporary 并索引新建的活动 Task。

## Task Memory

Task Memory 包含公开 Task 上下文，以及每个已调用 Worker 的 Current State。执行单元状态回答：

- `objective`
- `work_done`
- `key_findings`
- `important_paths`
- `open_items`
- `recommended_next`
- `history_refs`

保持当前状态能直接服务下一次调用。较旧但仍有价值的细节移入 `references/`；不要为常规搜索、
重复输出或已丢弃噪声创建 Reference。可变 Task 与 Worker Markdown 使用 [storage.md](storage.md)
中的 revision front matter 和写入协议。

## Long-term Memory

Long-term Memory 保存未来 Task 可能仍有价值的项目知识：架构、稳定模块职责、已验证事实、API
边界、约定和持久决策。

长期经验是声明式项目知识，说明此项目中已经验证或学到什么。带明确触发条件、有序步骤和检查的
可复用流程应放入 Playbook Candidate。不要把同一套流程同时写成 Long-term 条目和 Playbook
Candidate。

Long-term Memory 是维护中的经验库，不是 Task 记录归档。绝不要把 Temporary 或 Task 内容直接
复制进去。链路数据、常规命令输出、已否定假设、过程日志和一次性实施细节留在来源层；只有从中
提炼出的可复用、已验证声明才能进入 Long-term。

每个当前 Long-term 条目拥有稳定 `entry_id`、`memory_kind`（`fact`、`experience`、`principle`、
`decision`、`constraint` 或 `other`）、简洁内容和可达 `source_refs`。每次 Memory Worker 请求都
将它们暴露为 `current_memory.long_term_entries`。空存储以空数组表示，不得省略索引。稳定 ID
使候选能声明比较过哪些条目，而不依赖 Markdown 标题或宿主 API。

新写入采用一条 Memory 一个文件，当前或待确认条目的规范路径是
`long-term/entries/<entry_id>.md`。文件名必须与稳定 `entry_id` 完全一致；每个文件必须有独立的
`revision`、`updated_at`、`updated_by` front matter，并且只能包含一个机器可读 JSON fenced block：

````markdown
---
revision: 0
updated_at: 2026-09-10T08:00:00Z
updated_by: xiao-tao/session-or-run-id
---

# Long-term Memory Entry

```xiaotao-memory-entry
{
  "entry_id": "lt-home-startup-trace",
  "title": "优化前先收集启动证据",
  "memory_kind": "experience",
  "content": "修改首页初始化前先捕获 trace。",
  "source_refs": [".xiaotao/tasks/archive/task-startup/evidence/trace.md"],
  "tags": ["performance", "trace"],
  "aliases": ["首屏性能"],
  "search_hints": ["优化首页启动前需要做什么", "首屏 trace 怎么抓"],
  "status": "active"
}
```
````

`search_hints`（可选，字符串数组）定义“问什么问题时本条知识应被召回”，作为检索提示词匹配，不作为事实正文内容。检索打分命中 `search_hints` 时享受相关性加权并在候选理由中输出 `"search hint"`。

`fact` 和 `constraint` 可选携带带时区的 `valid_from`、`valid_until`（RFC 3339）。两者同时存在时
必须满足 `valid_from < valid_until`。Catalog 按查询基准时间派生 `timeless`、`not-yet-valid`、
`current` 或 `expired`；状态不写回 entry，也不使用文件 `mtime`。普通 `overview`、`recent`、
`search` 只把 `timeless` 和 `current` 当作当前知识。审计时使用 `--include-inactive` 查看尚未生效、
过期或生命周期非活动的条目。过期不会删除、移动或改写原文件；恢复为当前事实必须提供新证据并走
现有 Memory Worker 候选和独立评审。

`entries/` 只允许 `active` 或 `disputed`。经评审变为 `superseded` 或 `rejected` 的 entry snapshot
移动到 `long-term/history/<entry_id>.md`，同时保留 decisions、conflicts 和 source refs。历史仍可通过
Catalog 的 `show --include-inactive` 审计，但不会进入常规检索。`long-term/current.md` 在新格式中只
保留固定的人类说明，不聚合 entry 内容，也不随 entry 更新而改写。

旧项目的聚合 `long-term/current.md` 继续可读，也可以和不同 ID 的新 entry 文件共存；同一
`entry_id` 同时出现在聚合文件、`entries/` 或 `history/` 时必须失败，不能猜测权威版本。普通
Session 不得自动拆分旧文件。用户需要迁移时先预检，再显式执行：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> migrate-long-term
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> migrate-long-term \
  --apply --actor <actor-id>
```

迁移保留 entry 内容、ID、状态、来源和 decision context，将旧聚合原文保存到
`long-term/migrations/<migration-id>/current.md`，验证前后 entry 完全一致后才报告成功。失败时原
`current.md` 保持或恢复为权威来源。若不同 ID 的新 entry 已存在，迁移只补入旧条目，已有文件及
revision 保持不变；任一 ID 冲突都在写入前失败。迁移期间所有 writer 必须遵守全局 migration lock。

`decision` 条目还可携带结构化上下文，不强制迁移旧条目：

```json
"decision_context": {
  "reason": "避免上下文持续增长，并保持 Worker 隔离。",
  "rejected_alternatives": [
    {
      "alternative": "持久 Worker Session",
      "reason": "它会增加隐藏状态和恢复复杂度。"
    }
  ]
}
```

新批准的 decision 必须记录非空 `reason`。只包含真正讨论过且有来源的替代方案；没有记录时省略
`rejected_alternatives` 或使用空数组。`decision_context` 用在其他 memory kind 上无效。没有该
字段的既有 decision 仍有效，不需要批量迁移。Decision context 解释项目知识，绝不为未来动作
授予权限。

`tags`、`aliases`、`status` 可选，status 默认为 `active`。目录构建器同时读取旧聚合格式与新拆分
格式，拒绝重复 ID、文件名不匹配、缺少独立 revision、无效 block 和非结构化当前声明，不得静默
创建不完整索引。

这是 Memory Worker 请求生产者的兼容变更。没有 Long-term 条目时，过去发送
`"current_memory": {}` 的生产者必须迁移为
`"current_memory": {"long_term_entries": []}`，否则请求校验失败。

Memory Worker 输出只是候选。提升前，小涛或强模型评审者必须验证其稳定、在当前 Task 之外仍有
价值，并有可达 `source_refs` 支撑。在 `memory/long-term/decisions/` 记录批准或拒绝；保留被拒绝
候选，避免反复评审同一薄弱声明。

### 不可变 Decision Record

新评审使用 `.xiaotao/memory/long-term/decisions/<decision-id>.decision.json` 作为不可变的通用决策
记录，并使用 [decision-record.schema.json](schemas/decision-record.schema.json) 校验。文件名必须与
`decision_id` 一致。示例：

```json
{
  "schema_version": 1,
  "record_type": "decision",
  "decision_id": "decision-activity-timeline",
  "title": "Activity 加入 Decision 时间线",
  "outcome": "approved",
  "importance": "milestone",
  "decided_at": "2026-09-11T10:00:00Z",
  "decided_by": "xiao-tao/session-or-run-id",
  "reason": "让用户能按时间回顾关键取舍。",
  "target_ids": ["issue-54"],
  "source_refs": ["docs/plans/2026-09-10-activity-timeline-design.md"]
}
```

先在规范路径外生成暂存文件，再执行严格校验；通过后才原子发布到规范路径：

```text
python xiaotao/scripts/validate.py decision-record <staged-record.json> --project-root <root>
```

`decided_at` 是批准、取代或拒绝实际发生的时间，发布后不可修改；不得用 `updated_at`、文件 mtime
或 Git 时间代替。`superseded` 记录还必须通过 `superseded_by` 指向替代项。`importance: milestone`
只用于会影响项目方向、架构或用户可感知能力的关键决策；日常评审使用 `routine`。Activity 只投影
`milestone` 的 `approved` 与 `superseded`，`rejected` 和 `routine` 仍保留为审计记录。

通用 Decision Record 仅在 `outcome: rejected` 时允许 `target_ids: []`，用于没有受影响目标的
候选拒绝记录；此时用 `source_refs` 引用候选。批准和取代仍要求至少一个目标 ID。

旧 Decision 格式继续可读且无需迁移；没有规范文件和可靠 `decided_at` 的历史记录不会进入
Activity。Decision Record 本身是评审权威，不依赖 Activity 存在。发布时必须严格验证其
`source_refs` 可达；后续读取只检查路径格式、项目内边界和不可逃逸，不因本地 Task 已归档或另一台
机器未同步历史证据而阻塞 Activity 重建。重要决策应优先引用纳入 Git 且路径稳定的证据。

### 演进提案

对每个提取出的 Long-term 候选，将其持久声明与索引条目比较，并在 `long_term_candidates` 中
输出一项提案。提案记录稳定 `candidate_id`、`memory_kind`、匹配分类、动作、冲突状态、理由、
结构化来源元数据和可达 `source_refs`。`fact` / `constraint` 候选可以携带 `valid_from`、
`valid_until`，但时间字段不能替代来源证据。更新已过期事实时必须引用新证据并正常评审。

先分类，再选择动作：

- `novel`：没有条目覆盖该声明；
- `duplicate`：已有条目覆盖同一声明；
- `overlap`：现有条目应吸收或合并新证据；
- `conflict`：当前证据与某条目矛盾；
- `low-value`：材料临时、一次性或不可复用。

优先维护而不是增加条目。符合 Long-term Memory 条件的候选按以下顺序处理：

```text
UPDATE → MERGE → CREATE
```

`UPDATE` 准确指向一个重叠或冲突条目；`MERGE` 指向至少两个；只有声明为 novel 且不指向任何
条目时才允许 `CREATE`。duplicate 或 low-value 输出 `SKIP`：duplicate 指向已经覆盖它的条目，
low-value 不指向目标。不得用 `CREATE` 逃避与现有主题比较。

`source` 元数据包含 `type: temporary | task`、来源 ID、创建时间和可选的宿主 `workspace_id`。
它有助于路由和审计，但不能替代 `source_refs`；可达来源文件才是权威证据。

这些动作是提案，不是写入。Memory Worker 不能应用或批准。小涛或强模型评审者要验证价值、
稳定性、匹配目标和来源，可以改变提议动作，并在任何已批准更新使用可变状态协议前，发布不可变
决策。

### 证据优先级与冲突

按以下顺序解决事实冲突：

```text
当前代码或运行时证据
> 当前 Task 已验证发现
> Long-term Memory
> 历史 References
```

高优先级证据不会让低优先级历史消失。当前证据与 Long-term 条目矛盾时，停止把旧条目当作当前
事实，并创建评审记录，包括：

- 稳定条目或候选 ID，以及正在评审的准确声明；
- 结果：`approved`、`rejected` 或 `superseded`；
- 旧声明与冲突证据双方可达的 `source_refs`；
- 评审者、时间戳、理由，以及 superseded 时的替代条目 ID。

评审后，通过可变状态写入协议更新目标 entry 文件。用新批准声明替换该 entry，或把被拒绝、已
取代的 entry 移入 `history/`。发布不可变决策，存在替代项时链接它。不得静默把旧声明改写成新
说法、删除其来源，或在已知矛盾未解决时仍作为当前事实。

矛盾尚未验证时，在当前上下文把旧条目标为 disputed，并在本次决策中采用较高优先级证据。
Memory Worker 可以提议 supersession，但必须由小涛或强模型评审者批准。

冲突提案在证据验证前使用 `conflict_status: pending-confirmation`，矛盾本身验证后使用
`confirmed`。两者在修改前都仍需评审。非冲突提案使用 `none`。不得为了通过校验而把 conflict
重新标成 overlap 或 novel。

## Current + References

默认加载 `current.md` 或 `current-state.md`。References 是历史锚点，只在当前决策、冲突、解释
或用户请求需要时加载。绝不要自动注入整个 Reference 树。

## Memory Awareness 与渐进检索

`.xiaotao/memory/` 下的 `manifest.md` 和 `index.json` 是本地派生目录文件。正式 Temporary、Task
和 Long-term 文件仍是权威来源。目录可删除并重建；绝不能通过编辑目录来修改 Memory 声明。

新 Session 启动时只读取已有且可验证的轻量快照（或运行 `overview --cached`），不扫描、刷新或重建 Catalog。用户提出具体记忆查询，或明确要求检查当前记忆全局状态时，再按需检查 freshness；缺失或陈旧时可重建：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> check
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> build
```

### Memory Catalog 有界访问协议与四层路由

`.xiaotao/memory/index.json` 是确定性脚本的内部派生索引，用于确定性过滤、排序与 freshness 校验。
**正常 Memory 使用中，模型严禁直接 Read / cat 完整 `index.json`**，避免把整个派生索引 dump 进上下文。
只有在用户明确要求调试、审计或检查原始 Catalog 结构时才允许直接读取。

日常 Memory 查询统一路由到有界接口：

| 用户意图 | 路由路径 | 接口与约束 |
| --- | --- | --- |
| 有什么记忆 / 当前记忆总览 | 轻量概览 | 读取 `.xiaotao/memory/manifest.md` 或运行 `overview` |
| 最新记忆 / 最近记了什么 / 最近几条 | 有界最新 | `recent --limit N`（默认 5 条有界元数据） |
| 之前有没有讨论过 X / 关于 X 的记忆 | 渐进检索 | `search "<query>"`（最多 5 条相关候选） |
| 看看这条记忆的详情 | 精确下钻 | `show <memory-id>`（单条完整当前内容） |

### 轻量总览（overview）

Session 启动时只读取已有且可验证的 `manifest.md` / Catalog 快照，或运行 `overview --cached`；用户明确要求检查当前记忆全局状态时，可运行会自动刷新的概览：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> overview
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> overview --format json
```

`overview` 默认检查 Catalog freshness，缺失或陈旧时自动重建，因此不用于 Session 启动。默认以 Markdown 文本形式返回
`manifest.md` 内容；传入 `--format json` 时返回结构化摘要（包含活跃 Temporary、活跃 Task、各层
计数与 `has_active_work` 标记）。启动阶段只使用已有快照形成初始 Runtime Context；快照缺失或不可用时如实说明，不将其解释为没有项目记忆。

### 最新记忆（recent）

查询最近更新的 Memory 条目元数据：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> recent --limit 5
```

- **有界输出：** 默认最多返回 5 条（`--limit` 必须在 1 到 5 之间）。输出仅包含路由所需的有界
  元数据（`memory_id`、`layer`、`record_type`、`title`、`summary`、`status`、`path`、`locator`、
  `updated_at`、可选的 `memory_kind`、`stale`、有效期与派生 `temporal_state`），不展开完整 `content`、`search_hints` 或
  Reference 树。需要详情时通过 `show <memory-id>` 单条加载。
- **支持过滤：** 支持 `--layer {temporary,task,long-term}` 过滤特定层级；默认仅返回 `active`
  且时间有效的条目，传入 `--include-inactive` 可包含历史、非活跃、尚未生效或过期条目；支持 `--no-refresh`。
- **时间语义与降级规则：** 排序基于权威记录中的 `updated_at`（规范 ISO-8601 UTC）降序排列，时间
  相同时以 `memory_id` 升序决胜。**绝不使用文件系统不可靠的 `mtime`、Git 提交时间或当前时间伪造
  Memory 时间**。没有可靠时间字段的记录（如未标注时间的 worker-state 或旧记录）排在所有具备可靠
  时间戳的记录之后，按 `memory_id` 升序确定性降级排序。

### 渐进检索（search 与 show）

初始概览只读取 `manifest.md`。用户请求可能受益于既往上下文时，使用当前请求、活动 Skill 或
Worker 上下文，以及已知绑定的 Temporary 或 Task 查询索引：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> search \
  "<current request>" --context "<skill or worker context>" --binding <memory-id>
```

检索器最多返回五个活动候选及 `relevance_reason`，也可以不返回候选。不得强行注入无关 Memory，
也不要打开每个返回来源。判断候选相关后，只通过稳定 ID 加载该记录：

```bash
python <xiaotao-skill-root>/scripts/memory_catalog.py --project-root <project-root> show <memory-id>
```

#### 同义词语义扩展与分层评分

为解决“由于自然语言用词差异（如搜‘鉴权’找不到只写了‘auth’或‘认证’的条目）导致的漏检”，检索器引入了受控的工程同义词网络：
- **内置工程领域词网：** 内置覆盖认证鉴权（auth/token/login/鉴权）、数据库（db/database/sql/数据库）、缓存（cache/redis/缓存）、存储持久化（storage/persist/落盘）、性能优化（perf/optimization/latency/耗时）、部署发布（deploy/release/ci/cd）、构建打包（build/bundle/compile）、搜索索引（search/query/index）、架构模块（arch/design/module）、调度派工（dispatch/worker/agent）、测试验证（test/verify/assert）、接口契约（api/protocol/contract）、异常排查（error/bug/debug）、生命周期（lifecycle/hook/startup）以及配置环境（config/setting/env）等高频工程场景；
- **项目级配置扩展：** 支持在 `.xiaotao/config.yaml` 中配置 `synonyms` 列表（逗号分隔或列表字符串），无缝扩展业务专有名词与缩写；
- **字面匹配优先原则：** 精确包含与词元匹配获得主导权重（如精确匹配 30/20 分），同义词召回作为受控补充权重（1~4 分）。精确匹配条目的排序始终绝对优先于纯近义词命中；
- **召回透明性：** 若条目因同义词扩展被召回，其 `relevance_reason` 会明确标注 `(synonym)`（例如 `title (synonym)`），确保检索决策透明可解释。

#### 项目全景概览条目与逐层下钻

为解决“碎片化记忆难以回答项目全局组织与模块地图”的问题，系统支持标准项目全景概览条目 `project.overview`（存放在 `.xiaotao/memory/long-term/entries/project.overview.md`）：
- **全景内容规范：** 包含核心模块职责划分、关键代码入口文件、全局设计约定以及指向深层知识条目的下钻索引；
- **宽泛提问优先召回：** 条目配置 `tags`（如 `overview`, `architecture`）、`aliases`（如 `项目结构`, `架构概览`, `代码地图`）与 `search_hints`（覆盖“项目怎么组织的”、“入口文件是哪个”等宽泛自然语言问题）；
- **渐进下钻路径：** 用户提出宏观问题时，检索器通过 `search "项目结构"` 首位召回 `project.overview`；小涛或 Worker 通过 `show project.overview` 获取系统概貌与子模块线索，再按需检索具体子模块条目，避免一次性倾倒全量知识。

#### 基于代码关联的新鲜度复核提示 (Code Freshness Check)

代码演进可能使过去的知识描述与最新代码实现脱节。为避免模型将已过期的记忆当成当前事实，系统在检索与展示阶段提供客观的代码变动复核提示：
- **代码关联机制：** 条目可在元数据中声明关联代码文件列表 `code_refs`，并记录代码基准指纹 `code_fingerprints`（SHA-256 映射）。若未显式声明 `code_refs`，检索器会自动识别 `source_refs` 中的代码文件（排除 `.xiaotao/` 内部文件）；
- **新鲜度状态推断：**
  - `fresh`：全部关联代码文件存在且 SHA-256 指纹与基准一致，表示知识与当前代码匹配；
  - `review_needed`：检测到关联代码文件内容已发生变动或文件被删除，输出明确依据 `reason: "关联代码文件 '<path>' 内容已变动"`，提示小涛/用户注意该知识可能已陈旧；
  - `unknown`：条目无关联代码文件（如仅来源于对话讨论或外部文档），或缺少指纹基线，系统如实标记为 `unknown`，**绝不虚报检测成功**；
- **底线守卫与防篡改原则：**
  - 复核提示（`code_freshness`）仅在 `search` 与 `show` 诊断输出中呈现，**绝对禁止自动化脚本或 Worker 静默改写、删除或篡改权威 entry**；
  - 知识库的更新与纠偏必须严格遵循既有的 Memory Worker 提案、小涛/用户评审确认以及决策归档流程。


审计查询在 `search`、`recent` 或 `show` 后增加 `--include-inactive`；返回结果会明确携带
`temporal_state`，避免把历史事实误当作当前证据。

`show` 从旧聚合或对应单文件提取一个 Long-term JSON block，或提取 Temporary、Task、Worker 的
有界当前章节。这是只使用一个条目而不注入全部 Long-term entries 的受支持方式。

确定性目录包含：

- 所有结构化 Long-term 条目；inactive 状态为审计保留，但从常规检索排除；
- 来自 `meta.yaml` 和简短当前章节的活动 Temporary 路由上下文；
- 活动 Task 目标和各 Worker `current-state.md`；
- 来自 `.xiaotao/memory/followups/pending/` 的待办跟进项。

### Temporary 陈旧感知

`manifest.md` 呈现活动 Temporary 时，根据其 `updated_at` 展示稳定的更新日期与陈旧状态标签（如 `(updated 2026-09-01, stale)` 或 `(updated 2026-09-09)`）。当时间超过陈旧阈值（默认 7 天，可在 `.xiaotao/config.yaml` 中配置 `temporary_stale_days`）时，索引将其标记为 `stale: true`。一旦跨越陈旧阈值，目录自动识别为 stale 并触发刷新。支持通过 `--now` 或环境变量 `XIAOTAO_CURRENT_TIME` 注入基准时间进行确定性构建与测试。

### 决策待办跟进项（Pending Follow-ups）

决策产生需要后续跟踪或跨 Session 执行的待办时，遵循决策不可变原则，不得直接改写历史决策文件，而是通过 `.xiaotao/memory/followups/` 独立管理：

- **待办创建：** 待办项存放于 `.xiaotao/memory/followups/pending/<followup_id>.yaml`，声明 `followup_id`、`title`、`status: pending`、`created_at`、指向决策等来源的 `source_refs` 以及可选的 `related_ids`；
- **目录可见：** 目录自动将 pending 待办索引入 `index.json` 的 `pending_followups`，并在 `manifest.md` 的 `## Pending follow-ups` 区块展示未完结事项；
- **解决归档：** 待办落地后，移入 `.xiaotao/memory/followups/resolved/<followup_id>.yaml`，设置 `status: resolved`，记录 `resolved_at`、`resolution` 解决说明与可选的 `resolution_refs`；
- **统一查询：** 支持通过 `show <followup-id>` 统一查看待办详情（已解决事项需使用 `--include-inactive`）。待办项不是第 4 层 Memory，而是决策落地与闭环的可见性跟进机制。

它不索引历史 Reference 树。`search` 默认刷新缺失或陈旧目录；`--no-refresh` 会把陈旧状态变成
可见错误。批准 Long-term 写入、Temporary 生命周期变化、Task 生命周期变化或 current-state
更新后重建。重建失败不得阻塞业务 Task：报告诊断，并回退到现有有界、source-first 读取。

## Memory Worker

只在以下边界触发：

1. Worker 完成一次较大的委派。
2. 用户确认 Session Handoff。
3. 从 Temporary Memory 创建正式 Task。
4. Task 完成或归档。
5. 用户明确要求“创建这条 Memory”或“记住这条信息”。

宿主支持模型选择时使用配置的 memory model。瞬时故障或无效输出后重试一次，再回退到主模型。
宿主具备原生隔离子代理（如 Codex `spawn_agent`）时，委派给独立原生 Memory Worker（中文称呼“记忆整理员”）；
宿主不支持独立隔离子代理时，如实报告 `unsupported`，由当前 Agent 执行相同的有界压缩（In-Session Fallback），
并在执行记录中明确标记 `execution: in-session-fallback`，绝不得虚报为独立 Worker 运行。
如果全部尝试失败，将完整请求和来源写入 `memory/pending/`，并继续业务 Task。

Memory Worker 负责整理 memory 和有界 Experience Review。它按 memory 能力选择或生成，不是
预置角色。它受 [Core Guard](guard.md) 与 [workers.md](workers.md) 约束：
- **工具白名单仅限只读**：仅允许读取指定文件与查询已知条目；严禁提供写工具、修改工具、命令执行或网络工具。
- **提案禁止自我批准**：Memory Worker 只能生成候选提案，绝对不能批准自己的 Long-term 或
  Playbook 候选，严禁直接改写 `.xiaotao/memory/long-term/entries/` 或 `.xiaotao/playbooks/`。
- **职责不得越权**：它不得选择其他 Worker、作出架构决定，或改变 Task 范围。该评审不会引入新的预置角色或 Runtime。

每个请求还暴露 `current_playbooks`；项目没有 Playbook 时也要提供空数组。每个索引 Playbook
包含稳定 `playbook_id`、规范 `file_path`、标题、触发条件、有序步骤、检查、active 状态、
revision 元数据和可达 `source_refs`，使 Worker 在提出新指导前比较已有流程。省略
`current_playbooks` 的生产者必须迁移，否则请求校验失败。

### 显式 CREATE 轻量路径

用户明确要求创建或记住一条内容，且 `memory_kind` 与来源清楚、不需要从历史推导时，请求使用
`operation: explicit-create`。先以待保存内容查询 Catalog，最多通过 `show` 读取 3 条相关 Long-term
详情，然后构造：

```json
{
  "current_memory": {
    "scope": "bounded",
    "query": "待保存内容",
    "limit": 3,
    "long_term_entries": []
  }
}
```

`scope: full` 表示完整镜像；旧请求省略 `scope` 时按 `full` 解释，以保持兼容。只有
`explicit-create` 可以使用 `bounded`，并且 `query`、`limit` 必填，`limit` 与实际条目数均不得超过
3。其他持久边界继续使用 `full`，不能用轻量路径规避 UPDATE、MERGE 或冲突检查。

用户明确的创建指令同时授权无冲突的 `CREATE`：Memory Worker 仍只输出候选，由小涛独立校验、
发布 Decision Record 并写入 entry，无需重复询问。duplicate 直接 `SKIP`；任何 conflict、UPDATE
或 MERGE 都停止轻量写入并请求用户确认。

用户提供文件时直接引用该文件；来自 Task 时引用 Task source。只有聊天内容可作为来源时，将用户
原话保存到 `.xiaotao/memory/sources/<source-id>.json`，使用
[memory-source.schema.json](schemas/memory-source.schema.json) 校验后再放入 `source_files` 与
`source_refs`，Memory Worker 候选的 `source.type` 使用 `user-message`，`source.id` 使用同一个
`source_id`。`source_id` 是正文 SHA-256 前 16 位，正文摘要必须匹配；文件使用独占创建，已存在时
只允许字节完全相同，禁止覆盖。不得保存秘密、凭据或用户未授权持久化的敏感内容；遇到此类内容时
改用安全的项目来源或请求用户提供可持久化版本。

合并流程有界且有序：

1. 从提供的来源文件提取可复用事实、经验、原则、决策或约束。
2. 按内容、适用性、证据和稳定 ID，将每项声明与 `current_memory.long_term_entries` 比较。
3. 分类为 novel、duplicate、overlap、conflict 或 low-value。
4. 按上述演进规则提出 `UPDATE`、`MERGE`、`CREATE` 或 `SKIP`。
5. 单独识别拥有明确触发条件、有序可复用步骤、检查和真实执行证据的流程。与
   `current_playbooks` 比较，并按 [playbooks.md](playbooks.md) 输出 `playbook_candidates`。
   讨论、常规命令、Task 时间线和未经验证的建议都是 `SKIP` 材料。
6. 返回两个提案集合供校验和独立评审；不得修改 Long-term Memory 或 Playbooks。

每个 Memory Worker 响应记录生成它的已校验请求的项目相对 `request_file`。调用方通过
`--request` 独立传入该请求；响应不能选择自己的校验上下文。响应守卫加载并校验外部请求，确认
`request_file` 为审计解析到同一文件，然后强制执行：

```text
match.playbook_ids ⊆ current_playbooks.playbook_id
```

外部请求无效或不可达、审计路径不匹配，或目标 ID 不在该请求的当前 Playbook 快照中，都会使
响应无效。`CREATE`、`UPDATE`、`MERGE` 至少需要一个可达 `evidence_ref`；`SKIP` 可以使用
`evidence_refs: []`，但仍需要可达 `source_refs` 以供审计。

使用以下 Schema 校验结构化输入和输出：

- [memory-worker-request.schema.json](schemas/memory-worker-request.schema.json)
- [memory-worker-response.schema.json](schemas/memory-worker-response.schema.json)
- [memory-source.schema.json](schemas/memory-source.schema.json)

持久化任一正式工件前，立即运行对应的工件触发协议守卫：

```bash
python xiaotao/scripts/validate.py memory-request <file> --project-root <project-root>
python xiaotao/scripts/validate.py memory-response <file> --request <request-file> --project-root <project-root>
python xiaotao/scripts/validate.py memory-source <file> --project-root <project-root>
```

只有校验成功才能持久化规范工件。失败后修复并再校验一次；第二次仍失败时，将完整原始结果以
`.invalid.json` 后缀保存在预期工件旁，记录诊断，并按现有回退规则继续；不得把无效文件当成
Memory Worker 请求或响应。
此校验不得创建或转换 Task、Temporary、Workflow、delegation、phase 或固定角色调用；它只检查
工件及其可达的项目相对文件引用。

## 团队共享 Memory 与 Git 语义合并

多个开发者或 Agent 在并行 Git 分支工作时，提交进 Git 的团队共享 memory
（`.xiaotao/memory/long-term/entries/*.md`、历史、Playbooks 和已评审决策）可能分叉。不同 entry
文件的独立修改由 Git 正常合并；同一 entry 或多文件语义变化发生冲突时，标准 Git 文本
合并无法解决语义演进或发现矛盾。

### 三方语义合并协议

语义合并需要三方输入：`BASE`（共同祖先版本）、`OURS`（当前分支版本）、`THEIRS`（传入分支
版本），以及项目相对 `file_path`。

Memory Merger 按以下确定性规则，将 `OURS`、`THEIRS` 与 `BASE` 比较：

1. **相加且不冲突：** 两个分支分别引入新颖、独立声明时，两者都保留。
2. **等价或重复经验：** 表达同一已验证经验的条目合并为一个，去重表述并保留双方所有可达
   `source_refs`。
3. **矛盾发现：** 分支得出互斥声明时（例如异步与同步初始化），AI 不得静默决定胜者。将两项
   声明保留在 `unresolved_conflicts`，设置 `status: pending-confirmation` 和
   `requires_human_review: true`。
4. **防止已取代/拒绝 Memory 复活：** 某条目在一个分支的决策历史中标记为 `superseded` 或
   `rejected` 后，合并仍含 active 旧条目的陈旧分支时，不得使其复活。Tombstone 状态优先。

### 冲突来源协议

每个未解决冲突分配稳定 `conflict_id`，推荐格式为 `cnf-<timestamp>-<suffix>`（例如
`cnf-20260828t120000z-a1b2`），并为 `ours`、`theirs` 双方记录完整来源：

- `author`：提交开发者或 Agent；
- `branch`：来源 Git 分支；
- `commit`：来源 commit hash 或引用；
- `task_id`：来源 Task 或 Temporary ID；
- `memory_path`：规范 memory 文件路径；
- `claim`：有争议的准确声明；
- `source_refs`：可达证据和链路文件；
- `created_at`：RFC 3339 时间戳。

### 冲突生命周期

通过四个状态跟踪未解决冲突：

```text
发现冲突 → pending-confirmation → resolved → active / superseded / rejected
```

冲突以 `status: pending-confirmation` 持久化到 `.xiaotao/memory/long-term/conflicts/`。经过人工或
证据评审后，发布不可变决策，把状态转换为 `resolved`，将已确认声明写入目标 entry 文件，并用
`superseded_by` 引用标记被取代声明，保证完整可审计性。

### Memory Merger Worker

Memory Merger Worker 处理三方 memory 合并请求，不改变 Task 范围，也不单方面作出架构决定。
使用以下 Schema 校验其结构化输入输出：

- [memory-merge-request.schema.json](schemas/memory-merge-request.schema.json)
- [memory-merge-response.schema.json](schemas/memory-merge-response.schema.json)

运行工件触发协议守卫：

```bash
python xiaotao/scripts/validate.py memory-merge-request <file> --project-root <project-root>
python xiaotao/scripts/validate.py memory-merge-response <file> --project-root <project-root>
```
