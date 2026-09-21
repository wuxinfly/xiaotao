# 结果与 Handoff

一次较大的 Worker 委派会产生三项工件。小涛消费这些工件、判断证据，并且只告诉用户下一次
决策所需的内容。

## Detailed Result

将完整工作产物写入：

```text
.xiaotao/tasks/<task-id>/workers/<worker-id>/runs/<timestamp>-result.md
```

持久化的探索性 Worker 使用对应的
`.xiaotao/memory/temporary/active/<temporary-id>/workers/<worker-id>/` 路径。Session 作用域的
一次性工作直接返回结果，不声称拥有可恢复状态路径。

内容包括已执行工作、证据、分析、结论、风险、Open questions 和相关 Artifact 路径。这是技术
记录，不要把它复制到日常的用户进度更新里。

## Current State

使用 [memory.md](memory.md) 中的字段更新 Worker 的 `current-state.md`。它使同一执行单元无需
此前的 Agent Session 也能恢复。Worker 恢复时，同时加载不可变 `spec.yaml` 快照和 Current
State。

## 轻量 Handoff

只返回小涛判断结果和决定下一步所需的内容：

```json
{
  "status": "completed",
  "summary": "已测量启动开销，并定位到占比最高的模块。",
  "result_path": ".xiaotao/tasks/<task-id>/workers/frontend-performance/runs/<timestamp>-result.md",
  "worker_state_path": ".xiaotao/tasks/<task-id>/workers/frontend-performance/current-state.md",
  "needs_user_input": false,
  "questions": [],
  "recommended_next": [
    {
      "capabilities": ["architecture-design", "runtime-analysis"],
      "reason": "使用已记录的性能数据评估边界调整"
    }
  ]
}
```

下一步建议使用非空 `capabilities` 列表重新解析，绝不直接选择下一个 Worker，也不授予其权限。

Worker 因等待用户而阻塞时，Handoff 携带小涛需要询问的准确问题：

```json
{
  "status": "blocked",
  "summary": "仍有两种安全实施路径，选择会影响初始化顺序。",
  "result_path": ".xiaotao/tasks/<task-id>/workers/startup-design/runs/<timestamp>-result.md",
  "worker_state_path": ".xiaotao/tasks/<task-id>/workers/startup-design/current-state.md",
  "needs_user_input": true,
  "questions": [
    {
      "question": "是否允许修改启动初始化顺序？",
      "reason": "这个决定会改变后续优化方案和回归测试范围"
    }
  ],
  "recommended_next": []
}
```

`needs_user_input: true` 要求 `status: blocked`，并至少包含一个简洁问题及其决策上下文。小涛
可以直接依据 Handoff 提问，只有回答需要更多支撑细节时才读取 Detailed Result。`blocked`
不代表一定需要用户输入：Worker 也可能正在等待其他依赖。当 `needs_user_input` 为 false 时，
省略 `questions` 或使用空数组；不得沿用过期问题。

每个持久化 Handoff 必须提供一个 `worker_state_path`。下一步只使用非空 `capabilities` 重新
解析；旧 `role_state_path` 和 `role` 字段不再接受，也不提供旧任务迁移或恢复。

校验器是由工件触发的协议守卫，不是 Workflow 触发器。持久化机器生成的 Handoff 前立即运行：

```bash
python xiaotao/scripts/validate.py handoff <file> --project-root <project-root>
```

只有校验成功才能持久化规范 Handoff。校验失败后，修复工件一次并重新校验。如果仍然失败，
将完整原始结果以 `.invalid.json` 后缀保存在预期工件旁，并记录校验诊断；绝不能静默接受为
Handoff。校验不得创建 Task 或 Temporary、启动 Workflow、委派工作、调用固定角色或引发阶段
转换。

CLI 强制执行 [handoff.schema.json](schemas/handoff.schema.json)、可移植的项目相对路径，以及
结果与状态文件的可达性。Task 作用域 Handoff 位于 `.xiaotao/tasks/<task-id>/handoffs/`；
持久化的 Temporary 作用域 Handoff 位于
`.xiaotao/memory/temporary/active/<temporary-id>/handoffs/`。Session 作用域工作不持久化
Handoff。

小涛不应读取每个 Detailed Result。仅在 Worker 阻塞、结论冲突、决策所需细节超过结构化问题、
用户要求查看分析，或其他 Worker 需要来源时读取。

在 Worker 之间直接传递路径，不要让完整结果反复经过小涛的上下文。

## 面向用户的结果

小涛先说结论，并且只包含适用的内容：

- 结果；
- 验证或证据路径；
- 不确定性、限制或阻塞项；
- 需要用户决定的事项，或推荐的下一步。

除非用户主动询问，否则不要暴露常规代码搜索步骤、命令、内部 ID、能力路由或完整 Detailed
Result。
