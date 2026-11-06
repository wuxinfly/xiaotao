# Qoder 活动签到

## 范围与接口

支持已登录 Qoder 国内和海外账号的状态查询、单账号领取及来源分组一键打卡。复用现有账号卡片、详情与反馈，不增加后台定时任务。

活动 API 使用账号区域对应 OpenAPI：CN 为 openapi.qoder.com.cn，Global 为 openapi.qoder.sh。查询 GET /sash/api/v1/me/campaigns?forceRefresh=true；领取 POST /sash/api/v1/me/campaigns/{campaignId}/claim，body {}。请求使用 Bearer 设备 token、User-Agent: Qoder、Cosy-ClientType: 10、Cosy-Version: 0.4.3，不走聊天的 COSY 签名。没有真实机器指纹时不编造机器头，不读桌面日志或本机 Qoder 凭据。

## 状态与一致性

仅处理当前 startAt/endAt 窗口中 actionType=CLAIM_BENEFIT、benefit.kind=CREDITS 的奖励。CLAIMABLE 才发领取请求；VIEW_DETAILS、过期、未开始和非积分权益不自动领取。同次按 campaignId 去重，每次重新获取列表，不跨天缓存活动 ID。

领取返回 status=CLAIMED 才确认成功。replayed=true 或列表已 CLAIMED 表示已领取，不记新增积分；金额优先使用领取结果，结果未包含 benefit 时使用该活动列表的上游金额，不写死 100。连续天数没有可靠来源，不虚构。没有活动、状态接口失败、领取失败分别显示；缺少 campaigns 或非法 JSON 不伪装成空活动。

过期凭据复用账号刷新锁；活动接口 401 时刷新并重试一次。刷新结果通过现有凭据版本检查持久化。领取后沿用配额刷新机制；进入账号页重新查询 Qoder 当前活动状态。

分组批量入口新增可选 provider 查询参数，点击 Qoder 只领取启用的 Qoder 账号，保留无参数全来源接口。批量结果的 error 字段用于报告失败账号数量，避免失败被显示成“已全部签过”。

## 参考

- 本机 E:/code_my/utils/buddy-proxy/src/buddy_proxy/qoder/campaigns.py 与对应测试。
- https://github.com/Zhengyuuuui/qoder2api
- https://github.com/YSwo-fei/dsh-qoder-connect/blob/main/lib/account.js
- https://github.com/mmqz/cpa-multi-plugins/blob/main/docs/PROTOCOL.md

按协议事实独立实现，未复制第三方代码。旧 daily-check-in 领取接口不使用。

## 验收

Provider 测试覆盖双区域、客户端标识、窗口筛选、重复领取、非积分活动、空列表、错误响应；服务测试覆盖状态存储、来源过滤、停用账号跳过和 401 刷新轮换。前端隔离 API 浏览器测试覆盖双区域卡片、无活动、领取、重放、分组参数、批量失败和窄窗口。运行：frontend 下 node --test tests/qoder-checkin.browser.test.mjs。

真实账号是否下发活动、领取到账和桌面壳交互仍待验收；没有活动可能涉及上游资格或设备定向，不能保证每个账号都有奖励。没有新增规则候选：协议变化已落在模块代码与本记录，暂无需要提升为仓库通用约定的证据。

本次验证：`go test ./...`、`go vet ./...`、隔离 API 浏览器回归和规则检查通过；`./build.ps1 -Env uat -Verify` 完成，输出 `xt-client/dist/verify/xiaotao-uat.exe`，不递增版本。仓库原有 VERSION 变更存在 CRLF 空白提示，本任务未改动版本值。
