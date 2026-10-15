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
