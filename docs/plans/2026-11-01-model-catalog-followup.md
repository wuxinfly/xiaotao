# 模型目录跟进（2026-11-01）

## 第二轮修正

- WorkBuddy 按 buddy-proxy 的 web/models_config.json 同步国内 CodeBuddy 23 项目录，仍标为内置，不猜测动态端点。
- Trae 兼容 display_config 对象/字符串，过滤内部 function、custom_model、隐藏、关闭和自定义条目，对 ID 去重。保留原始大小写名称用于调用。
- Antigravity 请求 fetchAvailableModels，过滤旧代和 chat_/tab_ 条目，合并六项底表。新模型保留实际档位路由，通过请求凭据传入调用，不修改全局表。未实现生图响应路径，暂不收录生图模型。
- 刷新处理已知过期凭据，失败优先返回账号上次成功快照并标记 cached，无快照回退 builtin。聊天复用五分钟内目录，显式刷新始终请求，不写磁盘。
- 修正 Antigravity 前缀、消息构造、流式 provider 分派及 reasoning_effort 透传。

验收覆盖字符串元数据、过滤与原始名称、动态档位路由、底表保留及失败快照。自动化使用合成响应，不代表真实账号可调用；未主动执行逐模型付费探测。

以下第一轮记录中缓存、模型项数及 Antigravity 行为以本节为准。

目标：跟进 buddy-proxy main 的模型目录变化，让 Trae 的真实目录进入实际调用链。

实现步骤：
1. 核对参考仓库 CodeBuddy 模型来源；其仍用本地配置，无已确认的动态目录接口，因此 WorkBuddy 继续静态表，补充 mimo-v2.6-pro，不猜接口。
2. 页面模型刷新和 /v1/models 共用 proxy.Service.AccountModels，Trae 按账号凭据请求 get_detail_param；成功时仅采用目录，不合并静态模型。当前没有确认的目录外例外。
3. 排除关闭、隐藏、自定义及 builder 模型；目录请求限制 10 秒，HTTP、解析、空目录或凭据错误回退静态表。
4. 用替代 HTTP transport 验证成功替换、过滤、失败回退以及 WorkBuddy MiMo 条目，再运行客户端检查。

限制：未执行真实账号调用，MiMo 可用性及多模态能力来自参考仓库 f7db320 的配置，仍需真实账号验收。空目录视为接口不可用并回退，静态兜底不承诺当前账号可调用。目录每次刷新获取，不持久化缓存；聊天允许用户明确指定目录之外的模型名，不新增拦截。
