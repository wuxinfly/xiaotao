# Issue #92（第二阶段）验收与场景验证：项目记忆时间线派生与逐层下钻

## 背景与目标

本方案针对 GitHub Issue #92 第二阶段，实现了项目记忆时间线（`.xiaotao/memory/timeline/`）由权威记录驱动的自动化派生、增量聚合与逐层有界检索机制。

## 核心机制与场景验证

### 场景 1：分层结构与自动化派生
- **行为**：从 `.xiaotao/tasks/archive/` 中的已归档 Task 以及 `memory/long-term/decisions/`、`playbooks/decisions/` 等不可变决策中提取事件，运行 `python xiaotao/scripts/timeline_catalog.py build`；
- **产物验证**：
  - `.xiaotao/memory/timeline/summary.md`：项目经历总览（跨年份主线与年度导航）；
  - `years/<year>/summary.md`：年度总览；
  - `years/<year>/<month>/summary.md`：月度摘要；
  - `years/<year>/<month>/<day>.yaml`：当日事件列表（符合 `timeline-event.schema.json`）。
- **底层不丢失**：上层摘要在事件增加时动态刷新，底层 `<day>.yaml` 及其指向 Task/Decision 的原始 `source_refs` 永久保留，绝不被摘要吞噬。

### 场景 2：逐层有界下钻查询（Progressive Drilling Down）
- **行为**：
  1. `timeline_catalog.py summary`：仅读取项目经历总览，展示项目大脉络；
  2. `timeline_catalog.py summary --year 2026`：下钻读取 2026 年度摘要；
  3. `timeline_catalog.py summary --year 2026 --month 09`：下钻读取 2026年9月重点交付；
  4. `timeline_catalog.py events --date 2026-09-23`：精确获取当日事件及其原始证据路径。
- **守卫验证**：会话启动时不加载整条时间线；查询时仅读取单层单个文件，零全量内存加载。

### 场景 3：Activity 与 Timeline 的协同一致性
- **Activity（机器查询视图）**：扁平索引（`.xiaotao/activity/index.json`），支持时间范围与事件过滤；
- **Timeline（人类与模型叙事视图）**：按年月日分层组织，专为向用户汇报“项目经历了什么”而设计；
- **数据源一致**：两者复用权威记录解析逻辑，事件 ID 确定性对齐，不构造互斥的事实来源。

## 测试覆盖
- `test/timeline-catalog.test.js`：覆盖时间线构建、多层级文件派生、有界下钻查询、空项目优雅降级以及参数边界约束。
