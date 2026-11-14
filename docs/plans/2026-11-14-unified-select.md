# 统一下拉选择器（Select）实施与验收

状态：已实现并通过前端/Go 自动验证；UAT 桌面壳内的人工交互验收待执行。

关联变更：`openspec/changes/add-unified-select/`（proposal、select-field spec、design、tasks）。

## 背景

替换前 13 处业务下拉全部是原生 `<select>`，散落在 10 个文件、至少 8 套自写样式；展开列表由操作系统绘制，与深海/浅海主题割裂，且缺少悬停、展开态和开合过渡，不满足 `xt-client/AGENTS.md` 的交互动效规则。

## 组件用法

```tsx
<label><span>模型</span>
  <Select searchable value={model} onChange={setModel} disabled={busy}
    options={usableModels.map(item => ({value: item.model, label: item.label}))}/>
</label>
```

- 受控 `value/onChange`，选项为 `{value, label, hint?, disabled?}`；`value` 不在选项里时显示 `placeholder`。
- `size="sm"` 用于工具栏/筛选，默认 `md` 用于表单；宽度由页面布局 CSS 控制，组件不写死。
- `searchable` 启用搜索框；无匹配显示“没有匹配项”，空列表显示 `emptyText`（默认“暂无可选项”）。
- 键盘：Enter/Space/↑↓ 展开，↑↓/Home/End 移动，Enter/Space 选中，Esc 关闭并回焦触发器。

## 约定

- 业务页面不得新增原生 `<select>`；外观样式写在 `components/select/select.css`，页面只保留宽度等布局规则。
- 模型、账号等可能较长的列表启用 `searchable`；来源筛选、上游格式、插件枚举等小型列表不启用。
- 弹窗内的 listbox 面板依赖 `data-escape-layer`：ModalFrame 在 window 捕获阶段处理 Esc，缺少标记会先关掉弹窗。
- 标题栏手写模型菜单与能力筛选按钮组不在统一范围内。

## 实现清单

- `frontend/src/components/select/Select.tsx`、`SelectPanel.tsx`、`select.css`。
- `components/popover/Popover.tsx`：新增 `listbox` 角色、优先聚焦选中项、listbox 面板 `data-escape-layer`。
- 调用方：办公室 6 处（OfficePage、OfficeMenu、CreateTaskModal、MemberChat、RoleEditor×2）、Agent 模型库 2 处、接口/账号 5 处。
- 回归：`frontend/tests/select.fixture.html`、`select.fixture.tsx`、`select.browser.test.mjs`，`package.json` 增加 `test:select`。

## 自动验证

- `npm run lint`、`npm run check:size`、`npm run build` 通过；无新增超限文件，`agent.css` 1093→1082、`accounts.css` 1444→1435。
- `npm run test:select`、`test:history`、`test:timeline` 全绿。
- `node tools/check-go-size.mjs`、`go test ./...`、`go vet ./...` 通过（Go 无逻辑改动）。
- `./build.ps1 -Env uat -Verify` 通过，产物 `dist/verify/xiaotao-uat.exe`，版本号未递增，未更新快捷方式。

## 人工验收清单

- 办公室：项目切换后任务列表刷新；安排任务的模型选择（含来源后缀、busy 禁用）；成员聊天模型；角色编辑模型与造型；办公室菜单项目选择。
- 接口/账号：模型调用清单来源筛选；来源模型目录账号切换（搜索）；插件设置枚举；新建/编辑接口上游格式（保存中 fieldset 禁用）。
- Agent：模型库来源筛选（“全部来源 · N”聚合项）与思考等级（“跟随上游”）。
- 交互：纯键盘完成选择；弹窗内打开下拉后按 Esc 只关下拉、焦点回触发器，再按 Esc 才关弹窗；点击外部关闭；长列表搜索、无匹配、清空；窄窗口与视口底部自动向上翻转；浅海主题可读；快速反复开合；系统减少动态效果时无位移缩放。
- 入口：浏览器预览与 WebView2 桌面壳各过一遍，确认真实数据下行为与替换前一致。

## 已知边界

- 不做多选、异步选项、虚拟滚动和首字母 typeahead；列表规模超过千级时另立变更。
- listbox 采用焦点移动模式，语义属性齐全；后续如有无障碍反馈再评估 activedescendant。
