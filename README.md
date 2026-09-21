# XiaoTao

XiaoTao 是面向结果的软件协作 Agent Skill。一个小型 npm CLI 会把同一份 Skill Core 安装到各个
受支持 AI 编码宿主所需的项目本地目录。

用户只和 **小涛（Xiao Tao）** 对话，他是 XiaoTao 唯一预置角色。小涛理解目标，把技术工作
委派给按能力选择的项目 Worker 或动态生成的 Worker，判断其证据，再用大白话报告结果。XiaoTao
通过项目内的 Temporary、Task 和 Long-term 三层 Memory 延续工作，不强迫所有请求经过固定流程。

## 从源码安装（无需发布 npm）

需要 Node.js 20.19 或更高版本。在本地 XiaoTao 仓库根目录直接运行 CLI；以下安装命令无需全局
安装、构建或 `npm install`：

```bash
node ./bin/xiaotao.js init "/path/to/your-project" --tools codex
node ./bin/xiaotao.js doctor "/path/to/your-project"
```

Windows 示例：在 `D:\code\xiaotao` 中运行，并把目标路径换成要使用 XiaoTao 的项目：

```powershell
node .\bin\xiaotao.js init "D:\code\your-project" --tools codex
node .\bin\xiaotao.js doctor "D:\code\your-project"
```

这会把当前仓库的 `xiaotao/` Core 复制到目标项目的 `.agents/skills/xiaotao/`。若要安装到 XiaoTao
仓库本身，目标使用 `.`：

```bash
node ./bin/xiaotao.js init . --tools codex
```

更新 XiaoTao 源码仓库后，从同一仓库刷新已安装副本：

```powershell
node .\bin\xiaotao.js update "D:\code\your-project"
node .\bin\xiaotao.js doctor "D:\code\your-project"
```

更新使用本地源文件，不从 npm 下载 release。其他受支持宿主使用相同命令，`--tools` 值见下表。

在 Codex 中使用本地项目时，打开目标项目并新建对话，然后要求它使用 XiaoTao。目标必须是该
Codex Session 可见的项目目录；安装到本地 Windows 文件夹不会同时安装到远程环境。这一步安装
的是可移植 Skill，不是 Codex 插件，也不包含自动 checkpoint Hook。

## 使用多宿主 CLI 安装

包发布到 npm 后，也可以全局安装 CLI。需要 Node.js 20.19 或更高版本：

```bash
npm install -g xiaotao-ai-workflow
```

### 从旧 Role 协议升级

`0.2.0` 是一次不兼容升级：Handoff 必须包含 `worker_state_path`，不再接受
`role_state_path` 或 `recommended_next[].role`；Worker 来源也不再接受 `builtin`，并以
`worker-compress` 取代 `role-compress`。XiaoTao 不会自动发现、迁移或恢复旧 Role 状态。

升级前请归档仍需保留的 `.xiaotao/` 旧任务；升级后用当前 Worker 协议重新创建需要继续的任务。
不要直接复用旧 Handoff 或 Worker 状态文件。后续 `0.x` 阶段的不兼容协议变更将通过次版本号发布。

然后在项目中初始化 XiaoTao：

```bash
cd your-project
xiaotao init
```

交互式初始化会检测可能的宿主，并让你选择一个或多个。脚本或 CI 中应明确传入：

```text
xiaotao init --tools codex,claude,opencode
```

MVP 支持：

| Tool ID | 宿主 | 生成的 Skill 目录 |
| --- | --- | --- |
| `codex` | Codex 及共享 Agent Skills 的宿主 | `.agents/skills/xiaotao/` |
| `claude` | Claude Code | `.claude/skills/xiaotao/` |
| `opencode` | OpenCode | `.opencode/skills/xiaotao/` |

`xiaotao init --tools all` 安装全部三种，`xiaotao init --tools none` 只初始化 XiaoTao 项目元数据。
现有非空 Skill 目录绝不会被静默接管；只有检查目标后才能使用 `--force`。

刷新由 CLI 管理的 Skill 文件并检查安装：

```text
xiaotao update
xiaotao doctor
xiaotao doctor --json
```

CLI 在 `.xiaotao/installation.json` 保存本地安装选择，并在每个生成 Skill 中写入
`.xiaotao-managed.json` 所有权标记。更新会覆盖规范 XiaoTao 文件，同时保留无关的用户文件。

