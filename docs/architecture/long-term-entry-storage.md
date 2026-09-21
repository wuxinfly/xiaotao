# 长期记忆单文件存储

状态：Issue #51 首版实现。旧聚合格式可读，新写入使用单文件；迁移必须由用户显式执行。

## 权威路径

| 状态 | 路径 | 规则 |
| --- | --- | --- |
| active / disputed | `.xiaotao/memory/long-term/entries/<entry-id>.md` | 一个文件一个 entry，文件名等于 entry ID |
| superseded / rejected | `.xiaotao/memory/long-term/history/<entry-id>.md` | 保留失效 snapshot，常规搜索不可见 |
| 旧格式 | `.xiaotao/memory/long-term/current.md` | 多 entry 聚合仍可读，不再写入新 entry |
| 新格式说明 | `.xiaotao/memory/long-term/current.md` | 固定指引，不保存 entry 内容 |

每个 entry 文件包含独立 `revision`、`updated_at`、`updated_by` front matter 和且仅一个
`xiaotao-memory-entry` JSON block。Catalog 同时扫描三类来源；同一 ID 重复时失败。

## 写入边界

- CREATE 创建一个 revision 0 entry 文件。
- UPDATE 只锁定并替换目标文件，revision 加一。
- MERGE 更新 survivor，并把其他目标移入 history；所有路径通过同一个多文件 transaction 发布。
- SKIP 只写不可变 decision。
- `current.md` 不参与普通 entry 更新，因此不同 entry 的 Git 修改不会争用同一文件。

## 显式迁移

`memory_catalog.py migrate-long-term` 默认只做完整预检。只有提供 `--apply --actor` 才会写入。
迁移取得全局 migration lock，在隐藏 staging 中生成并校验全部目标文件，保存旧聚合原文和 intent，
发布后再次比较所有 entry。验证失败会恢复旧聚合且不报告成功。普通 build/search/show 永不触发迁移。
若旧聚合与不同 ID 的新单文件共存，迁移只补入旧 entry，已有单文件的内容和 revision 保持不变；
任一 ID 冲突都会在写入前失败。

迁移是旧存储格式的一次性维护操作，不代替正常 UPDATE/MERGE 的 lock、CAS 和 transaction 协议。
