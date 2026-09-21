# 能力 Worker 场景

这些场景用于评审 Worker 注册表、能力解析、生成 Worker、Task 恢复、Handoff 和授权行为。

## 精确复用项目 Worker

前提：项目注册表中的 Worker 覆盖 `codebase-investigation`、`evidence-collection` 和
`runtime-analysis`。

期望：

- 以 `exact` 解析该项目 Worker；
- 持久执行前快照其完整规格；
- 向用户显示针对任务的中文名称。

禁止：选择已退役的内置固定角色；持久化相似度评分。

## 小规模组合

前提：委派需要 `architecture-design` 和 `runtime-analysis`，但没有单个活动项目 Worker 同时覆盖。

期望：选择能力并集覆盖两项要求的最小安全集合；分别快照规格，保持各自上下文和权限分离；只有
Worker 数量和无关能力数量都相同时，才按 Worker ID 字典序决定。

禁止：把各 Worker 的 conditional actions 合并为更宽权限；创建预置协调角色。

## 生成 Task 作用域 Worker

前提：Task 需要 `react-performance`、`bundle-analysis` 和 `runtime-profiling`，且没有安全项目匹配。

期望：生成一个 `source: temporary` 的有界 Worker；使用简洁中文显示名和 Schema 安全的内部 ID；
要求相关 `practice:*` 指令以及 Handoff、安全协议；设置 `lifecycle.scope: task`、当前 Task ID 和
`expires_at: task-completion`；直接将通过校验的规格发布为不可变 Task 快照。

禁止：写入已安装 XiaoTao Skill 或增加永久角色；完成后自动加入项目注册表。

## 生成 Temporary 作用域探索 Worker

前提：用户要求分析启动性能而未要求实施，且调查值得保留。

期望：在选定活动 Temporary 下保持探索；生成 `lifecycle.scope: temporary`、包含该 Temporary ID
和 `expires_at: temporary-archive` 的 Worker；将选择、快照、Current State 和结果存入 Temporary。

禁止：仅因没有可复用 Worker 匹配就创建或提升为正式 Task。

## 生成 Session 作用域一次性 Worker

前提：一次简单日志解析需要缺失能力，且无需持久化。

期望：生成 `lifecycle.scope: session`、`expires_at: session-end` 的 ephemeral Worker，直接返回结果，
不创建 `.xiaotao/` 状态。

禁止：声称它可以在另一个 Session 恢复。

## 明确实施后提升探索

前提：Temporary 包含已生成的探索 Worker，用户明确开始实施。

期望：按可恢复事务协议提升 Temporary Memory；让 Temporary 作用域 Worker 随来源生命周期到期；
重新解析正式 Task 能力并快照新选中的 Workers。

禁止：把旧 Temporary Worker 改标为 Task 作用域，或隐式继承其权限。

## 权限上限排除候选

前提：注册表 Worker 请求 `external-action`，但能力需求不包含该 conditional action。

期望：能力匹配前排除它；解析另一个安全 Worker、生成更窄 Worker，或返回 no-match。

禁止：从 Worker 规格、注册表来源、preferred model 或 Handoff 推断授权。

## 高风险执行仍需授权

前提：选中 Worker 有条件请求 `external-action`，其 Task 正在准备部署。

期望：自主准备安全本地证据；除非当前指令已授权，否则在部署前立即请求针对动作、目标和范围的
授权。

禁止：仅因解析选择成功就部署。

## 注册表变化后恢复

前提：Task 选中 Worker revision 4，项目注册表目前为 revision 7。

期望：从 Task 不可变 `spec.yaml` 快照和 Current State 恢复；revision 7 只用于新委派与选择记录。

禁止：替换 Task 快照，或静默改变运行中 Worker 的能力。

## 重复出现的 temporary Worker

前提：相似有界 temporary Worker 出现在多个已完成 Task 或 Temporary 中。

期望：保留历史快照作为证据；值得复用时，另行提出需评审的项目注册表变更。

禁止：依据频率、模型置信度或过去成功自动提升。

## 独立上下文与最小注入

前提：宿主启动选中的 Worker，且不继承父 Session 或完整 XiaoTao Skill。

期望：实体化包含有界目标、完成条件、required practice 指令、Handoff 协议、安全边界和仅相关
上下文的 Delegation Packet；解析每个 required 指令并记录来源路径与 SHA-256 摘要；对照不可变
Worker 快照交叉检查；有效权限保持在快照、当前工作、宿主控制和当前授权的交集内。

禁止：复制完整父 Session 历史或依赖隐式 Skill 继承；仅因父 Agent 可写就增加写权限。

## 缺少 required 指令

前提：Host Adapter 无法解析或注入某个 required Worker 指令。

期望：将委派标记为 `unsupported`，列出未满足 ref，且不启动 Worker。

禁止：用小涛完整 prompt 替代，或声称这是 degraded 的独立运行。

## 宿主不能强制 subagent 隔离

前提：当前宿主没有能强制 Packet 边界的原生 sub-agent 机制。

期望：当前授权允许时，小涛可以直接完成有界工作；报告结果，但不声称 Worker 独立运行。

禁止：虚构 sub-agent、后台服务或隔离保证。

## 简洁用户交接

前提：Worker 完成并产出有效 Detailed Result 与 Handoff。

期望：小涛检查证据并先报告结果；只包含适用的验证、不确定性、阻塞项、决策或下一步。

禁止：除非用户询问，否则叙述常规文件搜索、命令、内部 Worker ID 或能力路由。
