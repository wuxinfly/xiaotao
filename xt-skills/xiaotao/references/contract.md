# XiaoTao Skill 契约

当 Host Adapter 或 Worker 需要解释或暴露 XiaoTao 的通用输入输出语义时，使用本 Reference。
这是 Agent 行为契约，不是 Runtime API；普通请求不强制输出 JSON、持久化状态或委派。
普通的 XiaoTao 介绍和使用说明不需要加载本文件。

## 输入契约

以下字段是概念输入。只有当前步骤需要时才获取；不要为了凑齐字段加载无关内容：

| 字段 | 含义 |
| --- | --- |
| `user_request` | 当前用户请求，包括明确限制和实施意图。 |
| `project_context` | 与当前工作有关的最少项目文件、事实和状态。 |
| `memory_context` | Manifest、当前记录或有限检索结果；默认不加载全部 Memory。 |
| `available_capabilities` | 宿主实际提供的工具、子代理、模型和隔离能力。 |
| `authorization_context` | 当前请求明确授权的操作、目标和范围。 |

始终只有 `user_request` 是必需的。缺少的可选输入继续保持缺失；不要通过编造或加载无关历史
来补齐它。

## 输出契约

根据请求返回尽可能简单的结果：

- 普通对话：自然语言回答；
- 实质执行：结果摘要、验证情况、限制和相关 Artifact 路径；
- 持久化委派：符合 [handoffs.md](handoffs.md) 的 Handoff；
- 需要决定：一个简短问题，并说明该决定会影响什么；
- 无法支持：明确的 `degraded` 或 `unsupported` 结果和原因。

只有当对应 schema 要求时才输出结构化数据。结构化结果写入前必须验证；不要把模型生成的
JSON、YAML、路径、权限或状态声明直接当成可信输入。

## 提案与权限

提案不能批准自身。解析结果、Worker 定义、Memory 候选、Playbook 候选、Handoff 或 Adapter
提醒只能描述建议和需要的权限，不能授予权限。实际执行仍受当前用户授权、宿主能力以及
[coordination.md](coordination.md) 中安全边界的约束。

## 宿主降级

Core 描述语义，不假设具体宿主功能。如果宿主无法注入必需指令、落实边界、恢复委派运行或提供
所需能力，按照对应 Reference 返回明确的 `degraded` 或 `unsupported`；不得声称不存在的
隔离或委派已经发生。

本契约不改变渐进式加载。对话和澄清仍只使用 Core，除非当前步骤确实需要本文件。
