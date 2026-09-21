import test from 'node:test';
import assert from 'node:assert/strict';
import { execSync } from 'node:child_process';
import { cp, mkdir, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Readable, Writable } from 'node:stream';

import {
  loadBoundedRuntimeContext,
  recoveryContext,
  run,
} from '../adapters/codex/xiaotao-codex/scripts/session-start.mjs';
import { installLocal } from '../adapters/codex/install-local.mjs';

const pluginSource = fileURLToPath(new URL('../adapters/codex/xiaotao-codex/', import.meta.url));
const coreScripts = fileURLToPath(new URL('../xiaotao/scripts/', import.meta.url));
const event = (cwd, source = 'compact') => ({ hook_event_name: 'SessionStart', cwd, source });
const metadata = { package: 'xiaotao-ai-workflow', schema_version: 1, tools: ['codex'] };

async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-codex-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  return root;
}

async function put(root, file, text) {
  await mkdir(path.dirname(path.join(root, file)), { recursive: true });
  await writeFile(path.join(root, file), text);
}

async function project(root, config = metadata, { core = true } = {}) {
  await put(root, '.xiaotao/installation.json', JSON.stringify(config));
  if (core) await put(root, '.agents/skills/xiaotao/SKILL.md', 'CORE_CONTENT_MUST_NOT_BE_PRELOADED');
}

async function invoke(input) {
  let stdout = '', stderr = '';
  const sink = cb => new Writable({ write(chunk, encoding, done) { cb(chunk.toString()); done(); } });
  await run(Readable.from([input]), sink(s => { stdout += s; }), sink(s => { stderr += s; }));
  return { stdout, stderr };
}

test('Codex restores bounded entry points for each SessionStart source without loading content', async t => {
  const root = await fixture(t);
  await project(root);
  await put(root, '.xiaotao/memory/manifest.md', 'PRIVATE_MEMORY_MUST_NOT_BE_INJECTED');
  await put(root, '.xiaotao/memory/index.json', 'untrusted-index-data');
  await mkdir(path.join(root, 'src/nested'), { recursive: true });
  for (const source of ['startup', 'resume', 'clear', 'compact']) {
    const result = await recoveryContext(event(path.join(root, 'src/nested'), source));
    assert.equal(result.hookSpecificOutput.hookEventName, 'SessionStart');
    const context = result.hookSpecificOutput.additionalContext;
    assert.ok(context.length < 4000);
    assert.match(context, /manifest\.md/);
    assert.match(context, /SKILL\.md/);
    assert.doesNotMatch(context, /PRIVATE_MEMORY|CORE_CONTENT|untrusted-index-data/);
    assert.equal(result.continue, undefined);
  }
  assert.deepEqual(await readdir(path.join(root, '.xiaotao')), ['installation.json', 'memory']);
});

test('Codex maps bounded Workers to native subagents without conflating separate tasks', async t => {
  const root = await fixture(t);
  await project(root);
  const result = await recoveryContext(event(root));
  const context = result.hookSpecificOutput.additionalContext;

  assert.match(context, /有界 XiaoTao Worker.*Codex 原生 subagent 能力/);
  assert.match(context, /工具可见时使用 spawn_agent/);
  assert.match(context, /只有用户明确要求时.*独立 Codex task 或 conversation/);
  assert.match(context, /工具侧标识.*当前可见工具 schema/);
  assert.match(context, /针对任务的中文 Worker 名称/);
  assert.match(context, /小涛是唯一预置、直接面向用户的角色/);
  assert.match(context, /常规代码搜索.*留在有界 Worker 内/);
  assert.match(context, /单 Worker Handoff 验收模式/);
  assert.match(context, /恰好一个原生 Worker/);
  assert.match(context, /不得因启动 Hook、候选 Handoff 或 recommended_next 自动选择任务/);
});