## 可选 Codex 桌面恢复插件

在 ChatGPT 桌面端 Codex 中处理本地项目时，可选 Codex 插件会在 Session 启动、恢复、清空或
压缩后重新提供最小 XiaoTao 规则与 Memory 入口。它在用户或项目作用域发现 Core Skill，不捆绑
第二份 Core。项目本地 `.xiaotao/` Memory 与 Task 状态仍可在不同宿主间共享；项目本地
`.agents/skills/xiaotao/SKILL.md` 只是 Hook 的可选路径提示。

新项目先安装 Core、创建 XiaoTao 元数据，再准备 personal plugin 源码：

```bash
node ./bin/xiaotao.js init "/path/to/your-project" --tools codex
npm run codex:install:local
```

源码安装器使用 Codex 标准 personal plugin 目录 `~/.codex/plugins/xiaotao-codex/`，并让 personal
marketplace 条目指向 `./.codex/plugins/xiaotao-codex`。

随后在桌面端 personal marketplace 安装/启用插件，检查并信任其 Hook，再在目标项目新建对话。
仅准备源码不会激活插件。当前版本提供只读恢复指导和单个持久 Worker 的有界 Packet 注入，不会在
压缩前自动保存，也不宣称工具/权限已被强制隔离。Windows 命令、更新和验收见
[Codex Adapter 指南](adapters/codex/README.md)。

## 从本地仓库安装到 DSH

无需向 npm 发布任何包即可安装 DSH Adapter。在本仓库运行：

```bash
npm run dsh:install:local -- --profile web
```

安装器会使用当前 `xiaotao/` Core 构建自包含 Adapter 压缩包，将 `.tgz` 安装到指定 DSH profile，
让 DSH 激活其 `inject: [skills]` bundle，并校验已安装模块与 `dsh --dump-config`。这里使用 tarball，
不使用本地目录链接，从而避免 Windows 上 `dsh plugin add <absolute-dir>` 的 junction 路径问题。

其他 profile 使用 `--profile <name>`。Core 或 Adapter 变更后重新运行同一命令，即可安装新的本地
压缩包。如果 PowerShell 阻止 `npm.ps1` shim，改用 `npm.cmd` 执行相同命令。

