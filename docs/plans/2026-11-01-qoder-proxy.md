# Qoder 反代接入实施计划（2026-11-01）

> 执行时使用 `executing-plans` 技能逐项落实；本文是实施计划，不代表功能已经完成或真实账号已经验证。

**目标：** 在小涛客户端接入 Qoder 国内版和海外版账号，提供浏览器授权、凭据刷新、账号级模型目录、OpenAI 兼容聊天及真实额度展示。

**架构：** 新增独立 Go Qoder Provider，封装区域端点、COSY 签名、请求适配和 SSE 拆包；复用现有账号存储、登录会话、代理访问密钥及用量统计。沿用模型前缀路由，来源内按启用账号顺序选择；模型目录绑定账号和区域。

**技术栈：** Go 标准库（HTTP、AES、RSA、MD5、SHA256、Base64）、React、TypeScript、Vite、现有 Windows DPAPI 凭据存储。

## 1. 范围与边界

本期包含：

- Qoder `cn` / `global` 两区浏览器设备授权，取消、超时、重新登录和账号去重。
- access token 刷新与 refresh token 轮换后的安全保存。
- 按账号获取、缓存及刷新模型目录，使用真实上游 key 调用模型。
- `/v1/models`、`/v1/chat/completions`，流式与非流式、reasoning、工具调用、图片内容透传及真实 token 用量。
- 订阅额度、加购包、活动专属资源包与各自到期时间。
- 账号页渠道、区域、授权状态、模型和额度展示。

本期不包含自动签到、机器指纹原生程序集成、桌面日志读取、从 Qoder 本机配置导入凭据、PAT 登录、多账号 failover、Responses / Anthropic 新协议入口、credit 统计数据库与图表扩展。可管理多个账号，实际调用按模型来源和启用账号顺序选择。

Provider ID 使用 `qoder`，账号区域使用 `cn` / `global`，模型对外统一为 `qoder/<公开模型名>`。请求由模型前缀确定来源账号，区域跟随所路由账号；错误区域不跨区兜底。参考项目的 `qoderintl` 属于其路由设计，不照搬到小涛。

## 2. 参考实现与当前约束

参考仓库：`E:\code_my\utils\buddy-proxy`。下列路径均相对于该仓库：

| 文件 | 参考内容 |
| --- | --- |
| `src/buddy_proxy/qoder/config.py` | 区域域名、端点、COSY 版本、公钥、编码字母表 |
| `src/buddy_proxy/qoder/cosy.py` | 身份加密、签名、请求体编码 |
| `src/buddy_proxy/qoder/credentials.py` | PKCE、设备授权、userinfo、token 刷新与并发轮换保护 |
| `src/buddy_proxy/qoder/catalog.py` | 目录解析、启用状态、模型 key 与公开名映射 |
| `src/buddy_proxy/qoder/provider.py` | 请求归因信封、模型配置、流式/非流式处理 |
| `src/buddy_proxy/qoder/convert.py`、`errors.py` | 消息修正、SSE 信封拆解、带内错误详情 |
| `src/buddy_proxy/qoder/quota.py` | 资源包过滤、剩余额度与到期语义 |
| `tests/test_qoder_provider.py`、`test_qoder_credentials.py` | 编码固定样例、签名、工具回合、凭据行为 |

以上为本地源码观察，参考代码中的“实测”说明不等于小涛已经完成实测。实施时记录参考提交版本，优先依据实际代码和测试样例，遇到注释冲突不凭注释定协议。

小涛当前约束：

- `providers.Provider.ListModels(region)` 无账号上下文，无法表达 Qoder 账号权益差异。
- `proxy/service.go` 的请求体转换、模型前缀和流式分派仍有渠道专属分支；不能仅注册 Provider 就接通。
- `authsessions/sessions.go`、`app/app.go` 和账号前端仍有 Trae / WorkBuddy 专属处理。
- 工作区存在进行中的 WorkBuddy、Antigravity 修改。实施前重新阅读最新代码与差异，在最新接口上接入，不覆盖这些修改，不将未完成渠道作为稳定基线。

