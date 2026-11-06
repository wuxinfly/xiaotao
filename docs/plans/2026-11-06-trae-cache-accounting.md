# Trae 缓存用量统计核对

日期：2026-11-06

## 结论

`/api/agent/v3/llm_utils_chat` 的请求路由本身没有明显接错。标准 Trae CN 使用
`function: "chat_v3"`，SOLO 产品才使用 `solo_work_lite`。小涛当前的 Trae 请求体、
消息数组格式和 Anthropic 风格缓存字段解析都与公开实现一致。

但“仪表盘里的 0 等于实际未命中”这个结论不成立。多个开源项目在 2026-10-27 后观察到：
固定长前缀、相同账号和模型、甚至显式复用会话标识时，Trae SSE 的
`cache_read_input_tokens` 仍持续返回 0，而同一请求的商业账单可能报告非零缓存读取。
因此小涛现在记录的是“上游 SSE 明确上报的 0”，不是经过账单核验的真实命中数。

## 公开证据

- `ProvidersPlugin/Agent2Providers#27`：同一账号、同一模型、固定前缀连续请求全部为 0；
  直连上游并显式携带相同 `session_id` / `prompt_cache_key` 后仍为 0，维护者最终归因为上游问题。
- `ranxianglei/billion-context#5713`：抓到的 `token_usage` 帧包含
  `cache_read_input_tokens: 0`、`cache_creation_input_tokens: 0`，而
  `metadata.prompt_cache_pool_record` 为 `null`；同时指出请求体重写会丢
  `prompt_cache_key` / `stream_options`，但默认路由下这不是当前 0% 的主因。
- `magpie-community/plugins#455`、`#732`：同一网关的其他渠道命中率在 90% 以上，
  Trae CN 的数百次请求全为 0；本地探针重复相同消息、切换模型和会话头均无变化。
- `Ebony-Vinyl/dsh-our-free-model#213`：2026-10-26 的实测中，相同 payload 第二次请求返回
  `cache_read_input_tokens: 4224` / `prompt_tokens: 4233`，说明该通道以前确实能命中，
  128 token 对齐；后续归零更像是上游行为变化，而不是“该端点从来不做缓存”。
- `autumnsentiment/Trae2api-cn` v1.0.11：明确记录 2026-10-27 后 SSE 缓存读写可能全零，
  同一轮商业账单却报告真实缓存读取。它在 SSE 缺失或全零时调用
  `POST /api/v1/commercial/get_session_usage`，仅在输入总量精确匹配（旧版端点还要求输出总量
  精确匹配）时补回缓存明细；该版本验证样本记录到 22,400 个缓存读取 token。

## 对小涛的含义

1. 小涛的 `normalizeTokenUsage` 已经正确读取 `cache_read_input_tokens`，没有因为字段名不匹配
   而把非零值丢掉。
2. 小涛每次 Trae 请求都会生成新的 `session_id` / `request_id`，与公开实现为避免商业账单
   累计而采用的每请求独立 ID 一致。
3. 已接入商业账单补查。Trae 请求使用同一次请求的 `request_id` / `session_id`，流式 metadata
   返回上游会话标识后替换为上游值。SSE 缓存明细缺失或为 0 时，代理最多等待 3 秒，按
   0、250ms、750ms 的间隔查询 `POST /api/v1/commercial/get_session_usage`；只有输入、输出
   token 与本次 SSE 用量完全一致，且缓存数不超过输入总量时才补回缓存明细。
4. 账单补查不会改写输入、输出或总 token 数；失败、超时、账单尚未落库或不匹配时保留原始
   SSE 用量。不要用相同前缀重发来推测或伪造缓存命中。

## 实现与验证

- Provider 层捕获 `event:metadata` 的 `session_id`，非流式结果和流式回调都可取得该标识。
- 账单解析兼容顶层、`data`、`user_usage_group_by_session`、`usage_group_details` 及
  下划线/驼峰、字符串数值等公开实现中出现过的形态；只接受明确的非负整数计数。
- 单元测试覆盖账单字段解析、真实请求格式、输入/输出精确匹配、非零缓存不重复查询、
  明细行 session 不匹配拒绝，以及失败时保留原 usage。
- `go test ./...`、`go vet ./...`、`./build.ps1 -Verify` 通过。使用本机真实 Trae 账号对不存在
  的会话做只读探测，账单接口返回可解析的全零计数，确认域名、鉴权和响应格式可用；全零账单
  不会被当作有效匹配覆盖请求。真实请求的缓存恢复仍需在重建客户端后跑一轮多轮对话人工验收。

## 参考

- [Agent2Providers#27](https://github.com/ProvidersPlugin/Agent2Providers/issues/27)
- [billion-context#5713](https://github.com/ranxianglei/billion-context/issues/5713)
- [magpie-community/plugins#455](https://github.com/magpie-community/plugins/issues/455)
- [magpie-community/plugins#732](https://github.com/magpie-community/plugins/issues/732)
- [dsh-our-free-model#213](https://github.com/Ebony-Vinyl/dsh-our-free-model/issues/213)
- [Trae2api-cn v1.0.11](https://github.com/autumnsentiment/Trae2api-cn/blob/v1.0.11/docs/release-v1.0.11.md)
- [Trae2api-cn session_usage.py](https://github.com/autumnsentiment/Trae2api-cn/blob/v1.0.11/src/session_usage.py)
- [trae-api-proxy README](https://github.com/muskke/trae-api-proxy/blob/main/README.md)
