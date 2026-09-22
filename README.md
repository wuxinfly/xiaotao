# XiaoTao

小涛（XiaoTao）是一个面向软件协作的 Agent Skill。你只需要告诉小涛目标；它按需要组织分析、实现、验证和交接。

## 最快开始

在本仓库根目录运行：

```bash
npm run xiaotao:init
```

它会检测本机环境并让你选择要安装的宿主：

```text
1. DSH
2. Codex / 通用 Agent Skills
3. Claude Code
```

选完会自动安装和自检。实际 Skill 装在用户级目录；当前项目只会生成：

```text
.xiaotao/installation.json
```

这个文件记录本项目启用了哪些宿主，`.xiaotao/` 默认不提交 Git。

## 常用命令

```bash
# 第一次安装：选择宿主并安装
npm run xiaotao:init

# 更新当前项目已选择的宿主
npm run xiaotao:update

# 检查安装是否正常，不修改任何文件
npm run xiaotao:doctor
```

`update` 会更新电脑上共享的 XiaoTao 安装。如果当前仓库版本比已安装版本新，会先提示你确认。`doctor` 会检查 Skill 文件、管理标记，以及 DSH 插件是否已激活。

## 安装位置

| 选择 | 安装位置 |
| --- | --- |
| Codex / 通用 Agent Skills | `~/.agents/skills/xiaotao/` |
| Claude Code | `~/.claude/skills/xiaotao/` |
| DSH | `~/.dsh/profiles/web/` 的插件配置 |

多个项目可以各自运行一次 `init`。每个项目都有自己的 `.xiaotao/installation.json`，但所有项目共用同一份用户级 XiaoTao；不会反复复制多份 Skill。

如果用户级目标目录已有不是 XiaoTao 管理的内容，初始化会停止保护它。确认需要接管时可运行：

```bash
npm run xiaotao:init -- --force
```

## 在宿主中使用

安装后，在项目里直接说：

```text
小涛，帮我评审这个方案，只说主要风险和建议。
小涛，先调查这个问题，不要改代码。
小涛，按刚才确定的方案开始修改。
```

小涛不会自动扩权或静默开启固定流程；需要持久化任务、记忆或 Playbook 时，会按 Skill 的规则处理。

## 开发与验证

需要 Node.js 20.19 或更高版本。

```bash
npm test
npm run test:contracts
```

DSH Adapter、Codex Desktop 插件和 Skill 内部协议的详细说明在 [`adapters/`](adapters/) 与 [`docs/`](docs/) 下。CLI 只负责安装、更新和诊断，不会调度 Worker 或直接运行工作流。