## 3. 协议约定

### 区域与认证

| 区域 | 推理服务 | 账号服务 | 授权页 |
| --- | --- | --- | --- |
| cn | `https://gateway.qoder.com.cn` | `https://openapi.qoder.com.cn` | `https://qoder.com.cn/device/selectAccounts` |
| global | `https://api3.qoder.sh` | `https://openapi.qoder.sh` | `https://qoder.com/device/selectAccounts` |

- 生成随机 verifier、S256 challenge、nonce 和稳定保存的 machine ID；授权链接包含 `challenge`、`challenge_method`、`nonce`、`machine_id`、`client_id`。verifier 仅留后端会话。
- GET `/api/v1/deviceToken/poll` 携带 nonce、verifier、challenge_method；404 继续等待，200 解析凭据，其余状态明确失败；总授权窗口 10 分钟，取消立即终止轮询。
- GET `/api/v1/userinfo` 补齐 UID、名称等身份信息；缺少有效 token 或 UID 不能保存为成功账号。
- POST `/api/v1/deviceToken/refresh` 使用 refresh token；解析秒、毫秒或 RFC3339 过期值，内部统一为 Unix 秒。保留未返回的账号身份、区域和 machine ID。
- 首期只使用明确的区域端点，不自动读取 Qoder 的 endpoint-cache 或跨区探测。

### COSY 与聊天

- 聊天走 `/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1`。标准 `/model/v1/chat/completions` 在参考实现中不提供 Qwen3.8，不作为首期聊天入口。
- 请求体先标准 Base64，再映射自定义字母表（`=` 转 `$`），最后按三段重排；输出原样发送，不再做 JSON 字符串包装。
- 身份 JSON 使用 AES-128-CBC 和 PKCS#7，随机 ASCII key 同时作为 IV；key 使用配置中的 RSA 公钥做 PKCS#1 v1.5 加密。具体字段、头部及版本对照参考实现。
- 签名串为 `payload_b64 + 换行 + cosy_key + 换行 + ts + 换行 + encoded_body + 换行 + signature_path`，MD5 十六进制；path 去掉 `/algo` 且不含 query。发送 `Bearer COSY.<payload>.<sig>`、`Cosy-User` 及其余所需 Cosy 头。
- 上游强制流式并请求 usage；保留原始 OpenAI messages / tools，在 Qoder 内补 request/session/business/model_config 等归因字段，不经过 Trae NativeMessages。
- `developer` 转 `system` 并移除其 `tool_calls`；有 `tool_calls` 且 content 为 null 的消息先改为空字符串；不改其他 Provider 的消息语义。
- SSE 外层 body 是内层 OpenAI JSON 字符串；处理 usage-only、工具参数分片、reasoning、完成帧、顶层错误和内层错误。错误原因解析 `details.error.message`，不能把 HTTP 200 当成功。
- 上游 EOF 未收到协议完成标记时按中断处理；非流式仅在正常完成后返回聚合结果。客户端取消关闭上游响应。
- 首期聊天空闲上限 300 秒、总时长上限 30 分钟，连接上限 15 秒；到期和取消必须释放读取与计时资源。收到内容后不自动重放；首期不做换号或排队自动重试。
- 非 2xx 使用 `providers.UpstreamError`；带内错误按明确错误码映射认证、权益/限流或上游失败，不把签名错误一律当登录过期。输出错误必须脱敏。

## 4. 实施任务

每项先补能证明协议行为的测试并运行确认失败，再实现并运行通过；提交时仅选择本任务文件或相关改动块。文档和纯展示调整不增加复刻实现的测试。

### 任务 1：区域配置与 COSY 签名

新增 `xt-client/internal/providers/qoder/config.go`、`cosy.go`、`cosy_test.go`、`http.go`。

