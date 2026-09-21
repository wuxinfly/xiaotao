import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { access, mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);
const repositoryRoot = path.resolve(import.meta.dirname, '..');
const python = process.platform === 'win32' ? 'python' : 'python3';
const activityScript = path.join(repositoryRoot, 'xiaotao', 'scripts', 'activity_catalog.py');

function runActivity(projectRoot, args) {
  return execFileAsync(python, [activityScript, '--project-root', projectRoot, ...args], {
    cwd: repositoryRoot,
    windowsHide: true,
  });
}

test('immutable Worker approval records become stable Activity events', async (t) => {
  const projectRoot = await createProject(t);
  await seedWorkerApproval(projectRoot);

  await runActivity(projectRoot, ['build', '--now', '2026-09-12T03:00:00Z']);
  const first = JSON.parse(await readFile(path.join(projectRoot, '.xiaotao/activity/index.json'), 'utf8'));
  assert.equal(first.events.length, 1);
  assert.equal(first.events[0].event_type, 'worker_approved');
  assert.equal(first.events[0].occurred_at, '2026-09-12T02:00:00Z');
  assert.equal(first.events[0].title, '小林（运行时排查）');
  assert.deepEqual(first.events[0].source_refs, [
    '.xiaotao/workers/approvals/approve-runtime-investigator-r3.approval.json',
  ]);

  await runActivity(projectRoot, ['build', '--now', '2026-09-12T04:00:00Z']);
  const rebuilt = JSON.parse(await readFile(path.join(projectRoot, '.xiaotao/activity/index.json'), 'utf8'));
  assert.equal(rebuilt.events[0].event_id, first.events[0].event_id);
  const searched = parseJson(await runActivity(projectRoot, [
    'search', '--year', '2026', '--event-type', 'worker_approved',
  ]));
  assert.equal(searched.total, 1);
});

test('projects without Worker approval records do not guess approval events', async (t) => {
  const projectRoot = await createProject(t);
  await writeProjectFile(projectRoot, '.xiaotao/workers/registry.yaml', [
    'schema_version: 1', 'id: project-workers', 'source: project', 'revision: 9',
    'updated_at: 2026-09-12T00:00:00Z', 'updated_by: old-zhou/test',
    'workers: []', 'aliases: {}', '',
  ].join('\n'));
  const result = parseJson(await runActivity(projectRoot, ['build']));
  assert.equal(result.events, 0);
});

test('invalid Worker approval authorities fail Activity rebuilding', async (t) => {
  const projectRoot = await createProject(t);
  await seedWorkerApproval(projectRoot, { id: 'approve-runtime-investigator-r3' }, 'wrong-name');
  const filenameError = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(filenameError.stderr, /filename must match approval_id/);

  await rm(path.join(projectRoot, '.xiaotao', 'workers'), { recursive: true, force: true });
  await seedWorkerApproval(projectRoot, { approvedAt: 'not-a-time' });
  const schemaError = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(schemaError.stderr, /invalid Worker approval/);
});

function parseJson(result) {
  return JSON.parse(result.stdout);
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
  return target;
}

async function createProject(t) {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-activity-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  return projectRoot;
}

function taskYaml({
  id = 'task-a',
  objective = '完成示例任务',
  status = 'completed',
  completedAt = '2026-09-01T10:30:00+08:00',
  updatedAt = '2026-09-02T00:00:00Z',
  sourceTemporary,
  promotionTransaction,
  promotedAt,
} = {}) {
  const lines = [
    `id: ${id}`,
    `objective: ${objective}`,
    `status: ${status}`,
    'created_at: 2026-08-31T00:00:00Z',
    `updated_at: ${updatedAt}`,
    'updated_by: old-zhou/test',
    'revision: 1',
  ];
  if (typeof sourceTemporary === 'string') lines.push(`source_temporary: ${sourceTemporary}`);
  if (typeof promotionTransaction === 'string') {
    lines.push(`promotion_transaction: ${promotionTransaction}`);
  }
  if (typeof promotedAt === 'string') lines.push(`promoted_at: ${promotedAt}`);
  if (typeof completedAt === 'string') lines.push(`completed_at: ${completedAt}`);
  return `${lines.join('\n')}\n`;
}

async function seedTask(projectRoot, options = {}, root = '.xiaotao/tasks') {
  const id = options.id ?? 'task-a';
  return writeProjectFile(projectRoot, `${root}/${id}/task.yaml`, taskYaml(options));
}

