import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);
const repositoryRoot = path.resolve(import.meta.dirname, '..');
const python = process.platform === 'win32' ? 'python' : 'python3';
const catalogScript = path.join(repositoryRoot, 'xiaotao', 'scripts', 'memory_catalog.py');
const validatorScript = path.join(repositoryRoot, 'xiaotao', 'scripts', 'validate.py');

function runCatalog(projectRoot, args, { env } = {}) {
  return execFileAsync(python, [catalogScript, '--project-root', projectRoot, ...args], {
    cwd: repositoryRoot,
    windowsHide: true,
    env: { ...process.env, PYTHONIOENCODING: 'utf-8', XIAOTAO_CURRENT_TIME: '2026-09-10T12:00:00Z', ...env },
  });
}

async function rejectedCommand(promise) {
  try {
    await promise;
  } catch (error) {
    return error;
  }
  assert.fail('Expected command to fail');
}

async function writeProjectFile(projectRoot, relativePath, content) {
  const target = path.join(projectRoot, ...relativePath.split('/'));
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(target, content, 'utf8');
}

function entryFile(entry, { revision = 0, updatedAt = '2026-09-10T01:00:00Z' } = {}) {
  return `---
revision: ${revision}
updated_at: ${updatedAt}
updated_by: old-zhou/test
---

# Long-term Memory Entry

\`\`\`xiaotao-memory-entry
${JSON.stringify(entry)}
\`\`\`
`;
}

async function createMemoryProject(t) {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-catalog-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/performance.md', '# Trace\nVerified startup bottleneck.\n');
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/current.md', `---
revision: 2
updated_at: 2026-09-01T06:00:00Z
updated_by: old-zhou/test
---

# Long-term Memory

\`\`\`xiaotao-memory-entry
{"entry_id":"lt-startup-performance","title":"Startup performance evidence","memory_kind":"experience","content":"Collect a trace before changing homepage initialization.","source_refs":[".xiaotao/evidence/performance.md"],"tags":["performance","trace"],"aliases":["首屏性能"],"status":"active"}
\`\`\`

\`\`\`xiaotao-memory-entry
{"entry_id":"lt-old-workflow","title":"Old workflow preference","memory_kind":"decision","content":"Always use a fixed workflow.","decision_context":{"reason":"The original design optimized for predictable stage order.","rejected_alternatives":[{"alternative":"Dynamic role selection","reason":"It was initially considered harder to test."}]},"source_refs":[".xiaotao/evidence/performance.md"],"tags":["workflow"],"status":"superseded"}
\`\`\`
`);
  await writeProjectFile(projectRoot, '.xiaotao/memory/temporary/active/temp-home/meta.yaml', `id: temp-home
topic: homepage startup investigation
status: active
created_at: 2026-09-01T05:00:00Z
updated_at: 2026-09-01T06:10:00Z
updated_by: old-zhou/test
revision: 3
aliases:
  - 首页启动
`);
  await writeProjectFile(projectRoot, '.xiaotao/memory/temporary/active/temp-home/current.md', `---
revision: 3
updated_at: 2026-09-01T06:10:00Z
updated_by: old-zhou/test
---

# Topic

Homepage startup investigation

## Current goal

Verify whether the analytics SDK must initialize synchronously.

## Confirmed

- A trace shows a long main-thread task.

## Open questions

- Can SDK initialization move after first paint?
`);
  await writeProjectFile(projectRoot, '.xiaotao/tasks/task-cache/task.yaml', `id: task-cache
objective: Reduce cache invalidation latency
status: active
created_at: 2026-09-01T05:30:00Z
updated_at: 2026-09-01T06:20:00Z
updated_by: old-zhou/test
revision: 1
`);
  await writeProjectFile(projectRoot, '.xiaotao/tasks/task-cache/context.md', `# Current state

Cache key analysis is complete.

## Open items

- Verify invalidation fan-out.
`);
  await writeProjectFile(projectRoot, '.xiaotao/tasks/task-cache/workers/cache-observer/current-state.md', `# Objective

Measure cache invalidation fan-out.

## Key findings

- One invalidation touches twelve regions.

## Recommended next

- Add a bounded batch size experiment.
`);
  return projectRoot;
}

test('builds a three-layer catalog and selectively returns one Memory detail', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const legacyPath = path.join(projectRoot, '.xiaotao', 'memory', 'long-term', 'current.md');
  const legacyBefore = await readFile(legacyPath, 'utf8');
  const build = JSON.parse((await runCatalog(projectRoot, ['build'])).stdout);
  assert.equal(build.status, 'built');
  assert.equal(build.entries, 5);
  assert.equal(await readFile(legacyPath, 'utf8'), legacyBefore);
  await assert.rejects(readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-startup-performance.md'), 'utf8'));

  const indexPath = path.join(projectRoot, '.xiaotao', 'memory', 'index.json');
  const index = JSON.parse(await readFile(indexPath, 'utf8'));
  assert.deepEqual(index.entries.map((entry) => entry.memory_id), [
    'lt-old-workflow',
    'lt-startup-performance',
    'task-cache',
    'task-cache.worker-state.cache-observer',
    'temp-home',
  ]);

  await execFileAsync(python, [
    validatorScript,
    'memory-index',
    indexPath,
    '--project-root',
    projectRoot,
  ]);

  const manifest = await readFile(path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md'), 'utf8');
  assert.match(manifest, /Startup performance evidence/);
  assert.match(manifest, /homepage startup investigation/);
  assert.doesNotMatch(manifest, /Always use a fixed workflow/);

  const search = JSON.parse((await runCatalog(projectRoot, ['search', 'homepage trace performance'])).stdout);
  assert.equal(search.candidates[0].memory_id, 'lt-startup-performance');
  assert.equal(search.candidates.some((entry) => entry.memory_id === 'lt-old-workflow'), false);
  assert.match(search.candidates[0].relevance_reason, /tag|title|summary/);

  const detail = JSON.parse((await runCatalog(projectRoot, ['show', 'lt-startup-performance'])).stdout);
  assert.equal(detail.detail.entry_id, 'lt-startup-performance');
  assert.equal(detail.detail.content, 'Collect a trace before changing homepage initialization.');
  assert.doesNotMatch(JSON.stringify(detail.detail), /fixed workflow/);

  const inactive = await rejectedCommand(runCatalog(projectRoot, ['show', 'lt-old-workflow']));
  assert.equal(inactive.code, 2);
  assert.match(inactive.stderr, /unavailable/);
});

test('detects stale catalogs and refreshes them before search', async (t) => {
  const projectRoot = await createMemoryProject(t);
  await runCatalog(projectRoot, ['build']);
  const currentPath = '.xiaotao/memory/temporary/active/temp-home/current.md';
  await writeProjectFile(projectRoot, currentPath, `# Current goal

Investigate hydrationwaterfall latency.

## Open questions

- Which component blocks hydrationwaterfall?
`);

  const stale = await rejectedCommand(runCatalog(projectRoot, ['check']));
  assert.equal(stale.code, 1);
  assert.match(stale.stderr, /missing or stale/);

  const search = JSON.parse((await runCatalog(projectRoot, ['search', 'hydrationwaterfall'])).stdout);
  assert.equal(search.catalog_refreshed, true);
  assert.equal(search.candidates[0].memory_id, 'temp-home');
  const current = JSON.parse((await runCatalog(projectRoot, ['check'])).stdout);
  assert.equal(current.status, 'current');

  const manifestPath = path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md');
  await rm(manifestPath);
  const missingManifest = await rejectedCommand(runCatalog(projectRoot, ['check']));
  assert.equal(missingManifest.code, 1);
  await runCatalog(projectRoot, ['search', 'hydrationwaterfall']);
  assert.match(await readFile(manifestPath, 'utf8'), /Memory Overview/);
});

test('returns no candidate instead of forcing unrelated Memory into context', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const search = JSON.parse((await runCatalog(projectRoot, ['search', 'database backup encryption'])).stdout);
  assert.deepEqual(search.candidates, []);
});

