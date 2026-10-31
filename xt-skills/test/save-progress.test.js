import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, readFile, rm, readdir, rename } from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
const python = process.platform === 'win32' ? 'python' : 'python3';
const script = path.resolve('xiaotao/scripts/save_progress.py');
const timeline = path.resolve('xiaotao/scripts/timeline_catalog.py');
const now = '2026-10-15T08:00:00Z';
const request = extra => ({ topic_id: 'stage', request_id: 'stage-1', title: '阶段结果', summary: '修改完成', validation: '测试通过', next_steps: '等待验收', ...extra });
async function put(root, relative, text) { const file = path.join(root, relative); await mkdir(path.dirname(file), { recursive: true }); await writeFile(file, text); }
async function fixture(t) { const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-save-')); t.after(() => rm(root, { recursive: true, force: true })); await mkdir(path.join(root, '.xiaotao')); return root; }
const invoke = (root, input, time = now) => new Promise((resolve, reject) => {
  const child = execFile(python, [script, '--project-root', root, '--now', time], { encoding: 'utf8' }, (error, stdout, stderr) => {
    try { resolve({ code: error?.code || 0, ...JSON.parse(stdout) }); } catch { reject(new Error(stderr || stdout)); }
  });
  child.stdin.end(JSON.stringify(input));
});
const command = (root, ...args) => new Promise((resolve, reject) => execFile(python, [timeline, '--project-root', root, '--now', now, ...args], { encoding: 'utf8' }, (error, stdout, stderr) => error ? reject(new Error(stderr)) : resolve(stdout)));
const work = '.xiaotao/tasks/topic/progress.md';
const original = '---\nrevision: 4\nupdated_at: 2026-10-14T00:00:00Z\nupdated_by: owner\n---\n\n# 既有状态\n不能丢失的事实\n';

test('one save handles date, revision, navigation and unchanged facts without manual setup', async t => {
  const root = await fixture(t);
  await put(root, '.xiaotao/config.yaml', 'memory_timezone: Asia/Shanghai\n');
  const saved = await invoke(root, request(), '2026-10-15T23:59:59Z');
  assert.equal(saved.status, 'saved'); assert.equal(saved.navigation_status, 'current');
  assert.match(saved.path, /2026\/10\/16/);
  const bytes = await readFile(path.join(root, saved.path));
  const retry = await invoke(root, request(), '2026-10-17T01:00:00Z');
  assert.equal(retry.status, 'duplicate'); assert.equal(retry.path, saved.path);
  assert.deepEqual(await readFile(path.join(root, saved.path)), bytes);
  assert.equal((await invoke(root, request({ request_id: 'stage-2' }), '2026-10-16T01:00:00Z')).status, 'unchanged');
  assert.equal((await invoke(root, request({ summary: 'different' }))).code, 1);
  await assert.rejects(readFile(path.join(root, work)), { code: 'ENOENT' });
});

test('linked work is appended once; the day contains only summary and link', async t => {
  const root = await fixture(t); await put(root, work, original);
  const input = request({ work_path: work, details: '只保存在任务中的实现细节' });
  const saved = await invoke(root, input);
  assert.equal(saved.work_status, 'saved'); assert.equal(saved.status, 'saved');
  const text = await readFile(path.join(root, work), 'utf8');
  assert.match(text, /revision: 5/); assert.match(text, /不能丢失的事实/); assert.match(text, /实现细节/);
  assert.doesNotMatch(await readFile(path.join(root, saved.path), 'utf8'), /实现细节/);
  const retry = await invoke(root, input);
  assert.equal(retry.work_status, 'duplicate'); assert.equal(retry.status, 'duplicate');
  assert.equal(await readFile(path.join(root, work), 'utf8'), text);
  assert.equal((await invoke(root, { ...input, request_id: 'stage-2' })).status, 'unchanged');
  assert.equal(await readFile(path.join(root, work), 'utf8'), text);
});

test('partial work commit resumes the original daily request without incrementing work twice', async t => {
  const root = await fixture(t); await put(root, work, original);
  await put(root, '.xiaotao/locks/timeline.lock', 'other writer');
  const input = request({ work_path: work });
  const failed = await invoke(root, input);
  assert.equal(failed.status, 'failed'); assert.equal(failed.work_status, 'saved');
  const bytes = await readFile(path.join(root, work));
  await rm(path.join(root, '.xiaotao/locks/timeline.lock'));
  const retry = await invoke(root, input);
  assert.equal(retry.status, 'saved'); assert.equal(retry.work_status, 'duplicate');
  assert.deepEqual(await readFile(path.join(root, work)), bytes);
});

test('conflicts, busy locks and invalid daily input never overwrite newer work', async t => {
  const root = await fixture(t); await put(root, work, original);
  await put(root, '.xiaotao/locks/tasks-topic-progress.md.lock', 'held');
  const input = request({ work_path: work });
  assert.equal((await invoke(root, input)).status, 'failed');
  assert.equal(await readFile(path.join(root, work), 'utf8'), original);
  await rm(path.join(root, '.xiaotao/locks/tasks-topic-progress.md.lock'));
  await put(root, work, original.replace('revision: 4', 'revision: 5') + 'newer work');
  const failed = await invoke(root, input);
  assert.match(failed.error, /conflict/);
  assert.match(await readFile(path.join(root, work), 'utf8'), /newer work/);
  assert.equal((await invoke(root, request({ request_id: 'invalid', work_path: work, details: 'x'.repeat(4001) }))).status, 'failed');
  assert.match(await readFile(path.join(root, work), 'utf8'), /revision: 5/);
});

test('incremental navigation matches full build and avoids unrelated historical details', async t => {
  const root = await fixture(t);
  for (const [id, time] of [['old', '2025-12-31T08:00:00Z'], ['month', '2026-09-30T08:00:00Z'], ['today', now]]) {
    assert.equal((await invoke(root, request({ request_id: id, summary: id }), time)).navigation_status, 'current');
  }
  await command(root, 'check');
  const old = '.xiaotao/memory/timeline/years/2025/12/31/details/stage.md';
  await put(root, old, 'damaged unrelated detail');
  const saved = await invoke(root, request({ request_id: 'more', summary: 'new fact' }));
  assert.equal(saved.navigation_status, 'current');
  assert.match(await command(root, 'summary', '--year', '2025'), /old/);
});

test('navigation errors preserve commits and retry repairs only the missing phase', async t => {
  const root = await fixture(t); await put(root, '.xiaotao/memory/timeline/summary.md', 'human text');
  const saved = await invoke(root, request());
  assert.equal(saved.status, 'saved'); assert.equal(saved.navigation_status, 'pending');
  assert.equal(await readFile(path.join(root, '.xiaotao/memory/timeline/summary.md'), 'utf8'), 'human text');
  const bytes = await readFile(path.join(root, saved.path));
  await rm(path.join(root, '.xiaotao/memory/timeline/summary.md'));
  assert.equal((await invoke(root, request())).navigation_status, 'current');
  assert.deepEqual(await readFile(path.join(root, saved.path)), bytes);
});

test('tampered recovery work target is not an instruction to overwrite project files', async t => {
  const root = await fixture(t); await put(root, work, original);
  await put(root, '.xiaotao/locks/tasks-topic-progress.md.lock', 'held');
  const input = request({ work_path: work }); await invoke(root, input);
  const directory = path.join(root, '.xiaotao/runtime/progress-saves');
  const file = path.join(directory, (await readdir(directory))[0]);
  const journal = JSON.parse(await readFile(file, 'utf8')); journal.work.path = 'README.md';
  await writeFile(file, JSON.stringify(journal)); await put(root, 'README.md', 'keep');
  assert.equal((await invoke(root, input)).code, 1);
  assert.equal(await readFile(path.join(root, 'README.md'), 'utf8'), 'keep');
});

test('completed retries resolve archived work and Windows CRLF progress without rewriting it', async t => {
  const root = await fixture(t); await put(root, work, original.replaceAll('\n', '\r\n'));
  const input = request({ work_path: work });
  const saved = await invoke(root, input); assert.equal(saved.status, 'saved');
  const before = await readFile(path.join(root, work));
  const archive = '.xiaotao/tasks/archive/topic/progress.md';
  await mkdir(path.dirname(path.join(root, archive)), { recursive: true });
  await rename(path.join(root, work), path.join(root, archive));
  const retry = await invoke(root, input);
  assert.equal(retry.status, 'duplicate'); assert.deepEqual(retry.work_paths, [archive]);
  assert.deepEqual(await readFile(path.join(root, archive)), before);
});

test('pre-highlight navigation is upgraded once and later local refresh remains build-equivalent', async t => {
  const root = await fixture(t); await invoke(root, request());
  const relative = '.xiaotao/memory/timeline/years/2026/10/15/summary.md';
  const text = await readFile(path.join(root, relative), 'utf8');
  await put(root, relative, text.replace(/^<!-- xiaotao:timeline-highlights:.* -->\n/m, ''));
  const saved = await invoke(root, request({ request_id: 'upgrade', topic_id: 'another', summary: 'another stage' }), '2026-10-16T08:00:00Z');
  assert.equal(saved.navigation_status, 'current');
  await command(root, 'check');
});

test('Chinese task IDs append linked progress, survive archive and produce byte-accurate CRLF retry receipts', async t => {
  const root = await fixture(t); const id = '20260928-定时任务迁移app-center';
  const work = `.xiaotao/tasks/${id}/progress.md`;
  await put(root, work, original);
  const input = request({ work_path: work });
  const saved = await invoke(root, input);
  assert.equal(saved.work_status, 'saved'); assert.equal(saved.navigation_status, 'current');
  const crlf = (await readFile(path.join(root, saved.path), 'utf8')).replaceAll('\n', '\r\n');
  await put(root, saved.path, crlf);
  await mkdir(path.join(root, '.xiaotao/tasks/archive'), { recursive: true });
  await rename(path.join(root, `.xiaotao/tasks/${id}`), path.join(root, `.xiaotao/tasks/archive/${id}`));
  const retry = await invoke(root, input);
  assert.equal(retry.status, 'duplicate'); assert.equal(retry.work_status, 'duplicate');
  assert.match(retry.work_paths[0], /archive/);
  const { createHash } = await import('node:crypto');
  assert.equal(retry.daily_sha256, createHash('sha256').update(crlf).digest('hex'));
});

test('incremental navigation fallback skips historical edited notes and provides repair commands', async t => {
  const root = await fixture(t); const first = await invoke(root, request());
  await put(root, first.path, (await readFile(path.join(root, first.path), 'utf8')) + '\n手工追加\n');
  // Force the normal incremental fallback through a missing historical summary.
  await rm(path.join(root, '.xiaotao/memory/timeline/years/2026/10/15/summary.md'));
  const next = await invoke(root, request({ request_id: 'day-two' }), '2026-10-16T08:00:00Z');
  assert.equal(next.status, 'saved'); assert.equal(next.navigation_status, 'partial');
  assert.equal(next.navigation_issues[0].path, first.path);
  assert.match(next.navigation_issues[0].repair_command, /reconcile/);
});

const minor = extra => {
  const input = request({ record_type: 'minor', project_id: 'app-center', module_id: 'scheduled-tasks', group_id: 'ui-fixes', ...extra });
  delete input.topic_id;
  return input;
};

test('minor fixes share one daily detail, retain timestamped entries and keep navigation current', async t => {
  const root = await fixture(t);
  let saved;
  for (let i = 0; i < 13; i++) {
    saved = await invoke(root, minor({ request_id: `fix-${i}`, title: `修改 ${i}`, summary: `事实 ${i}` }), `2026-10-15T08:${String(i).padStart(2, '0')}:00Z`);
    assert.equal(saved.status, 'saved'); assert.equal(saved.navigation_status, 'current');
  }
  const files = await readdir(path.dirname(path.join(root, saved.path)));
  assert.equal(files.length, 1);
  const text = await readFile(path.join(root, saved.path), 'utf8');
  for (let i = 0; i < 13; i++) assert.match(text, new RegExp(`修改 ${i}\\n`));
  assert.match(text, /08:00:00\+00:00 修改 0/);
  assert.match(text, /scheduled-tasks \/ ui-fixes 小修/);
  assert.equal((await invoke(root, minor({ request_id: 'same', title: '修改 12', summary: '事实 12' }))).status, 'unchanged');
  await command(root, 'check');
});

test('minor grouping separates project, module, group, day and standalone milestones', async t => {
  const root = await fixture(t); const paths = [];
  for (const extra of [{}, { project_id: 'other' }, { module_id: 'other' }, { group_id: 'logic-fixes' }]) {
    const saved = await invoke(root, minor({ ...extra, request_id: `save-${paths.length}` }));
    assert.equal(saved.status, 'saved'); paths.push(saved.path);
  }
  paths.push((await invoke(root, minor({ request_id: 'next-day' }), '2026-10-16T08:00:00Z')).path);
  paths.push((await invoke(root, request({ request_id: 'milestone' }))).path);
  assert.equal(new Set(paths).size, 6);
  for (const extra of [{ module_id: '../escape' }, { project_id: '' }, { record_type: 'auto' }]) {
    assert.equal((await invoke(root, minor({ request_id: 'invalid', ...extra }))).status, 'failed');
  }
  assert.equal((await invoke(root, request({ project_id: 'app' }))).status, 'failed');
});

test('minor retry across midnight is idempotent and different titles with identical facts append', async t => {
  const root = await fixture(t); const input = minor();
  const first = await invoke(root, input);
  const bytes = await readFile(path.join(root, first.path));
  const retry = await invoke(root, input, '2026-10-16T09:00:00Z');
  assert.equal(retry.status, 'duplicate'); assert.equal(retry.path, first.path);
  assert.deepEqual(await readFile(path.join(root, first.path)), bytes);
  const second = await invoke(root, minor({ request_id: 'other-title', title: '不同的小修' }));
  assert.equal(second.status, 'saved'); assert.equal(second.path, first.path);
  assert.match(await readFile(path.join(root, first.path), 'utf8'), /不同的小修/);
});

test('group locks, manual edits, work rebinding and capacity failures preserve prior facts', async t => {
  const root = await fixture(t); const input = minor();
  const saved = await invoke(root, input);
  const topic = path.basename(saved.path, '.md');
  const lock = `.xiaotao/locks/progress-group-${topic}.lock`;
  await put(root, lock, 'held');
  assert.match((await invoke(root, minor({ request_id: 'busy' }))).error, /busy/);
  await rm(path.join(root, lock));
  const before = await readFile(path.join(root, saved.path), 'utf8');
  const full = await invoke(root, minor({ request_id: 'full', details: 'x'.repeat(4000), validation: 'v'.repeat(4000) }));
  assert.equal(full.status, 'failed');
  assert.equal(await readFile(path.join(root, saved.path), 'utf8'), before);
  await put(root, saved.path, before + '\n人工修改\n');
  assert.equal((await invoke(root, minor({ request_id: 'edited', summary: '新事实' }))).status, 'failed');
  assert.equal(await readFile(path.join(root, saved.path), 'utf8'), before + '\n人工修改\n');

  const linkedRoot = await fixture(t); await put(linkedRoot, work, original);
  const first = await invoke(linkedRoot, minor({ work_path: work }));
  const otherWork = '.xiaotao/tasks/other/progress.md'; await put(linkedRoot, otherWork, original);
  assert.equal((await invoke(linkedRoot, minor({ request_id: 'rebind', work_path: otherWork }))).status, 'failed');
  assert.equal(await readFile(path.join(linkedRoot, otherWork), 'utf8'), original);
  assert.equal((await invoke(linkedRoot, minor({ request_id: 'more', work_path: work, summary: '另一个修复' }))).path, first.path);
});

test('concurrent minor requests can retry without losing either entry', async t => {
  const root = await fixture(t);
  const inputs = [minor({ request_id: 'parallel-a', summary: '并发 A' }), minor({ request_id: 'parallel-b', summary: '并发 B' })];
  const results = await Promise.all(inputs.map(input => invoke(root, input)));
  for (let i = 0; i < results.length; i++) {
    if (results[i].status === 'failed') {
      assert.match(results[i].error, /busy/);
      results[i] = await invoke(root, inputs[i]);
    }
    assert.equal(results[i].status, 'saved');
  }
  assert.equal(results[0].path, results[1].path);
  const text = await readFile(path.join(root, results[0].path), 'utf8');
  assert.match(text, /并发 A/); assert.match(text, /并发 B/);
  assert.equal((await readdir(path.dirname(path.join(root, results[0].path)))).length, 1);
});