function decisionRecord({
  id,
  title,
  outcome = 'approved',
  decidedAt,
  reason = '已有证据支持该决定。',
  supersededBy,
  targetIds = ['lt-example'],
} = {}) {
  const record = {
    schema_version: 1,
    record_type: 'decision',
    decision_id: id,
    title,
    outcome,
    importance: 'milestone',
    decided_at: decidedAt,
    decided_by: 'old-zhou/test',
    reason,
    target_ids: targetIds,
    source_refs: ['.xiaotao/evidence/decision.md'],
  };
  if (supersededBy !== undefined) record.superseded_by = supersededBy;
  return `${JSON.stringify(record, null, 2)}\n`;
}

async function seedDecision(projectRoot, options, fileId = options.id) {
  await writeProjectFile(projectRoot, '.xiaotao/evidence/decision.md', 'decision evidence\n');
  return writeProjectFile(
    projectRoot,
    `.xiaotao/memory/long-term/decisions/${fileId}.decision.json`,
    decisionRecord(options),
  );
}

async function seedPlaybookDecision(projectRoot, options, fileId = options.id) {
  await writeProjectFile(projectRoot, '.xiaotao/evidence/decision.md', 'decision evidence\n');
  return writeProjectFile(
    projectRoot,
    `.xiaotao/playbooks/decisions/${fileId}.decision.json`,
    decisionRecord({ targetIds: ['pb-20260829t000000z-a1b2'], ...options }),
  );
}

function workerApproval({
  id = 'approve-runtime-investigator-r3',
  workerId = 'runtime-investigator',
  workerName = '小林（运行时排查）',
  approvedAt = '2026-09-12T10:00:00+08:00',
} = {}) {
  return `${JSON.stringify({
    schema_version: 1,
    record_type: 'worker-approval',
    approval_id: id,
    worker_id: workerId,
    worker_name: workerName,
    registry_id: 'project-workers',
    registry_revision: 3,
    worker_digest: 'a'.repeat(64),
    approved_at: approvedAt,
    approved_by: 'old-zhou/test',
  }, null, 2)}\n`;
}

async function seedWorkerApproval(projectRoot, options = {}, fileId = options.id ?? 'approve-runtime-investigator-r3') {
  return writeProjectFile(
    projectRoot,
    `.xiaotao/workers/approvals/${fileId}.approval.json`,
    workerApproval(options),
  );
}

const sha256 = (content) => createHash('sha256').update(content).digest('hex');

async function seedCheckpointObservation(projectRoot, {
  kind = 'task', targetId = 'checkpoint-task', requestId = 'checkpoint_recovery_1',
  completion = 'recovery', committedAt = '2026-09-12T11:28:04.506Z', lifecycle = 'active',
} = {}) {
  let root;
  if (kind === 'task') {
    root = lifecycle === 'archive'
      ? `.xiaotao/tasks/archive/${targetId}`
      : `.xiaotao/tasks/${targetId}`;
    await seedTask(projectRoot, {
      id: targetId, objective: 'Checkpoint target', status: lifecycle === 'archive' ? 'archive' : 'active',
      completedAt: null,
    }, lifecycle === 'archive' ? '.xiaotao/tasks/archive' : '.xiaotao/tasks');
  } else {
    root = `.xiaotao/memory/temporary/${lifecycle}/${targetId}`;
    await writeProjectFile(projectRoot, `${root}/meta.yaml`, [
      `id: ${targetId}`, 'topic: Checkpoint target', `status: ${lifecycle === 'archive' ? 'archive' : 'active'}`,
      'created_at: 2026-09-01T00:00:00Z', 'updated_at: 2026-09-01T00:00:00Z',
      'updated_by: old-zhou/test', 'revision: 1', '',
    ].join('\n'));
  }
  const request = JSON.stringify({
    schema_version: 1, request_id: requestId, project: projectRoot, kind, target_id: targetId,
    session_id: 'session-test', base_revision: 7, base_hash: 'a'.repeat(64),
    input_hash: 'b'.repeat(64), snapshot: { objective: '恢复关键工作', confirmed: [], rejected: [],
      in_progress: [], next: [], open_questions: [], source_refs: [] }, source_hash: 'c'.repeat(64),
    proposal: 'proposal', proposal_hash: 'd'.repeat(64),
  });
  const directory = `${root}/references/checkpoints`;
  await writeProjectFile(projectRoot, `${directory}/${requestId}.json`, request);
  const observation = {
    request_id: requestId, record_hash: sha256(request), proposal_hash: 'd'.repeat(64), revision: 8,
  };
  if (completion !== 'legacy') Object.assign(observation, { completion, committed_at: committedAt });
  const observationPath = `${directory}/${requestId}.committed.json`;
  await writeProjectFile(projectRoot, observationPath, JSON.stringify(observation));
  return { observationPath, requestPath: `${directory}/${requestId}.json` };
}

