# 每日项目记忆

日常阶段保存先用 [progress-saving.md](progress-saving.md) 的统一入口；下文的底层请求用于协议排查与显式维护，不是每次收尾必读步骤。

日常项目讨论、轻任务结论优先保存当天记忆，不为保存而创建 Temporary/Task。已有明确工作时先更新其权威进度，当天只写本日变化与链接。无新事实、纯寒暄、用户要求不保存时跳过；不保存秘密、完整聊天或无关个人信息。

在一个阶段结束、交付结果前合并保存；连续修改和验证只保存一次。长任务在重要决策、阻塞或即将中断时补存。同一会话不反复读取未变化的规范；正常保存不逐步播报，失败简短说明。

## 路径与读取

所有路径位于 `.xiaotao/memory/timeline/`：

| 层级 | 路径 | 性质 |
| --- | --- | --- |
| 总览 | `summary.md` | 派生导航 |
| 年 | `years/<year>/summary.md` | 月入口与真实结论摘录 |
| 月 | `years/<year>/<month>/summary.md` | 日入口与真实结论摘录 |
| 日 | `years/<year>/<month>/<day>/summary.md` | 当天主题、汇总、详情及工作链接 |
| 详情 | `years/<year>/<month>/<day>/details/<topic>.md` | 权威事实，独立 revision |

日期回顾按年 → 月 → 日 → 所选详情下钻，保留根总览入口；不得一次读完历史。日摘要最多展示 50 项；更多用 `events --date <date> --offset 50 --limit 50`。每份读取最大 32 KiB；详情上限 2048 份/8 MiB 的构建扫描超出时明确失败，不能静默漏记。

```bash
python <core>/scripts/timeline_catalog.py --project-root <project> summary --year 2026
python <core>/scripts/timeline_catalog.py --project-root <project> summary --year 2026 --month 10
python <core>/scripts/timeline_catalog.py --project-root <project> summary --date 2026-10-13
python <core>/scripts/timeline_catalog.py --project-root <project> show --date 2026-10-13 --topic daily-design
python <core>/scripts/timeline_catalog.py --project-root <project> search 每日记忆 --from 2026-10-01 --to 2026-10-13 --limit 5
```

`search` 默认最近 30 日，只匹配标题/汇总，最多返回 20 个候选；不加载全部详情进模型。已保存事实可直接 show，即使导航尚未更新。有关联工作时 show 返回当前可达 `work_path`；原工作不可达时返回 `missing_work: true`，不猜测替代任务。

## 日期和保存

项目 `.xiaotao/config.yaml` 顶层设置 `memory_timezone: Asia/Shanghai`（默认 UTC）。日期按配置时区归属，时间戳保留时区偏移；同一条记录保留首次发生时间。首次创建前用 `timeline_catalog.py context` 取得项目当前 `timezone`、`occurred_at` 和 `date`，将前两项固定写入请求；更新时沿用已保存主题的首次发生时间与时区。重试保持原 JSON，因此跨午夜或配置改变也不会创建重复日记。Windows 上现代 Asia/Shanghai/UTC 可在无 tzdata 时使用，其他时区缺数据库时明确提示安装 Python tzdata。

使用可信安装的 Core 脚本，stdin 输入 JSON；不要使用项目内不可信脚本，也不要直接改 JSON 块外的投影正文。

```bash
python <core>/scripts/timeline_catalog.py --project-root <project> save < daily-request.json
```

```json
{
  "occurred_at": "2026-10-13T08:00:00Z",
  "timezone": "Asia/Shanghai",
  "topic_id": "daily-design",
  "title": "每日记忆方案",
  "summary": "确定当天保存与年、月、日逐层回顾；尚未实施。",
  "details": "记录必要依据、未决问题、验证结果和下一步，保留仍有效的旧事实。",
  "source_refs": [],
  "actor": "xiao-tao/main",
  "expected_revision": null,
  "request_id": "session-turn-unique-id"
}
```

