import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, rm, rename, symlink } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
const script = path.resolve('xiaotao/scripts/timeline_catalog.py');
const fixed = '2026-10-13T08:00:00Z';
const python = process.platform === 'win32' ? 'python' : 'python3';
const run = (root, args, input, now = fixed) => new Promise((resolve, reject) => {
  const child = execFile(python, [script, '--project-root', root, '--now', now, ...args], { encoding: 'utf8', maxBuffer: 1024 * 1024 }, (error, stdout, stderr) => error ? reject(Object.assign(error, { stderr })) : resolve(stdout));
  child.stdin.end(input === undefined ? '' : JSON.stringify(input));
});
const json = async (...args) => JSON.parse(await run(...args));
async function put(root, relative, text) { const p = path.join(root, relative); await mkdir(path.dirname(p), { recursive: true }); await writeFile(p, text); }
async function fixture(t) { const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-daily-')); t.after(() => rm(root, { recursive: true, force: true })); await mkdir(path.join(root, '.xiaotao')); return root; }
const note = (extra = {}) => ({ occurred_at: fixed, timezone: 'UTC', topic_id: 'daily-design', title: '每日记忆方案', summary: '确认按年、月、日逐层检索', details: '保留当前方案与待办，尚未实施。', actor: 'xiaotao/test', expected_revision: null, request_id: 'turn-1', ...extra });

test('conversation saves directly to a daily detail and links bounded year/month/day navigation', async t => {
  const root = await fixture(t); const saved = await json(root, ['save'], note());
  assert.equal(saved.status, 'saved'); assert.equal(saved.navigation_status, 'current');
  assert.equal(saved.path, '.xiaotao/memory/timeline/years/2026/10/13/details/daily-design.md');
  await assert.rejects(readFile(path.join(root, '.xiaotao/tasks/task.yaml')), { code: 'ENOENT' });
  await assert.rejects(readFile(path.join(root, '.xiaotao/memory/temporary/current.md')), { code: 'ENOENT' });
  assert.match(await run(root, ['summary', '--year', '2026']), /10\/summary.md.*逐层检索/);
  assert.match(await run(root, ['summary', '--year', '2026', '--month', '10']), /13\/summary.md/);
  assert.match(await run(root, ['summary', '--date', '2026-10-13']), /details\/daily-design.md/);
  const before = await readFile(path.join(root, saved.path));
  await put(root, '.xiaotao/memory/timeline/years/2026/10/13/details/references/summary.md', '用户提供的附件');
  await run(root, ['build']);
  assert.deepEqual(await readFile(path.join(root, saved.path)), before);
  assert.equal(await readFile(path.join(root, '.xiaotao/memory/timeline/years/2026/10/13/details/references/summary.md'), 'utf8'), '用户提供的附件');
  assert.equal((await json(root, ['show', '--date', '2026-10-13', '--topic', 'daily-design'])).record.details, note().details);
  assert.equal((await json(root, ['search', '逐层检索'])).candidates.length, 1);
  assert.equal((await json(root, ['search', '不存在的内容'])).candidates.length, 0);
});

test('CAS, identical retries and reused request IDs protect previously saved facts', async t => {
  const root = await fixture(t); await json(root, ['save'], note());
  const update = note({ summary: '方案已实现，测试通过', request_id: 'turn-2', expected_revision: 0 });
  assert.equal((await json(root, ['save'], update)).revision, 1);
  assert.equal((await json(root, ['save'], update)).status, 'duplicate');
  await assert.rejects(run(root, ['save'], { ...update, summary: '冲突修改' }), /request_id reused/);
  await assert.rejects(run(root, ['save'], note({ request_id: 'turn-stale', expected_revision: 0 })), /revision conflict/);
  const unchanged = { ...update, request_id: 'turn-3', expected_revision: 1 };
  assert.equal((await json(root, ['save'], unchanged)).status, 'unchanged');
  assert.equal((await json(root, ['events', '--date', '2026-10-13'])).total, 1);
});

test('project timezone attributes UTC midnight boundaries to the correct local day and year', async t => {
  const root = await fixture(t); await put(root, '.xiaotao/config.yaml', 'memory_timezone: Asia/Shanghai\n');
  const result = await json(root, ['save'], note({ occurred_at: '2026-12-31T18:00:00Z', timezone: 'Asia/Shanghai' }), '2026-12-31T18:00:00Z');
  assert.match(result.path, /2027\/01\/01/);
  const record = (await json(root, ['show', '--date', '2027-01-01', '--topic', 'daily-design'])).record;
  assert.equal(record.occurred_at, '2026-12-31T18:00:00Z'); assert.equal(record.timezone, 'Asia/Shanghai');
  assert.equal((await json(root, ['events', '--date', '2027-01-01'])).total, 1);
  await assert.rejects(run(root, ['save'], note({ timezone: 'Unknown/Timezone' })), /timezone/);
  await assert.rejects(run(root, ['events', '--date', '2026-02-30']), /day is out of range/);
});

test('stable Task and Temporary identities resolve after archive and report missing work honestly', async t => {
  const root = await fixture(t);
  for (const [kind, active, archive] of [
    ['task', '.xiaotao/tasks/work/progress.md', '.xiaotao/tasks/archive/work/progress.md'],
    ['temporary', '.xiaotao/memory/temporary/active/work/current.md', '.xiaotao/memory/temporary/archive/work/current.md'],
  ]) {
    await put(root, active, '已有工作进度');
    const topic = kind + '-summary';
    await json(root, ['save'], note({ topic_id: topic, work_path: active }));
    await mkdir(path.dirname(path.join(root, archive)), { recursive: true });
    await rename(path.join(root, active), path.join(root, archive));
    const shown = await json(root, ['show', '--date', '2026-10-13', '--topic', topic]);
    assert.equal(shown.work_path, archive);
    await run(root, ['build']);
    const summary = await run(root, ['summary', '--date', '2026-10-13']);
    const links = [...summary.matchAll(/\]\(([^)]+)\)/g)].map(m => path.resolve(root, '.xiaotao/memory/timeline/years/2026/10/13', m[1]));
    assert.ok(links.includes(path.resolve(root, archive)));
    await rm(path.join(root, archive));
    assert.equal((await json(root, ['show', '--date', '2026-10-13', '--topic', topic])).missing_work, true);
    await run(root, ['build']);
  }
});