test('explicit retry completion becomes one deterministic checkpoint recovery event', async (t) => {
  const projectRoot = await createProject(t);
  const { observationPath } = await seedCheckpointObservation(projectRoot);

  const first = parseJson(await runActivity(projectRoot, [
    'search', '--month', '2026-09', '--event-type', 'checkpoint_recovered',
  ]));
  assert.equal(first.total, 1);
  assert.equal(first.events[0].occurred_at, '2026-09-12T11:28:04Z');
  assert.equal(first.events[0].title, '恢复 checkpoint：恢复关键工作');
  assert.deepEqual(first.events[0].source_refs, [observationPath]);

  await rm(path.join(projectRoot, '.xiaotao', 'activity'), { recursive: true, force: true });
  const rebuilt = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(rebuilt.events[0].event_id, first.events[0].event_id);
});

test('save and legacy checkpoint observations are omitted without guessing recovery events', async (t) => {
  const projectRoot = await createProject(t);
  await seedCheckpointObservation(projectRoot, { requestId: 'checkpoint_save_1', completion: 'save' });
  await seedCheckpointObservation(projectRoot, {
    targetId: 'legacy-task', requestId: 'checkpoint_legacy_1', completion: 'legacy',
  });
  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 0);
});

test('the same checkpoint request id on different targets produces distinct events', async (t) => {
  const projectRoot = await createProject(t);
  await seedCheckpointObservation(projectRoot, { targetId: 'checkpoint-a', requestId: 'shared_request' });
  await seedCheckpointObservation(projectRoot, { targetId: 'checkpoint-b', requestId: 'shared_request' });
  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 2);
  assert.notEqual(result.events[0].event_id, result.events[1].event_id);
});

test('recovery observations require an intact adjacent request and invalidate the Activity cache', async (t) => {
  const projectRoot = await createProject(t);
  const seeded = await seedCheckpointObservation(projectRoot, { kind: 'temporary', targetId: 'temp-recovery' });
  const before = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(before.total, 1);
  await rm(path.join(projectRoot, ...seeded.requestPath.split('/')));
  const failure = await rejectedCommand(runActivity(projectRoot, ['search', '--year', '2026']));
  assert.match(failure.stderr, /recovery request record is missing or unsafe/);
});

test('Temporary metadata without checkpoints is not parsed as recovery authority', async (t) => {
  const projectRoot = await createProject(t);
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/temporary/active/unrelated/meta.yaml',
    'topic: unrelated malformed metadata\n',
  );
  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 0);
});

test('duplicate active and archived Temporary checkpoint targets fail with the target identity', async (t) => {
  const projectRoot = await createProject(t);
  await seedCheckpointObservation(projectRoot, { kind: 'temporary', targetId: 'duplicate-temp' });
  await seedCheckpointObservation(projectRoot, {
    kind: 'temporary', targetId: 'duplicate-temp', lifecycle: 'archive', requestId: 'archived_request',
  });
  const failure = await rejectedCommand(runActivity(projectRoot, ['search', '--year', '2026']));
  assert.match(failure.stderr, /duplicate checkpoint target 'temporary\/duplicate-temp'/);
});

test('completed Task automatically becomes a UTC Activity event without an event journal', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot);

  const result = parseJson(await runActivity(projectRoot, [
    'search', '--month', '2026-09', '--now', '2026-09-10T00:00:00Z',
  ]));

  assert.equal(result.catalog_refreshed, true);
  assert.equal(result.total, 1);
  assert.equal(result.events[0].event_type, 'task_completed');
  assert.equal(result.events[0].occurred_at, '2026-09-01T02:30:00Z');
  assert.deepEqual(result.events[0].source_refs, ['.xiaotao/tasks/task-a/task.yaml']);
  await assert.rejects(access(path.join(projectRoot, '.xiaotao', 'activity', 'events')));
});