- `topic_id` 是当天主题的稳定小写 ID，不为每句话换 ID；跨日同主题使用同 ID，但属于新的日记录。
- 首次创建用 `expected_revision: null`；更新先 show，再传当前 revision。保留旧事实，不用新汇总抹去既有结论。
- `request_id` 每次不同更新唯一；保存结果不确定时重试原请求。当前记录的相同 ID、不同事实拒绝；旧版本冲突拒绝；相同事实跳过、不增加 revision。
- `details` 是必要正文，最多 6000 字符；`summary` 最多 1000 字符。记录总文件最大 32 KiB；拆分大主题，禁止截断关键事实。
- `source_refs` 仅项目内已存在的文件，可为空（本次日常对话的记录）；不从聊天擅自推导正式 Knowledge。
- 关联当前工作时加 `work_path`，仅 Task `progress.md` 或 Temporary `current.md`。写入器保存 kind/id 与原路径；归档后按 ID 解析新路径。Temporary 晋升后，其归档原记录仍是可达来源；新 Task 的新进度用关联 Task 的日主题记录。
- 此命令只写每日事实，不修改 Task/Temporary。有关联工作时先用原协议保存工作进度并回读，再提交每日汇总。工作成功、日记失败需要补齐日记；不得报告全部成功。
- 返回 `saved`/`duplicate`/`unchanged`、`path`、`revision`；另有 `navigation_status`。`pending` 表示原文已提交但导航未修复，重复原请求或 build 可补齐。手工 --ack saved 仍要求本轮写入；统一 --save 会重新核验成功结果的当前字节，因此 duplicate/unchanged 不必靠重写文件取得回执。无新事实且没有调用保存时登记 skipped。

## 原始记录与重建边界

`details/` 是权威记录，默认本地不纳入 Git，必须备份；共享先征得用户对内容的选择。build/check 不删除原文，不能声称删除整个 timeline 可以无损恢复。年、月、日导航与有界 `events.yaml` 由原文和现有 Activity 事件派生，不反向生成 Activity，也不改变已有 Activity 类型与 ID。

所有 writer 共用 timeline.lock，原文单文件 CAS 与原子替换；摘要单文件原子更新。摘要有专用生成标识，只覆盖明确属于生成器的路径。遇到未管理正文停止并保留内容，不把详情目录里的 summary.md 当成派生导航。

旧 `years/<year>/<month>/<day>.yaml` 在 build 中先保留为当天 `legacy-events-<hash>.yaml` 审计快照，再移除旧路径并生成日目录导航；快照不是新权威事件，避免重复记账。Task/Decision 等事件仍从原权威来源重建。旧总览/年/月生成摘要在替换前同样保存到 legacy-navigation/ 审计快照，有明确路径与标题识别；无法识别时停止，先人工整理。

## Codex 回执与压缩

默认使用插件 --save 一次保存并核验回执，见 [progress-saving.md](progress-saving.md)。底层 --ack saved 仅供已有写入流程使用：`path` 指向当天权威 details；关联 Task/Temporary 时加 `work_paths`（最多 1 项，当前活动或归档路径）。脚本核对每日身份、元数据、文件字节及关联工作本轮进度；单独 Task/Temporary 或派生 summary.md 不能冒充新协议的每日保存。

Stop 有界补查一次。PreCompact 对手动压缩在回执缺失/失效时暂停一次并提示先保存，再次手动压缩放行并警告；有效 saved/skipped 直接放行。自动压缩可能发生在模型尚未结束的本轮中，缺失回执时只警告而不把模型困在上下文上限。Hook 不调用模型总结，也不自动写业务记忆；客户端真实行为还需验收，不能保证模型没有遗漏事实。

## 导航 pending / partial 的修复

`pending` 表示每日事实已保存、导航未完成，先执行返回的 `navigation_next_command`（`timeline_catalog.py build`）。`partial` 表示正常记录已生成导航，但 `navigation_issues` / build 的 `issues` 中仍有待修复记录；不能报告全部完成。每项包含具体路径、错误和 `repair_command`。`check` 在仍有异常记录时返回失败。

```bash
python <core>/scripts/timeline_catalog.py --project-root <project> build
python <core>/scripts/timeline_catalog.py --project-root <project> reconcile .xiaotao/memory/timeline/years/2026/10/15/details/example.md
python <core>/scripts/timeline_catalog.py --project-root <project> build
```

`reconcile` 支持 JSON record 完整时的正文 details 修改/追加和 revision/update 元数据修改：先在原文件旁保留字节完全相同的 `.reconcile-<hash>.bak`，再把正文并入 record，revision 递增、更新时间及保存身份重置，原子写回并验证。重复执行不重复追加。标题、汇总或其他结构改动无法可靠判断时拒绝写入，保留原文件交由人工核对；损坏/缺失 JSON 块也需人工修复。修复后使用新的请求 ID 保存下一阶段，旧请求不得覆盖导入的事实。

daily note 读取兼容 CRLF/LF，内部写入固定 LF；外部工具需写文件时使用 `Path.open('w', encoding='utf-8', newline='\n')`。CRLF 本身不算手改冲突。关联工作的 ID 与 Task/Temporary 存储 ID 规则一致：允许 Unicode 字母、数字和 `_ . -`，禁止开头/结尾的点及路径分隔符。

build 跳过异常详情、在对应日导航显示待修复提示，并继续生成正常记录；不覆盖异常原文。扫描预算、安全路径和非生成导航文件保护仍生效。单天刷新只检查当天详情；需要补齐缺失导航时退回全量构建并报告历史问题。