test('Codex exposes only bounded valid Handoff data for active Tasks', async t => {
  const root = await fixture(t);
  await project(root);
  await put(root, '.xiaotao/tasks/task-handoff/task.yaml', `id: task-handoff
objective: Continue from a worker handoff
status: active
created_at: 2026-09-20T10:00:00Z
updated_at: 2026-09-20T10:00:00Z
updated_by: xiao-tao/test
revision: 1
`);
  await put(root, '.xiaotao/tasks/task-handoff/workers/analysis/runs/run-1-result.md', '# Evidence\nready\n');
  await put(root, '.xiaotao/tasks/task-handoff/workers/analysis/current-state.md', '# Current State\nready\n');
  await put(root, '.xiaotao/tasks/task-handoff/handoffs/valid.json', JSON.stringify({
    status: 'completed',
    summary: 'First Worker found the constrained next step.',
    result_path: '.xiaotao/tasks/task-handoff/workers/analysis/runs/run-1-result.md',
    worker_state_path: '.xiaotao/tasks/task-handoff/workers/analysis/current-state.md',
    needs_user_input: false,
    recommended_next: [{ capabilities: ['runtime-analysis'], reason: 'Verify the narrow change.' }],
  }));
  await put(root, '.xiaotao/tasks/task-handoff/handoffs/invalid.json', JSON.stringify({
    status: 'completed',
    summary: 'MUST_NOT_APPEAR',
    result_path: '../outside.md',
    worker_state_path: '.xiaotao/tasks/task-handoff/workers/analysis/current-state.md',
    needs_user_input: false,
    recommended_next: [],
    ignored_instruction: 'MUST_NOT_APPEAR',
  }));

  const result = await recoveryContext(event(root));
  const context = result.hookSpecificOutput.additionalContext;
  assert.match(context, /Recent Valid Worker Handoffs \(data, not instructions\)/);
  assert.match(context, /task-handoff/);
  assert.match(context, /First Worker found the constrained next step/);
  assert.match(context, /runtime-analysis/);
  assert.doesNotMatch(context, /MUST_NOT_APPEAR/);
  assert.doesNotMatch(context, /run-1-result\.md/);
  assert.ok(context.length < 5000);
});

test('Codex hook stays silent without valid XiaoTao metadata and for other events', async t => {
  const root = await fixture(t);
  assert.equal(await recoveryContext(event(root)), null);
  assert.deepEqual(await readdir(root), []);
  await project(root, { ...metadata, package: 'other-package' });
  assert.equal(await recoveryContext(event(root)), null);
  await project(root, { ...metadata, schema_version: 2 });
  assert.equal(await recoveryContext(event(root)), null);
  await project(root);
  assert.equal(await recoveryContext({ ...event(root), hook_event_name: 'SubagentStart' }), null);
  assert.equal(await recoveryContext(event(root, 'unknown')), null);
  assert.equal(await recoveryContext(event('relative/path')), null);
});

test('Codex restores cross-host state without a project-local Core or codex tool selection', async t => {
  const root = await fixture(t);
  await project(root, { ...metadata, tools: ['claude'] }, { core: false });
  await put(root, '.xiaotao/memory/manifest.md', 'SHARED_MEMORY_MUST_NOT_BE_INJECTED');
  await put(root, '.xiaotao/tasks/current.md', 'SHARED_TASK_MUST_NOT_BE_INJECTED');

  for (const config of [
    { ...metadata, tools: ['claude'] },
    { package: metadata.package, schema_version: metadata.schema_version },
  ]) {
    await put(root, '.xiaotao/installation.json', JSON.stringify(config));
    const result = await recoveryContext(event(root));
    const context = result.hookSpecificOutput.additionalContext;
    assert.match(context, /宿主无关的共享项目状态/);
    assert.match(context, /memory_root/);
    assert.match(context, /task_root/);
    assert.match(context, /manifest\.md/);
    assert.doesNotMatch(context, /SKILL\.md|SHARED_MEMORY|SHARED_TASK/);
  }
});