test('active and legacy completed Tasks are omitted instead of guessing timestamps', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, { id: 'active-task', status: 'active', completedAt: null });
  await seedTask(projectRoot, {
    id: 'legacy-task', status: 'completed', completedAt: null, updatedAt: '2026-09-08T00:00:00Z',
  });

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 0);
  assert.deepEqual(result.events, []);
});

test('Task metadata files below artifacts or references are ignored', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot);
  await writeProjectFile(
    projectRoot,
    '.xiaotao/tasks/task-a/artifacts/task.yaml',
    taskYaml({ id: 'task-a', objective: '不应进入时间线' }),
  );
  await writeProjectFile(
    projectRoot,
    '.xiaotao/tasks/task-a/references/task.yaml',
    '这不是 Task 元数据，也不应让构建失败\n',
  );

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 1);
  assert.equal(result.events[0].title, '完成示例任务');
});

test('moving a Task to archive keeps the event and refreshes its source reference', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot);
  const before = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));

  const activeDir = path.join(projectRoot, '.xiaotao', 'tasks', 'task-a');
  const archiveDir = path.join(projectRoot, '.xiaotao', 'tasks', 'archive', 'task-a');
  await mkdir(path.dirname(archiveDir), { recursive: true });
  await rename(activeDir, archiveDir);

  const after = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(after.catalog_refreshed, true);
  assert.equal(after.events[0].event_id, before.events[0].event_id);
  assert.deepEqual(after.events[0].source_refs, ['.xiaotao/tasks/archive/task-a/task.yaml']);
  await access(path.join(projectRoot, ...after.events[0].source_refs[0].split('/')));
});

test('a promoted Task becomes a promotion event at its promotion time', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'promoted',
    objective: '优化登录流程',
    status: 'active',
    completedAt: null,
    sourceTemporary: '20260831-登录流程梳理',
    promotionTransaction: '20260831T120000Z-p7q8r9',
    promotedAt: '2026-08-31T12:00:15Z',
  });

  const result = parseJson(await runActivity(projectRoot, ['search', '--month', '2026-08']));
  assert.equal(result.total, 1);
  const [event] = result.events;
  assert.equal(event.event_type, 'temporary_promoted');
  assert.equal(event.occurred_at, '2026-08-31T12:00:15Z');
  assert.notEqual(
    event.occurred_at,
    '2026-08-31T12:00:00Z',
    'the transaction-open marker time must not be used as the event time',
  );
  assert.equal(event.title, '优化登录流程');
  assert.match(event.summary, /20260831-登录流程梳理/);
  assert.deepEqual(event.source_refs, ['.xiaotao/tasks/promoted/task.yaml']);

  const filtered = parseJson(await runActivity(projectRoot, [
    'search', '--year', '2026', '--event-type', 'temporary_promoted',
  ]));
  assert.equal(filtered.total, 1);
  const completions = parseJson(await runActivity(projectRoot, [
    'search', '--year', '2026', '--event-type', 'task_completed',
  ]));
  assert.equal(completions.total, 0);
});

test('a promoted and completed Task keeps promotion and completion events distinct', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'promoted-done',
    objective: '优化登录流程',
    status: 'completed',
    completedAt: '2026-09-02T09:00:00Z',
    sourceTemporary: '20260831-登录流程梳理',
    promotionTransaction: '20260831T120000Z-p7q8r9',
    promotedAt: '2026-08-31T12:00:15Z',
  });

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 2);
  assert.deepEqual(
    result.events.map((event) => event.event_type),
    ['temporary_promoted', 'task_completed'],
  );
  assert.notEqual(result.events[0].event_id, result.events[1].event_id);
});

test('promoted Tasks without promoted_at never invent a promotion event', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'legacy-promoted',
    objective: '旧提升记录',
    status: 'completed',
    completedAt: '2026-09-03T00:00:00Z',
    sourceTemporary: '20260901-主题',
    promotionTransaction: '20260831T120000Z-p7q8r9',
  });
  await seedTask(projectRoot, {
    id: 'source-only',
    objective: '只有来源没有晋升时间',
    status: 'completed',
    completedAt: '2026-09-04T00:00:00Z',
    sourceTemporary: '20260901-主题',
    promotionTransaction: '20260901T000000Z-abcdef12',
  });

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 2);
  assert.deepEqual(
    result.events.map((event) => event.event_type),
    ['task_completed', 'task_completed'],
  );
  assert.ok(
    result.events.every((event) => event.event_type !== 'temporary_promoted'),
    'a well-formed promotion_transaction alone must never produce a promotion event',
  );
});

