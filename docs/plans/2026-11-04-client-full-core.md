# 客户端完整小涛 Core

用户要求客户端使用完整版小涛规则。实现以 xt-skills/xiaotao 为唯一资源来源，构建时压缩嵌入，发送前释放到当前环境的 Agent 数据目录。首次完整注入核心；后续以精简守卫和可读取的完整资源入口维持规则，遵守 Skill 不永久常驻全量文档的要求。

没有将所有 references 每轮注入，避免上下文膨胀；没有依赖开发机绝对路径，确保独立 EXE 可用。脚本与 schema 一并携带。无 Worker、Hook、自动恢复等宿主能力时如实降级，Python 未安装时不得宣称保存成功。

验收：首轮完整原文、后续精简守卫、核心脚本/参考资料一致性、修改安装副本后修复、不同环境目录隔离由自动测试覆盖。`go test ./...`、`go vet ./...`、规则 check 与 `./build.ps1 -Env uat -Verify` 均通过，验证包位于 xt-client/dist/verify/xiaotao-uat.exe。真实模型调用、记忆脚本运行与新 EXE 桌面体验待人工验收。本次不停止或覆盖正在运行的客户端。

资源生成入口为 xt-client/tools/bundle-core.mjs，build.ps1 自动调用；开发者直接 go build 前若改动 Core，需先 go generate ./internal/agent。
