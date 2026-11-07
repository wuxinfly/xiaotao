> 已被 [结果气泡展开](2026-11-07-agent-result-expansion.md) 替代。以下为此前方案记录，控制工具、answering 与二次回答请求均已移除，不代表当前行为。

# Agent 执行与回答阶段

日期：2026-11-07。

用户已给出明确方案，采用每轮 working → answering → completed / failed / canceled，不使用关键词、长度或计时器判断正式回答。显式切换增加一次无工具模型请求；完整纯文本兼容路径直接交付。

## 实现

- 当时方案由后端 `answer.go` 定义 begin_answer 及独立收尾；同批工具先完成并按原 ID 回传，控制工具不计数。现行实现已取消独立收尾和固定轮数上限。
- answer_start/answer_delta/answer_end 共用 runId 和 messageId。开始保存占位，片段只更新当前 SQLite 消息行，结束更新同一条消息；异常时保留正文。
- AgentPage 独立维护进度草稿和正式消息；阶段绑定 runId。回答阶段只用一个头像，过程轨迹收起，正文实时追加；完成、失败、停止保留相应状态和入口。轨迹高度、图标摘要与请求/速度统计已按 [过程轨迹与结束摘要](2026-11-07-agent-trace-summary.md) 调整。
- 历史消息按 ID 单调合并，统计不从结束态回退。修复切换会话时旧 saveDraft 闭包覆盖新 SSE 缓存的问题。
- 检查分页边界：没有用户消息也能按 runId 显示正文；当页相关轮次统计额外补齐，不改游标。补充统计最多 80 个，跨页允许重复，由客户端按 runId 合并。
- 服务退出后识别中断任务时，轮次统计也标 failed 并结束，避免刷新后运行气泡复活。

## 验证

当时的 Go 回归包括混合控制/业务工具、第 40 轮边界、完整工具回传、不计控制工具、实时片段持久化、回答断流与取消、纯文本兼容无重生成、分页补充统计及中断恢复。现行回归覆盖超过 40 轮后完成及取消。

浏览器隔离全部 API，不访问正式实例：agent-answer 覆盖执行草稿不入正文、第一片段立即显示、迟到活动不删除正文、长回答底部跟随和上翻不跳转、后台输出与切换恢复、停止保留、刷新一致、第二轮隔离、兼容回答；agent-timeline 覆盖两行气泡、自适应轨迹、抽屉、完成元数据、窄屏与减少动态效果；history 覆盖 1 万会话、分页失败重试、位置保持、发送保留历史与切换后的迟到响应隔离。

验证结果：

- `go test ./... -timeout 120s`、`go vet ./...` 全部通过。
- 四个隔离浏览器入口 agent-answer、agent-stream、agent-timeline、history 全部通过；补充旧 start/end 快照不能截断正文和 working 统计不能回退 answering 后，再运行 agent-answer 与 history，2/2 通过。已查看合成场景截图，确认正文、完成统计与无正文失败入口。
- `./build.ps1 -Env uat -Verify` 通过，输出 `xt-client/dist/verify/xiaotao-uat.exe`；构建前后 VERSION 原文相同（1.3.11），不覆盖正式/UAT 常规产物或快捷方式，也未启动或停止正式实例。TypeScript/Vite 构建通过，保留原有大 bundle 与第三方 use client 提示。
- `node tools/agent-rules.mjs check` 通过；本次无规则源修改。

真实模型是否主动调用 begin_answer、真实渠道逐片段速度和 WebView2 壳交互仍需 UAT 人工验收；合成场景不代表真实账号实测。

## 文档与规则

已同步 README 与 Agent 架构说明。本次是明确的功能协议，记录在实现文档，不扩展仓库开发约定；没有新增待确认规则候选。