test('a preparing Task never projects a promotion event', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'not-committed',
    objective: '尚未提交的晋升',
    status: 'preparing',
    completedAt: null,
    sourceTemporary: '20260905-主题',
    promotionTransaction: '20260905T080000Z-abcdef12',
    promotedAt: '2026-09-05T08:00:00Z',
  });

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 0);
});

test('a malformed promoted_at fails the build', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'bad-promotion-time',
    objective: '晋升时间非法',
    status: 'active',
    completedAt: null,
    sourceTemporary: '20260901-主题',
    promotionTransaction: '20260901T000000Z-abcdef12',
    promotedAt: 'not-a-time',
  });

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /invalid timestamp/);
});

test('promoted_at without source_temporary fails the build', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'orphan-promotion-time',
    objective: '缺少来源的晋升时间',
    status: 'active',
    completedAt: null,
    promotedAt: '2026-09-05T08:00:00Z',
  });

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /'promoted_at' requires a non-empty 'source_temporary'/);
});

test('moving a promoted Task to archive keeps its promotion event stable', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, {
    id: 'promoted',
    objective: '优化登录流程',
    status: 'archive',
    completedAt: null,
    sourceTemporary: '20260831-登录流程梳理',
    promotionTransaction: '20260831T120000Z-p7q8r9',
    promotedAt: '2026-08-31T12:00:15Z',
  });
  const before = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));

  const activeDir = path.join(projectRoot, '.xiaotao', 'tasks', 'promoted');
  const archiveDir = path.join(projectRoot, '.xiaotao', 'tasks', 'archive', 'promoted');
  await mkdir(path.dirname(archiveDir), { recursive: true });
  await rename(activeDir, archiveDir);

  const after = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(after.catalog_refreshed, true);
  assert.equal(after.events[0].event_id, before.events[0].event_id);
  assert.equal(after.events[0].event_type, 'temporary_promoted');
  assert.equal(after.events[0].occurred_at, '2026-08-31T12:00:15Z');
  assert.deepEqual(after.events[0].source_refs, ['.xiaotao/tasks/archive/promoted/task.yaml']);
});

test('approved and superseded Decision Records become Activity events', async (t) => {
  const projectRoot = await createProject(t);
  await seedDecision(projectRoot, {
    id: 'decision-approve',
    title: '采用派生 Activity',
    decidedAt: '2026-09-04T10:00:00+08:00',
  });
  await seedDecision(projectRoot, {
    id: 'decision-supersede',
    title: '取代旧索引方案',
    outcome: 'superseded',
    decidedAt: '2026-09-05T03:00:00Z',
    supersededBy: 'decision-approve',
  });
  await seedDecision(projectRoot, {
    id: 'decision-reject',
    title: '拒绝事件日志方案',
    outcome: 'rejected',
    decidedAt: '2026-09-06T03:00:00Z',
  });
  const routine = JSON.parse(decisionRecord({
    id: 'decision-routine',
    title: '普通评审决定',
    decidedAt: '2026-09-07T03:00:00Z',
  }));
  routine.importance = 'routine';
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/decisions/decision-routine.decision.json',
    `${JSON.stringify(routine)}\n`,
  );

  const result = parseJson(await runActivity(projectRoot, ['search', '--month', '2026-09']));
  assert.equal(result.total, 2);
  assert.deepEqual(
    result.events.map((event) => event.event_type),
    ['decision_approved', 'decision_superseded'],
  );
  assert.equal(result.events[0].occurred_at, '2026-09-04T02:00:00Z');
  assert.deepEqual(result.events[0].source_refs, [
    '.xiaotao/memory/long-term/decisions/decision-approve.decision.json',
  ]);

  const approved = parseJson(await runActivity(projectRoot, [
    'search', '--year', '2026', '--event-type', 'decision_approved',
  ]));
  assert.equal(approved.total, 1);
  assert.equal(approved.events[0].title, '采用派生 Activity');
});

test('legacy and nested Decision files are ignored', async (t) => {
  const projectRoot = await createProject(t);
  await seedDecision(projectRoot, {
    id: 'decision-current',
    title: '规范决定',
    decidedAt: '2026-09-04T02:00:00Z',
  });
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/decisions/legacy.json',
    '{old unstructured record',
  );
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/decisions/archive/nested.decision.json',
    '{broken nested record',
  );

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 1);
  assert.equal(result.events[0].title, '规范决定');
});

