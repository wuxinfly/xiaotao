# 协作角色共享格式 v1

`registry.yaml` 与 `team.yaml` 是身份和归属的权威记录，不替代 `office.db` 的任务、消息、产物与对话运行记录。全局注册表位于当前客户端环境的 `collaboration/roles/registry.yaml`；项目记录位于项目 `.xiaotao/roles/registry.yaml` 和 `.xiaotao/team.yaml`。正式版/UAT 使用各自环境目录，不会自动导入旧固定成员。

- `schema_version` 当前只支持 `1`；`revision` 是单调递增整数，写请求必须基于当前版本。
- 角色 ID 为服务端生成的 `role_` 加 16 位小写十六进制；名称不是外键。
- `scope` 只能为 `global` 或 `project`；`lifecycle` 为 `active`、`staged`、`archived`。成员只能引用 active 角色。
- 能力标签最多 8 个，每项 1–24 字符；名称最多 40 字符，职责最多 240 字符。
- `model_id` 仅是当前客户端模型目录的偏好；未知模型在对话时明确报错。上下文至少 1024，输出至少 16 且小于上下文。
- 外观 `shape` 为 `round`、`square`、`triangle`、`hexagon`；颜色为 `#rrggbb` 小写十六进制。
- 每项目最多 32 名成员，每注册表最多 128 个角色；未知字段、重复 ID、错误引用和非法外观都视为损坏，不自动修复。

当前只有客户端单写者；不支持 Skill 或其他进程并发写入。提升事务日志存于环境 `collaboration/transactions/`，仅由客户端启动或角色操作恢复。后续 Skill 只读消费需单独变更与验收，职责/能力文本不授予工具、文件、命令、审批或跨项目记忆权限。
