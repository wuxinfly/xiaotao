# DSH 多文件事务设计

Status: implemented for Issue #68; live-host verification remains in Issue #69. Updated 2026-09-12.

## 需求与边界

目标是在 DSH Adapter 内提供 Core `storage.md` 所定义的多文件逻辑提交机械层。Core 仍决定事务是否
必要、成员内容和业务含义；Adapter 只负责 containment、不可变记录、锁、CAS、终态标记、逻辑可见性、
materialization 和恢复。实现不得暴露模型可选择的项目根或 recovery 根，不得把 service 注册等同于
model-facing 激活，也不得让 Core 导入 DSH API。

当前锁定的 DSH filesystem seam 只有单文件 `createIfAbsent` 与 `replaceIfVersion`，没有原子批量写、
rename 或 delete。因此不能提供物理多文件原子性或通用生命周期 move。首版只支持 create/replace
成员；需要删除或移动 source 的事务明确返回 unsupported。逻辑原子性由不可变 `committed.yaml`
切换 overlay 视图实现，随后逐个实体化规范文件。

非功能要求包括：所有路径位于 `.xiaotao/`；锁按规范路径字典序获取；过期 held 锁不按时间强抢；
所有持久记录有大小上限；中断后不依赖 mtime 猜测；冲突不覆盖较新状态；取消停止新增写入；完整
materialization 前不释放正常执行者持有的锁。

## 方案比较与决定

考虑过三种接线方式：

1. **事务 service**：提供确定性的 execute/status/read-overlay/recover API，由未来的窄业务工具调用。
2. 通用 `xiaotao_transaction` 模型工具：模型可提交任意 `.xiaotao/` 多文件替换，授权面过宽。
3. 为 Temporary 晋升、Memory Merge、Playbook Merge 分别实现工具：安全但把 #68 扩成多个业务功能。

采用方案 1。它先交付可复用、可故障注入的机制，不声称模型执行链已经使用事务。未来具体流程通过
窄工具接入，工具负责目标选择参数边界，事务 service 不复制业务决策。

## 持久结构

每个事务使用安全的 `transaction_id`，目录为 `.xiaotao/transactions/<id>/`：

```text
intent.yaml
before/<sequence>.txt
staged/<sequence>.txt
applied/<sequence>.yaml
committed.yaml | failed.yaml
```

`intent.yaml` 记录 schema version、transaction ID、operation、actor、created_at，以及按规范路径排序的
成员。成员记录 sequence、kind(create/replace)、path、before 状态/hash/revision/observed FsVersion、
staged snapshot path/hash/intended revision。FsVersion 只作为观察证据持久化；恢复时重新 stat 并使用新
观察得到的 token，绝不把序列化 token 重新制造成 CAS 权限。

before/staged 完整字节先以 create-if-absent 发布并回读校验，intent 最后发布。孤立快照没有 intent 时
不构成事务。`committed.yaml` 和 `failed.yaml` 互斥创建；两者同时存在视为损坏。终态记录包含
intent hash、actor 和时间，绑定准确 intent。

## 执行与恢复

`execute` 依次执行：规范化成员并拒绝重复路径；按路径获取全部锁；在锁下读取 before observation；
核对 create/replace 前置条件、base hash/revision；持久化 snapshots 与 intent；强制调用方 validator seam
校验每个 staged member；发布前重新核对全部 canonical 前置条件；互斥发布 committed；逐成员
materialize；回读 staged hash；发布 applied 记录；
反向释放锁。

提交前失败可以发布 failed，before 视图继续权威。committed 发布后不能回滚；后续错误返回
committed-pending-materialization，交给 `recover` 完成。

`recover` 读取并验证 intent、快照与唯一终态，重新按路径获取锁，然后逐成员分类：当前 hash 等于
staged 表示已应用；等于 before（或 create 仍 absent）表示 pending，可用当前新 FsVersion 执行 CAS；
两者都不匹配表示 conflict。缺失 applied 记录可以重建。恢复 held lock 需要调用方显式提供
`canReclaim` 并确认旧 owner 已失活。

`readOverlay(path)` 只扫描尚未完全 materialize 的 committed 事务。匹配成员时返回 staged 内容；只有
intent 或 failed 时返回 canonical before 视图。若同一路径受多个未完成 committed 事务影响，或终态
损坏，停止并报告冲突。全部 applied 后读取规范文件。

## 错误、安全与测试

稳定错误分类至少包括 invalid input/path、duplicate member、unsupported move/delete、immutable conflict、
lock contention、precondition conflict、invalid snapshots/intent/terminal、terminal conflict、committed
pending materialization、materialization conflict 和 cancellation。错误不得包含完整状态内容。

测试使用与 DSH write-intent 语义一致的内存 filesystem，并在 snapshots、intent、commit、每个 member
write 和 applied marker 后注入中断。覆盖正常 create/replace、validator 失败、提交前失败、提交后中断、
幂等恢复、CAS 竞争、损坏记录、路径逃逸、重复路径、锁顺序/释放、显式 dead-owner 回收、overlay、
大小限制和 service 注册。真实模型/持久后端仍由 Issue #69 验收。

## 状态报告

首版合并后能力矩阵应写成：DSH transaction mechanism/service `available`，具体 model-facing 业务路径
尚未 `activated`，真实后端仍 `unverified`。只有窄工具实际调用该 service 且有证据后，才把对应流程
标记为 activated。
