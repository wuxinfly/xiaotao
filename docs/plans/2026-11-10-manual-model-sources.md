# 统一模型来源与手动 API 接入（第 1、2 部分）

日期：2026-11-10。

状态：已实现并通过隔离回归和 Windows UAT 验证构建；真实上游与桌面壳人工验收未完成。第 3、4 部分仅写成[插件与平台迁移计划](2026-11-10-model-source-plugins.md)，本轮不实现插件加载器或安装入口。

## 用户流程

1. 打开左下角“模型接口”，添加来源名称、API URL 和 API Key。
2. 手动添加模型上游 ID（允许 `org/model`）、名称、上下文窗口、最大输出、图片/推理能力和可选推理档位。一个来源可以包含多个模型。
3. 保存接口；编辑时 API Key 留空表示保留，不回显原密钥。支持来源启停、编辑和确认删除。
4. 可主动“测试连接”，对来源第一个模型发一次 `max_tokens=16` 的小请求；可能产生上游用量，不计入本地正式聊天仪表盘。打开页面、读目录或刷新目录不会触发测试或模型发现。
5. 在 Agent 设置中选择常用模型、辅助默认模型及可选参数覆盖，再从会话输入区选择模型使用。

首期仅支持 OpenAI 兼容 Chat Completions。API URL 是实际 API 前缀（如 `https://example.com/v1`）或完整聊天路径，规范化只去掉尾部 `/chat/completions` 和斜线，不猜测补 `/v1`。根 URL 会调用根目录下的 `/chat/completions`。Responses、Anthropic 或平台专用协议不在本轮上游支持范围。

## 第 1 部分：统一来源与模型定义

`internal/modelsource` 管理来源元数据、模型和加密 API Key，不引用平台 Provider。

| 对象 | 字段与作用 |
| --- | --- |
| Source | `id` 稳定来源 ID，`label`，`kind=manual`，`protocol=openai-chat`，`baseURL`，`enabled`，只读 `hasAPIKey`，`models` |
| Model | `id` 稳定内部调用 ID，`upstreamModelId`，`label`，`contextWindow`，`maxOutputTokens`，`supportsImages`，`supportsReasoning`，`reasoningEfforts` |
| Input | 可写来源/模型字段；`apiKey` 只写，创建必填，编辑省略保留，显式空字符串拒绝 |

模型内部 ID 为路由键，不能用上游名称或首个斜杠推断平台。跨来源相同上游 ID 保留独立模型，不合并；更新保留原模型 ID，省略 ID 但上游 ID 未变时也保留；显式使用其他来源模型 ID 会被拒绝。删除再添加生成新 ID，不复用旧会话引用。

窗口/输出必须为整数；非空窗口至少 1024，非空输出至少 16，输出小于窗口。未知窗口/输出可以留空。推理档位为能力列表，不是默认选中项；请求不指定时不自动下发第一个档位。能力由用户声明，不以名称猜测图片或推理支持。

## 存储、凭据与环境隔离

- `model-sources/sources.json` 同时持久化来源元数据和加密 API Key。一次临时文件写入、Sync、Rename 成功后才更新内存；损坏配置或解密失败明确报错，不能覆盖原文件。
- `keys/secret.key` 延续 AES-GCM 与 Windows DPAPI 保护主密钥的实现；管理读取不含 API Key 或密文，浏览器不保存上游 Key。
- 本地网关密钥单独写 `keys/gateway.json`，首次升级可以只读沿用本环境原 `accounts` 中的网关密钥；之后轮换不改旧账号文件。
- 原 `accounts/` 平台账号及密文凭据保留，当前模型目录与网关不加载其平台账号，不运行登录、续期、配额或签到。
- UAT 首次只读导入正式版新的手动来源与 API Key，保留 ID、在 UAT 密钥环重新加密，一次提交；目标非空不覆盖，`model-sources/import.json` 标记完成。后续来源在 UAT 独立维护，删除后不再自动导入。
- 本地网关密钥、用量、其他配置、Agent 会话、日志和运行时状态继续隔离；不恢复原有全平台登录态导入。

