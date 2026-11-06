# WorkBuddy 国内原生渠道接入

用户确认 CodeBuddy 与 WorkBuddy 权益不通用，要求改为 WorkBuddy 原生登录。

## 实现

固定上游 https://www.workbuddy.cn，先 POST /v2/plugin/auth/state?platform=workbuddy，再打开返回 authUrl 并轮询 token。账号 UID 从 JWT sub 读取，过期时间兼容秒/毫秒及 JWT exp；JWT 本地解析仅提取元数据，签名由上游验证。不再调用 CodeBuddy login/account。刷新沿用 plugin 路径，保留轮换 token 与账号身份校验。

聊天保持标准 OpenAI 消息与现有超时，身份头改为 WorkBuddy / SaaS。额度和资源包查询使用同一原生域名、轻量计费头。签到保留现有路径并改走原生域名，真实活动是否可用待验收。

模型改从 GET /v3/config 按 default 标签、cli、首个非空 Agent 顺序选择聊天模型；排除生图、按 ID 去重。复用按账号凭据版本隔离的五分钟缓存，失败可保留同登录的上次目录，无缓存时返回错误，不再使用 CodeBuddy 的 23 项底表。外部调用 ID 仍为 workbuddy/<id>。

旧账号文件不删除、不自动转换：domain 不为 www.workbuddy.cn 的凭据在目录、刷新、聊天及计费请求前拒绝，提示重新登录。用户需在账号页重新登录；Agent 设置中刷新模型列表。只支持国内版，海外版不在本次范围。

## 参考证据

- https://github.com/Chikage0o0/stravia-vendor-workbuddy ，本次检出提交 `8eb766c1e1450c1bfeb1b250b80d96a73d672a00` 的 auth.rs、models.rs、lib.rs。使用协议事实重新实现，未复制其代码。
- 真实无凭据授权请求成功返回 code=0、www.workbuddy.cn/login 与 state；未保留登录链接或 state。

## 验证与边界

来源行添加入口：各来源行直接发起对应登录，不再重复选择来源；Qoder 行仅展示国内/海外区域选择，顶部添加入口仍保留完整来源列表。前端 TypeScript 检查和 Vite 构建通过，桌面壳点击与真实授权跳转待新包验收。

额度汇总修复：前端原先只识别“订阅额度 / 加购包”，导致 WorkBuddy 的“订阅套餐 / 资源包”回退展示第一项；现纳入积分合计，保留各包明细、未知数值及显式总额优先。`node --test tests/quota-summary.test.mjs` 覆盖三包 2,100 积分、部分消耗、未知余额、显式总额与其他渠道窗口；前端构建通过。桌面壳新包显示仍待验收。

`go test ./...`、`go vet ./...`、规则检查通过，`./build.ps1 -Env uat -Verify` 完成前端与 EXE 构建（版本不递增，输出 `xt-client/dist/verify/xiaotao-uat.exe`）；原生授权 URL 已在线验证。完整登录、真实模型目录、积分、签到、聊天及工具调用还需用户在验证版完成验收，不冒充真实账号已跑通。不会读取或改写本机 WorkBuddy 凭据。
