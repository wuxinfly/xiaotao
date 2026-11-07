# 客户端代码质量规范

本规范适用于 `xt-client/` 的 Go 服务、React/TypeScript 前端和 CSS。目标不是按行数机械拆分，
而是防止单个文件或函数承担过多职责，降低修改时的耦合、异步竞态和回归风险。

## 适用范围

检查以下代码：

- `xt-client/cmd/**/*.go`
- `xt-client/internal/**/*.go`
- `xt-client/frontend/src/**/*.{ts,tsx,css}`

不直接按本规范判断：

- `*_test.go` 和前端 `tests/**`
- 生成目录，例如 `internal/app/web/`
- `node_modules`、`dist`、构建产物、日志和本机数据
- 协议常量、映射表、迁移脚本等天然较长的数据文件

测试文件和表格代码不套用生产代码的大小门槛，但仍需按行为域拆分，避免一个测试文件同时覆盖
大量不相关功能。

## 硬指标

行数只用于触发检查，不作为唯一结论。超过“必须拆解”线时，应拆分或记录明确豁免理由。

| 项目 | 预警线 | 必须拆解线 | 说明 |
| --- | ---: | ---: | --- |
| Go 生产文件 | 600 行 | 1000 行 | 不含测试、生成代码和数据表 |
| React 页面或组件文件 | 400 行 | 600 行 | 页面、弹窗、列表和可复用组件 |
| TS 纯逻辑模块 | 400 行 | 600 行 | API、工具函数、类型和状态逻辑 |
| CSS 文件 | 500 行 | 800 行 | 按页面、组件和状态拆分 |
| Go 函数或方法 | 80 行 | 150 行 | 简单表驱动分支需人工判断 |
| React 组件函数 | 150 行 | 250 行 | JSX 长不等于坏，先看职责 |
| TS 普通函数或 Hook | 80 行 | 150 行 | 请求、校验、转换和副作用应分离 |
| 嵌套层级 | 4 层 | 6 层 | 优先提前返回、抽函数和状态机 |
| 圈复杂度 | 15 | 25 | 大量条件和异常分支需重点检查 |
| 函数参数 | 5 个 | 7 个 | 参数多时考虑配置对象或命令对象 |
| 单组件 `useState` | 10 个 | 15 个 | 超过后通常应按状态域拆分 |

检查脚本会统计文件行数和组件函数行数。Go 函数长度、圈复杂度、嵌套层级和参数数量当前由人工
复核；后续接入 Go 静态分析工具时，应沿用同一门槛。

## 存量基线

以下文件在规范建立时已经超过“必须拆解”线，属于待清理存量。修改这些文件时必须遵守：

- 不得继续扩大文件总行数。
- 新增功能优先放到新的模块或子组件。
- 触碰某一段逻辑时，优先把该职责抽取出去，而不是继续叠加条件分支。
- 不要求一次重写全部历史逻辑。

当前前端超标文件：

| 文件 | 规范建立时行数 | 主要问题 |
| --- | ---: | --- |
| `xt-client/frontend/src/features/agent/AgentPage.tsx` | 1933 | 页面、会话状态、历史、编辑器、引用、图片、复盘和浮层混在一起 |
| `xt-client/frontend/src/features/accounts/accounts.css` | 1444 | 账号页样式过度集中 |
| `xt-client/frontend/src/features/agent/agent.css` | 1093 | Agent 页面样式过度集中 |
| `xt-client/frontend/src/features/dashboard/dashboard.css` | 907 | 仪表盘样式过度集中 |
| `xt-client/frontend/src/ocean-tech.css` | 606 | 全局主题令牌和页面覆盖集中 |
| `xt-client/frontend/src/lib/api.ts` | 599 | 请求类型、接口目录和请求封装集中 |

当前 Go 超标文件：

| 文件 | 规范建立时行数 | 主要问题 |
| --- | ---: | --- |
| `xt-client/internal/agent/service.go` | 1207 | Agent 编排、运行状态和多种行为集中在一个服务文件 |
| `xt-client/internal/proxy/service.go` | 1079 | 代理服务职责较多 |
| `xt-client/internal/app/app.go` | 961 | 路由、应用装配和处理逻辑集中 |
| `xt-client/internal/usage/service.go` | 850 | 用量服务需按读取、聚合和写入职责复核 |

以下组件当前也超过“必须拆解”线，纳入同一份存量基线：

| 组件 | 规范建立时行数 | 说明 |
| --- | ---: | --- |
| `AccountsPage` | 525 | 账号加载、登录、配额、打卡和弹窗状态集中 |
| `UsageHeatmap` | 306 | 数据转换、布局和渲染集中 |
| `ReviewListModal` | 274 | 列表请求、轮询、操作和详情承载集中 |

基线数字用于跟踪趋势，不要求为了对齐数字而创建没有职责的薄文件。

## 结构规范

### 通用

- 一个文件只承担一类主要职责。文件顶部应能清楚回答“这里负责什么”。
- 同一个逻辑重复三次以上应抽取；仅使用一次的抽象不应过度包装。
- 错误必须保留上下文并向上传递，不能静默吞掉后伪装成功。
- 新增代码不得引入未使用变量、未使用导入、调试输出和无理由的类型逃逸。
- 拆分必须以职责为边界，不能按行号平均切分。
- 公共组件只有在存在多个调用方或明确独立职责时才抽取。

