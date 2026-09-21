# Checkpoint Recovery Activity 设计

## 目标与边界

Issue #66 只把具有用户价值的恢复完成结果投影到 Activity，不改变 checkpoint/recovery
本身的保存、冲突或恢复语义。

Activity 仅纳入一次事件：一个尚未提交的 checkpoint 请求通过显式 `retry` 成功提交。
事件类型为 `checkpoint_recovered`。普通 `save` 成功、自动 checkpoint、
`already_committed`、`inspect`、`status`、CAS 冲突、失败记录和锁操作都不投影。

这样 Activity 表达的是“项目从一次可恢复故障中恢复完成”，而不是内部保存频率。

## 权威记录

恢复真正完成的边界是 `retry` 已确认 canonical 内容等于请求的 `proposal_hash`，并成功发布
不可变的 `<request_id>.committed.json`。新 observation 增加：

```json
{
  "request_id": "xiaotao-checkpoint-example-1",
  "record_hash": "...",
  "proposal_hash": "...",
  "revision": 8,
  "completion": "recovery",
  "committed_at": "2026-09-12T11:28:04.506Z"
}
```

`completion: recovery` 只能由 `retry` 的成功路径写入。普通 `save` 发布的新 observation 使用
`completion: save`，用于协议验证但不进入 Activity。`committed_at` 在 observation 首次发布时
生成并保持不可变，是 Activity 唯一认可的事件时间。

调用入口是 completion 语义的唯一判据：`save` 与 `retry` 会把明确的 completion 参数传给共享
提交函数。即使同一个 `request_id` 的既有 pending 请求由再次调用 `save` 最终提交，它仍是
`completion: save`，不解释为恢复，也不进入 Activity。只有操作者显式调用 `retry` 并成功提交，
才形成 `completion: recovery`。这一选择刻意把“恢复成功”限定为现有 recovery 协议的显式动作，
避免根据请求是否曾存在或是否有 failure event 反推用户意图。

旧 observation 不迁移。它们继续参与既有幂等与提交状态判断，但因为缺少 `completion` 或
`committed_at`，Activity 不猜测时间，也不投影事件。`updated_at`、失败事件的 `recorded_at`、
文件 mtime 和 Git 时间均不能替代恢复完成时间。

## Observation 验证协议

当前实现通过重新生成无时间 observation 后做逐字比较。加入权威时间后，新增
`checkpoint-observation.schema.json`，并让原生 validator 与 Adapter 读取方共同执行双版本校验：

- `request_id`、`record_hash`、`proposal_hash`、`revision` 必须与请求记录严格一致；
- 新格式必须同时包含 `completion` 和带时区的 `committed_at`；只出现其中一个属于损坏；
- `completion` 只允许 `save` 或 `recovery`；
- 旧格式只能是原有四字段形式，仍按旧规则接受；
- 多余字段、无效时间、字段不匹配或无法解析均明确报 `invalid_observation`。

Schema 使用互斥分支表达旧四字段格式和新六字段格式；原生 validator 负责与请求记录有关的
跨文件约束。Adapter 不会在读取时重新生成 `committed_at`，而是验证已发布 observation 的时间
与稳定字段。首次创建 observation 时生成 `new Date().toISOString()`；已有合法 observation 永不
重写，因此重复 `save`、`retry` 或 `status` 不会改变完成时间或 completion。

不可变写入与现有锁、CAS、secondary recovery 规则保持不变。Activity 的读取或重建不参与
checkpoint 提交事务，也不能影响恢复是否成功。

## Activity 派生

派生器扫描规范 Task 与 Temporary 的 checkpoint observation 目录，包括活动和归档目标。
Task metadata 仍只扫描规范 `task.yaml`，但 checkpoint recovery 是独立来源模式：只扫描每个规范
目标直属 `references/checkpoints/*.committed.json`，不递归读取其他 references、failure event 或
artifact。它以相邻的 `<request_id>.json` 请求记录作为绑定依据。

