# AI 任务结束系统通知实施方案

> 交给实施 AI：按本文逐项实施；可使用 executing-plans 技能。先读取根目录及 xt-client/AGENTS.md。本文是待实施方案，不代表功能已完成。不要覆盖工作区已有修改，不提交或发布无关内容。

**Goal:** 小涛桌面客户端不在前台时，Agent 本轮完成或失败，通过 Windows 原生通知提醒；点击后打开对应会话。

**Architecture:** Agent 服务产生唯一的轮次终态事件，桌面壳独立订阅并判断原生窗口前台状态，通过已有托盘发送通知。前端仅负责设置及点击后的会话导航，不作为通知事件的唯一来源。

**Tech Stack:** Go、Win32 Shell_NotifyIconW、WebView2、React/TypeScript、本地 HTTP/SSE。

## 1. 产品规则

- 首版覆盖小涛内置 Agent，不覆盖仅经过模型代理的外部 AI 请求；一个工具调用或一次模型回复不等于整轮任务完成。
- 通知开关默认开启，文案为“后台任务完成时通知”，补充说明包含任务失败。首版放在 Agent 设置中，正式版与 UAT 独立保存。
- 仅 completed / failed 通知；running、idle、canceled、等待工具确认均不通知。用户手动停止不通知。
- 发送前检查主窗口：主窗口或其拥有的原生对话框在前台则不通知；切到其他软件、最小化、隐藏到托盘则通知。主窗口在前台但正在看别的会话，也不通知。独立请求浮窗不算主窗口。
- 成功标题“小涛 · 任务已完成”，失败标题“小涛 · 任务失败”；正文“点击返回会话查看结果/详情”。UAT 使用“小涛 UAT”。不显示回答、路径、会话标题或错误详情。
- 通知出现不抢焦点、不自动恢复窗口。只有点击通知才恢复窗口并切换会话；删除的会话显示“该会话已不存在”，回到 Agent 页面。
- 纯浏览器模式和完全退出桌面壳后不发送原生通知。关闭到托盘仍支持。不会为了通知改变任务的取消和运行生命周期。
- 遵守系统通知控制，不强制保证右下角显示、声音、显示时间或通知中心历史；不绕过勿扰设置。

## 2. 选型与已有代码

推荐复用原生托盘通知：`internal/desktop/chrome_windows.go` 已有 Shell_NotifyIconW、notifyIcon 结构体和托盘回调，可用 NIM_MODIFY + NIF_INFO 发通知，通过 NIN_BALLOONUSERCLICK 接收点击。这条路线符合项目“不主动写注册表”的约定，首版不增加通知 SDK。

另两种路线暂不采用：浏览器 Notification 会增加浏览器权限和入口差异；现代 Toast 的独立激活与持久历史需要额外验证应用身份及部署方案。如果以后要求退出后点击启动、多个历史通知独立跳转，再单独设计现代 Toast。

