# Skill 工具包开发指引

遵守根目录 AGENTS.md。本目录是 `@wxfly/xiaotao` npm 包根目录。

## 分工
- `xiaotao/SKILL.md`：Skill 主入口；references、workers、scripts 提供协议、角色与运行支持。
- `bin/xiaotao.js`：命令入口；`cli/` 负责安装、更新和诊断，不调度 Worker。
- `adapters/`：Codex、Antigravity、DSH 和 ZCode 宿主适配。DSH 是独立 npm 包。
- `test/`：Node 测试；`scripts/`：契约和语义验证。
- 共享架构与验收文档位于 `../docs/`。

## 命令与路径
在本目录运行 `npm test`、`npm run test:contracts`、`npm pack --dry-run`，或 `npm run verify`。需要 Node.js >= 20.19、Python 和 PowerShell；契约验证通过 npx 使用 AJV。

安装管理命令为 `npm run xiaotao:init`、`npm run xiaotao:update`、`npm run xiaotao:doctor`。init 是交互命令；安装与更新会修改用户级宿主目录，不作为自动测试运行。

Skill 源码目录仍叫 xiaotao，用户级安装位置保持不变，xt-skills 是仓库的分类目录。发布范围由 package.json 的 files 字段决定。协议修改需同步 schema、示例与验证脚本。
