# 项目开发入口

## 项目定位
XiaoTao（小涛）是面向软件协作的 AI 项目。目前包含 Agent Skill、安装管理 CLI 和多宿主适配，并非传统后端服务；客户端尚未实现。

## 目录
- `xt-skills/`：现有 Skill npm 包，开发前阅读该目录 AGENTS.md 和 README.md。
- `xt-client/`：客户端预留目录，技术栈和功能待确定。
- `website/`：独立 Astro 网站，使用自己的 package.json。
- `docs/`：共享架构、计划及验收文档。历史计划中的旧根目录代码路径需对应到 xt-skills/。
- `.github/`：仓库级 CI 与部署。
- `.xiaotao/`：当前仓库的运行记录及项目记忆，不是 Skill 源码。

## 开发约定
- 根目录没有 npm 包，在各模块目录执行命令。
- 不提交 node_modules、构建产物、日志和本机安装记录。
- 修改路径时同步 CI、脚本、文档链接和打包清单。
- 调整目录不应改变 Skill 安装位置、协议和产品行为。
- 模块职责、命令发生变化时更新 README 和相应 AGENTS.md。

## 验证
- Skill：在 xt-skills 下运行 `npm test`、`npm run test:contracts`、`npm pack --dry-run`。
- DSH：在 xt-skills/adapters/deepseek-harness 下运行 `npm run typecheck`、`npm test`、`npm run test:package`。
- 网站：在 website 下运行 `npm run build`。
- 客户端：在 xt-client 下运行 go test ./...、go vet ./...、./build.ps1，详见局部 AGENTS.md。