test('rejects unstructured Long-term Memory instead of silently creating a weak index', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-invalid-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/current.md', '# Long-term Memory\n\n- An unstructured claim\n');

  const failure = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(failure.code, 2);
  assert.match(failure.stderr, /xiaotao-memory-entry/);
});

test('rejects decision context on a non-decision Long-term entry', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-decision-invalid-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/source.md', '# Evidence\n');
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/current.md', `# Long-term Memory

\`\`\`xiaotao-memory-entry
{"entry_id":"lt-invalid","title":"Invalid context","memory_kind":"fact","content":"Facts do not carry decision context.","decision_context":{"reason":"Invalid fixture."},"source_refs":[".xiaotao/evidence/source.md"]}
\`\`\`
`);

  const failure = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(failure.code, 2);
  assert.match(failure.stderr, /decision_context.*only.*decision/);
});

test('derives Long-term temporal states and hides non-current knowledge by default', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-temporal-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/source.md', '# Evidence\n');
  const entries = [
    { entry_id: 'lt-timeless', title: 'Policy window timeless', memory_kind: 'fact', content: 'Timeless policy window.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active' },
    { entry_id: 'lt-future', title: 'Policy window future', memory_kind: 'constraint', content: 'Future policy window.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active', valid_from: '2026-09-11T00:00:00Z' },
    { entry_id: 'lt-current', title: 'Policy window current', memory_kind: 'fact', content: 'Current policy window.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active', valid_from: '2026-09-01T00:00:00+08:00', valid_until: '2026-09-11T00:00:00+08:00' },
    { entry_id: 'lt-expired', title: 'Policy window expired', memory_kind: 'fact', content: 'Expired policy window.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active', valid_until: '2026-09-10T12:00:00Z' },
  ];
  for (const entry of entries) {
    await writeProjectFile(projectRoot, `.xiaotao/memory/long-term/entries/${entry.entry_id}.md`, entryFile(entry));
  }
  const expiredPath = path.join(projectRoot, '.xiaotao/memory/long-term/entries/lt-expired.md');
  const expiredBefore = await readFile(expiredPath, 'utf8');

  await runCatalog(projectRoot, ['build']);
  const index = JSON.parse(await readFile(path.join(projectRoot, '.xiaotao/memory/index.json'), 'utf8'));
  assert.deepEqual(Object.fromEntries(index.entries.map((entry) => [entry.memory_id, entry.temporal_state])), {
    'lt-current': 'current',
    'lt-expired': 'expired',
    'lt-future': 'not-yet-valid',
    'lt-timeless': 'timeless',
  });

  const search = JSON.parse((await runCatalog(projectRoot, ['search', 'policy window'])).stdout);
  assert.deepEqual(search.candidates.map((entry) => entry.memory_id).sort(), ['lt-current', 'lt-timeless']);
  const auditSearch = JSON.parse((await runCatalog(projectRoot,
    ['search', 'policy window', '--include-inactive'])).stdout);
  assert.equal(auditSearch.candidates.length, 4);
  assert.equal(auditSearch.candidates.find((entry) => entry.memory_id === 'lt-expired').temporal_state, 'expired');

  const recent = JSON.parse((await runCatalog(projectRoot, ['recent', '--layer', 'long-term'])).stdout);
  assert.deepEqual(recent.entries.map((entry) => entry.memory_id).sort(), ['lt-current', 'lt-timeless']);
  const overview = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json'])).stdout);
  assert.equal(overview.long_term_count, 2);
  const hidden = await rejectedCommand(runCatalog(projectRoot, ['show', 'lt-expired']));
  assert.match(hidden.stderr, /unavailable/);
  const audited = JSON.parse((await runCatalog(projectRoot,
    ['show', 'lt-expired', '--include-inactive'])).stdout);
  assert.equal(audited.memory.temporal_state, 'expired');
  assert.equal(await readFile(expiredPath, 'utf8'), expiredBefore);
});

test('rejects invalid Long-term validity fields', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-temporal-invalid-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/source.md', '# Evidence\n');
  const base = { title: 'Invalid validity', content: 'Invalid.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active' };
  const cases = [
    [{ ...base, entry_id: 'lt-no-zone', memory_kind: 'fact', valid_from: '2026-09-10T00:00:00' }, /valid_from.*timezone/],
    [{ ...base, entry_id: 'lt-reversed', memory_kind: 'constraint', valid_from: '2026-09-11T00:00:00Z', valid_until: '2026-09-10T00:00:00Z' }, /valid_from.*earlier/],
    [{ ...base, entry_id: 'lt-wrong-kind', memory_kind: 'principle', valid_until: '2026-09-10T00:00:00Z' }, /allowed only.*fact.*constraint/],
  ];
  for (const [entry, message] of cases) {
    const target = `.xiaotao/memory/long-term/entries/${entry.entry_id}.md`;
    await writeProjectFile(projectRoot, target, entryFile(entry));
    const failure = await rejectedCommand(runCatalog(projectRoot, ['build']));
    assert.match(failure.stderr, message);
    await rm(path.join(projectRoot, ...target.split('/')));
  }
});

test('indexes split Long-term files and updates one entry without rewriting another', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-split-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/source.md', '# Evidence\n');
  const first = {
    entry_id: 'lt-api-boundary', title: 'API boundary', memory_kind: 'principle',
    content: 'Keep API boundaries explicit.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active',
  };
  const second = {
    entry_id: 'lt-worker-isolation', title: 'Worker isolation', memory_kind: 'decision',
    content: 'Workers receive bounded context.', source_refs: ['.xiaotao/evidence/source.md'], status: 'active',
    decision_context: { reason: 'Bounded context avoids hidden authority.' },
  };
  const old = {
    entry_id: 'lt-fixed-roles', title: 'Fixed roles', memory_kind: 'decision',
    content: 'Use fixed roles.', source_refs: ['.xiaotao/evidence/source.md'], status: 'superseded',
  };
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-api-boundary.md', entryFile(first));
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-worker-isolation.md',
    entryFile(second, { revision: 4, updatedAt: '2026-09-10T02:00:00Z' }));
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/history/lt-fixed-roles.md', entryFile(old));

  await runCatalog(projectRoot, ['build']);
  const index = JSON.parse(await readFile(path.join(projectRoot, '.xiaotao/memory/index.json'), 'utf8'));
  assert.deepEqual(index.entries.map((entry) => entry.path), [
    '.xiaotao/memory/long-term/entries/lt-api-boundary.md',
    '.xiaotao/memory/long-term/history/lt-fixed-roles.md',
    '.xiaotao/memory/long-term/entries/lt-worker-isolation.md',
  ]);
  assert.equal(index.entries.find((entry) => entry.memory_id === 'lt-worker-isolation').updated_at,
    '2026-09-10T02:00:00Z');
  const before = await readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-worker-isolation.md'), 'utf8');
  first.content = 'Keep public API boundaries explicit and versioned.';
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-api-boundary.md',
    entryFile(first, { revision: 1, updatedAt: '2026-09-10T03:00:00Z' }));
  await runCatalog(projectRoot, ['build']);
  assert.equal(await readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-worker-isolation.md'), 'utf8'), before);
  const detail = JSON.parse((await runCatalog(projectRoot, ['show', 'lt-api-boundary'])).stdout);
  assert.equal(detail.detail.content, 'Keep public API boundaries explicit and versioned.');
  const inactive = JSON.parse((await runCatalog(projectRoot,
    ['show', 'lt-fixed-roles', '--include-inactive'])).stdout);
  assert.equal(inactive.detail.status, 'superseded');
});

