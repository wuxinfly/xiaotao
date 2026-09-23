import test from 'node:test';
import assert from 'node:assert/strict';
import { access, readFile } from 'node:fs/promises';

const json = async path => JSON.parse(await readFile(path, 'utf8'));

test('new work has no built-in execution Workers and uses capability practices', async () => {
  const builtinWorkers = await json('xiaotao/references/workers/builtin-registry.json');
  const instructions = await json('xiaotao/references/instructions/builtin-registry.json');
  const projectWorkers = await json(
    'xiaotao/references/scenarios/schema-fixtures/worker-registry-valid.json',
  );

  assert.deepEqual(builtinWorkers.workers, []);
  assert.deepEqual(builtinWorkers.aliases, {});

  const knownRefs = new Map(instructions.instructions.map(item => [item.ref, item]));
  assert.ok([...knownRefs.keys()].some(ref => ref.startsWith('practice:')));
  assert.ok([...knownRefs.keys()].every(ref => !ref.startsWith('role:')));

  for (const worker of projectWorkers.workers) {
    assert.ok(worker.instructions.required.includes('contract:handoff'));
    assert.ok(worker.instructions.required.includes('policy:safety-boundary'));
    assert.ok(worker.instructions.required.some(ref => ref.startsWith('practice:')));
    assert.ok(worker.instructions.required.every(ref => !ref.startsWith('role:')));
  }

  for (const instruction of instructions.instructions) {
    for (const source of instruction.source_paths) {
      await access(`xiaotao/${source}`);
    }
  }
});

test('generated Worker examples use task-specific Chinese display names', async () => {
  for (const fixture of [
    'worker-temporary-valid.json',
    'worker-temporary-memory-valid.json',
    'worker-session-valid.json',
  ]) {
    const worker = await json(`xiaotao/references/scenarios/schema-fixtures/${fixture}`);
    assert.match(worker.name, /[\p{Script=Han}]/u);
    assert.match(worker.id, /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/);
    assert.ok(worker.instructions.required.some(ref => ref.startsWith('practice:')));
  }
});

test('XiaoTao entry point exposes only Xiao Tao and ships no fixed role files', async () => {
  const skill = await readFile('xiaotao/SKILL.md', 'utf8');
  assert.doesNotMatch(skill, /\]\(references\/roles\//);
  assert.match(skill, /小涛（Xiao Tao）.*唯一预置、直接面向用户的角色/);
  await assert.rejects(access('xiaotao/references/roles'), { code: 'ENOENT' });
});

test('active Core guidance is Chinese and has no legacy Role compatibility contract', async () => {
  for (const file of [
    'xiaotao/SKILL.md',
    'xiaotao/references/contract.md',
    'xiaotao/references/coordination.md',
    'xiaotao/references/guard.md',
    'xiaotao/references/handoffs.md',
    'xiaotao/references/memory.md',
    'xiaotao/references/playbooks.md',
    'xiaotao/references/storage.md',
    'xiaotao/references/workers.md',
  ]) {
    assert.match(await readFile(file, 'utf8'), /[\p{Script=Han}]/u, `${file} must contain Chinese guidance`);
  }

  const combined = await Promise.all([
    readFile('xiaotao/SKILL.md', 'utf8'),
    readFile('xiaotao/references/workers.md', 'utf8'),
    readFile('xiaotao/references/storage.md', 'utf8'),
  ]);
  assert.doesNotMatch(combined.join('\n'), /role_state_path|role:\*|roles\/<|历史角色快照/);
});

test('coordination defines light task fast-path and rules for upgrading to persistent task', async () => {
  const coordination = await readFile('xiaotao/references/coordination.md', 'utf8');
  const skill = await readFile('xiaotao/SKILL.md', 'utf8');

  // Coordination defines light task fast-path
  assert.match(coordination, /轻任务直通（默认路径）/);
  assert.match(coordination, /不创建 Temporary、Task、Worker 选择记录、快照或持久 Handoff/);
  assert.match(coordination, /升级为持久 Task 的触发条件/);
  assert.match(coordination, /中途变复杂的平滑升级协议/);

  // Skill defines fast-path in intro and core constraints
  assert.match(skill, /单会话内明确的小改动默认直通执行/);
  assert.match(skill, /单会话小改动默认轻任务直通，不创建 Task、快照或持久 Handoff/);
});

