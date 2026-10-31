# 外部工作文档导入（#104）

仅在用户显式选择文档后运行。先预检是否含秘密、隐私或不宜提交版本库的材料；受管副本默认进入版本管理，不自动扫描目录。由负责外部记忆整合的 Worker 读取副本、现有 Activity 与 Memory，整理事件提案，用户确认后才发布权威记录。

```sh
python xiaotao/scripts/external_memory.py import /path/to/work.md --project-root .
python xiaotao/scripts/external_memory.py list --project-root .
python xiaotao/scripts/external_memory.py confirm import-<hash> proposal.json --project-root .
python xiaotao/scripts/activity_catalog.py build --project-root .
python xiaotao/scripts/timeline_catalog.py build --project-root .
```

`proposal.json` 格式：`{"events":[{"occurred_at":"2026-01-02T10:00:00+08:00","title":"完成原型","summary":"完成交付和评审","source_refs":[".xiaotao/memory/imports/YYYY/MM/import-.../sources/work.md"]}]}`。日期必须是可证实的发生时间，带时区；导入时间不能替代。发生日期、项目归属或冲突不明时暂停确认，向用户核实。若无事件，用 `skip import-<hash> --reason '原因'` 留存审计；不要编造 Task 或 Decision。

同一内容哈希再次导入返回原导入 ID；不同版本保留不同副本。多个来源描述同一事件时先核对已有 Activity 和已确认导入，人工选择合并来源、更新已有事件或跳过；一旦确认的记录不可覆盖，如需更正应走人工审计流程。复制后原文件不会被修改；`source_refs` 仅引用项目内副本。确认的 manifest 是权威记录，Activity 索引和 Timeline 均可重建；可用 Activity `search --event-type external_work` 检索。当前仍有效的长期知识交现有 Memory Worker 作为候选，按独立评审及 Decision Record 执行，不直接写入 knowledge/entries。

## 导入目录

统一使用 `.xiaotao/memory/imports/YYYY/MM/import-<hash>/`，年月取自 `imported_at` 的 UTC 时间，不使用事件发生日期。目录内保留 `manifest.json` 与 `sources/`；导入 ID 不变，按年月扫描进行全局去重与 ID 定位。只读取新结构，不兼容旧版平铺目录。

## 旧数据一次性整理提示词

将本项目旧版平铺的 `.xiaotao/memory/imports/import-*/` 整理为最新的 `imports/YYYY/MM/import-*/` 结构。先备份并列出移动计划；年月按 manifest 的 imported_at 转为 UTC 后确定，缺失或无效时暂停该条并报告，不用文件修改时间猜测。保持导入 ID、状态、事件内容和 sources 原始字节不变。目标冲突时停止该条，不覆盖。更新 manifest 内 sources.ref、events.source_refs 以及项目内其他指向旧位置的文本引用；不要改动 sources 原文。已确认事件的 event_id 包含来源路径，必须按最新 external_memory.py / activity_catalog.py 中的算法重新计算。此次明确授权调整已确认记录的路径及派生 ID，其余字段保持不变。随后重建 Activity 和 Timeline 等派生索引，校验数量、来源哈希、引用可用性与事件内容一致，报告未完成项。全部验证通过后再清除备份。