## 第 2 部分：运行链路和管理界面

```text
模型接口页 ── 管理 API ── modelsource（配置与加密 Key）
                            │
Agent / 外部工具 ── 本地 gateway ── 指定来源的 Chat Completions 上游
                       │
                       └─ 真实 usage ── 仪表盘 / HUD
```

`internal/gateway` 保留本地独立 Bearer 鉴权、流式和非流式、图片/工具/reasoning 字段、请求取消和真实用量。上游请求替换为显式绑定的 `upstreamModelId`，下游响应使用稳定模型 ID。配置输出是默认值，明确请求输出参数优先；流式向上游请求真实 usage，下游是否包含 usage 依其 `stream_options` 决定。

连接上限 15 秒、总时长 30 分钟、读取空闲 300 秒，取消关闭上游，不携密钥跟随重定向。请求、非流响应及单个 SSE 事件上限 32 MiB。流式保留工具参数增量，不在已输出后重试换号。带内错误、畸形事件、截断帧和无完成标志断流明确失败，不伪造成功终态。缺失 token 保持未知，保留显式零值，不估算本地仪表盘用量。

`app` 不再注册或依赖原 `authsessions`、`proxy` 和各平台 Provider。旧源码保留为后续插件迁移材料，不编入客户端运行依赖。账号页替换为手动来源列表；本地调用入口卡仍独立显示本机 URL/Key，不能与上游 URL/Key 混用。来源增删启停通知 Agent 重读目录，按 sourceId/sourceLabel 分组。

Agent 模型选项新增来源身份、上游 ID、默认最大输出；窗口和输出在新会话创建时快照。已存在的会话、常用/默认模型引用保留；来源失效或未迁移平台模型显示不可用，需要用户明确切换，不能按同名静默重绑。原消息复制、回答分支、项目、工具和会话历史逻辑保留。

## API

| 路径 | 行为 |
| --- | --- |
| `GET /api/model-sources` | `{sources: [...]}`，只含安全元数据 |
| `POST /api/model-sources` | 创建完整来源和模型，返回 Source |
| `PUT /api/model-sources/{id}` | 更新完整来源，省略 Key 保留，返回 Source |
| `DELETE /api/model-sources/{id}` | 删除来源配置，返回 `{ok:true}` |
| `POST /api/model-sources/{id}/test` | 主动测试已保存来源，35 秒请求兜底 |
| `GET /api/agent/models` | 启用手动来源的 Agent 模型选项 |
| `POST /api/agent/models/refresh` | 重读本地配置，返回 models/errors，不访问上游 |
| `GET /api/accounts` | 仅保留手动来源标签兼容视图，供仪表盘/HUD 使用 |
| `GET/POST /api/proxy/key` | 读取/轮换本地网关访问密钥，沿用兼容路径 |
| `GET /v1/models` | 独立密钥鉴权后的启用模型列表；扩展字段沿用 `supports_images`、`supports_reasoning`、`max_input_tokens`，新增 `max_output_tokens` 与来源身份 |
| `POST /v1/chat/completions` | 标准 Chat 调用，model 为目录中的可读调用别名（兼容旧稳定内部 ID） |

旧 `/api/auth/*`、`/api/providers`、平台账号排序/备注/配额/签到管理和 `/api/proxy/models` 不再注册，返回 404，不回落静态 HTML。管理写入继续遵循同源校验，本地模型 API 使用独立访问密钥。

## 验证记录

已通过：

