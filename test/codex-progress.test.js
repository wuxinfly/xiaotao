import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm, symlink } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Readable, Writable } from 'node:stream';
import { progressContext, acknowledge, run } from '../adapters/codex/xiaotao-codex/scripts/progress-check.mjs';
const script = fileURLToPath(new URL('../adapters/codex/xiaotao-codex/scripts/progress-check.mjs', import.meta.url));
const today = new Date().toISOString().slice(0, 10);
const target = `.xiaotao/memory/timeline/years/${today.replaceAll('-', '/')}/details/topic.md`;
async function put(root, file, content) { await mkdir(path.dirname(path.join(root, file)), { recursive: true }); await writeFile(path.join(root, file), content); }
async function fixture(t) {
  const cwd = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-progress-'));
  t.after(() => rm(cwd, { recursive: true, force: true }));
  await put(cwd, '.xiaotao/installation.json', JSON.stringify({ package: 'xiaotao-ai-workflow', schema_version: 2 }));
  return { cwd, session_id: 'session-a', turn_id: 'turn-a', hook_event_name: 'UserPromptSubmit' };
}
const stop = e => ({ ...e, hook_event_name: 'Stop', stop_hook_active: false });
async function save(e, relative = target, revision = 0, workRef = null) {
  const updated = new Date().toISOString();
  const front = `---\nrevision: ${revision}\nupdated_at: ${updated}\nupdated_by: xiao-tao/test\n---\n`;
  if (!relative.includes('/timeline/')) return put(e.cwd, relative, front + '已改，尚未验证。\n');
  const record = { schema_version: 1, record_id: `daily-${today}-topic`, topic_id: 'topic', date: today, timezone: 'UTC', title: '本轮工作', summary: '已改，尚未验证', details: '', source_refs: [], revision, updated_at: updated, updated_by: 'xiao-tao/test', occurred_at: updated, work_ref: workRef };
  await put(e.cwd, relative, front + '\n```xiaotao-daily-record\n' + JSON.stringify(record) + '\n```\n');
}

test('missing receipt continues once, repeated Stop and active continuation cannot loop', async t => {
  const e = await fixture(t); await progressContext(e);
  assert.equal((await progressContext(stop(e))).decision, 'block');
  assert.equal((await progressContext(stop(e))).decision, undefined);
  assert.equal(await progressContext({ ...stop(e), stop_hook_active: true }), null);
  // Replaying the same prompt cannot clear the once-only guard.
  await progressContext(e); assert.equal((await progressContext(stop(e))).decision, undefined);
});

test('saved receipt checks bounded file bytes; changed, missing and old files cannot masquerade as saved', async t => {
  const e = await fixture(t); await progressContext(e); await save(e);
  await acknowledge({ ...e, status: 'saved', path: target });
  assert.equal(await progressContext(stop(e)), null);
  await save(e, target, 1); assert.equal((await progressContext(stop(e))).decision, 'block');
  await rm(path.join(e.cwd, target));
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target }));
  await put(e.cwd, target, '---\nrevision: 1\nupdated_at: 2000-01-01T00:00:00Z\nupdated_by: x\n---\nold');
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target }), /this_turn/);
});

test('explicit skip and failure let chat end without creating Task or Memory files', async t => {
  const e = await fixture(t); await progressContext(e);
  await acknowledge({ ...e, status: 'skipped', reason: 'no-new-facts' });
  assert.equal(await progressContext(stop(e)), null);
  assert.deepEqual((await readdir(path.join(e.cwd, '.xiaotao'))).sort(), ['installation.json', 'runtime']);
  const next = { ...e, turn_id: 'next' }; await progressContext(next);
  await acknowledge({ ...next, status: 'failed', reason: 'save-failed' });
  assert.match((await progressContext(stop(next))).systemMessage, /未保存/);
  await assert.rejects(acknowledge({ ...next, status: 'skipped', reason: 'arbitrary' }), /invalid_reason/);
});

test('sessions and turns are isolated; unknown turns and missing identifiers fail open', async t => {
  const e = await fixture(t); await progressContext(e);
  const other = { ...e, session_id: 'session-b' }; await progressContext(other);
  await acknowledge({ ...e, status: 'skipped', reason: 'user-opt-out' });
  assert.equal((await progressContext(stop(other))).decision, 'block');
  const next = { ...e, turn_id: 'turn-b' }; await progressContext(next);
  await assert.rejects(acknowledge({ ...e, status: 'skipped', reason: 'no-new-facts' }), /turn_mismatch/);
  assert.equal(await progressContext(stop(e)), null);
  assert.equal(await progressContext({ ...stop(e), turn_id: undefined }), null);
});

