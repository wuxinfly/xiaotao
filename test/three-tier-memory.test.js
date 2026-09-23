import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const json = async path => JSON.parse(await readFile(path, 'utf8'));

test('global preference schema verifies valid and invalid fixtures', async () => {
  const schema = await json('xiaotao/references/schemas/global-preference.schema.json');
  assert.equal(schema.title, 'XiaoTao Global User Preference');
  assert.ok(schema.properties.preference_id);
  assert.ok(schema.properties.category.enum.includes('communication'));
  assert.ok(schema.properties.category.enum.includes('coding_style'));
  assert.ok(schema.properties.confidence.enum.includes('confirmed'));

  const valid = await json(
    'xiaotao/references/scenarios/schema-fixtures/global-preference-valid.json',
  );
  assert.equal(valid.category, 'communication');
  assert.ok(valid.source_refs.length >= 1);

  const invalid = await json(
    'xiaotao/references/scenarios/schema-fixtures/global-preference-invalid.json',
  );
  assert.ok(!schema.properties.category.enum.includes(invalid.category));
});

test('timeline event schema verifies valid and invalid fixtures', async () => {
  const schema = await json('xiaotao/references/schemas/timeline-event.schema.json');
  assert.equal(schema.title, 'XiaoTao Project Memory Timeline Event');
  assert.ok(schema.properties.event_type.enum.includes('task_completed'));
  assert.ok(schema.properties.event_type.enum.includes('milestone'));

  const valid = await json(
    'xiaotao/references/scenarios/schema-fixtures/timeline-event-valid.json',
  );
  assert.equal(valid.event_type, 'task_completed');
  assert.ok(valid.source_refs.length >= 1);

  const invalid = await json(
    'xiaotao/references/scenarios/schema-fixtures/timeline-event-invalid.json',
  );
  assert.ok(!schema.properties.event_type.enum.includes(invalid.event_type));
});

test('memory reference specifies three-tier memory architecture and routing', async () => {
  const memory = await readFile('xiaotao/references/memory.md', 'utf8');

  // Architecture definition
  assert.match(memory, /长期记忆三层架构/);
  assert.match(memory, /用户目录：全局用户记忆/);
  assert.match(memory, /项目目录：项目记忆/);
  assert.match(memory, /项目目录：项目知识库/);

  // Progressive retrieval
  assert.match(memory, /逐层有界下钻/);
  assert.match(memory, /项目总览 → 年度总览 → 月度摘要 → 每日事件/);
  assert.match(memory, /项目总览 → 主题\/标签 → 当前说明 → 来源与历史/);
  assert.match(memory, /用户总览 → 偏好主题或年月 → 简要经历 → 项目记忆入口/);

  // Admission and routing
  assert.match(memory, /长期记忆入库与路由规则/);
  assert.match(memory, /经历及过程进入项目记忆/);
  assert.match(memory, /已核实的当前项目说明进入知识库/);
  assert.match(memory, /跨项目经历简述或稳定偏好进入全局用户记忆/);
  assert.match(memory, /不能仅凭一次行为推断稳定偏好/);
});

test('storage reference documents three-tier layout and schema bindings', async () => {
  const storage = await readFile('xiaotao/references/storage.md', 'utf8');

  assert.match(storage, /timeline\/\s+summary\.md/);
  assert.match(storage, /全局用户记忆存储布局/);
  assert.match(storage, /preferences\/\s+communication\.yaml/);
  assert.match(storage, /journeys\/\s+<project-slug>\.yaml/);
  assert.match(storage, /global-preference\.schema\.json/);
  assert.match(storage, /timeline-event\.schema\.json/);
});
