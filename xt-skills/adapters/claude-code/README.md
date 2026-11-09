# Claude Code 宿主适配

Claude Code 有两种用法：

- **裸 Skill（默认）**：把 Core 安装到 `~/.claude/skills/xiaotao/`，Claude Code 自动按需加载。
- **增强插件（可选）**：在裸 Skill 之上增加会话入口提醒、阶段保存回执检查和只读记忆整理子代理。

插件只提供 Hook 和子代理定义，不携带第二份 Core，避免和用户级 Skill 重复注册。

## 安装与更新

在项目目录运行：

```bash
npx @wxfly/xiaotao@latest claude
```

命令会：

1. 安装或刷新用户级 Skill `~/.claude/skills/xiaotao/`；
2. 在 `~/.xiaotao/hosts/claude/` 准备本地插件市场与 `xiaotao-claude` 插件；
3. 写入项目的 `.xiaotao/installation.json`，把 `claude` 记为本项目启用的宿主。

然后在 Claude Code 中运行：

```text
/plugin marketplace add ~/.xiaotao/hosts/claude
/plugin install xiaotao-claude@xiaotao-local
```

安装并启用 `xiaotao-claude`，再新开会话。也可以退出会话后使用 `claude plugin marketplace add ~/.xiaotao/hosts/claude` 和 `claude plugin install xiaotao-claude@xiaotao-local`。这里只准备文件；CLI 和 `doctor` 都无法证明插件已经启用。

更新时分别执行 `npx @wxfly/xiaotao@latest update`（刷新 Skill）和 `npx @wxfly/xiaotao@latest claude`（刷新插件来源与市场清单），回到 Claude Code 刷新市场、更新插件，再新开会话。只跑 `update` 不会更新已安装的插件。

## 进度与记忆

Hook 只对当前 `cwd` 下、安装记录明确包含 `claude` 的项目生效；普通目录不自动初始化。插件不会读取聊天记录、调用模型或代写总结。

- `SessionStart`：注入 Core 入口、宿主边界和阶段保存要求。来源覆盖 `startup / resume / clear / compact`。
- `UserPromptSubmit`：开始新一轮并注入保存说明，包括本轮 `session_id` / `turn_id` 和 `progress-check.mjs` 路径。
- `Stop`：核对本轮保存或跳过的回执。缺少有效回执时最多要求补查一次；`stop_hook_active` 防止循环。回执是模型声明加上文件字节校验，不能判断总结语义是否完整，也不能发现模型错误地登记 skipped。
- `PreCompact`：手动压缩缺少回执时暂停一次并提示先保存，再次压缩放行；自动压缩不拦截（上下文上限时拦截会卡住工作）。Hook 不自动保存，强退、崩溃或压缩前保存仍不保证。

Claude Code 的 Hook 不带 `turn_id`，插件在 `UserPromptSubmit` 时按会话和 prompt 派生一个本轮标识，保存在 `.xiaotao/runtime/claude/progress/<session-hash>.json`，供 `Stop`、`PreCompact` 和 `--save` / `--ack` 使用。该文件是本地检查元数据，不是业务记忆，应排除 Git。

保存走可信用户级 Core：

```bash
node ~/.xiaotao/hosts/claude/xiaotao-claude/scripts/progress-check.mjs --save
```

stdin 为 `{cwd, session_id, progress: {topic_id, title, summary, details?, validation?, next_steps?, work_path?, request_id?}}`。无新事实或用户要求不保存时用 `--ack` 传入 `{cwd, session_id, status: "skipped", reason}`。插件只执行用户安装的 Core（默认 `~/.claude/skills/xiaotao`），可配置 `XIAOTAO_CORE_ROOT`、`XIAOTAO_PYTHON`，不执行项目内脚本。

设置 `XIAOTAO_CLAUDE_PROGRESS_CHECK=0` 可以关闭保存检查，保留会话入口提醒。

## Worker 映射

- 只读记忆整理优先使用插件提供的 `xiaotao-claude:xiaotao-memory-worker` 子代理，它的 `tools` 只含 `Read / Grep / Glob`，由宿主强制执行工具白名单。
- 其他 Worker 使用 Claude Code 原生子代理，按 Core 的 Delegation Packet 派工。插件不附带通用 Worker 定义；未声明或宿主未强制的工具、路径和权限隔离必须标为 `degraded` / `unsupported`，不能靠提示词声称已隔离。
- 宿主没有原生子代理可用时，按 Core 做 In-Session Fallback 并如实标记。

## 验证

```bash
node --test test/claude-adapter.test.js
```

测试覆盖市场准备、Skill 安装、doctor、Hook 的普通项目跳过、会话注入、Stop 单次补查、PreCompact 警告和损坏输入放行。真实 Claude Code 客户端中的市场添加、插件启用、Hook 执行和端到端保存仍需人工验收：

| 场景 | 预期 |
| --- | --- |
| 未初始化项目 | 无上下文注入，不写 `.xiaotao/` |
| 已选择 `claude` 的项目 | `SessionStart` 注入入口与保存要求 |
| 阶段结束但未保存 | `Stop` 提醒一次；再次结束放行 |
| 已登记 saved / skipped | `Stop` 不再提醒 |
| 手动压缩前未保存 | 暂停一次并提示保存；再次压缩放行 |
| 自动压缩前未保存 | 放行，不拦截 |
| 派发记忆整理 | 使用 `xiaotao-claude:xiaotao-memory-worker`，只能读 |
| 禁用插件 | 裸 Skill 仍可显式使用 |

官方协议以 Claude Code 当前版本的插件、Hook 和子代理文档为准。