1. 从参考测试提取无凭据的固定编码向量，覆盖空串、中文、填充与三段边界；固定测试随机源和时钟，验证签名输入、路径去前缀且去 query。
2. 用 Go 标准库实现编码、AES/RSA、签名和必要请求头；所有依赖可在测试中替换，不引入厂商二进制。
3. 增加 mock HTTP 端点，断言原始编码 body、所需头部、取消与错误状态。
4. 在 `xt-client` 运行 `go test ./internal/providers/qoder -run 'TestEncode|TestSign|TestHTTP' -v`，预期全部通过。

### 任务 2：设备授权与凭据刷新

新增 `xt-client/internal/providers/qoder/auth.go`、`auth_test.go`；修改 `xt-client/internal/authsessions/sessions.go`、`xt-client/internal/providers/registry.go`、`xt-client/internal/proxy/account.go`、`xt-client/internal/proxy/service.go`。按职责新增 `xt-client/internal/authsessions/qoder.go` 与测试文件，避免将全部逻辑塞进 sessions.go。

1. mock 授权轮询 404 → 200、用户取消、10 分钟超时、异常状态及缺身份响应，断言 PKCE 参数和 verifier 不向前端暴露。
2. 实现授权 URL、轮询、userinfo 与刷新；只有确实需要的新身份字段才扩展 Credential，凭据继续经现有 DPAPI 保存。
3. 合并聊天和额度查询的同账号刷新入口；在锁内重读最新凭据，只有一个请求刷新，等待者复用已保存的新 token。刷新失败不覆盖可用凭据；账号删除或重新登录后，旧刷新结果不得覆盖新状态或复活账号。
4. 测试 refresh token 轮换、返回字段缺省、过期单位转换，以及聊天与额度同时触发刷新。核对所有持久化结果。
5. 在 `xt-client` 运行 `go test ./internal/providers/qoder ./internal/authsessions ./internal/proxy`，预期通过。

### 任务 3：账号级模型目录

新增 `xt-client/internal/providers/qoder/models.go`、`models_test.go`、`provider.go`；修改 `xt-client/internal/providers/registry.go`、`xt-client/internal/proxy/service.go`、`xt-client/internal/app/app.go`。

1. 为动态目录增加带 context 和 Credential 的可选能力（如 `ModelFetcher`），现有静态 `ListModels` 接口继续兼容其他渠道。管理页与 `/v1/models` 共享同一目录服务。
2. 实现签名 GET `/algo/api/v2/model/list?Encode=1`，解析场景去重、enable、BYOK、自带图片与 reasoning 能力；公开名映射到上游 key，未知新模型生成稳定且不冲突的公开 ID。
3. 缓存按账号 ID 和区域隔离，建议 TTL 5 分钟；登录成功后预热，过期访问时刷新，合并并发刷新。删除/重新登录清理对应缓存。
4. 首次获取失败明确返回目录不可用；后续刷新失败保留该账号最后成功目录并向管理页标明陈旧/错误，不把静态候选表或其他账号目录冒充账号真实权益。
5. `/v1/models` 聚合启用账号目录并按完整模型 ID 去重；调用按模型前缀与账号目录匹配，对未知/已禁用模型快速失败，不静默转其他模型。
6. 覆盖两账号不同目录、同 UID 不同区域、禁用项、首次失败、刷新失败保留旧值和来源内账号顺序路由。运行 `go test ./internal/providers/qoder ./internal/proxy ./internal/app`。

### 任务 4：聊天适配与统一代理分派

新增 `xt-client/internal/providers/qoder/chat.go`、`convert.go`、`chat_test.go`；修改 `xt-client/internal/proxy/service.go`、`bridge.go`、`service_test.go`，按需补充 `xt-client/internal/providers/registry.go` 的输出能力。