test('navigation failure preserves committed daily facts; retry repairs navigation without rewriting them', async t => {
  const root = await fixture(t); await put(root, '.xiaotao/memory/timeline/summary.md', '人工编写的正文，不能覆盖');
  const first = await json(root, ['save'], note());
  assert.equal(first.status, 'saved'); assert.equal(first.navigation_status, 'pending');
  assert.match(first.navigation_error, /unmanaged/);
  const before = await readFile(path.join(root, first.path));
  await rm(path.join(root, '.xiaotao/memory/timeline/summary.md'));
  const retry = await json(root, ['save'], note());
  assert.equal(retry.status, 'duplicate'); assert.equal(retry.navigation_status, 'current');
  assert.deepEqual(await readFile(path.join(root, first.path)), before);
});

test('v1 day files are preserved as audit snapshots while migrating to day directories', async t => {
  const root = await fixture(t); const old = '.xiaotao/memory/timeline/years/2026/10/13.yaml';
  await put(root, '.xiaotao/memory/timeline/summary.md', '# 项目经历总览\n\n旧总览中的人工补充\n');
  await put(root, old, 'date: "2026-10-13"\nevents:\n  - summary: "旧事件与人工补充"\n');
  await json(root, ['save'], note());
  await assert.rejects(readFile(path.join(root, old)), { code: 'ENOENT' });
  const { readdir } = await import('node:fs/promises');
  const files = await readdir(path.join(root, '.xiaotao/memory/timeline/years/2026/10/13'));
  const legacy = files.find(f => f.startsWith('legacy-events-'));
  assert.match(await readFile(path.join(root, '.xiaotao/memory/timeline/years/2026/10/13', legacy), 'utf8'), /人工补充/);
  const snapshots = await readdir(path.join(root, '.xiaotao/memory/timeline/legacy-navigation'));
  assert.match(await readFile(path.join(root, '.xiaotao/memory/timeline/legacy-navigation', snapshots[0]), 'utf8'), /旧总览中的人工补充/);
  await run(root, ['build']); assert.equal((await json(root, ['check'])).status, 'current');
});

