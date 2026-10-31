# Agent 规则演进

根目录 `AGENTS.md` 是 AI 开发入口，不是手写规则仓库。它的唯一权威来源是
[`rules.json`](rules.json)，由 [`tools/agent-rules.mjs`](../../tools/agent-rules.mjs) 生成和校验。

## 目标

规则应随着项目事实、用户纠正和验证经验持续变准，但不能因为一次误判被永久污染。机制采用
“自动发现候选、显式确认应用、CI 校验结果”的闭环：

```text
任务事实 / 用户纠正 / 验证失败
        -> 规则候选
        -> 审查与证据校验
        -> rules.json
        -> AGENTS.md
        -> CI 防漂移
```

## 规则源

`rules.json` 只保存项目级、可复用的规则。每条规则必须包含：

- 稳定 `id`、`kind` 和可执行的 `text`
- 至少一个可达的 `source_refs`
- 所属章节

`kind` 分为：

| 类型 | 含义 | 应用门槛 |
| --- | --- | --- |
| `fact` | 可由当前仓库验证的事实，例如路径、模块职责、命令 | 明确确认 |
| `convention` | 团队约定或工作方式 | 明确确认 + 审查人 |
| `preference` | 跨任务稳定偏好 | 明确确认 + 审查人 + 至少两次观察 |
| `safety` | 权限、安全、发布、删除等边界 | 明确确认 + 审查人 + 至少两次观察 |

任何候选都不能自动放宽权限、安全、发布、删除数据或访问控制边界。事实类规则也只允许更新
可验证的当前事实，不能把模型总结当作证据。

## 候选

待审候选放在 `docs/agent/candidates/<candidate-id>.json`。候选格式：

```json
{
  "schema_version": 1,
  "candidate_id": "project-position-client-status",
  "created_at": "2026-10-31T00:00:00Z",
  "action": "update",
  "kind": "fact",
  "section_id": "project-position",
  "rule_id": "project-position.summary",
  "text": "新的规则文本",
  "rationale": "为什么现有规则不准确",
  "source_refs": ["README.md", "xt-client/README.md"],
  "observations": 1
}
```

`action` 支持 `add`、`update`、`remove`。`add` 的 `rule_id` 必须尚不存在；`update` 和
`remove` 必须指向现有规则。候选不直接修改正式规则。

以下内容不进入候选：

- 一次性命令或只对当前文件有效的临时要求
- 没有仓库证据的推测
- 用户没有明确表达、仅由沉默推断出的偏好
- 凭据、敏感数据或第三方隐私

## 操作

在仓库根目录运行：

```bash
node tools/agent-rules.mjs check
node tools/agent-rules.mjs render
node tools/agent-rules.mjs apply <candidate-id> --confirm <candidate-id>
node tools/agent-rules.mjs apply <candidate-id> --confirm <candidate-id> --reviewed-by <name>
node tools/agent-rules.mjs list
```

`check` 会验证规则源、证据路径、候选格式和生成的 `AGENTS.md` 是否一致。`render` 只重建
生成文件。`apply` 需要重复传入候选 ID作为确认；非 `fact` 规则还必须有 `--reviewed-by`。

应用成功后：

- 更新 `rules.json`
- 重新生成 `AGENTS.md`
- 在 `docs/agent/decisions/` 写入不可变决策记录
- 将候选归档到 `docs/agent/candidates/applied/`
- 删除原待审候选

任一步失败会保留候选并尝试恢复已写入文件。已发布决策记录不重写；同一候选 ID不得重复应用。

## 自动化边界

Agent 可以在任务收尾时发现并提交候选，但不能把“我观察到”“用户没反对”当成批准。候选
应用是显式动作；高风险规则永远需要用户确认。CI 只验证结构和证据，不代替产品或安全判断。
