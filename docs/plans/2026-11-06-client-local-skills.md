# Client Local Skills Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在客户端设置中管理本地 Skill 目录与启用状态，保持小涛 Core 默认加载，第三方技能按描述匹配后读取全文。

**Architecture:** Agent 数据目录的 skills.json 保存环境独立配置。Go 服务有界扫描显式目录，解析 SKILL.md 的 YAML 元数据，返回摘要与诊断；每次发送前刷新当前启用的无冲突技能摘要，不自动执行技能脚本。React 在已有 Agent 设置中新增 Skills 页签，支持草稿扫描、保存、刷新、错误重试。

**Tech Stack:** Go、官方 YAML 组织维护的 go.yaml.in/yaml/v3（稳定 v3 API，见 https://pkg.go.dev/go.yaml.in/yaml/v3）、React/TypeScript、现有 ModalFrame、Go tests、Playwright。

## 已确认范围与选择

用户确认先做本地目录、启用/禁用、名称/描述发现与按需全文读取。暂不做在线市场、下载/更新或额外脚本运行器。采用“模型每次拿到启用摘要，再通过已有 read_file 工具读取原文”，不全文注入全部技能，也不复制第三方内容到内置 Core。

目录支持自身 SKILL.md，或一级子目录中的 SKILL.md；不递归全盘，不自动扫描其他工具的全局配置。小涛内置 Core 不可被第三方同名技能覆盖。技能内容不授予权限，依赖问题由模型执行前核对，沿用当前访问模式和命令审批，不能声称 OS 级沙箱。

### Task 1: 配置、扫描、提示词

- 新增 `xt-client/internal/agent/skills.go` 和 `skills_test.go`：持久化、配置错误、UTF-8/文件大小/扫描上限、YAML 折叠文本、禁用、重复路径、同名冲突、路径联接边界、环境隔离。
- 修改 `service.go`：发送前生成启用摘要和诊断，保持完整小涛 Core/持续守卫；原文只按需读取，脚本不自动启动；新发送使用新配置，运行中请求保持既有目录摘要。
- 通过 `instructions_test.go`/集成用例核对真实模型请求只带摘要，无第三方全文，停用或文件丢失后下次发送正确变化。

### Task 2: 同源 API

- 新增 `xt-client/internal/app/skills_handlers.go` 和测试，注册 GET/PUT `/api/agent/skills` 与 POST `/api/agent/skills/scan`。
- 管理接口都校验同源；限制请求大小、拒绝未知字段和尾随 JSON；草稿扫描不保存、不执行脚本。

### Task 3: 设置 UI

- 新增 `SkillsSettings.tsx`、`skills-settings.css`，修改 `AgentModals.tsx` 和 `lib/api.ts`。
- 沿用海洋主题、ModalFrame、键盘焦点、加载/成功/失败反馈，支持窄屏、浅海/深海和 reduced-motion。
- 可输入绝对路径，桌面支持目录选择；草稿有改动时提示未保存，切换页签保留；禁用可逐目录或逐技能；显示冲突、无效文件和无法访问目录，不将错误显示成空列表。
- `frontend/tests/skills-settings.browser.test.mjs` 覆盖扫描不保存、保存失败保留草稿、单技能开关、页签切换、重新打开、窄屏与主题。

### Task 4: 文档、构建与验证

- 同步 `xt-client/README.md`、`docs/architecture/xt-client-agent.md` 与验收说明；检查局部 AGENTS/规则源，只有模块职责变化才调整正式规则。
- 刷新内置 Core（包括刚新增的 evolution），不手改压缩资源。
- 前端 `npm ci`、`npm run build` 和相关浏览器测试；客户端 `go test ./...`、`go vet ./...`，`./build.ps1 -Verify` 构建验证，不递增版本、不覆盖正式运行产物。
- 收尾说明真实模型/桌面验收边界，检查规则候选，不将一次性产品需求升为通用规则。

## 完成与验证（2026-11-06）

- 已实现本地配置、扫描与诊断、目录及单技能开关、同源 API 和 Agent 设置页；每次发送提供当前摘要，通过已有 read_file 按需读取原文，小涛 Core 保持默认加载。
- 前端 npm ci、npm run build 通过；Skills、MCP 设置与主题共 3 个浏览器测试通过，包含草稿、错误重试、保存、窄屏和主题。Go 全量 go test ./...、go vet ./... 通过。
- Windows 目录联接越界回归通过；文件符号链接用例因本机创建权限不足跳过。模型请求集成用例验证只预载元数据、读取结果保留工具调用 ID，停用或删除技能影响下一次发送。
- ./build.ps1 -Verify 通过，输出 xt-client/dist/verify/xiaotao.exe；未递增版本或覆盖正式客户端。内置 Core 已按源码重新打包，包含已有 evolution 资源。
- README、Agent 架构、总览和手动验收说明已同步。真实模型选用技能的稳定性、桌面原生目录选择仍需人工验收；浏览器自动化不替代这两项。
- 规则检查通过。模块职责与开发命令未变化，无需调整 AGENTS/规则源；本次产品功能没有产生有独立证据支持的新通用规则候选。
