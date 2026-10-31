import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const json = async path => JSON.parse(await readFile(path, 'utf8'));

test('task schema supports decoupled memory_pending and memory_reviewed_at fields', async () => {
  const schema = await json('xiaotao/references/schemas/task.schema.json');
  assert.equal(schema.properties.memory_pending.type, 'boolean');
  assert.equal(schema.properties.memory_reviewed_at.type, 'string');
  assert.equal(schema.properties.memory_reviewed_at.format, 'date-time');

  // Verify fixtures exist and conform
  const validPending = await json(
    'xiaotao/references/scenarios/schema-fixtures/task-completed-pending-valid.json',
  );
  assert.equal(validPending.memory_pending, true);

  const validReviewed = await json(
    'xiaotao/references/scenarios/schema-fixtures/task-completed-reviewed-valid.json',
  );
  assert.equal(validReviewed.memory_pending, false);
  assert.ok(validReviewed.memory_reviewed_at);

  const invalidPending = await json(
    'xiaotao/references/scenarios/schema-fixtures/task-memory-pending-invalid.json',
  );
  assert.notEqual(typeof invalidPending.memory_pending, 'boolean');
});

test('coordination defines decoupled task completion delivery and on-demand memory review', async () => {
  const coordination = await readFile('xiaotao/references/coordination.md', 'utf8');

  // Decoupled delivery
  assert.match(coordination, /完成与收尾解耦/);
  assert.match(coordination, /任务事实落盘交付.*长期记忆提炼审查.*解耦流程/);
  assert.match(coordination, /写入并校验 `completion\.md`/);
  assert.match(coordination, /memory_pending: true/);
  assert.match(coordination, /\.xiaotao\/memory\/pending\/tasks\/<task-id>\.json/);
  assert.match(coordination, /即时向用户交付/);
  assert.match(coordination, /不等待 Memory Worker 审查/);

  // On-demand review
  assert.match(coordination, /长期记忆与经验按需审查/);
  assert.match(coordination, /会话启动时不扫描全部已归档 Task/);
  assert.match(coordination, /清除待审状态/);
  assert.match(coordination, /memory_pending: false/);
  assert.match(coordination, /memory_reviewed_at/);
  assert.match(coordination, /轻量与幂等保证/);
  assert.match(coordination, /先在归档 Task 的 `task\.yaml` 中更新 `memory_pending: false`[\s\S]*最后移除/);
  assert.match(coordination, /`memory_pending: false` 仅核验已提交记录并删除指针，不得再次执行写入/);
  assert.match(coordination, /同一 Task 的审查获取 Task 级锁/);
  assert.match(coordination, /持久化的候选 ID 固定每项 Decision Record ID 与目标 entry\/playbook ID/);
});

test('storage and memory references define pending task pool and review protocol', async () => {
  const storage = await readFile('xiaotao/references/storage.md', 'utf8');
  const memory = await readFile('xiaotao/references/memory.md', 'utf8');

  // Storage protocol
  assert.match(storage, /memory\/pending\/`：尚未压缩或解析的 Memory Worker 原始输入与待审任务工作池/);
  assert.match(storage, /正式 Task 完成归档时，将业务事实持久化到 `completion\.md`/);
  assert.match(storage, /先独占创建[\s\S]*再在 `task\.yaml` 中标记 `memory_pending: true`/);
  assert.match(storage, /严禁启动或维护时对 `tasks\/archive\/` 进行全库遍历/);
  assert.match(storage, /先从归档 `task\.yaml` 中更新 `memory_pending: false` 并记录 `memory_reviewed_at`[\s\S]*最后清理/);

  // Memory review protocol
  assert.match(memory, /### 待审任务的长期记忆审查（Pending Task Review）/);
  assert.match(memory, /业务 Task 完成后已同步落盘 `completion\.md` 并标记 `memory_pending: true`/);
  assert.match(memory, /会话启动时绝不遍历扫描 `tasks\/archive\/` 全库/);
  assert.match(memory, /以 Task ID、归档后的 `completion\.md` 路径及持久化的 `candidate_id` 固定每项 `decision_id`/);
  assert.match(memory, /已有完整结果直接跳过，部分提交按可变状态事务协议恢复/);
  assert.match(memory, /用户明确说“记住这条”时继续走下文的即时轻量路径/);
});