- 来源存储：密文/重载、损坏配置保护、原子写入失败、稳定 ID 和所有权、多来源同上游 ID、禁用和 UAT 重加密/只读源文件。
- 网关：流式/非流式、工具增量、reasoning、真实和缺失 usage、断流、取消、超时、请求上限、URL 路径、重定向拒绝、密钥轮换失败。
- 应用：新 API CRUD/同源保护、目录不访问远端、平台路由移除且旧数据不变、旧网关密钥兼容、UAT 一次导入、Agent 经真实本地 HTTP 网关访问隔离假上游、密钥轮换后继续调用。
- 前端隔离浏览器测试：theme、navigation-layout、model-catalog、model-pricing、qoder-checkin（现验证无平台入口及手动来源错误/删除）、agent-model-settings、agent-message-actions。覆盖空 Key 编辑、来源/模型稳定 ID、同名模型、能力和参数、目录同步、不可用状态、窄屏、Escape 与确认。
- `npm run lint`、`npm run check:size`、`npm run build`、`node tools/check-go-size.mjs`、`go test ./...`、`go vet ./...`。
- `./build.ps1 -Env uat -Verify` 成功，版本未递增、不改正式产物或快捷方式。
- `go list -deps ./cmd/xiaotao` 不含 `internal/proxy`、`internal/authsessions` 或平台 `internal/providers` 依赖。

一次全量复跑中，未修改的原生窗口用例 `TestStartupHandoffKeepsMinimizedWindow` 出现“startup handoff restored a window minimized by the user”；隔离连续复跑 3 次及随后全量回归通过，保留该波动记录；桌面窗口时序仍需人工验收。

未验证：真实外部 API、Windows WebView2 的实际交互；race 检查因当前环境 CGO 未开启未运行。Vite 仍提示已有大包与 Tiptap 的 use-client 指令警告，不阻断构建。

## 保留项与限制

自动审批拒绝了删除旧平台前端组件的操作，旧组件及为其编译保留的旧 API 类型/方法仍在源码中，但未被当前账号页面引用、没有后端可达路由。删除未执行；后续移植插件时再统一整理。这不影响新手动来源功能，不能称为旧源码已全部物理删除。

本轮无插件安装/进程机制，无平台插件实装，无外部 Responses/Anthropic 入口。平台余额和签到界面不在核心内展示，历史平台方案文档仅供迁移参考。

## 模型接口页布局修正（2026-11-10）

用户实机截图发现新页面漏用内容容器，标题和卡片贴边；外层 flex 拉伸还放大了空状态及区块间距。页面现使用 `style.css` 的通用 `.content-column`，统一读取 `--content-max` / `--content-gutter`，按内容高度排列，空列表不占额外网格间隔。本地调用入口改为标题说明、地址/密钥、操作的分行布局，避免说明挤成窄列；表单文本框和复选框隔离全局输入框宽度，保留窄屏单列与 Portal 弹窗。

回归入口：在 `xt-client/frontend` 运行 `node --test tests/model-sources-layout.browser.test.mjs tests/model-pricing.browser.test.mjs`。使用隔离 API，在 1280、1920、540、375 像素宽度检查实际留白、最大宽度、空态高度、区块距离、无横向溢出和表单控件尺寸。截图与测试结果不替代 WebView2 新包人工验收；本次不停止运行中的客户端。已有共享内容列规则覆盖此次修正，无新增规则候选。

添加/编辑接口弹窗进一步收紧为 660px 宽，减少字段间距与模型区内边距；来源名称和只读的上游格式并排，窄屏保持单列。明确当前仅支持 OpenAI Chat Completions，不支持 Responses / Messages；API URL 提示实际前缀不会自动补 `/v1`。本次为局部密度调整，README 和架构中的协议范围不变，无新增规则候选。


### 2026-11-12 可读网关调用 ID

`/v1/models` 公开 `<来源标识>/<模型标识>` 别名；内部 ID 保留供 Agent、会话与旧调用使用。手动别名独立持久化并保留删除记录；插件别名由完整插件 ID 与稳定 key 生成。详细格式、兼容边界与验收入口见 [客户端 README](../../xt-client/README.md) 和 [插件验收说明](../plugins/README.md)。