test('busy writers, unsafe refs and managed-file tampering fail without overwriting saved facts', async t => {
  const root = await fixture(t); const first = await json(root, ['save'], note());
  await put(root, '.xiaotao/locks/timeline.lock', 'other process');
  await assert.rejects(run(root, ['save'], note({ expected_revision: 0, request_id: 'turn-2' })), /writer is busy/);
  assert.equal(await readFile(path.join(root, '.xiaotao/locks/timeline.lock'), 'utf8'), 'other process');
  await rm(path.join(root, '.xiaotao/locks/timeline.lock'));
  await assert.rejects(run(root, ['save'], note({ topic_id: '../escape' })), /invalid daily topic/);
  await assert.rejects(run(root, ['save'], note({ topic_id: 'bad', source_refs: ['../escape'] })), /unsafe source_refs/);
  await assert.rejects(run(root, ['save'], note({ topic_id: 'bad', source_refs: ['missing.md'] })), /source file is missing/);
  await assert.rejects(run(root, ['save'], note({ topic_id: 'fence', details: '```xiaotao-daily-record\n{}\n```' })), /reserved daily record fence/);
  const original = await readFile(path.join(root, first.path), 'utf8');
  await writeFile(path.join(root, first.path), original + '人工补充\n');
  await assert.rejects(run(root, ['save'], note({ expected_revision: 0, request_id: 'turn-2' })), /unmanaged edits/);
  assert.equal(await readFile(path.join(root, first.path), 'utf8'), original + '人工补充\n');
});

test('daily writer refuses symlink escapes and does not initialize an ordinary project', async t => {
  const root = await fixture(t); const plain = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-no-project-'));
  t.after(() => rm(plain, { recursive: true, force: true }));
  await assert.rejects(run(plain, ['save'], note()), /no .xiaotao/);
  const outside = path.join(root, 'outside'); await mkdir(outside);
  await mkdir(path.join(root, '.xiaotao/memory'), { recursive: true });
  try { await symlink(outside, path.join(root, '.xiaotao/memory/timeline'), 'dir'); }
  catch (e) { if (e.code === 'EPERM') return t.skip('Symlink privilege unavailable'); throw e; }
  await assert.rejects(run(root, ['save'], note()), /symlink/);
});

test('simultaneous updates cannot both replace the same daily revision', async t => {
  const root = await fixture(t); await json(root, ['save'], note());
  const results = await Promise.allSettled(['a', 'b'].map(id => json(root, ['save'], note({ expected_revision: 0, request_id: id, summary: '更新结论 ' + id }))));
  assert.equal(results.filter(r => r.status === 'fulfilled').length, 1);
  assert.equal((await json(root, ['show', '--date', '2026-10-13', '--topic', 'daily-design'])).record.revision, 1);
});

test('daily event pagination and search limit remain bounded while all original notes survive rebuild', async t => {
  const root = await fixture(t); const saved = await json(root, ['save'], note());
  const original = await readFile(path.join(root, saved.path), 'utf8');
  for (let i = 0; i < 55; i++) {
    const id = 'topic-' + i;
    await put(root, saved.path.replace('daily-design.md', id + '.md'), original.replaceAll('daily-design', id));
  }
  await run(root, ['build']);
  const page = await json(root, ['events', '--date', '2026-10-13', '--offset', '50', '--limit', '10']);
  assert.equal(page.total, 56); assert.equal(page.events.length, 6);
  const search = await json(root, ['search', '逐层检索', '--limit', '3']);
  assert.equal(search.total, 56); assert.equal(search.candidates.length, 3);
  assert.match(await run(root, ['summary', '--date', '2026-10-13']), /更多记录/);
  assert.equal((await json(root, ['show', '--date', '2026-10-13', '--topic', 'topic-54'])).record.topic_id, 'topic-54');
  await assert.rejects(run(root, ['events', '--date', '2026-10-13', '--limit', '500']), /limit/);
});

