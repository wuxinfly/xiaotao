# Antigravity 原生接入计划（2026-11-01）

## 范围

在 `xt-client` 中用 Go 原生接入 Google Antigravity，认证和协议转换参考 `E:\code_my\utils\buddy-proxy`（MIT 许可）。首期完成：Google OAuth 添加账号 → `loadCodeAssist`/`onboardUser` 项目初始化 → 内置模型表 → 配额查询 → 设为默认账号 → 外部工具调用。不含自动换号、账号冷却、跨渠道路由、agy 登录态导入、图片生成、Responses 和 Anthropic Messages 接口。

## 模块

| 模块 | 位置 | 说明 |
| --- | --- | --- |
| Provider | `xt-client/internal/providers/antigravity/` | OAuth PKCE 登录、loadCodeAssist/onboardUser、OpenAI→Gemini 转换、thoughtSignature 缓存、Collector/Streamer、Refresh、FetchQuota |
| 账号会话 | `xt-client/internal/authsessions/sessions.go` | Antigravity 登录分支，启动本地 OAuth 回调服务并返回授权链接 |
| 路由 | `xt-client/internal/app/app.go` | `POST/GET/DELETE /api/auth/antigravity[/{id}]` |
| 代理 | `xt-client/internal/proxy/service.go` | 按注册表分派模型列表和聊天，Antigravity 模型 ID 必须带 `antigravity/` 前缀 |
| 前端 | `xt-client/frontend/src/features/accounts/` | 登录入口、图标、Google 授权等待文案 |

## 认证流程

1. 前端调用 `POST /api/auth/antigravity`，后端启动 `127.0.0.1` 随机端口回调监听。
2. 生成 PKCE S256 verifier/challenge 和随机 `state`，拼装 Google OAuth 授权链接并返回。
3. 用户在系统浏览器完成 Google OAuth（`access_type=offline` + `prompt=consent`）。
4. 回调命中 `/oauth-callback`，校验 `state` 后用 `authorization_code` 换 token。
5. 用 access token 调 Google userinfo 获取邮箱，再依次调 `loadCodeAssist` → 必要时 `onboardUser` → 轮询 LRO 完成。
6. 取得 `project_id` 后标记登录成功；重复回调不会重复保存账号。
7. 会话 15 分钟超时，取消或结束时关闭监听。

凭据经 Windows DPAPI 加密存储在 accounts/keys 下，token、project ID 和 tier 一并保存。凭据临近过期时自动刷新（提前 5 分钟），401 时最多强制刷新并重试一次。

## 协议转换

### 请求方向（OpenAI → Gemini）

OpenAI messages 中 `system` / `developer` 内容转为 Gemini `systemInstruction`。Antigravity 服务端风控要求请求自称 Antigravity，因此在 systemInstruction 前注入两段身份文本（原文 + `[ignore]` 包裹副本，让模型读到身份但不自称）；用户自己的 system/developer 追加在后面，其中已知竞品身份断言（"You are Claude/Codex/ChatGPT/GPT"）替换为中性表述，用户正文、工具参数和工具结果不做替换。

`user` / `assistant` / `tool` 角色转为 Gemini `contents`。`tool` 消息转 `functionResponse` 并并入前一个 user turn（当它只含 functionResponse 时）；`assistant.tool_calls` 转 `functionCall`，保持 id 配对，为对外调用生成唯一 `call_*` ID。tools 声明转为 `functionDeclarations`（JSON Schema 做最小清洗），`tool_choice` 映射为 `functionCallingConfig.mode`。`temperature`、`top_p`、`max_tokens`、`stop` 和 `reasoning_effort` 映射到 `generationConfig`。

### 响应方向（Gemini → OpenAI）

`candidates[0].content.parts[]` 中的 `text`、`thought`（→ `reasoning_content`）、`functionCall`（→ `tool_calls`）分别转换。`finishReason` 映射为 OpenAI finish_reason。`usageMetadata` 转换为 `prompt_tokens`/`completion_tokens`/`total_tokens`/`prompt_tokens_details.cached_tokens`，缓存命中不重复累加。

流式路径逐行读 SSE `data:`，将每个 Gemini payload 转为 OpenAI chunk 并立即推送。空闲 90 秒断开，非流式总上限 120 秒。上游错误先解析再决定透传状态码。

### thoughtSignature

Gemini 3 系在 `functionCall` part 上返回 `thoughtSignature`，多轮工具调用时上游强制校验。小涛使用按账号隔离的有界内存 LRU 缓存（2048 条）回传签名；客户端显式带 `gemini_thought_signature` 时优先使用。签名不可用时（缓存 miss 或重启后）对 Gemini 模型注入 `skip_thought_signature_validator` 哨兵，Claude/GPT-OSS 模型不注入。

## 模型表

采用内置模型表（buddy-proxy 同款条目作为初始候选）：

| 对外 ID | 上游名 | 思考档位 | 组 |
| --- | --- | --- | --- |
| `gemini-3.1-pro` | 同名 | low / high | gemini |
| `gemini-3.6-flash` | 同名 | low / medium / high | gemini |
| `gemini-3.8-flash` | `gemini-3.8-flash-tiered` | 无（tiered 自动） | gemini |
| `claude-opus-4-6-thinking` | 同名 | 无 | claude-gpt |
| `claude-sonnet-4-6` | 同名 | 无 | claude-gpt |
| `gpt-oss-120b` | `gpt-oss-120b-medium` | 无 | claude-gpt |

未在表内的模型名返回 400。上游可通过 `fetchAvailableModels` 返回真实目录，但首期不做动态发现与自动下架。

## 配额口径

`fetchAvailableModels` 返回每个模型变体的 `quotaInfo.remainingFraction`（0–1）和 `resetTime`。按组（gemini / claude-gpt）聚合，组内取最低剩余比例作为概览；`resetTime` 是滚动窗口刷新点，不代表整组或每周池重置。当前实现把组内每个有数据的模型变体拆成单独 QuotaItem，`used` = (1 − remainingFraction) × 1000，单位 permille。缺失比例显示未知，不构造虚拟积分或周额度。

## 已知限制

- 单账号：无自动换号和冷却，配额耗尽直接报错。
- 首事件闸门（上游 429/403 带内错误时换号）未实现；本版本对带内错误直接返回。
- 图片生成模型（`gemini-3.1-flash-image`）未纳入模型表。
- 真实账号验收尚未完成：模型族可用性、连续工具调用、签名缓存未命中等均需用户实测确认。

## 许可声明

本模块复用了 buddy-proxy（MIT License, Copyright (c) 2026 Mayer）的认证流程、模型表条目、协议转换思路和 thoughtSignature 哨兵策略。许可文本保留在 `xt-client/internal/providers/antigravity/LICENSE`。
