# Issue #92 验收与场景验证：全局用户记忆、项目记忆与项目知识库分层

## 背景与目标

本方案针对 GitHub Issue #92，重构长期内容组织架构，将原先扁平堆叠的 Long-term 内容划分为**全局用户记忆**、**项目记忆**与**项目知识库**三层，解决长久以来内容混杂、难以沿时间回看项目经历以及跨项目协作习惯无法沉淀的问题。

## 三层结构与定位验证

| 层次 | 规范路径 | 核心回答 | Schema / 规范 |
| :--- | :--- | :--- | :--- |
| **全局用户记忆** | `~/.xiaotao/memory/` 或 `$XIAOTAO_HOME/.xiaotao/memory/` | “我通常怎么协作？在各项目做过什么？” | `global-preference.schema.json` |
| **项目记忆** | `.xiaotao/memory/timeline/` | “这个项目经历了什么？当时为什么这么做？” | `timeline-event.schema.json` |
| **项目知识库** | `.xiaotao/memory/long-term/` | “项目现在是什么样？规则规范是什么？” | `long-term-entry` 现行主题库 |

## 核心规则与场景验证

### 场景 1：候选分流路由（Admission & Routing）
- **经历进入时间线**：Task 完成曲折、调试经过、架构选型讨论进入 `timeline/`，保留日期层及 Task 原始证据；
- **规范进入知识库**：已核实的代码风格、现行架构说明与技术约定进入 `long-term/entries/`，按主题分类；
- **习惯进入全局记忆**：跨项目的通用协作偏好（如简洁大白话沟通）进入 `preferences/`。**硬性约束**：严禁单次推断稳定偏好，必须具备两次以上跨会话/任务事实佐证。

### 场景 2：逐层有界下钻（Progressive Retrieval）
- **项目记忆下钻**：`项目总览` → `年度总览 (years/2026/summary.md)` → `月度摘要 (09/summary.md)` → `每日事件 (23.yaml)` → `具体 Task/Temporary 证据`；
- **知识库下钻**：`项目总览` → `主题/标签` → `当前说明条目` → `来源与历史`；
- **全局记忆下钻**：`用户总览` → `偏好分类/经历年月` → `跨项目经历简述` → `项目记忆入口`。

### 场景 3：权威记录与平滑兼容
- 现存 `long-term/entries/` 平滑演进为项目知识库，稳定 ID 与历史决策保持完全兼容；
- 机器 Activity（`.xiaotao/activity/index.json`）与项目记忆时间线事件严格对齐事件 ID 与发生时间，避免构造两套相互矛盾的事实。

## 测试与契约覆盖
1. `xiaotao/references/schemas/global-preference.schema.json`：定义了全局用户偏好格式，支持 `category`, `statement`, `confidence`, `source_refs` 等校验；
2. `xiaotao/references/schemas/timeline-event.schema.json`：定义了项目记忆事件格式，支持确定性 `event_id`, `event_type`, `occurred_at`, `source_refs` 等校验；
3. `test/three-tier-memory.test.js`：覆盖 Schema 结构、Fixtures、规范分层与存储绑定的自动化测试。
