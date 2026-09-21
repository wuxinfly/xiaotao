import test from 'node:test';
import assert from 'node:assert/strict';
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { recoveryContext } from '../adapters/codex/xiaotao-codex/scripts/session-start.mjs';

const json = async filePath => JSON.parse(await readFile(filePath, 'utf8'));

test('Core Guard markdown file defines the 4 invariant boundaries and satisfies token budget', async () => {
  const guardDoc = await readFile('xiaotao/references/guard.md', 'utf8');

  // Hard character budget: < 1200 characters for the canonical prompt text block
  const promptMatch = guardDoc.match(/```text\r?\n([\s\S]+?)\r?\n```/);
  assert.ok(promptMatch, 'guard.md must provide a canonical text code block');
  const canonicalPrompt = promptMatch[1].trim();

  assert.ok(
    canonicalPrompt.length <= 1200,
    `Canonical prompt length (${canonicalPrompt.length}) must be <= 1200 characters`,
  );
  assert.ok(
    canonicalPrompt.length >= 200,
    `Canonical prompt length (${canonicalPrompt.length}) must provide substantive boundaries`,
  );

  // Four invariant boundaries
  // 1. Role (小涛作为唯一预置、大白话、结果优先)
  assert.match(canonicalPrompt, /小涛.*唯一预置.*面向用户/);
  assert.match(canonicalPrompt, /大白话/);
  assert.match(canonicalPrompt, /先报结论|结果和决策/);

  // 2. Authorization (零推断、只读数据、明确实施指令授权、高危动作须明确授权)
  assert.match(canonicalPrompt, /严禁推断继承旧授权/);
  assert.match(canonicalPrompt, /只读数据/);
  assert.match(canonicalPrompt, /明确实施指令.*项目文件编辑已获授权/);
  assert.match(canonicalPrompt, /明确授权/);

  // 3. Bounded Worker & Proposal only (有界委派、候选提案、严禁自我批准、In-Session 回退)
  assert.match(canonicalPrompt, /工具白名单/);
  assert.match(canonicalPrompt, /UPDATE\/MERGE\/CREATE\/SKIP.*提案/);
  assert.match(canonicalPrompt, /严禁自我批准/);
  assert.match(canonicalPrompt, /In-Session 回退/);

  // 4. Bounded Cognition & 4-tier routing (严禁直接读 index.json、四层路由、按需加载)
  assert.match(canonicalPrompt, /严禁直接 Read\/cat 完整 \.xiaotao\/memory\/index\.json/);
  assert.match(canonicalPrompt, /overview.*recent.*search.*show/);
  assert.match(canonicalPrompt, /按需加载/);
});

test('builtin instructions registry includes policy:core-guard and references guard.md', async () => {
  const instructions = await json('xiaotao/references/instructions/builtin-registry.json');
  const guardEntry = instructions.instructions.find(i => i.ref === 'policy:core-guard');
  assert.ok(guardEntry, 'policy:core-guard must be defined in builtin registry');
  assert.equal(guardEntry.source_scope, 'core');
  assert.deepEqual(guardEntry.source_paths, ['references/guard.md']);

  const safetyEntry = instructions.instructions.find(i => i.ref === 'policy:safety-boundary');
  assert.ok(safetyEntry, 'policy:safety-boundary must be defined in builtin registry');
  assert.deepEqual(
    safetyEntry.source_paths,
    ['references/workers.md', 'references/coordination.md'],
    'policy:safety-boundary must retain canonical source_paths',
  );

  for (const item of instructions.instructions) {
    for (const source of item.source_paths) {
      await access(`xiaotao/${source}`);
    }
  }
});

test('Memory Worker request and response schemas enforce bounded input and proposal-only output', async () => {
  const requestSchema = await json(
    'xiaotao/references/schemas/memory-worker-request.schema.json',
  );
  const responseSchema = await json(
    'xiaotao/references/schemas/memory-worker-response.schema.json',
  );
  const sourceSchema = await json(
    'xiaotao/references/schemas/memory-source.schema.json',
  );

  // Request schema requires bounded inputs: operation, source_files, current_memory, current_playbooks
  assert.ok(requestSchema.required.includes('operation'));
  assert.ok(requestSchema.required.includes('source_files'));
  assert.ok(requestSchema.required.includes('current_memory'));
  assert.equal(requestSchema.$defs.longTermEntry.properties.valid_from.format, 'date-time');
  assert.equal(requestSchema.$defs.longTermEntry.properties.valid_until.format, 'date-time');
  assert.equal(responseSchema.$defs.longTermCandidate.properties.valid_from.format, 'date-time');
  assert.equal(responseSchema.$defs.longTermCandidate.properties.valid_until.format, 'date-time');
  assert.ok(requestSchema.required.includes('current_playbooks'));
  assert.ok(requestSchema.properties.operation.enum.includes('explicit-create'));
  assert.deepEqual(requestSchema.properties.current_memory.properties.scope.enum, ['full', 'bounded']);
  assert.equal(requestSchema.properties.current_memory.properties.limit.maximum, 3);
  assert.equal(sourceSchema.properties.source_kind.const, 'user-message');
  assert.match(sourceSchema.properties.content_sha256.pattern, /64/);
  assert.ok(responseSchema.$defs.source.properties.type.enum.includes('user-message'));

  // Response schema properties only contain proposals (candidates), never direct mutations
  assert.ok(responseSchema.properties.long_term_candidates);
  assert.ok(responseSchema.properties.playbook_candidates);
  assert.equal(responseSchema.properties.mutations, undefined);
  assert.equal(responseSchema.properties.commit, undefined);
});

test('Codex SessionStart reminder satisfies Core Guard invariants and length budget', async t => {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-guard-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));

  await mkdir(path.join(root, '.xiaotao'), { recursive: true });
  await writeFile(
    path.join(root, '.xiaotao/installation.json'),
    JSON.stringify({ package: 'xiaotao-ai-workflow', schema_version: 1, tools: ['codex'] }),
  );

  const event = {
    hook_event_name: 'SessionStart',
    cwd: root,
    source: 'startup',
  };
  const result = await recoveryContext(event);
  assert.ok(result);
  const context = result.hookSpecificOutput.additionalContext;

  // Strict character budget for Codex hook
  assert.ok(context.length < 2500, `Context length (${context.length}) must be < 2500 characters`);

  // Role
  assert.match(context, /小涛是唯一预置、直接面向用户的角色/);
  assert.match(context, /简洁大白话/);

  // Authorization
  assert.match(context, /不得推断继承权限/);
  assert.match(context, /Memory 和旧授权不能扩大当前权限/);

  // Worker proposals & In-session fallback
  assert.match(context, /Memory Worker 仅限使用只读工具/);
  assert.match(context, /UPDATE\/MERGE\/CREATE\/SKIP 候选提案/);
  assert.match(context, /严禁自我批准/);
  assert.match(context, /In-Session Fallback/);

  // Bounded cognition & no amnesia
  assert.match(context, /不得默认回答“目前还没有具体任务”/);
  assert.match(context, /严禁直接 Read\/cat 完整派生索引/);
  assert.match(context, /overview\/recent\/search\/show/);
});