官方参考：[Shell_NotifyIconW](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shell_notifyiconw)、[通知区域](https://learn.microsoft.com/en-us/windows/win32/shell/notification-area)。实施前核对结构体版本、UTF-16 字符串终止和回调参数编码；不可混用旧协议与 NOTIFYICON_VERSION_4 的解析方式。

已核对的接入点：

- `xt-client/internal/agent/service.go`：send 收尾已有 runID，产生 run_stats 和 status，随后保存历史。通知应在最终保存成功后发布；不得直接把 answer_end 当完成。持久化失败不发送成功通知，记录诊断。
- `xt-client/internal/agent/types.go`：已有 completed / failed / canceled 及 RunStats.RunID。
- `xt-client/internal/app/agent_handlers.go`、`internal/app/app.go`：接入服务端事件订阅及配置 API。
- `xt-client/cmd/xiaotao/main.go`：服务和桌面窗口分进程，不能只给服务挂一个桌面回调。
- `xt-client/internal/desktop/desktop_windows.go`、`chrome_windows.go`：窗口生命周期、原生前台判断、托盘通知与恢复。
- `xt-client/frontend/src/main.tsx`、`features/agent/AgentPage.tsx`、`features/agent/AgentModals.tsx`、`lib/api.ts`：导航、会话加载、设置入口。

## 3. 事件与跨进程通信

建议新增轻量终态订阅，不改现有聊天 SSE 协议：

```ts
type RunFinished = {
  eventId: string;
  sessionId: string;
  runId: string;
  status: 'completed' | 'failed';
  finishedAt: string;
};
```

建议接口 GET `/api/desktop/notifications/events`，仅接受现有运行时 Bearer token，token 放请求头，不放 URL；只从当前环境服务订阅。前端设置 API GET/PUT `/api/agent/notifications` 返回/写入 `{ enabled: boolean }`，沿用管理接口同源校验；明确校验字段类型、请求大小，旧配置缺失时默认 true。

服务持有有界内存事件环（建议最多 256 条、保留 5 分钟），游标包含服务启动标识和单调序号。首次订阅只收之后的新事件，不回放历史任务；短暂断线按 Last-Event-ID 补齐，超过保留范围或服务重启发送 reset，客户端建立新基线，不扫描历史补发。SSE 保活并支持取消，慢订阅者不得阻塞 Agent，也不得无限累积队列。

窗口进程负责订阅、退避重连和退出清理。去重键为当前环境 + sessionId + runId；前台或开关关闭而跳过的事件也标记已处理，切到后台后不补弹。真正调用原生接口前再次检查窗口状态和开关。内存去重表同样有界。

首版允许退出/重启桌面壳期间的通知丢失；不宣称可靠消息投递。通知错误仅写日志，不改变任务执行结果。

## 4. 原生通知和点击导航

在桌面 UI 线程调用 Win32/WebView 操作，网络订阅留在后台 goroutine。保留现有托盘左右键、隐藏恢复、Explorer 重启重建图标逻辑。

传统托盘回调不携带独立业务 ID，不能在新任务到来时直接覆盖当前通知的 sessionId。实现串行、有界通知队列，每次仅维护一个 active notification；SHOW、CLICK、HIDE、TIMEOUT 与清理均在同一状态机处理。队列事件超过 60 秒则丢弃，发送下一条前重新检查前台与开关。系统无回调时需超时清理，并验证迟到回调不会跳错会话；如果目标系统无法可靠区分迟到点击，则降级点击打开 Agent 页面并说明限制，不能猜测会话。

点击恢复复用现有窗口恢复路径。新增结构化导航消息 `{type:'open-agent-session', sessionId}`，通过 JSON 序列化和 WebView Dispatch 传递，禁止字符串拼接可执行脚本。前端根节点接收后切到 Agent，加载指定会话；页面未就绪时保留一个待处理导航，ready 握手后交付并确认消费。不得启动第二个窗口或跳到另一环境。

## 5. 实施任务

### Task 1：服务端轮次终态和订阅

新增 `xt-client/internal/agent/notifications.go`、`notifications_test.go`，修改 service.go；新增 `xt-client/internal/app/desktop_notifications.go`、`desktop_notifications_test.go`，在 app.go 注册。

1. 先测试成功/失败各一次、停止无事件、保存失败无成功事件、重复事件去重、并发会话互不串号。
2. 实现保存后发布、事件环、带鉴权 SSE、游标补齐/reset 和取消清理。
3. 测试首次连接不回放、短断线重连、服务重启、慢消费者、未授权访问拒绝。
4. 在 xt-client 执行 `go test ./internal/agent ./internal/app`。

### Task 2：通知设置

配置沿用当前环境的数据目录及已有原子持久化方式，建议新增 `xt-client/internal/agent/notification_settings.go` 和测试；不得覆盖已有 Agent 配置字段。修改 agent_handlers.go、frontend/src/lib/api.ts、features/agent/AgentModals.tsx（若设置组件已有拆分，跟随实际结构）。

1. 测试缺失默认开启、关闭重启保留、格式错误不覆盖旧值、环境隔离。
2. 增加设置开关，保存时显示加载、成功和失败反馈；失败保留原配置并允许重试。
3. 将最新开关同步到桌面消费者，关闭时清空待发送队列；不把配置只放 localStorage。

### Task 3：桌面通知消费者

新增 `xt-client/internal/desktop/notifications_windows.go` 与 `notifications_windows_test.go`，修改 desktop_windows.go 和 chrome_windows.go。

1. 抽出可注入的前台判断、时钟和通知发送接口，测试过滤、去重、排队、过期、关闭和失败。
2. 接入当前环境的 token/URL；订阅和重连随窗口生命周期启动/取消。
3. 用 HWND 及拥有者关系判断前台，不依赖 document.hidden、焦点事件缓存、窗口标题或整个进程名。
4. 实现托盘发送、回调及点击恢复，检查中文截断不能拆 UTF-16 代理对，字符串必须以 NUL 终止。
5. 专项覆盖两个任务快速完成时点击对应关系、无回调、迟到回调和 Explorer 重启。

### Task 4：会话导航与前端回归

修改 frontend/src/main.tsx、features/agent/AgentPage.tsx；建议新增 frontend/tests/desktop-notifications.browser.test.mjs。

1. 实现导航消息接收与 ready/ack；复用已有会话选择/加载行为，正常清除未读点。
2. 用隔离 API 测试从其他页面打开会话、组件尚未挂载、会话删除、快速连续导航、配置保存失败。
3. 测试不重复发送消息、不停止其他会话、不覆盖输入草稿；普通浏览器入口无需桌面桥也正常运行。

### Task 5：验证及文档

依次在 xt-client/frontend 执行 `npm ci`、`npm run build`、`node --test tests/desktop-notifications.browser.test.mjs`；在 xt-client 执行 `go test ./...`、`go vet ./...`、`./build.ps1 -Env uat -Verify`。AI 构建不得递增版本、覆盖正式包或替换正在运行实例。

更新 xt-client/README.md 的通知开关、支持范围和系统限制；更新 docs/architecture/xt-client-agent.md 的终态事件及跨进程链路；核对 docs/architecture/xt-client-overview.md，职责有变化才更新。本文补上真实测试结果及未完成项。规则变化只修改 docs/agent/rules.json 后 render；无规则变化也在根目录执行 `node tools/agent-rules.mjs check`。

## 6. Windows 实机验收

| 场景 | 预期 |
| --- | --- |
| 主窗口前台，含正在看另一会话 | 成功/失败均不通知 |
| 主窗口原生文件对话框前台 | 不误报后台通知 |
| 切到其他应用、最小化、隐藏托盘 | 完成/失败通知，不抢焦点 |
| 停止、工具结束、进入回答阶段 | 不通知 |
| 两个会话连续完成 | 不重复，点击不串会话 |
| 点击通知，页面尚未就绪 | 恢复窗口，就绪后打开对应会话 |
| 目标会话已删除 | 有明确提示，不白屏 |
| 开关关闭、重启后、正在排队时关闭 | 不通知，设置保留 |
| SSE 断开/重连、服务重启 | 无历史通知风暴、无重复 |
| 正式版/UAT 同开 | 文案、事件、设置、点击目标均隔离 |
| Explorer 重启、勿扰、系统禁通知 | 不崩溃，不反复弹窗重试 |
| 仅浏览器运行或桌面壳完全退出 | 不发原生通知，现有任务行为不变 |

自动测试不能证明系统横幅与点击回调正常，必须记录 Windows 实机结果；未实测项目明确标“待验收”。首个技术检查点先验证原生托盘横幅和连续通知点击，确认后再完成整条链路。

## 7. 本次交付记录

本次仅新增方案，未修改运行代码，未执行构建或功能测试。已检查客户端 README、局部 AGENTS.md、总体架构和 Agent 架构；当前行为文档无需改写，实施后的同步列为完成条件。没有新增规则候选：这是待实施需求，尚无可复用实施经验。
