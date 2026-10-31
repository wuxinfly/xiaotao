# XiaoTao

小涛是面向软件协作的 AI 项目，目前提供 Agent Skill、安装管理 CLI 和多宿主适配，客户端待开发。

## 目录

- [xt-skills/](xt-skills/)：Skill、CLI、宿主适配器与相关测试，使用说明见该目录 README。
- [xt-client/](xt-client/)：客户端预留目录。
- [website/](website/)：独立 Astro 网站。
- [docs/](docs/)：共享架构、计划和验收文档。历史计划中的旧根目录代码路径现位于 xt-skills/ 下。
- [AGENTS.md](AGENTS.md)：AI 开发入口。

## Skill 开发与验证

需要 Node.js >= 20.19、Python 和 PowerShell。

```bash
cd xt-skills
npm test
npm run test:contracts
npm pack --dry-run
```

安装、更新、诊断命令也在 xt-skills 下执行，详见 [Skill 使用说明](xt-skills/README.md)。根目录不再是 npm 包。

## 网站开发

```bash
cd website
npm install
npm run dev
```