test('linked lightweight work requires both daily and archived Current State receipts', async t => {
  const e = await fixture(t); await progressContext(e);
  const archived = '.xiaotao/memory/temporary/archive/topic/current.md';
  await save(e, archived);
  await save(e, target, 0, { kind: 'temporary', id: 'topic', path: '.xiaotao/memory/temporary/active/topic/current.md' });
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target }), /linked_work_receipt_missing/);
  await acknowledge({ ...e, status: 'saved', path: target, work_paths: [archived] });
  assert.equal(await progressContext(stop(e)), null);
  await save(e, archived, 1);
  assert.equal((await progressContext(stop(e))).decision, 'block');
});

test('ordinary and nested repositories do not borrow parent project state', async t => {
  const e = await fixture(t);
  const nested = path.join(e.cwd, 'nested'); await mkdir(path.join(nested, '.git'), { recursive: true });
  assert.equal(await progressContext({ ...e, cwd: nested }), null);
  const plain = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-plain-'));
  t.after(() => rm(plain, { recursive: true, force: true }));
  assert.equal(await progressContext({ ...e, cwd: plain }), null);
});

test('path escape and symlink targets are rejected', async t => {
  const e = await fixture(t); await progressContext(e);
  for (const relative of ['../outside', '/etc/passwd', '.xiaotao/tasks/../topic/progress.md', 'README.md']) {
    await assert.rejects(acknowledge({ ...e, status: 'saved', path: relative }));
  }
  await save(e); const outside = path.join(e.cwd, 'outside.md'); await writeFile(outside, 'external'); await rm(path.join(e.cwd, target));
  try { await symlink(outside, path.join(e.cwd, target)); } catch (err) { if (err.code === 'EPERM') return t.skip('Symlink privilege unavailable'); throw err; }
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target }), /unsafe_path/);
});

test('oversized or malformed hook input reports diagnosis and emits valid fail-open JSON', async () => {
  for (const input of ['invalid', 'x'.repeat(33000)]) {
    let stdout = '', stderr = '';
    const sink = cb => new Writable({ write(c, enc, done) { cb(c.toString()); done(); } });
    await run(Readable.from([input]), sink(s => stdout += s), sink(s => stderr += s));
    assert.deepEqual(JSON.parse(stdout), {}); assert.match(stderr, /skipped/);
  }
});

test('installed Hook launcher and ack CLI work with special-character paths', async t => {
  const e = await fixture(t); const plugins = path.join(e.cwd, 'plugin space & 中文');
  await put(plugins, 'scripts/progress-check.mjs', await readFile(script));
  const hooks = JSON.parse(await readFile('adapters/codex/xiaotao-codex/hooks/hooks.json'));
  const command = hooks.hooks.UserPromptSubmit[0].hooks[0].command;
  const execute = event => execFileSync(process.env.ComSpec || '/bin/sh', process.platform === 'win32' ? ['/d', '/s', '/c', command] : ['-c', command], { input: JSON.stringify(event), encoding: 'utf8', env: { ...process.env, PLUGIN_ROOT: plugins } });
  assert.equal(JSON.parse(execute(e)).hookSpecificOutput.hookEventName, 'UserPromptSubmit');
  execFileSync(process.execPath, [path.join(plugins, 'scripts/progress-check.mjs'), '--ack'], { input: JSON.stringify({ ...e, status: 'skipped', reason: 'no-current-work' }) });
  assert.deepEqual(JSON.parse(execute(stop(e))), {});
});

test('oversized progress is rejected, failed ack CLI exits nonzero', async t => {
  const e = await fixture(t); await progressContext(e);
  await put(e.cwd, target, 'x'.repeat(33000));
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target }), /invalid_file/);
  assert.throws(() => execFileSync(process.execPath, [script, '--ack'], { input: JSON.stringify({ ...e, status: 'saved', path: target }), stdio: ['pipe', 'pipe', 'pipe'] }), err => err.status === 1);
});

