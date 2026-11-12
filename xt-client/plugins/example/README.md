# 最小插件模板

这是一个可以复制的最小模型来源插件，用于验证协议和起步开发。

```
plugins/example/
├── manifest.json
├── build.ps1
├── provider.example.json   # 示例上游响应，仅本地假数据
└── main.go
```

## 使用

```powershell
cd xt-client
go build -trimpath -o dist/plugins/example/provider.exe ./plugins/example
```

然后在“模型接口 → 安装插件”里手动填写 `xt-client/dist/plugins/example` 安装，或者把它加进 `plugins/index.json`。

## 协议要点

- stdin/stdout 每行一条 JSON-RPC 2.0，stdout 只能有协议消息。
- `initialize` 返回插件 ID 和 `contractVersion`，必须落在宿主给出的区间内。
- 其他方法收到 `Operation`，返回 `Result`。
- 失败用 `RPCError`，优先使用保留码：`-32001` 登录失效、`-32002` 限流、`-32003` 不支持、`-32004` 输入无效、`-32005` 上游异常、`-32006` 超时。
- 需要宿主打开浏览器的登录用 `host-login` 能力：`login/prepare` 返回描述符，`login/complete` 收回调参数。
- 展示类结果可以直接返回 `display`，宿主只按纯文本渲染。

更完整的说明见 [插件开发文档](../../../docs/plugins/README.md)。