test('rejects duplicate IDs across legacy and split Long-term sources', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-duplicate-source-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/source.md', '# Evidence\n');
  const entry = {
    entry_id: 'lt-duplicate', title: 'Duplicate', memory_kind: 'fact', content: 'One claim.',
    source_refs: ['.xiaotao/evidence/source.md'], status: 'active',
  };
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/current.md', `# Long-term Memory

\`\`\`xiaotao-memory-entry
${JSON.stringify(entry)}
\`\`\`
`);
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-duplicate.md', entryFile(entry));
  const failure = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(failure.code, 2);
  assert.match(failure.stderr, /duplicate Long-term entry_id/);
});

test('split Long-term files enforce one matching entry and independent metadata', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-memory-invalid-split-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await writeProjectFile(projectRoot, '.xiaotao/evidence/source.md', '# Evidence\n');
  const entry = {
    entry_id: 'lt-right-name', title: 'Right name', memory_kind: 'fact', content: 'A fact.',
    source_refs: ['.xiaotao/evidence/source.md'], status: 'active',
  };
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-wrong-name.md', entryFile(entry));
  let failure = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.match(failure.stderr, /filename must match entry_id/);

  await rm(path.join(projectRoot, '.xiaotao/memory/long-term/entries'), { recursive: true, force: true });
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-right-name.md', `# Entry

\`\`\`xiaotao-memory-entry
${JSON.stringify(entry)}
\`\`\`
`);
  failure = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.match(failure.stderr, /revision.*non-negative integer/);
});

test('explicit migration preserves legacy entries and leaves an auditable snapshot', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const currentPath = path.join(projectRoot, '.xiaotao/memory/long-term/current.md');
  const original = await readFile(currentPath, 'utf8');
  const beforeActive = JSON.parse((await runCatalog(projectRoot,
    ['show', 'lt-startup-performance'])).stdout).detail;
  const beforeInactive = JSON.parse((await runCatalog(projectRoot,
    ['show', 'lt-old-workflow', '--include-inactive'])).stdout).detail;

  const preview = JSON.parse((await runCatalog(projectRoot, ['migrate-long-term'])).stdout);
  assert.equal(preview.status, 'ready');
  assert.equal(preview.entries, 2);
  await assert.rejects(readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-startup-performance.md'), 'utf8'));
  assert.equal(await readFile(currentPath, 'utf8'), original);

  const migrated = JSON.parse((await runCatalog(projectRoot,
    ['migrate-long-term', '--apply', '--actor', 'old-zhou/migration'])).stdout);
  assert.equal(migrated.status, 'migrated');
  assert.equal(migrated.current_entries, 1);
  assert.equal(migrated.history_entries, 1);
  assert.doesNotMatch(await readFile(currentPath, 'utf8'), /xiaotao-memory-entry/);
  assert.equal(await readFile(path.join(projectRoot, ...migrated.legacy_snapshot.split('/')), 'utf8'), original);

  const activeFile = await readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-startup-performance.md'), 'utf8');
  const historyFile = await readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/history/lt-old-workflow.md'), 'utf8');
  assert.match(activeFile, /revision: 2/);
  assert.match(historyFile, /status.*superseded/);
  const afterActive = JSON.parse((await runCatalog(projectRoot,
    ['show', 'lt-startup-performance'])).stdout).detail;
  const afterInactive = JSON.parse((await runCatalog(projectRoot,
    ['show', 'lt-old-workflow', '--include-inactive'])).stdout).detail;
  assert.deepEqual(afterActive, beforeActive);
  assert.deepEqual(afterInactive, beforeInactive);
  const check = JSON.parse((await runCatalog(projectRoot, ['check'])).stdout);
  assert.equal(check.entries, 5);
});

test('mixed-mode migration preserves existing split entries byte for byte', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const existing = {
    entry_id: 'lt-existing-split', title: 'Existing split entry', memory_kind: 'principle',
    content: 'Keep this independently written entry unchanged.',
    source_refs: ['.xiaotao/evidence/performance.md'], status: 'active',
  };
  const existingPath = path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-existing-split.md');
  await writeProjectFile(projectRoot, '.xiaotao/memory/long-term/entries/lt-existing-split.md',
    entryFile(existing, { revision: 7, updatedAt: '2026-09-10T04:00:00Z' }));
  const before = await readFile(existingPath, 'utf8');

  const preview = JSON.parse((await runCatalog(projectRoot, ['migrate-long-term'])).stdout);
  assert.equal(preview.preserved_entries, 1);
  const migrated = JSON.parse((await runCatalog(projectRoot,
    ['migrate-long-term', '--apply', '--actor', 'old-zhou/migration'])).stdout);
  assert.equal(migrated.preserved_entries, 1);
  assert.equal(await readFile(existingPath, 'utf8'), before);
  assert.match(await readFile(path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-startup-performance.md'), 'utf8'),
  /lt-startup-performance/);
  const check = JSON.parse((await runCatalog(projectRoot, ['check'])).stdout);
  assert.equal(check.entries, 6);
});

test('migration ID conflict never changes either storage format', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const currentPath = path.join(projectRoot, '.xiaotao/memory/long-term/current.md');
  const original = await readFile(currentPath, 'utf8');
  const duplicate = {
    entry_id: 'lt-startup-performance', title: 'Duplicate', memory_kind: 'experience',
    content: 'A conflicting split copy.', source_refs: ['.xiaotao/evidence/performance.md'],
    status: 'active',
  };
  const duplicatePath = path.join(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-startup-performance.md');
  await writeProjectFile(projectRoot,
    '.xiaotao/memory/long-term/entries/lt-startup-performance.md', entryFile(duplicate));
  const duplicateBefore = await readFile(duplicatePath, 'utf8');
  const collision = await rejectedCommand(runCatalog(projectRoot,
    ['migrate-long-term', '--apply', '--actor', 'old-zhou/migration']));
  assert.equal(collision.code, 2);
  assert.match(collision.stderr, /duplicate Long-term entry_id/);
  assert.equal(await readFile(currentPath, 'utf8'), original);
  assert.equal(await readFile(duplicatePath, 'utf8'), duplicateBefore);

  await rm(path.join(projectRoot, '.xiaotao/memory/long-term/entries'), { recursive: true, force: true });
  const missingActor = await rejectedCommand(runCatalog(projectRoot, ['migrate-long-term', '--apply']));
  assert.equal(missingActor.code, 2);
  assert.match(missingActor.stderr, /--actor/);
  assert.equal(await readFile(currentPath, 'utf8'), original);
});