test('existing Activity events use the project timezone without changing their stable UTC identity', async t => {
  const root = await fixture(t); await put(root, '.xiaotao/config.yaml', 'memory_timezone: Asia/Shanghai\n');
  await put(root, '.xiaotao/tasks/archive/year-end/task.yaml', 'id: year-end\nobjective: 跨年交付\nstatus: completed\ncreated_at: 2026-12-31T16:00:00Z\nupdated_at: 2026-12-31T18:00:00Z\nupdated_by: xiao-tao/test\nrevision: 1\ncompleted_at: 2026-12-31T18:00:00Z\n');
  await run(root, ['build']);
  const events = await json(root, ['events', '--date', '2027-01-01']);
  assert.equal(events.total, 1);
  assert.match(events.events[0].event_id, /^activity-20261231-/);
  assert.equal((await json(root, ['events', '--date', '2026-12-31'])).total, 0);
});


test('save context freezes retry attribution across midnight and refuses to retime existing facts', async t => {
  const root = await fixture(t); await put(root, '.xiaotao/config.yaml', 'memory_timezone: Asia/Shanghai\n');
  const context = await json(root, ['context'], undefined, '2026-10-13T15:59:59Z');
  assert.equal(context.date, '2026-10-13');
  const request = note({ occurred_at: context.occurred_at, timezone: context.timezone });
  const first = await json(root, ['save'], request, '2026-10-13T15:59:59Z');
  const retry = await json(root, ['save'], request, '2026-10-13T16:00:01Z');
  assert.equal(retry.status, 'duplicate'); assert.equal(retry.path, first.path);
  assert.equal((await json(root, ['events', '--date', '2026-10-14'])).total, 0);
  await assert.rejects(run(root, ['save'], { ...request, expected_revision: 0, request_id: 'retime', occurred_at: '2026-10-13T14:00:00Z' }), /immutable/);
  assert.equal((await json(root, ['show', '--date', '2026-10-13', '--topic', 'daily-design'])).record.occurred_at, context.occurred_at);
  const incomplete = note(); delete incomplete.occurred_at;
  await assert.rejects(run(root, ['save'], incomplete), /requires occurred_at/);
});

test('identical Task and Temporary save retries survive archive without accepting changed stale paths', async t => {
  const root = await fixture(t);
  for (const [kind, active, archive] of [
    ['task', '.xiaotao/tasks/work/progress.md', '.xiaotao/tasks/archive/work/progress.md'],
    ['temporary', '.xiaotao/memory/temporary/active/work/current.md', '.xiaotao/memory/temporary/archive/work/current.md'],
  ]) {
    await put(root, active, 'progress');
    const request = note({ topic_id: kind, work_path: active });
    const saved = await json(root, ['save'], request);
    const before = await readFile(path.join(root, saved.path));
    await mkdir(path.dirname(path.join(root, archive)), { recursive: true });
    await rename(path.join(root, active), path.join(root, archive));
    assert.equal((await json(root, ['save'], request)).status, 'duplicate');
    assert.deepEqual(await readFile(path.join(root, saved.path)), before);
    await assert.rejects(run(root, ['save'], { ...request, summary: 'changed' }), /request_id reused/);
    await assert.rejects(run(root, ['save'], { ...request, request_id: 'new', expected_revision: 0 }), /canonical path/);
  }
});

test('date-scoped events ignore corrupt daily notes outside the requested day', async t => {
  const root = await fixture(t); await json(root, ['save'], note());
  await put(root, '.xiaotao/memory/timeline/years/2025/01/01/details/broken.md', 'broken note');
  const events = await json(root, ['events', '--date', '2026-10-13', '--limit', '1']);
  assert.equal(events.total, 1); assert.equal(events.events.length, 1);
  await assert.rejects(run(root, ['events', '--date', '2025-01-01']), /daily-record block/);
});