test('missing historical Decision evidence does not block Activity rebuild', async (t) => {
  const projectRoot = await createProject(t);
  await seedDecision(projectRoot, {
    id: 'decision-local-evidence',
    title: '依赖本地 Task 证据的决定',
    decidedAt: '2026-09-04T02:00:00Z',
  });
  await rm(path.join(projectRoot, '.xiaotao', 'evidence', 'decision.md'));

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 1);
  assert.equal(result.events[0].title, '依赖本地 Task 证据的决定');
});

test('unsafe Decision evidence paths still block Activity rebuild', async (t) => {
  const projectRoot = await createProject(t);
  const unsafe = JSON.parse(decisionRecord({
    id: 'decision-unsafe-ref',
    title: '包含越界证据路径的决定',
    decidedAt: '2026-09-04T02:00:00Z',
  }));
  unsafe.source_refs = ['../outside.md'];
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/decisions/decision-unsafe-ref.decision.json',
    `${JSON.stringify(unsafe)}\n`,
  );

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /invalid Decision Record.*must not contain.*\.\./);
});

test('invalid canonical Decision Records fail instead of inventing an event', async (t) => {
  const projectRoot = await createProject(t);
  await writeProjectFile(projectRoot, '.xiaotao/evidence/decision.md', 'decision evidence\n');
  const invalid = JSON.parse(decisionRecord({
    id: 'decision-invalid',
    title: '缺少可靠时间',
    decidedAt: '2026-09-04T02:00:00Z',
  }));
  delete invalid.decided_at;
  await writeProjectFile(
    projectRoot,
    '.xiaotao/memory/long-term/decisions/decision-invalid.decision.json',
    `${JSON.stringify(invalid)}\n`,
  );

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /invalid Decision Record.*decided_at/);
});

test('adding an authoritative Decision Record makes the Activity index stale', async (t) => {
  const projectRoot = await createProject(t);
  await runActivity(projectRoot, ['build', '--now', '2026-09-10T00:00:00Z']);
  await seedDecision(projectRoot, {
    id: 'decision-after-build',
    title: '索引构建后批准的新决定',
    decidedAt: '2026-09-10T01:00:00Z',
  });

  const stale = await rejectedCommand(runActivity(projectRoot, ['check']));
  assert.equal(stale.code, 1);
  assert.match(stale.stderr, /missing or stale/);

  const refreshed = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(refreshed.catalog_refreshed, true);
  assert.equal(refreshed.events[0].event_type, 'decision_approved');
});

test('approved and superseded Playbook Decision Records become Activity events', async (t) => {
  const projectRoot = await createProject(t);
  await seedPlaybookDecision(projectRoot, {
    id: 'playbook-approve',
    title: '证据优先的性能诊断',
    decidedAt: '2026-09-08T10:00:00+08:00',
  });
  await seedPlaybookDecision(projectRoot, {
    id: 'playbook-supersede',
    title: '旧版诊断流程',
    outcome: 'superseded',
    decidedAt: '2026-09-09T03:00:00Z',
    supersededBy: 'playbook-approve',
  });
  await seedPlaybookDecision(projectRoot, {
    id: 'playbook-reject',
    title: '证据不足的候选',
    outcome: 'rejected',
    decidedAt: '2026-09-10T03:00:00Z',
  });
  const routine = JSON.parse(decisionRecord({
    id: 'playbook-routine',
    title: '日常 Playbook 调整',
    decidedAt: '2026-09-11T03:00:00Z',
  }));
  routine.importance = 'routine';
  await writeProjectFile(
    projectRoot,
    '.xiaotao/playbooks/decisions/playbook-routine.decision.json',
    `${JSON.stringify(routine)}\n`,
  );

  const result = parseJson(await runActivity(projectRoot, ['search', '--month', '2026-09']));
  assert.equal(result.total, 2);
  assert.deepEqual(
    result.events.map((event) => event.event_type),
    ['playbook_approved', 'playbook_superseded'],
  );
  assert.equal(result.events[0].occurred_at, '2026-09-08T02:00:00Z');
  assert.equal(result.events[0].summary, '批准 Playbook：已有证据支持该决定。');
  assert.deepEqual(result.events[0].source_refs, [
    '.xiaotao/playbooks/decisions/playbook-approve.decision.json',
  ]);

  const approved = parseJson(await runActivity(projectRoot, [
    'search', '--year', '2026', '--event-type', 'playbook_approved',
  ]));
  assert.equal(approved.total, 1);
  assert.equal(approved.events[0].title, '证据优先的性能诊断');

  const memoryDecisions = parseJson(await runActivity(projectRoot, [
    'search', '--year', '2026', '--event-type', 'decision_approved',
  ]));
  assert.equal(memoryDecisions.total, 0);
});

