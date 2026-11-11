# 模型来源插件开发

状态：协议底层正在开发，尚未集成客户端，不能安装插件或调用 Trae。本文记录当前实现边界，不能作为已发布 SDK 承诺。

## 包结构与身份

包根目录包含 `manifest.json` 和可执行入口，例如：

```json
{
  "id": "example.platform",
  "version": "0.1.0",
  "contractVersion": 1,
  "name": "Example",
  "entrypoint": "bin/provider.exe",
  "capabilities": ["chat", "models"]
}
```

ID 使用小写点分命名空间，如 `vendor.platform`。入口必须是包内现存的普通文件，不能通过 `..` 或符号链接越出包目录。核心仅接受 contractVersion 1，chat 为必需能力；可选能力为 login、refresh、models、quota、checkin。入口检查不构成操作系统沙箱，安装管理仍需明确展示将运行的代码身份。

## 传输

插件为独立子进程，工作目录为包目录。stdin/stdout 使用 UTF-8、每行一条 JSON-RPC 2.0 消息，不使用 Content-Length。单条消息最多 32 MiB；stdout 不可输出日志。当前核心丢弃 stderr，避免将第三方敏感内容写入应用日志。

请求 ID 为正整数，响应必须原样带回；无 ID 的消息是通知。插件可以乱序回复并发请求。未知/过期 ID 的响应被丢弃。格式错误、非法版本、消息超限和进程退出会终止连接，使等待请求失败。错误对象包含整数 code 和字符串 message，核心不直接向用户转发第三方错误正文。

启动后核心首先发送：

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"contractVersion":1}}
```

插件需在 15 秒内响应，身份必须与 manifest 一致：

```json
{"jsonrpc":"2.0","id":1,"result":{"id":"example.platform","contractVersion":1}}
```

当前 `Process.Call` 的 context 取消只停止等待，尚未实现上游聊天取消协议。进程 Close 会终止并回收直接子进程；不保证清理插件自行创建的后代进程。

## 尚待实现的接入层

来源和模型注册、聊天事件、凭据引用和比较替换、登录会话、模型快照、配额和签到的 DTO 尚未落地。开发者暂勿依赖这些方法的最终字段。总体职责和目标见 `docs/plans/2026-11-10-model-source-plugins.md`。

安装、启停、卸载、网关路由、管理界面、Trae 迁移和真实账号验收也尚未完成。后续将补充可编译示例、打包步骤、完整协议及兼容性测试说明。