test('indexes long-term search hints, ranks by them with search hint reason, and preserves them on migration', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const hintEntry = {
    entry_id: 'lt-with-hints',
    title: 'Startup tracing guidance',
    memory_kind: 'experience',
    content: 'Always capture CPU profiles before touching bootstrap.',
    source_refs: ['.xiaotao/evidence/performance.md'],
    tags: ['startup'],
    aliases: ['启动分析'],
    search_hints: ['优化启动性能', 'how to profile startup'],
    status: 'active',
  };
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/entries/lt-with-hints.md',
    entryFile(hintEntry, { revision: 1, updatedAt: '2026-09-10T02:00:00Z' })
  );

  const build = JSON.parse((await runCatalog(projectRoot, ['build'])).stdout);
  assert.equal(build.status, 'built');

  const indexPath = path.join(projectRoot, '.xiaotao', 'memory', 'index.json');
  const index = JSON.parse(await readFile(indexPath, 'utf8'));
  const stored = index.entries.find((e) => e.memory_id === 'lt-with-hints');
  assert.ok(stored);
  assert.deepEqual(stored.search_hints, ['优化启动性能', 'how to profile startup']);
  assert.equal(stored.stale, null);

  const search = JSON.parse((await runCatalog(projectRoot, ['search', 'how to profile startup'])).stdout);
  const match = search.candidates.find((e) => e.memory_id === 'lt-with-hints');
  assert.ok(match);
  assert.match(match.relevance_reason, /search hint/);

  // Validate index file with validatorScript
  await execFileAsync(python, [validatorScript, 'memory-index', indexPath, '--project-root', projectRoot]);
});