1. mock COSY 流，覆盖文本、reasoning、工具调用与工具结果后续回合、图片消息、usage-only、DONE、带内错误、断流、取消、超时；非流式聚合与流式输出的语义一致。
2. 实现归因信封、模型 key/config 和消息专属修正；请求字段按明确白名单传递，校验 temperature、max_tokens 等支持字段能从代理入口到上游，不因入口结构体丢失。
3. 按账号 Provider 分派请求转换、collector、streamer 与模型前缀。保留现有 Trae 兼容行为，Qoder 要求 `qoder/`，不允许跨渠道前缀误投；未知 Provider 明确报错。
4. 确认现有 ChatEvents 能传达 finish_reason；不足时最小扩展并适配现有渠道，不能把工具调用完成统一改成 stop。
5. token / cache usage 接入现有真实用量流程，缺失值保持未知。Qoder `credits`、`original_credits`、`billable` 可保留在兼容响应中，本期不扩展数据库，不估算 credit。
6. 流开始前失败返回适当 HTTP 错误；流开始后错误通过 SSE 返回并终止，不伪装正常完成。
7. 运行 `go test ./internal/providers/... ./internal/proxy ./internal/usage`，预期新测试与既有渠道回归通过。

### 任务 5：额度、管理接口与账号页面

新增 `xt-client/internal/providers/qoder/quota.go`、`quota_test.go`；修改 `xt-client/internal/proxy/bridge.go`、`xt-client/internal/app/app.go`、`xt-client/frontend/src/lib/api.ts` 和以下账号页文件：

- `xt-client/frontend/src/features/accounts/AccountsPage.tsx`
- `xt-client/frontend/src/features/accounts/components/AddAccountModal.tsx`
- `xt-client/frontend/src/features/accounts/components/ProviderIcon.tsx`
- 按实际需要调整 `AccountDetailModal.tsx`、`quotaUtils.ts`、`accounts.css`。

1. 实现 GET `/api/v2/quota/usage`，展示 userQuota、加购与有效 dedicatedResourcePackages；过滤明确失效/不可用包，保留独立标签与到期时间。未知指标使用空值，不伪造零值。
2. 优先用 used/total 计算已用比例，不能直接将参考协议的 remaining percentage 当作已用百分比；`expiresAt` 映射现有 `QuotaItem.ExpireAt`。
3. 注册 Provider，接入 `POST /api/auth/qoder`、`GET /api/auth/qoder/{id}`、`DELETE /api/auth/qoder/{id}`，沿用同源校验和管理认证。按 Provider + Region + UID 去重，重新登录不重复建号。
4. 新增 Qoder 选项与两区选择、授权等待/取消/超时、复制链接和重新打开浏览器；将登录文案按 Provider 数据驱动，避免新增嵌套的渠道三元判断。
5. 沿用现有内容宽度变量、body portal 浮层规则、键盘与减少动态效果要求；浏览器和桌面壳检查弹窗、反馈与额度错误状态。
6. 运行 `go test ./internal/providers/qoder ./internal/authsessions ./internal/app ./internal/proxy`；在 frontend 执行 `npm ci`、`npm run build`，预期通过。

### 任务 6：文档、完整验证与真实账号验收

修改 `xt-client/README.md` 和本计划，检查 `docs/architecture/xt-client-overview.md` 中渠道接入及状态描述；必要时同步实际已变化的部分。若产生可复用开发约定，修改 `docs/agent/rules.json` 并 render，不能直接改 AGENTS.md。

1. 文档写明区域选择、登录、`qoder/` 模型调用、目录缓存/刷新错误、额度到期、超时与首期边界；不提前声称支持自动签到或多账号切换。
2. 按下述命令完整验证，记录执行结果。既有工作区导致的失败独立列出，不能以未相关为由宣称全部通过。
3. 使用明确获准的测试账号分别验收 CN / Global；缺少某区账号时标记该区未实测。真实聊天会消耗额度，只用最小提示与必要工具回合。
4. 把完成项、真实结果、剩余问题写回本计划，检查 README、架构、使用说明和验收内容一致。候选只收有可达证据的复用事实/经验，一次性范围决定不写成全局规则。

## 5. 完整验证命令