### React 与 TypeScript

- 页面组件负责布局和模块组合，数据请求、会话状态、编辑器、历史分页和复杂弹窗应各自拆分。
- 单组件状态超过 10 个时应检查是否可以按状态域提取 Hook。
- `useEffect` 必须明确依赖和清理逻辑；定时器、事件监听、订阅和请求都要释放或取消。
- 异步请求要防竞态。切换会话、关闭弹窗或重复发起请求后，旧响应不得覆盖新状态。
- 新增代码避免 `any`、无理由的非空断言和类型强转；必须使用时需有类型边界或注释理由。
- API 类型集中在 `src/lib/api.ts` 或对应领域类型模块，不在页面里重复定义大段接口字段。
- 浮层和弹窗优先使用现有 `Popover`、`ModalFrame`、`ModalPresence`，Portal 和焦点管理不应各写一套。
- 页面正文宽度和左右留白统一使用 `--content-max` 和 `--content-gutter`。
- 列表渲染使用稳定 key；交互必须覆盖空态、加载、失败、禁用、键盘和窄窗口。
- 临时调试输出、死代码和一次性兼容分支不进入提交。

### CSS

- 颜色、圆角、间距和动效时长优先使用主题变量，减少页面内硬编码。
- CSS 超过 500 行时按基础变量、页面布局、组件样式、状态和响应式拆分。
- 避免依赖会保留 `transform` 的祖先做 `position: fixed` 定位；浮层使用 Portal。
- 展开、收起、加载、成功、失败、键盘焦点和 `prefers-reduced-motion` 都必须有明确状态。
- 不新增无职责的全局覆盖和过多 `!important`；出现时需要能说明原因。

### Go

- 请求处理和工具执行必须使用 `context`，并设置合理超时与取消路径。
- 并发代码要说明 goroutine 由谁启动、何时退出、错误如何传递。
- 代理层、存储层、应用层和 Agent 编排层保持现有职责边界，不新增隐式全局状态。
- 大 `switch` 本身不是问题；分支内塞入大量独立业务逻辑时应抽成处理函数。
- 存储层不承担协议转换，代理层不直接管理页面状态。
- 新增公开行为需补对应回归测试；异常路径和取消路径同样属于验证范围。

## Lint 与自动检查

前端使用 ESLint。当前门禁规则：

- `react-hooks/rules-of-hooks`: `error`
- `react-hooks/exhaustive-deps`: `warn`
- `@typescript-eslint/no-explicit-any`: `error`
- `@typescript-eslint/no-unused-vars`: `error`
- `no-console`: 除 `warn`、`error` 外禁止
- `@eslint/js` 推荐规则和 `typescript-eslint` 推荐规则

Hook 依赖警告当前不阻断构建，但新增警告应优先消除。清理存量后，应把
`react-hooks/exhaustive-deps` 提升为错误，并评估 React 19 的 `refs`、
`set-state-in-effect` 和 `purity` 规则。

Go 当前使用 `gofmt`、`go vet` 和测试。暂不引入 `golangci-lint` 作为门禁；需要治理
`errcheck`、复杂度或重复代码时再单独评估，避免一次性引入大量历史告警。

## AI 检查流程

接到“客户端规范检测”或类似任务时，按以下顺序执行：

1. 阅读根 `AGENTS.md`、`xt-client/AGENTS.md`、`xt-client/README.md` 和本规范。
2. 排除测试、生成代码、构建产物和第三方代码。
3. 运行 `npm run lint`、`npm run check:size`、前端构建和 Go 验证。
4. 统计超大文件和超大组件；对超标项检查职责、状态数量、请求、副作用和嵌套。
5. 人工复核 Go 大函数、圈复杂度、超时取消、错误处理和并发退出。
6. 按严重程度输出问题，不在没有回归测试的情况下直接进行大范围拆分。

问题报告必须包含：

- 严重程度，例如 `P1`、`P2`、`P3`
- 文件路径和行号
- 实测指标，例如行数、函数数量、`useState` 数量
- 具体风险，例如状态耦合、竞态、难以测试或职责混杂
- 建议拆分边界
- 是否属于存量问题

示例：

```text
P1 文件：xt-client/frontend/src/features/agent/AgentPage.tsx:905
指标：组件 1083 行，46 个 useState，26 个 useEffect
判断：页面、会话状态、编辑器、引用、图片和弹窗流程混在一起
建议：先抽流事件和历史 Hook，再拆 conversation/composer 子目录
```

## 验证命令

前端在 `xt-client/frontend` 执行：

```powershell
npm run lint
npm run check:size
npm run build
```

Go 在 `xt-client` 执行：

```powershell
gofmt -l ./cmd ./internal ./tools
go vet ./...
go test ./...
```

结构规范或门槛调整后，还需在仓库根目录执行：

```powershell
node tools/agent-rules.mjs render
node tools/agent-rules.mjs check
```

## 例外与演进

协议对照表、自动生成映射、复杂 SQL 或明确的数据文件可以豁免行数门槛，但必须在文件顶部或
变更说明中写明原因。业务页面、服务编排和状态管理不属于“天然很长”的例外。

门槛应根据实际维护成本调整。调高门槛不能只为了消除告警；应说明现有拆分为何不可行，以及
用什么方式控制后续增长。