test('calculates active temporary staleness, marks stale in manifest and index, and honors config threshold', async (t) => {
  const projectRoot = await createMemoryProject(t);
  // temp-home has updated_at 2026-09-01T06:10:00Z (9 days before 2026-09-10) -> stale by default (7 days)
  await runCatalog(projectRoot, ['build']);

  const indexPath = path.join(projectRoot, '.xiaotao', 'memory', 'index.json');
  let index = JSON.parse(await readFile(indexPath, 'utf8'));
  let tempHome = index.entries.find((e) => e.memory_id === 'temp-home');
  assert.ok(tempHome);
  assert.equal(tempHome.stale, true);

  let manifest = await readFile(path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md'), 'utf8');
  assert.match(manifest, /homepage startup investigation.*\(updated 2026-09-01, stale\)/);

  // Write a fresh temporary entry
  await writeProjectFile(projectRoot, '.xiaotao/memory/temporary/active/temp-fresh/meta.yaml', `id: temp-fresh
topic: fresh exploration
status: active
created_at: 2026-09-09T10:00:00Z
updated_at: 2026-09-09T10:00:00Z
updated_by: old-zhou/test
revision: 1
`);
  await writeProjectFile(projectRoot, '.xiaotao/memory/temporary/active/temp-fresh/current.md', `# Topic\nFresh exploration\n`);

  await runCatalog(projectRoot, ['build']);
  index = JSON.parse(await readFile(indexPath, 'utf8'));
  const tempFresh = index.entries.find((e) => e.memory_id === 'temp-fresh');
  assert.ok(tempFresh);
  assert.equal(tempFresh.stale, false);

  manifest = await readFile(path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md'), 'utf8');
  assert.match(manifest, /fresh exploration.*\(updated 2026-09-09\)/);
  assert.doesNotMatch(manifest, /fresh exploration.*stale/);

  // Test time-crossing threshold via --now argument:
  // At 2026-09-05 (4 days after 2026-09-01), temp-home is fresh (within 7 days)
  await runCatalog(projectRoot, ['build', '--now', '2026-09-05T12:00:00Z']);
  index = JSON.parse(await readFile(indexPath, 'utf8'));
  assert.equal(index.entries.find((e) => e.memory_id === 'temp-home').stale, false);
  // Checking at 2026-09-10 (9 days after) detects catalog is stale because temp-home crossed threshold
  const thresholdCrossedCheck = await rejectedCommand(runCatalog(projectRoot, ['check', '--now', '2026-09-10T12:00:00Z']));
  assert.equal(thresholdCrossedCheck.code, 1);

  // Configure custom threshold in .xiaotao/config.yaml: 14 days
  await writeProjectFile(projectRoot, '.xiaotao/config.yaml', `temporary_stale_days: 14\n`);
  // Since config changed, check detects catalog is stale
  const staleCheck = await rejectedCommand(runCatalog(projectRoot, ['check']));
  assert.equal(staleCheck.code, 1);

  // Rebuild with 14 days threshold: 9-day-old temp-home is not stale
  await runCatalog(projectRoot, ['build']);
  index = JSON.parse(await readFile(indexPath, 'utf8'));
  tempHome = index.entries.find((e) => e.memory_id === 'temp-home');
  assert.equal(tempHome.stale, false);

  manifest = await readFile(path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md'), 'utf8');
  assert.match(manifest, /homepage startup investigation.*\(updated 2026-09-01\)/);
  assert.doesNotMatch(manifest, /homepage startup investigation.*stale/);

  // Timezone enforcement on --now and XIAOTAO_CURRENT_TIME:
  // Offset-aware timestamp with non-UTC offset succeeds
  await runCatalog(projectRoot, ['build', '--now', '2026-09-10T20:00:00+08:00']);
  await runCatalog(projectRoot, ['build'], { env: { XIAOTAO_CURRENT_TIME: '2026-09-10T20:00:00+08:00' } });

  // Naive timestamps without timezone offset fail
  const naiveNowErr = await rejectedCommand(runCatalog(projectRoot, ['build', '--now', '2026-09-10T12:00:00']));
  assert.equal(naiveNowErr.code, 2);
  assert.match(naiveNowErr.stderr, /--now timestamp must include timezone offset/);

  const naiveEnvErr = await rejectedCommand(runCatalog(projectRoot, ['build'], { env: { XIAOTAO_CURRENT_TIME: '2026-09-10T12:00:00' } }));
  assert.equal(naiveEnvErr.code, 2);
  assert.match(naiveEnvErr.stderr, /XIAOTAO_CURRENT_TIME must include timezone offset/);
});

test('indexes pending follow-ups, exposes them in manifest, and supports show command', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const pendingYaml = `followup_id: cleanup-redis
title: Clean up obsolete Redis cluster nodes
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
related_ids:
  - lt-startup-performance
`;
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/cleanup-redis.yaml', pendingYaml);

  const build = JSON.parse((await runCatalog(projectRoot, ['build'])).stdout);
  assert.equal(build.status, 'built');

  const indexPath = path.join(projectRoot, '.xiaotao', 'memory', 'index.json');
  const index = JSON.parse(await readFile(indexPath, 'utf8'));
  assert.ok(Array.isArray(index.pending_followups));
  assert.equal(index.pending_followups.length, 1);
  const item = index.pending_followups[0];
  assert.equal(item.followup_id, 'cleanup-redis');
  assert.equal(item.title, 'Clean up obsolete Redis cluster nodes');
  assert.equal(item.status, 'pending');
  assert.deepEqual(item.source_refs, ['.xiaotao/evidence/performance.md']);
  assert.deepEqual(item.related_ids, ['lt-startup-performance']);
  assert.equal(item.path, '.xiaotao/memory/followups/pending/cleanup-redis.yaml');

  // Validate index via validator
  await execFileAsync(python, [validatorScript, 'memory-index', indexPath, '--project-root', projectRoot]);

  // Check manifest
  const manifest = await readFile(path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md'), 'utf8');
  assert.match(manifest, /## Pending follow-ups/);
  assert.match(manifest, /- \*\*Clean up obsolete Redis cluster nodes\*\* \(`cleanup-redis`\) — from \.xiaotao\/evidence\/performance\.md/);

  // Show pending followup
  const showPending = JSON.parse((await runCatalog(projectRoot, ['show', 'cleanup-redis'])).stdout);
  assert.equal(showPending.followup.followup_id, 'cleanup-redis');
  assert.equal(showPending.followup.title, 'Clean up obsolete Redis cluster nodes');
  assert.equal(showPending.followup.status, 'pending');

  // Resolve the follow-up
  await rm(path.join(projectRoot, '.xiaotao/memory/followups/pending/cleanup-redis.yaml'));
  const resolvedYaml = `followup_id: cleanup-redis
title: Clean up obsolete Redis cluster nodes
status: resolved
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
related_ids:
  - lt-startup-performance
resolved_at: 2026-09-10T12:00:00Z
resolution: Decommissioned old nodes and updated routing config.
resolution_refs:
  - .xiaotao/evidence/performance.md
`;
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/resolved/cleanup-redis.yaml', resolvedYaml);

  await runCatalog(projectRoot, ['build']);
  const updatedIndex = JSON.parse(await readFile(indexPath, 'utf8'));
  assert.deepEqual(updatedIndex.pending_followups, []);

  const updatedManifest = await readFile(path.join(projectRoot, '.xiaotao', 'memory', 'manifest.md'), 'utf8');
  assert.match(updatedManifest, /## Pending follow-ups\s+- None/);

  // show without --include-inactive should fail
  const unavailable = await rejectedCommand(runCatalog(projectRoot, ['show', 'cleanup-redis']));
  assert.equal(unavailable.code, 2);
  assert.match(unavailable.stderr, /Follow-up 'cleanup-redis' is unavailable/);

  // show with --include-inactive succeeds
  const showResolved = JSON.parse((await runCatalog(projectRoot, ['show', 'cleanup-redis', '--include-inactive'])).stdout);
  assert.equal(showResolved.followup.status, 'resolved');
  assert.equal(showResolved.followup.resolution, 'Decommissioned old nodes and updated routing config.');
});

test('rejects invalid follow-ups on schema, status mismatch, filename, or ID collision', async (t) => {
  const projectRoot = await createMemoryProject(t);

  // 1. Filename mismatch
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/mismatched.yaml', `followup_id: correct-id
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
`);
  let err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /filename must match followup_id/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups/pending/mismatched.yaml'));

  // 2. Status mismatch (resolved in pending folder)
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/wrong-status.yaml', `followup_id: wrong-status
title: Some Title
status: resolved
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
resolved_at: 2026-09-10T12:00:00Z
resolution: done
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /status must be 'pending' in pending directory/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups/pending/wrong-status.yaml'));

  // 3. Pending has resolution forbidden fields
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/has-resolution.yaml', `followup_id: has-resolution
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
resolved_at: 2026-09-10T12:00:00Z
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /\$\.resolved_at: is not allowed/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups/pending/has-resolution.yaml'));

  // 4. Invalid source_refs (non-existent file)
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/bad-ref.yaml', `followup_id: bad-ref
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/missing-file.md
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /invalid follow-up/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups/pending/bad-ref.yaml'));

  // 5. Duplicate ID across pending and resolved
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/dup-id.yaml', `followup_id: dup-id
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
`);
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/resolved/dup-id.yaml', `followup_id: dup-id
title: Some Title
status: resolved
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
resolved_at: 2026-09-10T12:00:00Z
resolution: done
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /duplicate followup_id/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups'), { recursive: true, force: true });

  // 6. Followup ID collides with memory_id
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/lt-startup-performance.yaml', `followup_id: lt-startup-performance
title: Colliding Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /collides with memory_id/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups'), { recursive: true, force: true });

  // 7. Unknown field disallowed (additionalProperties: false)
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/unknown-field.yaml', `followup_id: unknown-field
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
unknown_field: disallowed
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /\$\.unknown_field: is not allowed/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups'), { recursive: true, force: true });

  // 8. Duplicate source_refs disallowed (uniqueItems: true)
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/dup-source-refs.yaml', `followup_id: dup-source-refs
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
  - .xiaotao/evidence/performance.md
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /\$\.source_refs\[1\]: must be unique/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups'), { recursive: true, force: true });

  // 9. Duplicate related_ids disallowed (uniqueItems: true)
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/dup-related-ids.yaml', `followup_id: dup-related-ids
title: Some Title
status: pending
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
related_ids:
  - lt-startup-performance
  - lt-startup-performance
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /\$\.related_ids\[1\]: must be unique/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups'), { recursive: true, force: true });

  // 10. Duplicate resolution_refs disallowed in resolved follow-ups (uniqueItems: true)
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/resolved/dup-resolution-refs.yaml', `followup_id: dup-resolution-refs
title: Some Title
status: resolved
created_at: 2026-09-09T12:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
resolved_at: 2026-09-10T12:00:00Z
resolution: Decommissioned old nodes
resolution_refs:
  - .xiaotao/evidence/performance.md
  - .xiaotao/evidence/performance.md
`);
  err = await rejectedCommand(runCatalog(projectRoot, ['build']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /\$\.resolution_refs\[1\]: must be unique/);
  await rm(path.join(projectRoot, '.xiaotao/memory/followups'), { recursive: true, force: true });
});

test('recent returns bounded metadata ordered by recency with degradation rules', async (t) => {
  const projectRoot = await createMemoryProject(t);
  await runCatalog(projectRoot, ['build']);

  // 1. Default recent query (limit 5)
  const defaultRecent = JSON.parse((await runCatalog(projectRoot, ['recent'])).stdout);
  assert.equal(defaultRecent.command, 'recent');
  assert.equal(defaultRecent.limit, 5);
  assert.equal(defaultRecent.catalog_refreshed, false);
  assert.equal(defaultRecent.entries.length, 4);

  // Check ordering:
  // 1st: task-cache (06:20)
  // 2nd: temp-home (06:10)
  // 3rd: lt-startup-performance (06:00)
  // 4th: task-cache.worker-state.cache-observer (no timestamp, degraded to end)
  assert.equal(defaultRecent.entries[0].memory_id, 'task-cache');
  assert.equal(defaultRecent.entries[0].updated_at, '2026-09-01T06:20:00Z');
  assert.equal(defaultRecent.entries[1].memory_id, 'temp-home');
  assert.equal(defaultRecent.entries[1].updated_at, '2026-09-01T06:10:00Z');
  assert.equal(defaultRecent.entries[2].memory_id, 'lt-startup-performance');
  assert.equal(defaultRecent.entries[2].updated_at, '2026-09-01T06:00:00Z');
  assert.equal(defaultRecent.entries[3].memory_id, 'task-cache.worker-state.cache-observer');
  assert.equal(defaultRecent.entries[3].updated_at, null);

  // Check bounded fields: search_hints, content, tags, aliases are NOT present
  for (const entry of defaultRecent.entries) {
    assert.equal(entry.search_hints, undefined);
    assert.equal(entry.content, undefined);
    assert.equal(entry.tags, undefined);
    assert.equal(entry.aliases, undefined);
    assert.ok(entry.memory_id);
    assert.ok(entry.layer);
    assert.ok(entry.record_type);
    assert.ok(entry.title);
    assert.ok(entry.summary);
    assert.ok(entry.status);
    assert.ok(entry.path);
    assert.ok(entry.locator);
  }

  // 2. Limit parameter
  const limited = JSON.parse((await runCatalog(projectRoot, ['recent', '--limit', '2'])).stdout);
  assert.equal(limited.limit, 2);
  assert.equal(limited.entries.length, 2);
  assert.equal(limited.entries[0].memory_id, 'task-cache');
  assert.equal(limited.entries[1].memory_id, 'temp-home');

  // Limit boundary checks (< 1 or > 5)
  const errLow = await rejectedCommand(runCatalog(projectRoot, ['recent', '--limit', '0']));
  assert.equal(errLow.code, 2);
  assert.match(errLow.stderr, /--limit must be between 1 and 5/);

  const errHigh = await rejectedCommand(runCatalog(projectRoot, ['recent', '--limit', '6']));
  assert.equal(errHigh.code, 2);
  assert.match(errHigh.stderr, /--limit must be between 1 and 5/);

  // 3. Layer filter
  const tempOnly = JSON.parse((await runCatalog(projectRoot, ['recent', '--layer', 'temporary'])).stdout);
  assert.equal(tempOnly.layer, 'temporary');
  assert.equal(tempOnly.entries.length, 1);
  assert.equal(tempOnly.entries[0].memory_id, 'temp-home');

  const taskOnly = JSON.parse((await runCatalog(projectRoot, ['recent', '--layer', 'task'])).stdout);
  assert.equal(taskOnly.layer, 'task');
  assert.equal(taskOnly.entries.length, 2);
  assert.equal(taskOnly.entries[0].memory_id, 'task-cache');
  assert.equal(taskOnly.entries[1].memory_id, 'task-cache.worker-state.cache-observer');

  // 4. Include inactive
  const withInactive = JSON.parse((await runCatalog(projectRoot, ['recent', '--include-inactive'])).stdout);
  assert.equal(withInactive.entries.length, 5);
  // lt-old-workflow (06:00) vs lt-startup-performance (06:00): lt-old-workflow sorts first alphabetically
  assert.equal(withInactive.entries[2].memory_id, 'lt-old-workflow');
  assert.equal(withInactive.entries[2].status, 'superseded');
  assert.equal(withInactive.entries[3].memory_id, 'lt-startup-performance');
  assert.equal(withInactive.entries[4].memory_id, 'task-cache.worker-state.cache-observer');

  // 5. Automatic refresh on stale index
  await writeProjectFile(projectRoot, '.xiaotao/memory/temporary/active/temp-new/meta.yaml', `id: temp-new
topic: new temporary exploration
status: active
created_at: 2026-09-02T01:00:00Z
updated_at: 2026-09-02T02:00:00Z
updated_by: old-zhou/test
revision: 1
`);
  const refreshedRecent = JSON.parse((await runCatalog(projectRoot, ['recent'])).stdout);
  assert.equal(refreshedRecent.catalog_refreshed, true);
  // temp-new (09-02) is newer than 09-01
  assert.equal(refreshedRecent.entries[0].memory_id, 'temp-new');
  assert.equal(refreshedRecent.entries[0].updated_at, '2026-09-02T02:00:00Z');
});

test('overview outputs lightweight manifest text or structured json summary with freshness handling', async (t) => {
  const projectRoot = await createMemoryProject(t);

  // 1. Overview when catalog is missing builds and prints manifest text by default
  const textOutput = (await runCatalog(projectRoot, ['overview'])).stdout;
  assert.match(textOutput, /# Memory Overview/);
  assert.match(textOutput, /## Active Temporary Memory/);
  assert.match(textOutput, /temp-home/);
  assert.match(textOutput, /## Active Tasks/);
  assert.match(textOutput, /task-cache/);
  assert.match(textOutput, /## Long-term Memory/);
  assert.match(textOutput, /lt-startup-performance/);
  assert.match(textOutput, /1 current Worker state\(s\)/);

  // 2. Overview with --format json outputs structured summary
  const jsonOutput = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json'])).stdout);
  assert.equal(jsonOutput.catalog_refreshed, false); // Already built in step 1
  assert.equal(jsonOutput.has_active_work, true);
  assert.equal(jsonOutput.active_temporary_count, 1);
  assert.equal(jsonOutput.active_temporaries[0].memory_id, 'temp-home');
  assert.equal(jsonOutput.active_task_count, 1);
  assert.equal(jsonOutput.active_tasks[0].memory_id, 'task-cache');
  assert.equal(jsonOutput.long_term_count, 1); // Only active long-term entries (lt-startup-performance)
  assert.equal(jsonOutput.worker_state_count, 1);
  assert.equal(jsonOutput.pending_followup_count, 0);

  // 3. Stale catalog is automatically refreshed
  await writeProjectFile(projectRoot, '.xiaotao/memory/followups/pending/followup-startup.yaml', `followup_id: followup-startup
title: Track startup trace
status: pending
created_at: 2026-09-02T10:00:00Z
source_refs:
  - .xiaotao/evidence/performance.md
`);
  const refreshedJson = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json'])).stdout);
  assert.equal(refreshedJson.catalog_refreshed, true);
  assert.equal(refreshedJson.pending_followup_count, 1);
  assert.equal(refreshedJson.pending_followups[0].followup_id, 'followup-startup');
  assert.equal(refreshedJson.has_active_work, true);

  // 4. Stale catalog with --no-refresh raises visible error
  await writeProjectFile(projectRoot, '.xiaotao/tasks/task-cache-2/task.yaml', `id: task-cache-2
objective: Second cache experiment
status: active
created_at: 2026-09-02T11:00:00Z
updated_at: 2026-09-02T11:00:00Z
updated_by: old-zhou/test
revision: 1
`);
  const unrefreshedErr = await rejectedCommand(runCatalog(projectRoot, ['overview', '--format', 'json', '--no-refresh']));
  assert.equal(unrefreshedErr.code, 2);
  assert.match(unrefreshedErr.stderr, /missing or stale/);

  // 5. Cached startup overview validates and reads the existing snapshot without deriving sources
  const cachedJson = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json', '--cached'])).stdout);
  assert.equal(cachedJson.catalog_cached, true);
  assert.equal(cachedJson.catalog_refreshed, false);
  assert.equal(cachedJson.active_task_count, 1);
  assert.ok(!cachedJson.active_tasks.some((entry) => entry.memory_id === 'task-cache-2'));

  // 6. Corrupt catalog fails with explicit diagnostic instead of silently reporting empty memory
  await writeProjectFile(projectRoot, '.xiaotao/tasks/task-corrupt/task.yaml', `id: wrong-id
objective: Invalid task id mismatch
status: active
`);
  const err = await rejectedCommand(runCatalog(projectRoot, ['overview']));
  assert.equal(err.code, 2);
  assert.match(err.stderr, /memory catalog error/);
});

test('overview strictly bounds model runtime context and supports --full and --limit', async (t) => {
  const projectRoot = await createMemoryProject(t);

  // Add 4 active tasks
  for (let i = 1; i <= 4; i++) {
    await writeProjectFile(projectRoot, `.xiaotao/tasks/task-batch-${i}/task.yaml`, `id: task-batch-${i}
objective: Task batch item ${i}
status: active
created_at: 2026-09-03T10:0${i}:00Z
updated_at: 2026-09-03T10:0${i}:00Z
updated_by: old-zhou/test
revision: 1
`);
  }

  // 1. Default JSON bounds to 3 entries and reports total count and more_tasks
  const jsonOutput = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json'])).stdout);
  assert.equal(jsonOutput.active_task_count, 5); // task-cache + 4 batch tasks
  assert.equal(jsonOutput.active_tasks.length, 3);
  assert.equal(jsonOutput.more_tasks, 2);
  assert.equal(jsonOutput.bounded_limit, 3);

  // 2. Custom --limit 2 bounds to 2 entries
  const limitJson = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json', '--limit', '2'])).stdout);
  assert.equal(limitJson.active_tasks.length, 2);
  assert.equal(limitJson.more_tasks, 3);

  // 3. Limit validation errors out for < 1 or > 5
  const errLow = await rejectedCommand(runCatalog(projectRoot, ['overview', '--limit', '0']));
  assert.equal(errLow.code, 2);
  assert.match(errLow.stderr, /--limit must be between 1 and 5/);
  const errHigh = await rejectedCommand(runCatalog(projectRoot, ['overview', '--limit', '6']));
  assert.equal(errHigh.code, 2);
  assert.match(errHigh.stderr, /--limit must be between 1 and 5/);

  // 4. Default text format includes truncation notice
  const textOutput = (await runCatalog(projectRoot, ['overview'])).stdout;
  assert.match(textOutput, /另外 2 项活动任务已省略/);

  // 5. --full flag prints full un-truncated manifest
  const fullOutput = (await runCatalog(projectRoot, ['overview', '--full'])).stdout;
  assert.match(fullOutput, /task-batch-1/);
  assert.match(fullOutput, /task-batch-4/);
  assert.doesNotMatch(fullOutput, /已省略/);
});

test('overview discovers recoverable checkpoints and marks has_active_work even without active tasks', async (t) => {
  const projectRoot = await createMemoryProject(t);

  // Archive or remove all existing active tasks and temporaries
  await rm(path.join(projectRoot, '.xiaotao/tasks'), { recursive: true, force: true });
  await rm(path.join(projectRoot, '.xiaotao/memory/temporary'), { recursive: true, force: true });

  // Rebuild empty catalog
  await runCatalog(projectRoot, ['build']);

  // Before checkpoint: has_active_work is false
  const beforeJson = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json'])).stdout);
  assert.equal(beforeJson.has_active_work, false);
  assert.equal(beforeJson.recoverable_checkpoint, null);

  // Add a recoverable checkpoint under .xiaotao/tasks/task-recovery/references/checkpoints/req-recovery-1.json
  const checkpointPayload = {
    schema_version: 1,
    request_id: 'req-recovery-1',
    project: 'test-project',
    kind: 'task',
    target_id: 'task-recovery',
    session_id: 'session-test',
    base_revision: 2,
    base_hash: 'a'.repeat(64),
    input_hash: 'b'.repeat(64),
    source_hash: 'c'.repeat(64),
    proposal: 'proposal content',
    proposal_hash: 'd'.repeat(64),
    snapshot: {
      objective: 'Recovered task objective',
      confirmed: ['step 1 done'],
      rejected: [],
      in_progress: ['step 2'],
      next: ['step 3'],
      open_questions: [],
      source_refs: ['.xiaotao/tasks/task-recovery/progress.md'],
    },
  };
  await writeProjectFile(
    projectRoot,
    '.xiaotao/tasks/task-recovery/references/checkpoints/req-recovery-1.json',
    JSON.stringify(checkpointPayload, null, 2),
  );

  // After checkpoint: has_active_work is true and recoverable_checkpoint is populated
  const afterJson = JSON.parse((await runCatalog(projectRoot, ['overview', '--format', 'json'])).stdout);
  assert.equal(afterJson.has_active_work, true);
  assert.ok(afterJson.recoverable_checkpoint);
  assert.equal(afterJson.recoverable_checkpoint.available, true);
  assert.equal(afterJson.recoverable_checkpoint.scope, 'task');
  assert.equal(afterJson.recoverable_checkpoint.binding, 'task-recovery');
  assert.equal(afterJson.recoverable_checkpoint.revision, 3);
  assert.equal(afterJson.recoverable_checkpoint.request_id, 'req-recovery-1');

  // Text output contains ## Recoverable Checkpoint block
  const textOutput = (await runCatalog(projectRoot, ['overview'])).stdout;
  assert.match(textOutput, /当前检测到项目存在活动工作/);
  assert.match(textOutput, /## Recoverable Checkpoint/);
  assert.match(textOutput, /Recoverable checkpoint: yes/);
  assert.match(textOutput, /scope: task/);
  assert.match(textOutput, /binding: task-recovery/);
  assert.match(textOutput, /revision: 3/);
});

test('searches memory with synonym expansion and ranks exact matches above synonyms', async (t) => {
  const projectRoot = await createMemoryProject(t);
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/entries/lt-auth.md',
    entryFile({
      entry_id: 'lt-auth',
      title: '统一鉴权中心设计',
      memory_kind: 'decision',
      content: '所有微服务必须通过统一鉴权中心校验 token。',
      source_refs: ['.xiaotao/evidence/performance.md'],
      tags: ['token', 'gateway'],
      status: 'active',
    }),
  );
  await runCatalog(projectRoot, ['build']);

  const searchSyn = JSON.parse((await runCatalog(projectRoot, ['search', 'auth'])).stdout);
  assert.equal(searchSyn.candidates.length, 1);
  assert.equal(searchSyn.candidates[0].memory_id, 'lt-auth');
  assert.match(searchSyn.candidates[0].relevance_reason, /synonym/);

  const searchExact = JSON.parse((await runCatalog(projectRoot, ['search', '鉴权'])).stdout);
  assert.equal(searchExact.candidates[0].memory_id, 'lt-auth');
  assert.match(searchExact.candidates[0].relevance_reason, /title/);
  assert.doesNotMatch(searchExact.candidates[0].relevance_reason, /title \(synonym\)/);
  assert.ok(searchExact.candidates[0].score > searchSyn.candidates[0].score);
});

test('loads custom synonym groups from .xiaotao/config.yaml', async (t) => {
  const projectRoot = await createMemoryProject(t);
  await writeProjectFile(
    projectRoot,
    '.xiaotao/config.yaml',
    `synonyms:
  - "checkout, checkoutflow, 结账"
`,
  );
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/entries/lt-payment.md',
    entryFile({
      entry_id: 'lt-payment',
      title: '结账流程异常重试',
      memory_kind: 'experience',
      content: '结账流程遇网络波动支持指数退避重试。',
      source_refs: ['.xiaotao/evidence/performance.md'],
      status: 'active',
    }),
  );
  await runCatalog(projectRoot, ['build']);

  const search = JSON.parse((await runCatalog(projectRoot, ['search', 'checkoutflow'])).stdout);
  assert.equal(search.candidates.length, 1);
  assert.equal(search.candidates[0].memory_id, 'lt-payment');
  assert.match(search.candidates[0].relevance_reason, /synonym/);
});

test('promotes active temporary to formal task, archives temporary, and refreshes catalog', async (t) => {
  const projectRoot = await createMemoryProject(t);
  await runCatalog(projectRoot, ['build']);

  const promoteResult = JSON.parse(
    (await runCatalog(projectRoot, ['promote-temporary', 'temp-home', '--task-id', 'task-home-opt', '--actor', 'test-actor'])).stdout
  );
  assert.equal(promoteResult.status, 'promoted');
  assert.equal(promoteResult.temporary_id, 'temp-home');
  assert.equal(promoteResult.task_id, 'task-home-opt');
  assert.equal(promoteResult.task_path, '.xiaotao/tasks/task-home-opt/task.yaml');

  const taskYamlPath = path.join(projectRoot, '.xiaotao/tasks/task-home-opt/task.yaml');
  const taskYaml = await readFile(taskYamlPath, 'utf8');
  assert.match(taskYaml, /id: task-home-opt/);
  assert.match(taskYaml, /source_temporary: "temp-home"/);
  assert.match(taskYaml, /promotion_transaction: "tx-promote-temp-home-/);
  assert.match(taskYaml, /promoted_at:/);
  assert.match(taskYaml, /updated_by: "test-actor"/);

  const progressMdPath = path.join(projectRoot, '.xiaotao/tasks/task-home-opt/progress.md');
  const progressMd = await readFile(progressMdPath, 'utf8');
  assert.match(progressMd, /# Task Progress: homepage startup investigation/);
  assert.match(progressMd, /Verify whether the analytics SDK must initialize synchronously/);
  assert.match(progressMd, /A trace shows a long main-thread task/);
  assert.match(progressMd, /Can SDK initialization move after first paint\?/);

  const activeTempDir = path.join(projectRoot, '.xiaotao/memory/temporary/active/temp-home');
  await assert.rejects(readFile(path.join(activeTempDir, 'meta.yaml')));

  const archivedMetaPath = path.join(projectRoot, '.xiaotao/memory/temporary/archived/temp-home/meta.yaml');
  const archivedMeta = await readFile(archivedMetaPath, 'utf8');
  assert.match(archivedMeta, /status: archive/);
  assert.match(archivedMeta, /revision: 4/);
  assert.match(archivedMeta, /updated_by: "test-actor"/);

  const archivedCurrentPath = path.join(projectRoot, '.xiaotao/memory/temporary/archived/temp-home/current.md');
  const archivedCurrent = await readFile(archivedCurrentPath, 'utf8');
  assert.match(archivedCurrent, /## Promoted to Task/);
  assert.match(archivedCurrent, /Promoted to Task `task-home-opt`/);

  const indexPath = path.join(projectRoot, '.xiaotao', 'memory', 'index.json');
  const index = JSON.parse(await readFile(indexPath, 'utf8'));
  const promotedEntry = index.entries.find((entry) => entry.memory_id === 'task-home-opt');
  assert.ok(promotedEntry);
  assert.equal(promotedEntry.status, 'active');

  const tempEntry = index.entries.find((entry) => entry.memory_id === 'temp-home');
  assert.equal(tempEntry, undefined);

  // Verify memory-index validates cleanly
  await execFileAsync(python, [
    validatorScript,
    'memory-index',
    indexPath,
    '--project-root',
    projectRoot,
  ]);
});

test('fails to promote non-existent or already archived temporary', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const nonExistent = await rejectedCommand(
    runCatalog(projectRoot, ['promote-temporary', 'temp-missing'])
  );
  assert.equal(nonExistent.code, 2);
  assert.match(nonExistent.stderr, /Active Temporary 'temp-missing' does not exist/);
});


test('does not overwrite an existing archived temporary during promotion', async (t) => {
  const projectRoot = await createMemoryProject(t);
  const archivedMarker = '.xiaotao/memory/temporary/archived/temp-home/keep.txt';
  await writeProjectFile(projectRoot, archivedMarker, 'preserve this archive');

  const failure = await rejectedCommand(
    runCatalog(projectRoot, ['promote-temporary', 'temp-home', '--task-id', 'task-home-opt'])
  );
  assert.equal(failure.code, 2);
  assert.match(failure.stderr, /Archived Temporary already exists/);
  assert.equal(await readFile(path.join(projectRoot, archivedMarker), 'utf8'), 'preserve this archive');
  await readFile(path.join(projectRoot, '.xiaotao/memory/temporary/active/temp-home/meta.yaml'));
  await assert.rejects(readFile(path.join(projectRoot, '.xiaotao/tasks/task-home-opt/task.yaml')));
});

test('rolls back temporary promotion when catalog persistence fails', async (t) => {
  const projectRoot = await createMemoryProject(t);
  await runCatalog(projectRoot, ['build']);

  const activeDir = path.join(projectRoot, '.xiaotao/memory/temporary/active/temp-home');
  const originalMeta = await readFile(path.join(activeDir, 'meta.yaml'), 'utf8');
  const originalCurrent = await readFile(path.join(activeDir, 'current.md'), 'utf8');
  const indexPath = path.join(projectRoot, '.xiaotao/memory/index.json');
  const manifestPath = path.join(projectRoot, '.xiaotao/memory/manifest.md');
  const originalIndex = await readFile(indexPath);
  const originalManifest = await readFile(manifestPath);

  const failAfterPersist = [
    'import runpy',
    'import sys',
    'from pathlib import Path',
    'script = Path(sys.argv[1]).resolve()',
    'project_root = Path(sys.argv[2])',
    'sys.path.insert(0, str(script.parent))',
    'namespace = runpy.run_path(str(script), run_name="memory_catalog_test")',
    'function_globals = namespace["promote_temporary"].__globals__',
    'real_persist = function_globals["persist_catalog"]',
    'def fail_after_persist(root, catalog):',
    '    real_persist(root, catalog)',
    '    raise RuntimeError("injected catalog persistence failure")',
    'function_globals["persist_catalog"] = fail_after_persist',
    'namespace["promote_temporary"](',
    '    project_root,',
    '    "temp-home",',
    '    task_id="task-home-opt",',
    '    now=namespace["resolve_reference_time"]("2026-09-10T12:00:00Z"),',
    ')',
  ].join('\n');

  await assert.rejects(
    execFileAsync(python, ['-c', failAfterPersist, catalogScript, projectRoot], {
      cwd: repositoryRoot,
      windowsHide: true,
      env: { ...process.env, PYTHONIOENCODING: 'utf-8' },
    }),
    /injected catalog persistence failure/,
  );

  assert.equal(await readFile(path.join(activeDir, 'meta.yaml'), 'utf8'), originalMeta);
  assert.equal(await readFile(path.join(activeDir, 'current.md'), 'utf8'), originalCurrent);
  await assert.rejects(readFile(path.join(projectRoot, '.xiaotao/memory/temporary/archived/temp-home/meta.yaml')));
  await assert.rejects(readFile(path.join(projectRoot, '.xiaotao/tasks/task-home-opt/task.yaml')));
  assert.deepEqual(await readFile(indexPath), originalIndex);
  assert.deepEqual(await readFile(manifestPath), originalManifest);
});