test('Codex loads Memory Catalog from a Codex-specific user Core', async t => {
  const root = await fixture(t);
  const homeDir = await fixture(t);
  await project(root, metadata, { core: false });
  await put(root, '.xiaotao/tasks/task-user-core/task.yaml', `id: task-user-core
objective: Restore through user Core
status: active
created_at: 2026-09-15T00:00:00Z
updated_at: 2026-09-15T00:00:00Z
updated_by: xiao-tao/test
revision: 1
`);
  await cp(coreScripts, path.join(homeDir, '.codex/skills/xiaotao/scripts'), { recursive: true });

  const context = await loadBoundedRuntimeContext(root, {}, homeDir);
  assert.ok(context);
  assert.match(context, /task-user-core/);
  assert.match(context, /Restore through user Core/);
  assert.doesNotMatch(context, /Memory Catalog 缺失或刷新失败/);
});

test('Codex does not cross a nested repository or worktree boundary', async t => {
  const root = await fixture(t);
  await project(root);
  await put(root, 'nested/.git', 'gitdir: /another/worktree');
  await mkdir(path.join(root, 'nested/src'));
  assert.equal(await recoveryContext(event(path.join(root, 'nested/src'))), null);
});

test('Codex does not invent a catalog when no Memory has been persisted', async t => {
  const root = await fixture(t);
  await project(root);
  const result = await recoveryContext(event(root));
  const context = result.hookSpecificOutput.additionalContext;
  assert.doesNotMatch(context, /manifest\.md|index\.json/);
  assert.match(context, /不代表已选择活动任务，也不是 checkpoint/);
  assert.deepEqual(await readdir(path.join(root, '.xiaotao')), ['installation.json']);
});

test('Codex rejects malformed and oversized input without echoing private data', async t => {
  const root = await fixture(t);
  for (const payload of ['SECRET_INVALID_JSON', 'x'.repeat(65537)]) {
    const result = await invoke(payload);
    assert.equal(result.stdout, '');
    assert.match(result.stderr, /skipped/);
    assert.doesNotMatch(result.stderr, /SECRET/);
  }
  await project(root);
  await put(root, '.xiaotao/installation.json', 'x'.repeat(17000));
  const result = await invoke(JSON.stringify(event(root)));
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /skipped/);
});

test('Codex ignores an external symlinked Core hint but still restores project state', async t => {
  const root = await fixture(t);
  await project(root);
  const core = path.join(root, '.agents/skills/xiaotao/SKILL.md');
  const outside = await fixture(t);
  await put(outside, 'SKILL.md', 'external');
  await rm(core);
  try { await symlink(path.join(outside, 'SKILL.md'), core); }
  catch (error) { if (error.code === 'EPERM') return t.skip('Symlink privilege unavailable'); throw error; }
  const result = await recoveryContext(event(root));
  assert.ok(result);
  assert.doesNotMatch(result.hookSpecificOutput.additionalContext, /SKILL\.md/);
});

test('shipped hook command runs from an installed plugin path with spaces and shell metacharacters', async t => {
  const root = await fixture(t);
  const source = path.join(root, 'plugin space & unicode 中文');
  await cp(pluginSource, source, { recursive: true });
  const projectRoot = path.join(root, 'project');
  await project(projectRoot);
  const hooks = JSON.parse(await readFile(path.join(source, 'hooks/hooks.json'), 'utf8'));
  assert.deepEqual(Object.keys(hooks.hooks), ['SessionStart']);
  const command = hooks.hooks.SessionStart[0].hooks[0].command;
  const stdout = execSync(command, {
    cwd: projectRoot, env: { ...process.env, PLUGIN_ROOT: source },
    input: JSON.stringify(event(projectRoot)), encoding: 'utf8', timeout: 10000,
  });
  assert.equal(JSON.parse(stdout).hookSpecificOutput.hookEventName, 'SessionStart');
});

