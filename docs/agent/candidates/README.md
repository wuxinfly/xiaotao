# 待审规则候选

每个待审候选使用一个 `<candidate-id>.json` 文件。格式和审查规则见
[`../README.md`](../README.md)。

应用后的候选归档在 `applied/`，不可变决策记录在 `../decisions/`。CI 会拒绝格式错误、证据
路径不可达或文件名与 `candidate_id` 不一致的候选。
