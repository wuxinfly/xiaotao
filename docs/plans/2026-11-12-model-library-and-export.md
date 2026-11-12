# Agent 模型选择与对外模型清单

Agent 设置使用可搜索的模型库，模型接口页增加“模型调用清单”入口。清单以弹窗显示，不在接口页新增常驻模型区块。

## Agent 设置

- 常用模型与全部模型切换；首次无选择时直接浏览全部模型，已有选择时默认显示常用模型。
- 搜索匹配模型名称、上游 ID、内部引用和来源；可按来源、图片或推理能力筛选。直接勾选可连续选择，不再每加一个模型都关闭列表，也可添加当前筛选结果。
- 模型卡片展示来源、名称、上游 ID、上下文与能力/倍率。执行参数按需展开，窗口、最大输出及思考等级仍是 Agent 的独立覆盖值。
- 辅助任务（标题、行为复盘、上下文压缩）使用可搜索的选择器，支持来源和上下文识别；仍可跟随会话模型，不要求先加入常用模型。
- 保存仍写既有浏览器偏好，使用内部模型身份；不会修改来源、网关调用别名或已有会话快照。不可用选择明确标记，可移除，不按名称自动换绑。
- 常用与辅助参数一并校验窗口/输出范围；修正后可重新保存。取消会丢弃草稿，包括关闭动效期间快速重新打开的情况。

设置分类与底部操作保持可见，右侧独立滚动。搜索工具栏在长列表滚动时保留；窄屏改为单列卡片和顶部分类。参数区和辅助选择器均支持展开/收起过渡及 reduced-motion。隐藏内容设置 inert/aria-hidden，不进入 Tab 循环；辅助选择器内按 Escape 只关闭选择器，再按 Escape 才关闭设置。支持方向键移动选项、Enter 选择和关闭后恢复触发按钮焦点。

## 模型调用清单

接口页标题操作区的“模型调用清单”按钮按需打开弹窗，支持：

- 搜索名称、来源、上游 ID 和实际对外调用 ID；按来源筛选。
- 显示对外 ID、来源、模型名、上下文窗口、最大输出、图片/推理能力及已声明的思考档位。
- 单个 ID、单个配置 JSON、当前筛选结果 ID 列表及配置 JSON 批量复制。复制配置为通用数据，不是发送聊天的请求体；按外部工具字段使用。
- 未配置的窗口和输出明确显示“未配置”，复制时省略未知字段，不生成默认容量。显示可读 K/M，复制保留精确 token 数。
- 显示真实复制成功或失败；WebView 无异步剪贴板时回退到 textarea，之后恢复原焦点。读取失败可重试，空目录/搜索无匹配分别显示对应提示。

`GET /api/gateway/models` 是同源管理读取接口，直接返回 `gateway.Models()`，与 `/v1/models` 共用目录。包含已启用的手动来源及插件已开放模型；没有上游网络发现或测试请求。保留旧 `/api/proxy/models` 平台接口的停用状态。

弹窗使用网关返回的公开 `id`，不从 Agent 列表、来源名称或上游 ID 在前端拼接。中文、转义、插件短名和重名后缀都沿用网关结果。配置 JSON 只包含 model、已知上下文/输出、能力及可选思考档位，不含上游或本地密钥。Agent 模型身份与公开调用别名继续分离。

## 验证

前端定向回归：

```sh
XIAOTAO_TEST_EXECUTABLE=/usr/bin/chromium node --test tests/model-library.browser.test.mjs tests/gateway-models.browser.test.mjs tests/agent-model-settings.browser.test.mjs tests/model-pricing.browser.test.mjs tests/model-catalog.browser.test.mjs tests/model-sources-layout.browser.test.mjs tests/source-protocols.browser.test.mjs
```

覆盖多选/批量添加、搜索/来源/能力、参数与辅助模型持久化、取消草稿、失效来源、参数范围、键盘、剪贴板成功/回退/失败、未知字段、错误重试、弹窗关闭焦点及375/540/700/1280宽度；浏览器使用隔离 mock API，不写真实用户偏好或调用上游。截图输出到仓库 `work/ui`，不更新历史已跟踪图片。

`internal/app/gateway_models_http_test.go` 核对管理清单与认证后的公开目录一致，公开别名/配置正确、不泄露密钥，禁用来源后移除。

运行前端 lint/大小检查/构建、Go 大小/规则一致性检查、Go 全量测试/vet及 Windows 目标全量 vet。Linux 下 Windows desktop/secrets 与 runtimeenv 既有平台限制仍会影响全量本机运行；Windows WebView2 实机交互和真实账号需要本地验收，Chromium 与 mock 验证不替代这些验收。

本次7项定向浏览器回归、前端 lint/大小检查/构建、Go 文件大小/规则一致性检查及 Windows 目标全量 vet 通过。HTTP 集成用例通过 Windows 目标静态检查，本环境无法运行 Windows 测试。Linux 全量 test/vet 仍受前述既有平台限制。

现有身份与配置规则已覆盖内部 ID、来源、上游 ID 分离及禁止同名重绑；此次为界面改造，无新增规则候选。