test('Playbook candidates and canonical Playbook files never become events', async (t) => {
  const projectRoot = await createProject(t);
  await writeProjectFile(
    projectRoot,
    '.xiaotao/playbooks/performance-diagnosis.md',
    [
      '---',
      'playbook_id: pb-20260829t000000z-a1b2',
      'file_path: .xiaotao/playbooks/performance-diagnosis.md',
      'title: 证据优先的性能诊断',
      'status: active',
      'revision: 3',
      'updated_at: 2026-09-05T00:00:00Z',
      'updated_by: old-zhou/test',
      '---',
      '',
      '步骤。',
      '',
    ].join('\n'),
  );
  await writeProjectFile(
    projectRoot,
    '.xiaotao/playbooks/candidates/pb-candidate.md',
    '---\ncandidate_id: pb-candidate\nstatus: candidate\nupdated_at: 2026-09-06T00:00:00Z\n---\n\n候选。\n',
  );

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(result.total, 0);
  assert.deepEqual(result.events, []);
});

test('a Playbook Decision Record filename must match decision_id', async (t) => {
  const projectRoot = await createProject(t);
  await seedPlaybookDecision(projectRoot, {
    id: 'playbook-real',
    title: '文件名与 ID 不一致',
    decidedAt: '2026-09-08T02:00:00Z',
  }, 'playbook-mismatched');

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(
    error.stderr,
    /filename must match decision_id as 'playbook-real\.decision\.json'/,
  );
});

test('invalid Playbook Decision Records fail instead of inventing events', async (t) => {
  const projectRoot = await createProject(t);
  await writeProjectFile(projectRoot, '.xiaotao/evidence/decision.md', 'decision evidence\n');
  const invalid = JSON.parse(decisionRecord({
    id: 'playbook-invalid',
    title: '缺少可靠时间',
    decidedAt: '2026-09-08T02:00:00Z',
  }));
  delete invalid.decided_at;
  await writeProjectFile(
    projectRoot,
    '.xiaotao/playbooks/decisions/playbook-invalid.decision.json',
    `${JSON.stringify(invalid)}\n`,
  );

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /invalid Playbook Decision Record.*decided_at/);
});

test('unsafe Playbook evidence paths still block Activity rebuild', async (t) => {
  const projectRoot = await createProject(t);
  const unsafe = JSON.parse(decisionRecord({
    id: 'playbook-unsafe-ref',
    title: '包含越界证据路径的评审记录',
    decidedAt: '2026-09-08T02:00:00Z',
  }));
  unsafe.source_refs = ['../outside.md'];
  await writeProjectFile(
    projectRoot,
    '.xiaotao/playbooks/decisions/playbook-unsafe-ref.decision.json',
    `${JSON.stringify(unsafe)}\n`,
  );

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /invalid Playbook Decision Record.*must not contain.*\.\./);
});

test('search filters by year, month, and from/to range', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, { id: 'august', objective: '八月事件', completedAt: '2026-08-15T10:00:00Z' });
  await seedTask(projectRoot, { id: 'september', objective: '九月事件', completedAt: '2026-09-10T10:00:00Z' });
  await seedTask(projectRoot, { id: 'october', objective: '十月事件', completedAt: '2026-10-01T10:00:00Z' });

  assert.equal(parseJson(await runActivity(projectRoot, ['search', '--year', '2026'])).total, 3);

  const byMonth = parseJson(await runActivity(projectRoot, ['search', '--month', '2026-09']));
  assert.equal(byMonth.total, 1);
  assert.equal(byMonth.events[0].title, '九月事件');

  const byRange = parseJson(await runActivity(projectRoot, [
    'search', '--from', '2026-09-01', '--to', '2026-09-30',
  ]));
  assert.equal(byRange.total, 1);
  assert.equal(byRange.events[0].title, '九月事件');
});