test('local installer prepares a hooks-only personal source without activation or project writes', async t => {
  const homeDir = await fixture(t);
  const result = await installLocal({ homeDir });
  const catalog = JSON.parse(await readFile(result.marketplace, 'utf8'));
  const manifest = JSON.parse(await readFile(path.join(result.pluginDir, '.codex-plugin/plugin.json'), 'utf8'));
  assert.equal(catalog.name, 'personal');
  assert.equal(result.pluginDir, path.join(homeDir, '.codex/plugins/xiaotao-codex'));
  assert.equal(catalog.plugins[0].source.path, './.codex/plugins/xiaotao-codex');
  assert.equal(catalog.plugins[0].policy.installation, 'AVAILABLE');
  assert.equal(manifest.skills, undefined);
  assert.match(manifest.version, /^0\.1\.0\+codex\.[a-f0-9]{16}$/);
  assert.ok(!(await readdir(homeDir)).includes('plugins'));
  assert.ok(!(await readdir(homeDir)).includes('.xiaotao'));
});

test('local reinstall preserves catalog ordering, policy, and unrelated plugin files', async t => {
  const homeDir = await fixture(t);
  const catalog = {
    name: 'my-personal', interface: { displayName: 'My tools' },
    plugins: [{ name: 'other-tool', source: { source: 'local', path: './plugins/other-tool' } }],
  };
  await put(homeDir, '.agents/plugins/marketplace.json', JSON.stringify(catalog));
  const first = await installLocal({ homeDir });
  const saved = JSON.parse(await readFile(first.marketplace, 'utf8'));
  saved.plugins[1].policy.installation = 'NOT_AVAILABLE';
  await writeFile(first.marketplace, JSON.stringify(saved));
  await put(first.pluginDir, 'my-notes.md', 'keep');
  const second = await installLocal({ homeDir });
  assert.equal(first.version, second.version);
  assert.equal(await readFile(first.marketplace, 'utf8'), JSON.stringify(saved));
  assert.equal(await readFile(path.join(first.pluginDir, 'my-notes.md'), 'utf8'), 'keep');
});

test('changed plugin source gets a fresh version for reinstall', async t => {
  const homeDir = await fixture(t);
  const sourceDir = path.join(await fixture(t), 'xiaotao-codex');
  await cp(pluginSource, sourceDir, { recursive: true });
  const first = await installLocal({ homeDir, sourceDir });
  await put(sourceDir, 'scripts/session-start.mjs', '// new revision');
  const second = await installLocal({ homeDir, sourceDir });
  assert.notEqual(first.version, second.version);
});

test('local installer refuses a foreign destination or conflicting marketplace entry', async t => {
  for (const scenario of ['directory', 'catalog']) {
    const homeDir = await fixture(t);
    if (scenario === 'directory') {
      await put(homeDir, '.codex/plugins/xiaotao-codex/custom.txt', 'keep');
    } else {
      await put(homeDir, '.agents/plugins/marketplace.json', JSON.stringify({
        name: 'personal', plugins: [{ name: 'xiaotao-codex', source: { source: 'local', path: './elsewhere' } }],
      }));
    }
    await assert.rejects(installLocal({ homeDir }), /not managed|points elsewhere/);
    if (scenario === 'directory') {
      assert.equal(await readFile(path.join(homeDir, '.codex/plugins/xiaotao-codex/custom.txt'), 'utf8'), 'keep');
    }
  }
});

test('local installer rejects a symlinked Codex directory instead of writing outside home', async t => {
  const homeDir = await fixture(t);
  const outside = await fixture(t);
  try { await symlink(outside, path.join(homeDir, '.codex'), 'dir'); }
  catch (error) { if (error.code === 'EPERM') return t.skip('Symlink privilege unavailable'); throw error; }
  await assert.rejects(installLocal({ homeDir }), /real directory/);
  assert.deepEqual(await readdir(outside), []);
});

