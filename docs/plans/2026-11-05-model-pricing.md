# 模型消耗倍率展示

日期：2026-11-05

用户要求 Qoder 模型列表显示消耗倍率，并覆盖其他有数据的渠道。实现仅展示明确目录字段，不推算扣费，不改变模型调用、账号路由或用量统计。

## 数据与行为

- Qoder：实时目录 price_factor / is_free；显式免费优先于非零基础倍率。
- Trae 国内版：display_contact_config.consumption_rate.data.rate。参考项目 src/buddy_proxy/trae/cn_catalog.py 记录了字段路径及含义；兼容对象/JSON 字符串与数字字符串。
- WorkBuddy：原生 `/v3/config` 的 `models[].credits` 为基础倍率（如 `x0.15`、`x0.00`）；`modelPromotions` 按 `modelIds` 关联当前生效优惠，校验 `enabled`、`priority`、`validFrom` / `validUntil` 及每日时段，支持跨午夜区间，优先级高者覆盖基础倍率。`auto` 未提供明确倍率时保持未知；Antigravity、Codex 当前目录接入未确定同类消耗字段，均不补静态值或公开 API 价格。
- ModelInfo 与 Agent ModelOption 增加可空 priceFactor / isFree；/v1/models 使用 price_factor / is_free。未知与显式零/false 可区分；非数值、负数、NaN、无穷大省略，坏计费元数据不破坏模型目录。
- 账号目录按所选账号展示；Agent 和公开目录同一模型按启用账号顺序保留第一条，防止后序账号价格覆盖路由账号价格。
- 共享 ModelPriceBadge 覆盖账号目录、Agent 设置、输入框模型菜单。免费或零倍率显示“免费”，否则显示原数值 ×；未知隐藏。提示说明仅同渠道比较、以上游结算为准。沿用主题变量与既有浮层交互。

## 验证

- go test ./...、go vet ./... 通过。
- frontend 下 npm ci、npm run build 通过；构建仅既有 chunk 体积提示。
- node --test tests/model-pricing.browser.test.mjs 通过：隔离全部 API，覆盖优惠免费优先、零、未知、三处展示、窄窗口与浅海主题；已查看合成截图，无溢出。
- Go 回归覆盖 Qoder 免费/付费/未知、Trae 对象及字符串目录、WorkBuddy 基础倍率/免费优惠/过期优惠/禁用优惠/跨午夜时段、零与无效值、公开接口保留 false、首账号元数据优先。
- 未替换正在运行的 EXE，未递增版本；真实账号和桌面壳展示需使用新包复核。

## 文档与规则

README、总体架构及 Qoder 实施记录已同步。模块职责、既有开发命令与规则边界没有变化，局部 AGENTS.md 和规则源无需修改；本次无新增规则候选。
