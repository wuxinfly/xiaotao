# 阶段进度：一次保存

在阶段结束交付前合并保存一次，连续修改和验证不分开写；长任务在重要决策、阻塞或即将中断时补存。无新事实或用户明确不保存时跳过。普通讨论与轻任务不因保存创建 Temporary/Task。

同一会话不重复读取未变化的规范。正常成功不播报内部步骤；失败简短说明哪部分已保存，不反复换 shell 或手工构造锁、快照、事务。

## 通用入口

调用可信安装的 Core 脚本，以 stdin 提交 JSON：

```text
python <core>/scripts/save_progress.py --project-root <project>
```

```json
{
  "request_id": "session-stage-unique-id",
  "topic_id": "activity-fix",
  "title": "活动修复",
  "summary": "修正活动白名单校验",
  "details": "补充实现依据与需要保留的细节。",
  "validation": "相关测试通过；尚未部署。",
  "next_steps": "等待验收。"
}
```

- `topic_id` 为稳定小写主题 ID；同日后续进度沿用它。`title`、`summary` 必填，其他事实字段可省略。
- `request_id` 每个不同阶段唯一；结果不确定或部分失败时重试原 JSON，不换 ID。脚本固定首次请求的日期、时区与待提交版本，跨午夜重试仍回到原日期。
- 有明确关联工作时添加 `work_path`，仅限已有 Task `progress.md` 或 Temporary `current.md`。脚本保留原文，追加本阶段进度，内部检查版本/hash、加锁并原子写入和回读；不修改任务生命周期或创建任务。当天只保留汇总、验证、下一步和工作链接，详细说明留在工作记录。
- 无关联工作时仅写当天详情；同日更新保留旧事实并追加新进展，完全相同的内容不制造新 revision。详情达到容量上限时明确失败，由用户决定整理或拆分，不截断历史。
- 返回 `status`、`path`、`revision`、`work_status`、`daily_status` 和 `navigation_status`。`saved`/`duplicate`/`unchanged` 表示每日事实可回读；导航 `pending` 不代表事实丢失。`failed` 时看各阶段状态，已提交部分不会回滚。
- 恢复记录在 `.xiaotao/runtime/progress-saves/`；它包含日期、请求摘要及必要的工作前后快照，属于本地记忆数据，不自动共享。不要在请求未解决前删除恢复记录。重试遇到更新版本会停止，不覆盖较新进度。
- 常规导航只读当天详情与相邻层的有界摘要，更新当天、月、年和根入口。升级缺少摘要元数据的旧导航时自动完整重建一次。手工正文仍受保护；需要明确修复历史导航时才用 `timeline_catalog.py build`。底层每日协议见 [daily-memory.md](daily-memory.md)，不是日常收尾的必读清单。

## Codex：保存与回执一次完成

优先调用可信插件脚本 `progress-check.mjs --save`，不用分别运行 Python 和 `--ack saved`：

```json
{
  "cwd": "<project>",
  "session_id": "<Hook 提供的 session_id>",
  "turn_id": "<Hook 提供的 turn_id>",
  "progress": {
    "topic_id": "activity-fix",
    "title": "活动修复",
    "summary": "修正活动白名单校验",
    "validation": "相关测试通过"
  }
}
```

一次阶段收尾可省略 `request_id`，插件从 Session/Turn 生成稳定 ID；同轮多个阶段补存时显式提供不同 ID。插件只执行用户安装的 Core（默认用户级 `.agents/skills/xiaotao`），不执行项目提供的脚本。自定义安装可由用户配置 `XIAOTAO_CORE_ROOT` 和 `XIAOTAO_PYTHON`。Python 使用独立参数和 stdin，避免 shell 重定向及平台删除命令。

插件核对每日与关联工作文件的当前字节后登记回执；重复或无变化的成功保存可在本轮重新核验，不靠重写时间戳冒充新事实。后续文件变化会使回执失效。保存失败自动登记 failed 并返回非零退出码。

无新事实或用户要求不保存时仍用 `--ack` 登记 skipped；普通跳过不能消除之前的未保存状态。Hook 不自动总结聊天，也不证明模型已涵盖所有事实。
