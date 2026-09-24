# 外部工作文档导入（#104）

仅在用户显式选择文档后运行。先预检是否含秘密、隐私或不宜提交版本库的材料；受管副本默认进入版本管理，不自动扫描目录。由负责外部记忆整合的 Worker 读取副本、现有 Activity 与 Memory，整理事件提案，用户确认后才发布权威记录。

```sh
python xiaotao/scripts/external_memory.py import /path/to/work.md --project-root .
python xiaotao/scripts/external_memory.py list --project-root .
python xiaotao/scripts/external_memory.py confirm import-<hash> proposal.json --project-root .
python xiaotao/scripts/activity_catalog.py build --project-root .
python xiaotao/scripts/timeline_catalog.py build --project-root .
```

`proposal.json` 格式：`{"events":[{"occurred_at":"2026-01-02T10:00:00+08:00","title":"完成原型","summary":"完成交付和评审","source_refs":[".xiaotao/memory/imports/import-.../sources/work.md"]}]}`。日期必须是可证实的发生时间，带时区；导入时间不能替代。发生日期、项目归属或冲突不明时暂停确认，向用户核实。若无事件，用 `skip import-<hash> --reason '原因'` 留存审计；不要编造 Task 或 Decision。

同一内容哈希再次导入返回原导入 ID；不同版本保留不同副本。多个来源描述同一事件时先核对已有 Activity 和已确认导入，人工选择合并来源、更新已有事件或跳过；一旦确认的记录不可覆盖，如需更正应走人工审计流程。复制后原文件不会被修改；`source_refs` 仅引用项目内副本。确认的 manifest 是权威记录，Activity 索引和 Timeline 均可重建；可用 Activity `search --event-type external_work` 检索。当前仍有效的长期知识交现有 Memory Worker 作为候选，按独立评审及 Decision Record 执行，不直接写入 long-term/entries。
