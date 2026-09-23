import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);
const repositoryRoot = path.resolve(import.meta.dirname, '..');
const python = process.platform === 'win32' ? 'python' : 'python3';
const timelineScript = path.join(repositoryRoot, 'xiaotao', 'scripts', 'timeline_catalog.py');

function runTimeline(projectRoot, args) {
  return execFileAsync(python, [timelineScript, '--project-root', projectRoot, ...args], {
    cwd: repositoryRoot,
    windowsHide: true,
  });
}

async function createProject(t) {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-timeline-test-'));
  t?.after(async () => {
    await rm(dir, { recursive: true, force: true });
  });
  return dir;
}

async function writeProjectFile(projectRoot, relativePath, content) {
  const full = path.join(projectRoot, relativePath);
  await mkdir(path.dirname(full), { recursive: true });
  await writeFile(full, content, 'utf8');
}

test('builds hierarchical timeline with year, month, and day levels from archived task', async (t) => {
  const projectRoot = await createProject(t);

  // Seed an archived task
  const taskYaml = [
    'id: 20260923-login',
    'objective: 优化登录体验',
    'status: completed',
    'created_at: 2026-09-23T08:00:00Z',
    'updated_at: 2026-09-23T12:00:00Z',
    'updated_by: xiao-tao/test',
    'revision: 2',
    'completed_at: 2026-09-23T12:00:00Z',
  ].join('\n') + '\n';

  await writeProjectFile(projectRoot, '.xiaotao/tasks/archive/20260923-login/task.yaml', taskYaml);
  await writeProjectFile(
    projectRoot,
    '.xiaotao/tasks/archive/20260923-login/completion.md',
    '# 完成：优化登录体验\n\n已成功优化登录流程并完成端到端测试验证。\n',
  );

  // Run build
  const { stdout: buildOut } = await runTimeline(projectRoot, ['build']);
  const buildResult = JSON.parse(buildOut);
  assert.equal(buildResult.status, 'built');
  assert.equal(buildResult.event_count, 1);
  assert.deepEqual(buildResult.years, ['2026']);

  // Verify timeline directory files
  const rootSummary = await readFile(path.join(projectRoot, '.xiaotao/memory/timeline/summary.md'), 'utf8');
  assert.match(rootSummary, /# 项目经历总览/);
  assert.match(rootSummary, /2026 年度总览/);

  const yearSummary = await readFile(path.join(projectRoot, '.xiaotao/memory/timeline/years/2026/summary.md'), 'utf8');
  assert.match(yearSummary, /# 项目记忆年度总览：2026/);
  assert.match(yearSummary, /09 月/);

  const monthSummary = await readFile(path.join(projectRoot, '.xiaotao/memory/timeline/years/2026/09/summary.md'), 'utf8');
  assert.match(monthSummary, /# 项目记忆月度摘要：2026-09/);
  assert.match(monthSummary, /2026-09-23/);
  assert.match(monthSummary, /优化登录体验/);

  const dayYaml = await readFile(path.join(projectRoot, '.xiaotao/memory/timeline/years/2026/09/23.yaml'), 'utf8');
  assert.match(dayYaml, /event_type: task_completed/);
  assert.match(dayYaml, /2026-09-23T12:00:00Z/);
  assert.match(dayYaml, /\.xiaotao\/tasks\/archive\/20260923-login\/task\.yaml/);

  // Test check command
  const { stdout: checkOut } = await runTimeline(projectRoot, ['check']);
  assert.equal(JSON.parse(checkOut).status, 'current');

  // Test progressive drill-down summary queries
  const { stdout: drillRoot } = await runTimeline(projectRoot, ['summary']);
  assert.match(drillRoot, /# 项目经历总览/);

  const { stdout: drillYear } = await runTimeline(projectRoot, ['summary', '--year', '2026']);
  assert.match(drillYear, /# 项目记忆年度总览：2026/);

  const { stdout: drillMonth } = await runTimeline(projectRoot, ['summary', '--year', '2026', '--month', '09']);
  assert.match(drillMonth, /# 项目记忆月度摘要：2026-09/);

  // Test daily events query
  const { stdout: dayEventsOut } = await runTimeline(projectRoot, ['events', '--date', '2026-09-23']);
  const dayEvents = JSON.parse(dayEventsOut);
  assert.equal(dayEvents.date, '2026-09-23');
  assert.equal(dayEvents.events.length, 1);
  assert.equal(dayEvents.events[0].event_type, 'task_completed');
  assert.equal(dayEvents.events[0].occurred_at, '2026-09-23T12:00:00Z');
});

test('handles projects with no events gracefully', async (t) => {
  const projectRoot = await createProject(t);
  const { stdout } = await runTimeline(projectRoot, ['build']);
  const result = JSON.parse(stdout);
  assert.equal(result.status, 'built');
  assert.equal(result.event_count, 0);
  assert.deepEqual(result.years, []);

  const rootSummary = await readFile(path.join(projectRoot, '.xiaotao/memory/timeline/summary.md'), 'utf8');
  assert.match(rootSummary, /已累计记录 \*\*0\*\* 项关键任务交付/);
});

test('enforces parameter constraints on drill-down queries', async (t) => {
  const projectRoot = await createProject(t);
  await runTimeline(projectRoot, ['build']);

  await assert.rejects(
    runTimeline(projectRoot, ['summary', '--month', '09']),
    /--month requires --year/
  );

  await assert.rejects(
    runTimeline(projectRoot, ['events', '--date', 'invalid-date-format']),
    /Invalid date format/
  );
});