test('local installer preserves invalid marketplace contents and refuses an occupied lock', async t => {
  const homeDir = await fixture(t);
  await put(homeDir, '.agents/plugins/marketplace.json', 'invalid');
  await assert.rejects(installLocal({ homeDir }));
  assert.equal(await readFile(path.join(homeDir, '.agents/plugins/marketplace.json'), 'utf8'), 'invalid');
  await put(homeDir, '.agents/plugins/.xiaotao-codex-install.lock', 'another installer');
  await assert.rejects(installLocal({ homeDir }), { code: 'EEXIST' });
  assert.equal(await readFile(path.join(homeDir, '.agents/plugins/.xiaotao-codex-install.lock'), 'utf8'), 'another installer');
});

test('Codex injects bounded live Runtime Context when active Task, Temporary, or follow-up exist', async t => {
  const root = await fixture(t);
  await project(root);

  await put(root, '.xiaotao/evidence/rules.md', '# Rules\nSome rules.\n');
  await put(root, '.xiaotao/tasks/task-cache/task.yaml', 'id: task-cache\nobjective: Implement caching layer\nstatus: active\ncreated_at: 2026-09-12T10:00:00Z\nupdated_at: 2026-09-12T10:00:00Z\nupdated_by: xiao-tao/test\nrevision: 1\n');
  await put(root, '.xiaotao/tasks/task-auth/task.yaml', 'id: task-auth\nobjective: OAuth integration\nstatus: active\ncreated_at: 2026-09-12T10:00:00Z\nupdated_at: 2026-09-12T10:00:00Z\nupdated_by: xiao-tao/test\nrevision: 1\n');
  await put(root, '.xiaotao/tasks/task-ci/task.yaml', 'id: task-ci\nobjective: Setup CI pipelines\nstatus: active\ncreated_at: 2026-09-12T10:00:00Z\nupdated_at: 2026-09-12T10:00:00Z\nupdated_by: xiao-tao/test\nrevision: 1\n');
  await put(root, '.xiaotao/tasks/task-db/task.yaml', 'id: task-db\nobjective: DB migration\nstatus: active\ncreated_at: 2026-09-12T10:00:00Z\nupdated_at: 2026-09-12T10:00:00Z\nupdated_by: xiao-tao/test\nrevision: 1\n');
  await put(root, '.xiaotao/memory/temporary/active/temp-investigate/meta.yaml', 'id: temp-investigate\ntopic: Investigate leak\nstatus: active\ncreated_at: 2026-09-12T10:00:00Z\nupdated_at: 2026-09-12T10:00:00Z\nupdated_by: xiao-tao/test\nrevision: 1\n');
  await put(root, '.xiaotao/memory/long-term/entries/lt-api-boundary.md', `---
revision: 1
updated_at: 2026-09-12T10:00:00Z
updated_by: xiao-tao/test
---

# Entry

\`\`\`xiaotao-memory-entry
{"entry_id":"lt-api-boundary","title":"API boundary rules","memory_kind":"experience","content":"Boundary rules","source_refs":[".xiaotao/evidence/rules.md"],"status":"active"}
\`\`\`
`);
  await put(root, '.xiaotao/memory/followups/pending/followup-review.yaml', 'followup_id: followup-review\ntitle: Review memory invariants\nstatus: pending\ncreated_at: 2026-09-12T10:00:00Z\nsource_refs:\n  - .xiaotao/evidence/rules.md\n');

  const result = await recoveryContext(event(root));
  assert.ok(result);
  const context = result.hookSpecificOutput.additionalContext;

  // Asserts bounded runtime context injection
  assert.match(context, /# Memory Overview \(Runtime Context\)/);
  assert.match(context, /当前检测到项目存在活动工作/);
  assert.match(context, /## Active Tasks \(4\)/);
  assert.match(context, /task-cache/);
  assert.match(context, /task-auth/);
  assert.match(context, /task-ci/);
  assert.match(context, /另外 1 项活动任务已省略/);
  assert.match(context, /## Active Temporary Memory \(1\)/);
  assert.match(context, /temp-investigate/);
  assert.match(context, /## Pending Follow-ups \(1\)/);
  assert.match(context, /followup-review/);
  assert.match(context, /## Long-term Memory \(1 项已索引\)/);
});

test('Codex SessionStart does not inject runtime context when index.json exists without any authoritative sources', async t => {
  const root = await fixture(t);
  await project(root);

  // Orphaned index.json without authoritative sources on disk
  const indexPayload = {
    schema_version: 1,
    entries: [
      { memory_id: 'task-orphaned', title: 'Orphaned task', record_type: 'task', status: 'active', path: '.xiaotao/tasks/task-orphaned/task.yaml' },
    ],
    pending_followups: [],
  };
  await put(root, '.xiaotao/memory/index.json', JSON.stringify(indexPayload));

  const result = await recoveryContext(event(root));
  assert.ok(result);
  assert.doesNotMatch(result.hookSpecificOutput.additionalContext, /# Memory Overview/);
});

test('Codex SessionStart rebuilds missing index when authoritative sources exist and injects live context', async t => {
  const root = await fixture(t);
  await project(root);

  // Authoritative task on disk, but NO .xiaotao/memory/index.json
  await put(root, '.xiaotao/tasks/task-live/task.yaml', `id: task-live
objective: Live active task
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: xiao-tao/test
revision: 1
`);

  const result = await recoveryContext(event(root));
  assert.ok(result);
  const context = result.hookSpecificOutput.additionalContext;

  // Asserts runtime context is injected with rebuilt task
  assert.match(context, /# Memory Overview \(Runtime Context\)/);
  assert.match(context, /当前检测到项目存在活动工作/);
  assert.match(context, /## Active Tasks \(1\)/);
  assert.match(context, /task-live/);

  // Asserts index.json was indeed rebuilt on disk
  const indexPath = path.join(root, '.xiaotao/memory/index.json');
  assert.ok(await readFile(indexPath, 'utf8'));
});

test('Codex SessionStart refreshes stale index when authoritative sources changed and injects new state', async t => {
  const root = await fixture(t);
  await project(root);

  // Authoritative new task on disk
  await put(root, '.xiaotao/tasks/task-updated/task.yaml', `id: task-updated
objective: Freshly updated task
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: xiao-tao/test
revision: 1
`);

  // Stale index containing only an obsolete task
  const staleIndex = {
    schema_version: 1,
    generated_at: '2026-09-01T00:00:00Z',
    source_digest: 'obsolete-hash',
    entries: [
      { memory_id: 'task-obsolete', title: 'Obsolete task', record_type: 'task', status: 'active', path: '.xiaotao/tasks/task-obsolete/task.yaml' },
    ],
    pending_followups: [],
  };
  await put(root, '.xiaotao/memory/index.json', JSON.stringify(staleIndex));

  const result = await recoveryContext(event(root));
  assert.ok(result);
  const context = result.hookSpecificOutput.additionalContext;

  // Asserts new task is injected, and obsolete task from stale index is NOT injected
  assert.match(context, /task-updated/);
  assert.doesNotMatch(context, /task-obsolete/);

  // Asserts index.json was refreshed on disk
  const refreshedIndex = JSON.parse(await readFile(path.join(root, '.xiaotao/memory/index.json'), 'utf8'));
  assert.ok(refreshedIndex.entries.some(e => e.memory_id === 'task-updated'));
  assert.ok(!refreshedIndex.entries.some(e => e.memory_id === 'task-obsolete'));
});

test('Codex SessionStart injects recoverable checkpoint runtime context when no active tasks or temporaries exist', async t => {
  const root = await fixture(t);
  await project(root);

  // No active tasks or temporaries, but a recoverable checkpoint exists
  const checkpointPayload = {
    schema_version: 1,
    request_id: 'req-chk-99',
    project: 'test-project',
    kind: 'task',
    target_id: 'task-suspended',
    session_id: 'session-prev',
    base_revision: 3,
    base_hash: 'a'.repeat(64),
    input_hash: 'b'.repeat(64),
    source_hash: 'c'.repeat(64),
    proposal: 'proposal body',
    proposal_hash: 'd'.repeat(64),
    snapshot: {
      objective: 'Suspended task objective',
      confirmed: ['step 1 done'],
      rejected: [],
      in_progress: ['step 2 in progress'],
      next: ['step 3 next'],
      open_questions: [],
      source_refs: ['.xiaotao/tasks/task-suspended/progress.md'],
    },
  };
  await put(
    root,
    '.xiaotao/tasks/task-suspended/references/checkpoints/req-chk-99.json',
    JSON.stringify(checkpointPayload, null, 2),
  );

  // 1. Pending request: outputs status: pending and proposed_revision: 4
  const pendingResult = await recoveryContext(event(root));
  assert.ok(pendingResult);
  const pendingContext = pendingResult.hookSpecificOutput.additionalContext;

  assert.match(pendingContext, /# Memory Overview \(Runtime Context\)/);
  assert.match(pendingContext, /当前检测到项目存在活动工作/);
  assert.match(pendingContext, /## Recoverable Checkpoint/);
  assert.match(pendingContext, /Recoverable checkpoint: yes/);
  assert.match(pendingContext, /status: pending/);
  assert.match(pendingContext, /scope: task/);
  assert.match(pendingContext, /binding: task-suspended/);
  assert.match(pendingContext, /proposed_revision: 4/);

  // 2. Committed checkpoint observation: outputs status: committed and revision: 4
  await put(
    root,
    '.xiaotao/tasks/task-suspended/references/checkpoints/req-chk-99.committed.json',
    JSON.stringify({ request_id: 'req-chk-99', revision: 4 }),
  );

  const committedResult = await recoveryContext(event(root));
  assert.ok(committedResult);
  const committedContext = committedResult.hookSpecificOutput.additionalContext;

  assert.match(committedContext, /## Recoverable Checkpoint/);
  assert.match(committedContext, /Recoverable checkpoint: yes/);
  assert.match(committedContext, /status: committed/);
  assert.match(committedContext, /scope: task/);
  assert.match(committedContext, /binding: task-suspended/);
  assert.match(committedContext, /revision: 4/);
});

test('Codex SessionStart does not consume stale index when refresh/rebuild fails and outputs degraded warning', async t => {
  const root = await fixture(t);
  await project(root);

  // Authoritative task on disk
  await put(root, '.xiaotao/tasks/task-current/task.yaml', `id: task-current
objective: Real current task
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: xiao-tao/test
revision: 1
`);

  // Stale index containing obsolete task
  await put(root, '.xiaotao/memory/index.json', JSON.stringify({
    schema_version: 1,
    generated_at: '2026-09-01T00:00:00Z',
    source_digest: 'obsolete',
    entries: [
      { memory_id: 'task-stale', title: 'Stale task that must not be consumed', record_type: 'task', status: 'active' },
    ],
    pending_followups: [],
  }));

  // Authoritative committed checkpoint exists
  await put(
    root,
    '.xiaotao/tasks/task-current/references/checkpoints/req-deg-1.committed.json',
    JSON.stringify({ request_id: 'req-deg-1', revision: 1 }),
  );

  // Force Python overview/rebuild to fail
  process.env.XIAOTAO_FORCE_PYTHON_FAIL = '1';
  t.after(() => { delete process.env.XIAOTAO_FORCE_PYTHON_FAIL; });

  const result = await recoveryContext(event(root));
  assert.ok(result);
  const context = result.hookSpecificOutput.additionalContext;

  // Stale task from index.json MUST NOT be consumed
  assert.doesNotMatch(context, /task-stale/);
  assert.doesNotMatch(context, /Stale task that must not be consumed/);

  // Explicit degraded warning MUST be output
  assert.match(context, /警告：检测到项目存在 XiaoTao 权威工作源，但 Memory Catalog 缺失或刷新失败/);

  // Authoritative checkpoint MUST still be presented
  assert.match(context, /## Recoverable Checkpoint/);
  assert.match(context, /Recoverable checkpoint: yes/);
  assert.match(context, /status: committed/);
  assert.match(context, /binding: task-current/);
  assert.match(context, /revision: 1/);
});