Checkpoint 工具默认按当前 DSH 会话的项目目录自动绑定，一个 profile 可以服务多个项目，
不需要逐项目填写路径。恢复文件默认保存在 DSH home 下的 `xiaotao-recovery/` 并按项目隔离。
显式保存需宿主提供 fs/tools 能力；也可选择启用实验性的上下文压力自动 checkpoint。它只在
DSH 能报告 canonical pressure projection 或 provider prompt usage 时提前请求当前 Agent 复用同一工具，不备份聊天或代码，
也不声称拥有 pre-compaction Hook。旧的固定 projectRoot 配置继续有效；删除该字段可切换到
自动项目绑定。详见 [DSH Adapter 指南](adapters/deepseek-harness/README.md#checkpoint自动配置项目)。

## 手动安装

完整可移植 Skill Core 是 [`xiaotao/`](xiaotao/) 目录。支持 Agent Skills 的宿主可以直接复制其
内容到宿主识别的 Skill 目录，并确保 `SKILL.md` 位于 Skill 根目录。不支持 Agent Skills 的宿主
需要单独的薄 Adapter。也可以把 `xiaotao/` 内容打成 ZIP，作为一个 Skill 上传或安装。

CLI 只负责安装、更新和诊断。它绝不调度 Worker、解释 Memory、授予权限或运行工作流状态机。
安装后，XiaoTao 通过选定 AI 宿主的原生文件系统和 Agent 能力运行，不需要后台 Runtime。

## 使用

在项目中自然地说：

```text
使用 XiaoTao 帮我分析这个项目的启动性能问题。
```

或者直接叫小涛：

```text
小涛，我想先讨论一下新架构，暂时不要正式开工。
```

用户不需要选择专家：

```text
小涛，评审一下这个设计，只告诉我主要风险和建议。
小涛，调查当前调用链，先不要改代码。
小涛，按刚才确定的方案开始修改。
```

能力路由可以复用或组合 `.xiaotao/workers/registry.yaml` 中的 Workers；没有安全的可复用匹配时，
创建有界 Worker。生成 Worker 使用针对任务的中文显示名，遵循现有 Task、Temporary 或一次性
Session 生命周期，不能给自己授权，也绝不会自动变为可复用 Worker。

Worker 不依赖父 Agent 的隐式继承。每个 Worker 声明 required 与 optional 指令引用；每次运行
实体化最小 Delegation Packet，包含已解析指令摘要、上下文引用、有效工具与权限，以及宿主支持
状态。缺少 required 指令或不能强制执行边界时停止委派。Codex Adapter 会只读解析不可变 Worker
快照，交叉校验工具、权限、上下文、输出和指令边界，并从可信 Core 或项目根目录重新计算指令摘要；
宿主 Hook 仍不能强制工具白名单，因此会如实保留 `tool-isolation` 降级声明。

只有请求需要持久化时，XiaoTao 才在项目的 `.xiaotao/` 下创建状态。把探索讨论变成正式 Task
之前，会请求确认。

在持久边界，Memory Worker 将有来源的 Temporary 或 Task 发现与当前 Long-term 条目比较，提出
需评审的 `UPDATE`、`MERGE`、`CREATE` 或 `SKIP`。它绝不把执行日志直接复制到 Long-term Memory，
也不能批准自己的提案。Git 并行分支间由 Memory Merger Worker 执行三方语义合并，并保留未解决
冲突双方的完整来源。

用户明确要求创建一条来源清楚的 Memory 时，轻量路径先通过 Catalog 查重，最多读取 3 条相关
Long-term 详情，并把 `current_memory` 标记为 `bounded`。无冲突 CREATE 不重复确认；重复项直接
SKIP，涉及冲突、UPDATE 或 MERGE 时仍需用户确认。聊天是唯一来源时，用户原话以内容摘要保护的
不可变记录保存在项目 `.xiaotao/memory/sources/`，不得把秘密或未授权敏感内容写入该目录。

Memory Awareness 会为活动 Temporary、Task 和 Long-term Memory 生成小型 Manifest 与机器可读
Index。Agent 先加载 Manifest，最多检索五个相关候选，再按稳定 ID 提取一个选定记录，不注入完整
Long-term 集合。新 Long-term Memory 按 `entries/<entry-id>.md` 独立存储，旧聚合 `current.md` 仍可
读取且不会静默迁移。`xiaotao/scripts/memory_catalog.py` 构建、检查、搜索、选择性读取，并提供显式
迁移预检；它不替代正式 Memory，也不增加后台 Runtime。

Activity Timeline 从带有可靠 `completed_at` 的 Task、带有 `promoted_at` 的晋升来源 Task，
以及带有 `decided_at` 的里程碑级不可变 Decision Record 与 Playbook 评审记录
（`playbooks/decisions/`）派生，回答“某段时间完成了什么、晋升了什么、做过哪些关键决定、
批准了哪些 Playbook”。它只生成本地可重建的
`.xiaotao/activity/index.json`，不维护第二套事件日志。小涛通过
`xiaotao/scripts/activity_catalog.py search` 查询有限时间窗口，不把完整历史装入上下文。

## 手动行为检查

自动测试套件有意不包含基于模型的行为 eval。XiaoTao 运行在不同宿主中，而 Codex 专用 live
runner 耗时、成本高，也无法验证真实 DSH 或裸 Skill 体验。

Core 与各宿主 Adapter 的职责、当前激活状态和降级边界见
[`docs/architecture/core-adapter-boundary.md`](docs/architecture/core-adapter-boundary.md)。

有重要指令变更后，在真实目标宿主抽查受影响行为。小型发布清单见
[`docs/manual-acceptance.md`](docs/manual-acceptance.md)。确定性单元测试与契约测试仍自动运行。

## npm 包内容

```text
bin/
  xiaotao.js
cli/
  hosts.js
  index.js
  install.js
xiaotao/
  SKILL.md
  scripts/
    activity_catalog.py
    memory_catalog.py
    validate.py
  references/
    coordination.md
    activity.md
    storage.md
    memory.md
    handoffs.md
    playbooks.md
    workers.md
    workers/
    practices/
    schemas/
```

所有协作行为都由 Skill 及其 references 表达。Node.js 文件只执行确定性的安装和校验；XiaoTao
本身由选定宿主的原生文件系统与 sub-agent 能力运行。