```powershell
# E:\code_my\xiaotao\xt-client\frontend
npm ci
npm run build

# E:\code_my\xiaotao\xt-client
go test ./...
go vet ./...
./build.ps1

# E:\code_my\xiaotao
node tools/agent-rules.mjs check
git diff --check
```

预期：测试、vet、前端和 EXE 构建成功，规则与 diff 检查无错误。规则源发生变化时先运行 `node tools/agent-rules.mjs render`。构建脚本会更新当前用户桌面“小涛”快捷方式，按项目现有约定执行。

## 6. 验收清单

- [ ] 两区授权链接域名正确；浏览器授权后账号保存；取消、超时、重新登录无遗留轮询或重复账号。
- [ ] 并发聊天/额度刷新只轮换一次凭据；重启后新凭据仍可用；日志、前端及错误无 token、verifier 泄漏。
- [ ] 模型目录来自实际账号；切账号/区域不串缓存；刷新失败有状态且保留正确旧值。
- [ ] 普通文本、reasoning、图片能力匹配、工具调用及回传后的下一轮、非流式与流式工作正常。
- [ ] 用量与上游确认值一致；缓存 token 不重复累计；缺失值保持未知；credits 不被误认成 token。
- [ ] HTTP 与带内错误能展示真实原因；断流不假成功；客户端取消和超时释放连接；无内容输出后的自动重放。
- [ ] 订阅、加购、活动包额度及到期时间与上游一致，不把到期写成重置。
- [ ] 模型前缀路由语义、代理密钥校验保持一致，Trae / WorkBuddy 及实施时已接入渠道回归通过。
- [ ] 桌面壳和浏览器两种入口的授权弹窗、键盘、加载/失败反馈通过检查。
- [ ] README、架构相关说明与本计划同步，未实测项单独记录。

## 7. 当前状态

2026-11-01：任务 1–5 的主体实现已落盘，任务 6 文档及本地验证进行中；上面的真实验收清单保持未勾选，不把 mock 通过当作真实账号通过。

参考 buddy-proxy 提交：39fb533d107988e38f30b4ef77cf80c6758096be。

- 已实现：ASCII AES key、签名校验测试、两区设备授权/取消/超时/userinfo、token 轮换、共享刷新互斥、比较替换保存、Provider 注册及管理接口、按 Provider+Region+UID 保存。
- 已实现：可选 ModelFetcher、按账号/区域/凭据版本的五分钟缓存、并发获取合并、显式刷新、失败旧目录与错误状态、账号目录路由、登录后目录预热。未知模型 ID 使用显示名及 key 摘要，保证稳定并避免同名冲突。
- 已实现：归因信封、消息修正、请求白名单、COSY SSE、reasoning/工具分片/usage、正常完成检查、取消及空闲/总时长上限；代理流开始前保留 HTTP 错误，开始后发错误帧并终止，不发伪成功完成帧。
- 已实现：订阅/加购/有效专属包，数值未知不补零，独立到期；账号页两区选择、重新登录、链接复制/打开、目录错误反馈和 body portal。
- 本地已通过：Qoder、authsessions、proxy、storage、app 及其他 Provider 回归；go test ./...、go vet ./...；npm ci、npm run build、./build.ps1。
- 浏览器已检查：添加入口、国内/海外选择、弹窗位置、打开后聚焦关闭按钮、Escape 关闭并恢复焦点；使用独立临时数据目录，未触碰现有账号。尚待 CN/Global 真实账号完整验收、桌面壳交互、减少动态效果、图片及完整工具回合和真实错误样本校准。
- 最终验证：go test ./...、go vet ./...、./build.ps1、node tools/agent-rules.mjs check、git diff --check 均通过。一次重建因测试实例占用 EXE 失败，停止该实例后重建通过。EXE 位于 xt-client/dist/xiaotao.exe。
- 文档已同步 README 与客户端总体架构说明；未修改规则源或自动应用候选。现有候选 model-catalog-route-identity 已覆盖目录展示与上游 ID 分离经验，本次不重复新增候选。
