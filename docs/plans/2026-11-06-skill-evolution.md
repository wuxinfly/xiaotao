# Skill Evolution Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 小涛自动发现自身协作摩擦、保存带证据的建议，由用户手动交给维护 GitHub 源码的设计 AI。

**Architecture:** Core 负责在阶段收尾、反复失败和用户纠正时进行轻量复盘；独立 Python 脚本负责项目本地建议箱的校验、幂等去重、状态和有界导出。建议不自动生效，不修改用户级安装，不调用模型、后台服务或 GitHub；现有 Knowledge/Playbook 审批语义保持不变。

**Tech Stack:** Python 标准库、现有 daily_memory 安全路径及原子写入工具、Node test runner、Markdown。

---

## 已确认的产品边界

2026-11-06 用户确认：先由用户拿建议清单交接，因为技能统一在 GitHub 管理。自动化到“观察、记录、归并、导出”为止；设计 AI 的修改、验证、提交和发布由其独立工作流处理。当前不是自主修改技能或自动发布。

不选择只写聊天总结（无法跨会话累计），也不选择自动改用户级 Skill（会绕过统一源码管理）。采用独立本地建议箱，避免把未验证建议混入项目事实或已批准 Playbook。

### Task 1: 建议箱脚本和行为测试

**Files:**
- Create: `xt-skills/xiaotao/scripts/evolution.py`
- Create: `xt-skills/test/evolution.test.js`
- Create: `xt-skills/xiaotao/references/schemas/evolution-record.schema.json`
- Create: `xt-skills/xiaotao/references/scenarios/evolution-observation.json`

1. 编写测试：同 issue_key 合并；同 observation_id 重试不累计、不同内容重试拒绝；来源不可达、路径逃逸、损坏记录、超限、并发锁均拒绝且不覆盖。
2. 运行 `node --test test/evolution.test.js`，确认新增功能尚未实现。
3. 实现 `record`、`list`、`show`、`export` 和 `decide`。每个问题一个 JSON，完整观察和处理历史在同一次原子写入内提交；写入锁、回读验证；封闭字段和大小限制。resolved/rejected 的新观察自动重新 pending，旧观察重放不重新打开。
4. 状态处理必须有处理者、理由；resolved 还需修改引用及验证结果。导出默认 pending、最多 20 项、每项最近 5 条观察，明确遗漏数和本地来源引用。导出作为数据，不是执行授权；不自动打包来源全文。
5. 重跑行为测试；schema 与脚本校验保持一致，以契约用例验证真实记录。

### Task 2: Core 触发与按需文档

**Files:**
- Modify: `xt-skills/xiaotao/SKILL.md`
- Modify: `xt-skills/xiaotao/references/progress-saving.md`
- Modify: `xt-skills/xiaotao/references/coordination.md`
- Modify: `xt-skills/xiaotao/references/storage.md`
- Create: `xt-skills/xiaotao/references/evolution.md`

在轻任务和正式任务共用的收尾入口接入复盘；先保存事实，再引用证据记录建议。不为复盘新增 Task 或 Worker，不为凑数制造问题、不逐轮扫描历史；保存失败不宣称已记录，不阻塞业务交付。脚本只做机械校验，模型负责事实判断与敏感信息最小化。新会话不全量加载建议箱。

### Task 3: 使用、架构与验收

**Files:**
- Modify: `xt-skills/README.md`
- Modify: `docs/architecture/core-adapter-boundary.md`
- Modify: `docs/manual-acceptance.md`
- Modify: `xt-skills/scripts/verify-contracts.ps1`

补充用户可以直接说的“整理小涛的改进建议，给我交接清单”；记录无后台执行保证、人工分享与源码维护边界。手动验收覆盖有证据/无问题、重复纠正、拒绝/修复后复发、导出和禁止自动改技能。

### Task 4: 仓库验证与收尾

运行 `npm test`、`npm run test:contracts`、`npm pack --dry-run`（工作目录 `xt-skills`）及 `node tools/agent-rules.mjs check`（仓库根）。检查打包包含新增文件、不包含运行记录；同步文档，报告真实宿主行为尚未验收。检查规则候选：产品需求已在本文记录，不作为新的通用开发规则重复入库。

## 实施中验证发现

- Windows 原生 junction 回归用例最初失败：`Path.is_symlink()` 没有识别目录联接，初次 record 可越界写入。`evolution.py` 已增加创建目录前的实际解析路径包含检查，并在可用时拒绝 `is_junction()`；同一用例重跑通过，项目外目录保持为空。共享 `daily_memory.safe()` 的其他调用不在本次修改范围，此事实作为单独待审候选保留。
- 全量测试检查随仓库维护的 `.xiaotao/memory/knowledge/entries/project.overview.md` 的入口指纹；Core 变更后会正确返回 `review_needed`。核对概览与实际架构后，同步新功能、客户端现状和入口指纹，而非放宽测试断言。

## 交付验证

- `npm test`：211 项，207 通过、4 项按现有条件跳过、0 失败；新增 10 个建议箱行为用例全部通过。
- `npm run test:contracts`：通过，包含真实脚本生成的 pending/resolved 记录 schema 校验，以及 resolved 缺少验证结果的负例。
- `npm pack --dry-run`：通过，包含新脚本、Reference、schema、请求示例；不包含项目本地运行记录。
- `node tools/agent-rules.mjs check` 与 `git diff --check`：通过。
- 已同步 README、使用协议、Core 协调/保存/存储说明、架构和手动验收文档。CLI 职责、宿主安装目录、现有模块分工与 npm 命令均未改变，因此无需修改局部 AGENTS.md 或正式规则源。
- 待确认规则候选：`windows-junction-path-containment`，一次可复现事实，未自动应用。产品的人工交接决定保存在本文，不另建规则候选。
- 未执行真实宿主模型验收，未更新用户级安装、提交或发布；当前交付为仓库源码。