test('operator can disable checks without creating Runtime metadata', async t => {
  const e = await fixture(t);
  const result = execFileSync(process.execPath, [script], { input: JSON.stringify(e), encoding: 'utf8', env: { ...process.env, XIAOTAO_CODEX_PROGRESS_CHECK: '0' } });
  assert.deepEqual(JSON.parse(result), {});
  assert.deepEqual(await readdir(path.join(e.cwd, '.xiaotao')), ['installation.json']);
});

test('a busy lock fails open and remains owned by its original writer', async t => {
  const e = await fixture(t); await progressContext(e);
  const runtime = path.join(e.cwd, '.xiaotao/runtime/codex/progress');
  const record = (await readdir(runtime)).find(p => p.endsWith('.json'));
  const lock = path.join(runtime, record.replace('.json', '.lock')); await writeFile(lock, 'other writer');
  const result = execFileSync(process.execPath, [script], { input: JSON.stringify(stop(e)), encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] });
  assert.deepEqual(JSON.parse(result), {});
  assert.equal(await readFile(lock, 'utf8'), 'other writer');
});

const compact = (e, trigger = 'manual') => ({ ...e, hook_event_name: 'PreCompact', turn_id: undefined, trigger });

test('manual compaction checks latest session receipt without requiring an active turn ID', async t => {
  const e = await fixture(t); await progressContext(e); await save(e);
  await acknowledge({ ...e, status: 'saved', path: target });
  assert.equal(await progressContext(compact(e)), null);
  await save(e, target, 1);
  assert.equal((await progressContext(compact(e))).continue, false);
  assert.equal((await progressContext(compact(e))).continue, undefined);
});

test('missing or failed compaction receipt pauses once and automatic mid-turn compaction warns without trapping', async t => {
  const e = await fixture(t);
  const first = await progressContext(compact(e));
  assert.equal(first.continue, false); assert.match(first.stopReason, /没有自动总结/);
  assert.equal((await progressContext(compact(e))).continue, undefined);
  const next = { ...e, turn_id: 'next' }; await progressContext(next);
  await acknowledge({ ...next, status: 'failed', reason: 'save-failed' });
  assert.equal((await progressContext(compact(next))).continue, false);
  assert.match((await progressContext(compact(next, 'auto'))).systemMessage, /自动压缩.*放行/);
  await acknowledge({ ...next, status: 'skipped', reason: 'no-new-facts' });
  assert.match((await progressContext(compact(next))).systemMessage, /缺少有效/);
  await acknowledge({ ...next, status: 'skipped', reason: 'user-opt-out' });
  assert.equal(await progressContext(compact(next)), null);
});

test('receipts reject generated summaries, mismatched daily identity and unrelated work', async t => {
  const e = await fixture(t); await progressContext(e); await save(e);
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target.replace('details/topic.md', 'summary.md') }), /invalid_progress_target/);
  const before = await readFile(path.join(e.cwd, target), 'utf8');
  await put(e.cwd, target, before.replace('daily-' + today + '-topic', 'daily-2000-01-01-topic'));
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target }), /invalid_daily_record/);
  await put(e.cwd, target, before);
  await assert.rejects(acknowledge({ ...e, status: 'saved', path: target, work_paths: ['.xiaotao/tasks/other/progress.md'] }), /unrelated_work/);
});

for (const resolution of ['saved', 'user-opt-out']) {
  test(`failed saves survive later ordinary skips until ${resolution}`, async t => {
    const e = await fixture(t); await progressContext(e);
    await acknowledge({ ...e, status: 'failed', reason: 'save-failed' });
    let next = e;
    for (const reason of ['no-new-facts', 'no-current-work']) {
      next = { ...next, turn_id: reason }; await progressContext(next);
      await acknowledge({ ...next, status: 'skipped', reason });
      assert.equal((await progressContext({ ...next, hook_event_name: 'PreCompact', trigger: 'manual' })).continue, false);
      assert.equal((await progressContext(stop(next))).decision, 'block');
    }
    next = { ...next, turn_id: 'resolved' }; await progressContext(next);
    if (resolution === 'saved') {
      await save(next); await acknowledge({ ...next, status: 'saved', path: target });
    } else await acknowledge({ ...next, status: 'skipped', reason: 'user-opt-out' });
    assert.equal(await progressContext({ ...next, hook_event_name: 'PreCompact', trigger: 'manual' }), null);
    assert.equal(await progressContext(stop(next)), null);
  });
}