test('search limits output to the newest matches while reporting the full total', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, { id: 'first', objective: '第一项', completedAt: '2026-09-01T00:00:00Z' });
  await seedTask(projectRoot, { id: 'second', objective: '第二项', completedAt: '2026-09-02T00:00:00Z' });
  await seedTask(projectRoot, { id: 'third', objective: '第三项', completedAt: '2026-09-03T00:00:00Z' });

  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026', '--limit', '2']));
  assert.equal(result.total, 3);
  assert.deepEqual(result.events.map((event) => event.title), ['第二项', '第三项']);
});

test('a malformed completed_at fails clearly', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot, { completedAt: 'not-a-time' });

  const error = await rejectedCommand(runActivity(projectRoot, ['build']));
  assert.match(error.stderr, /activity catalog error: invalid timestamp/);
});

test('missing and corrupt indexes rebuild from authoritative Tasks', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot);

  const first = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(first.catalog_refreshed, true);

  const indexPath = path.join(projectRoot, '.xiaotao', 'activity', 'index.json');
  await writeFile(indexPath, '{broken', 'utf8');
  const second = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.equal(second.catalog_refreshed, true);
  assert.equal(second.total, 1);
  const repairedIndex = await readFile(indexPath, 'utf8');
  assert.doesNotThrow(() => JSON.parse(repairedIndex));
});

test('check reports stale after an authoritative Task lifecycle change', async (t) => {
  const projectRoot = await createProject(t);
  const taskPath = await seedTask(projectRoot, { status: 'active', completedAt: null });
  await runActivity(projectRoot, ['build']);
  assert.equal(parseJson(await runActivity(projectRoot, ['check'])).status, 'current');

  await writeFile(taskPath, taskYaml({ status: 'completed' }), 'utf8');
  const error = await rejectedCommand(runActivity(projectRoot, ['check']));
  assert.match(error.stderr, /missing or stale/);
});

test('malformed Task YAML and duplicate Task IDs fail instead of being skipped', async (t) => {
  const malformedRoot = await createProject(t);
  await writeProjectFile(malformedRoot, '.xiaotao/tasks/broken/task.yaml', 'id: [broken\n');
  const malformed = await rejectedCommand(runActivity(malformedRoot, ['build']));
  assert.match(malformed.stderr, /activity catalog error/);

  const duplicateRoot = await createProject(t);
  await seedTask(duplicateRoot, { id: 'same' });
  await seedTask(duplicateRoot, { id: 'same' }, '.xiaotao/tasks/archive');
  const duplicate = await rejectedCommand(runActivity(duplicateRoot, ['build']));
  assert.match(duplicate.stderr, /duplicate Task id 'same'/);
});


test('rejecting a new Playbook candidate preserves audit without blocking Activity rebuild', async (t) => {
  const projectRoot = await createProject(t);
  await seedTask(projectRoot);
  await runActivity(projectRoot, ['build']);
  const candidatePath = '.xiaotao/playbooks/candidates/new.json';
  await writeProjectFile(projectRoot, candidatePath, '{"candidate_id":"new","action":"CREATE"}\n');
  const record = JSON.parse(decisionRecord({
    id: 'reject-new', title: '拒绝全新流程', outcome: 'rejected',
    decidedAt: '2026-09-08T02:00:00Z', targetIds: [],
  }));
  record.source_refs = [candidatePath];
  await writeProjectFile(projectRoot, '.xiaotao/playbooks/decisions/reject-new.decision.json', JSON.stringify(record));
  const stale = await rejectedCommand(runActivity(projectRoot, ['check']));
  assert.match(stale.stderr, /stale/);
  const result = parseJson(await runActivity(projectRoot, ['search', '--year', '2026']));
  assert.deepEqual(result.events.map((event) => event.event_type), ['task_completed']);
  await rm(path.join(projectRoot, candidatePath));
  await runActivity(projectRoot, ['build']);
  await runActivity(projectRoot, ['check']);
});

test('approved and superseded Playbook records still require a target', async (t) => {
  for (const outcome of ['approved', 'superseded']) {
    const projectRoot = await createProject(t);
    await seedPlaybookDecision(projectRoot, {
      id: 'empty-target', title: '无目标记录', outcome,
      decidedAt: '2026-09-08T02:00:00Z', targetIds: [],
      ...(outcome === 'superseded' ? { supersededBy: 'replacement' } : {}),
    });
    const error = await rejectedCommand(runActivity(projectRoot, ['build']));
    assert.match(error.stderr, /target_ids.*must contain at least 1/);
  }
});
