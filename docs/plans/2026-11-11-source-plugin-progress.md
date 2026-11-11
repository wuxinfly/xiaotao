# 模型来源插件实施进度

范围：实现插件机制，仅迁移 Trae；其他平台暂缓。用户已授权直接实施。

## 当前完成

- 新建 `xt-client/internal/sourceplugin`，校验 manifest 身份、版本、必需能力及入口物理路径。
- 初步实现 stdio JSON-RPC 子进程、并发调用分发、15 秒初始化超时、请求取消等待、协议损坏/退出失败与直接子进程回收。
- 增加 manifest 和符号链接越界回归测试。
- 新建 `docs/plugins/README.md`，明确当前协议与未实现边界。

## 未完成

安装管理与环境隔离持久化、聊天事件/取消、核心凭据管理、统一目录/网关/Agent 集成、插件 UI、示例插件、Trae 登录/迁移/续期/配额/签到、开发者完整示例与最终契约测试。

## 验证

环境 `/usr/bin/go` 实际不是 Go 编译器（`go version` 返回 `Go: Unknown option: version`），且无 gofmt。新增 Go 代码尚未编译、格式化或运行测试，不代表验收通过。真实 Trae 账号及 Windows 验证尚未进行。

本次未变更既有行为与规则约定，无新增规则候选。