仅当 observation 通过原生等价约束、`completion` 为 `recovery` 且存在合法 `committed_at` 时，
生成：

```json
{
  "event_type": "checkpoint_recovered",
  "occurred_at": "2026-09-12T11:28:04Z",
  "title": "恢复 checkpoint：验证 DSH checkpoint",
  "summary": "恢复 Temporary 20260912-验证DSH-checkpoint 到 revision 8",
  "source_refs": [
    ".xiaotao/memory/temporary/active/20260912-验证DSH-checkpoint/references/checkpoints/xiaotao-checkpoint-example-1.committed.json"
  ],
  "status": "completed"
}
```

稳定事件身份由 `checkpoint_recovered`、目标种类、目标 ID、`request_id` 和归一化后的
`committed_at` 生成。`request_id` 的复用检查只在同一目标内生效，因此目标种类和目标 ID 是
稳定身份的必要组成，允许不同目标安全使用相同 request ID。`committed_at` 可保存毫秒，但 Activity 沿用现有
`normalize_utc` 规则，截断为 UTC 秒精度；事件 ID 与 `occurred_at` 都使用这个截断后的值，避免
JavaScript 与 Python 各自处理精度。重复查询、重复 `retry` 和 Activity 重建得到同一个事件 ID。

`source_refs` 指向 `.committed.json`，因为它是恢复已经完成的不可变边界；pending 请求只描述待
提交提案，不能独立证明恢复成功。需要核查提案时，再由同目录 request ID 关联请求记录。

所有规范目标下直属的请求记录与 committed observation 都参与 `source_digest`，确保新增恢复、
请求链变化或权威记录损坏会使缓存失效。Activity Index 缺失、损坏或陈旧时仍可重建；删除
`.xiaotao/activity/index.json` 不会修改请求、canonical 状态、failure event 或 observation。

## 错误与兼容策略

- 没有新时间字段的旧 observation：合法但不投影；
- 新格式 `completion: recovery` observation 存在而请求记录缺失：权威链损坏，构建失败；
- 新格式 `completion: save` 与旧格式 observation 不投影；派生器不要求其相邻请求存在；
- JSON、字段、哈希、revision、时间或目标路径非法：构建失败；
- 同一规范目标出现重复 request identity：构建失败；
- secondary-only 记录尚未恢复时没有 committed observation，因此不投影；
- 历史 failure event 可保留审计价值，但不是 Activity 来源。

## 验证计划

1. Adapter 测试覆盖 save/retry 写入不同 `completion`，时间不可变，以及旧/新 observation 验证。
2. Schema 与原生 validator fixtures 覆盖 `checkpoint_recovered` 合法事件及未知类型失败。
3. Activity 测试覆盖 Task/Temporary、活动/归档、时间窗口、稳定 ID、重建去重、旧记录跳过。
4. 损坏 observation、缺失请求、哈希/revision 不匹配、无效时间和越界路径必须显式失败。
5. 删除 Activity Index 后重建，确认 checkpoint/recovery 权威文件字节不变。

## 落地文件

实现必须同步更新以下契约面，避免 schema、原生 validator、文档和派生器分叉：

1. Adapter 的 checkpoint 类型、提交入口、observation 写入/验证及测试；
2. 新增 `checkpoint-observation.schema.json` 并注册到原生 validator；
3. `activity-event.schema.json`、`validate.py` 与 `activity_catalog.py` 的事件类型；
4. Activity 来源发现、digest、恢复事件派生和目录边界测试；
5. `references/activity.md`、`references/storage.md` 与 `docs/manual-acceptance.md`；
6. `scripts/verify-contracts.ps1` 的必备契约列表；
7. `test/activity-catalog.test.js` 和 Adapter checkpoint 测试；
8. validator fixtures：合法 `checkpoint_recovered`、非法 observation 与未知事件类型回归。
