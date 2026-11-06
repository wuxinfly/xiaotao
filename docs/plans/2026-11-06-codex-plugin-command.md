# Codex Plugin Install Command

**Goal:** 让已发布的 `@wxfly/xiaotao` npm 包可以通过命令安装 Codex Skill 与 Codex 增强插件，同时保留桌面端手动安装和 Hook 信任边界。

## 实现

- 新增 `xiaotao codex [path] [--prepare] [--json]`。
- 默认安装用户级 Codex / 通用 Agent Skill，并写入项目 `.xiaotao/installation.json`。
- 生成独立 marketplace root：`~/.xiaotao/plugins/codex-marketplace/`，其中 `.agents/plugins/marketplace.json` 使用 marketplace 名称 `xiaotao`，插件路径为 `./plugins/xiaotao-codex`。
- 调用 `codex plugin marketplace add <root> --json`，再调用 `codex plugin add xiaotao-codex@xiaotao --json`。
- `--prepare` 只准备来源，不调用 Codex CLI；桌面端可从 Plugins 手动安装。
- `--json` 返回安装状态、插件 ID、marketplace 路径和诊断。
- npm `files` 增加 `adapters/codex/`，否则发布包里的脚本入口没有对应实现文件。

## 边界

- CLI 不修改 Codex 的 Hook 信任记录，也不自动启用 Hook。首次使用仍需在 Codex 中审查信任。
- 不覆盖用户已有 `personal` marketplace；使用独立的 `xiaotao` 名称和目录。
- Codex CLI 不可用、未登录或安装失败时，保留已准备好的 marketplace，并明确提示桌面端安装。
- 真实桌面 Hook 是否执行仍属于人工验收，不由安装命令自动证明。

## 验证

- `node --test test/cli.test.js test/codex-adapter.test.js`
- `npm test`
- `npm run test:contracts`
- `npm pack --dry-run --json`，确认包含 `adapters/codex/install-local.mjs` 与插件文件。
- 使用真实 Codex CLI 在隔离 marketplace 上验证 `plugin marketplace add`、`plugin list` 和 `plugin add`。
