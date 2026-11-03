# 浅海亮色主题

日期：2026-11-03。

用户选择“浅海：雾白、浅蓝灰、蓝绿点缀”。保留现有深海主题，以右上角太阳 / 月亮按钮切换，按钮文字代表切换目标。主题不改布局、字体与业务行为。

## 实现

- `frontend/src/shallow-sea.css` 集中适配页面、状态、代码块、菜单、Portal 弹窗、Toast 和独立浮窗。亮色正文使用深青色，热力图以颜色加深代表用量增加。
- `frontend/src/lib/theme.ts` 管理 `html[data-theme]`、原生表单的 `color-scheme` 和 `xiaotao.theme` 本地偏好，默认深海。读写存储失败时，主题模块仍可在当前页面应用选择。
- 两个 HTML 入口在样式加载前恢复偏好，减少亮色启动时的暗色闪烁。HUD 通过同源 storage 事件同步；正式版与 UAT 按 origin 隔离。原生启动动画保持现状。
- 切换按钮支持键盘和明确的可访问名称，窄窗口只显示图标；沿用项目减少动态效果约定。

## 验收

自动验证使用 Playwright / Edge 无头模式，API 全部拦截并提供合成数据，不调用真实账号、不修改运行实例。

- 键盘切换，刷新后恢复。
- Agent 设置及账号来源弹窗的 Portal 样式。
- 仪表盘正文、卡片与热力图。
- 独立 HUD 初始主题与跨窗口同步。
- 540px / 850px 窗口入口与导航不重叠。
- 存储不可用时主题模块仍可应用亮色。

浏览器截图用于检查浅海 Agent、仪表盘与账号弹窗，测试将截图写入系统临时目录。已通过 `npm ci`、`npm run build`、`node --test tests/history.browser.test.mjs tests/theme.browser.test.mjs`、`go test ./...`、`go vet ./...` 和 `node tools/agent-rules.mjs check`。构建保留现有 Tiptap 指令与大包体提示。桌面 WebView2、跨显示器缩放、真实账号数据状态与原生启动衔接仍待人工复核。

## 规则候选

此次是单次主题需求，暂无可复用规则变更，不新增候选；未修改 AGENTS.md 或规则源。
