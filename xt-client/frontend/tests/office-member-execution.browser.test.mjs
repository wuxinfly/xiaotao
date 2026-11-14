import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {test} from 'node:test';
import {chromium} from 'playwright';

const projectPath = 'E:\\work\\member-project';
const memberId = 'role_0123456789abcdef';
const now = '2026-11-14T09:00:00Z';

function member(overrides = {}) {
  return {id: memberId, name: '设计师', role: 'project', status: 'idle', todayTasks: 0,
    modelPreference: {modelId: 'test/model'}, responsibility: '负责界面方案', ...overrides};
}

function history(content = '先确认搜索页的方案。') {
  return {before: '', session: {id: 'run_chat', project: {path: projectPath, name: '演示项目'}, title: '讨论', model: 'test/model', status: 'completed',
    messages: [{id: 'user_1', role: 'user', content, runId: 'turn_chat', createdAt: now}, {id: 'answer_1', role: 'assistant', content: '可以先做空状态。', runId: 'turn_chat', createdAt: now}],
    activity: [], thoughts: [], runStats: [], createdAt: now, updatedAt: now}};
}

test('member accepts a confirmed task and returns to discussion', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5236', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited');})]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true, args: ['--enable-unsafe-swiftshader']});
    const context = await browser.newContext({viewport: {width: 1280, height: 850}, reducedMotion: 'reduce'});
    const tasks = [];
    let assignment;
    await context.route('**/api/**', route => {
      const req = route.request();
      const url = new URL(req.url());
      const path = url.pathname;
      if (path === '/api/agent/projects') return route.fulfill({json: {projects: [{path: projectPath, name: '演示项目', saved: true}]}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test/model', label: '测试模型', contextWindow: 32000}]}});
      if (path === '/api/office/team') return route.fulfill({json: {members: [member()], globalRoles: [], projectRoles: [], onboarding: 'completed', globalRevision: 0, projectRevision: 0, teamRevision: 0}});
      if (path === '/api/office/snapshot') return route.fulfill({json: {project: {path: projectPath, name: '演示项目'}, accessMode: 'safe', tasks,
        members: [member(tasks.length ? {status: 'running', taskId: 'task_member', taskTitle: '实现搜索空状态', taskStatus: 'running'} : {})], artifacts: [], capabilities: {notes: []}}});
      if (path === `/api/office/members/${memberId}/chat`) return route.fulfill({json: {memberId, sessionId: 'run_chat', status: 'completed'}});
      if (path === `/api/office/members/${memberId}/chat/history`) return route.fulfill({json: history()});
      if (path === '/api/office/tasks' && req.method() === 'POST') {
        assignment = req.postDataJSON();
        tasks.push({id: 'task_member', projectPath, projectName: '演示项目', title: assignment.title, goal: assignment.goal, status: 'running', memberId,
          executor: {kind: 'member', name: '设计师', role: 'member', model: 'test/model', modelLabel: '测试模型', sessionId: 'run_task'}, sessionId: 'run_task', stats: {}, createdAt: now, updatedAt: now});
        return route.fulfill({json: tasks[0]});
      }
      if (path === '/api/office/tasks/task_member/events') return route.fulfill({status: 200, contentType: 'text/event-stream', body: `event: run_stats\ndata: ${JSON.stringify({type: 'run_stats', sessionId: 'run_task', runStats: {runId: 'turn_task', startedAt: now, phase: 'working', toolCalls: 0, filesRead: 0, filesWritten: 0, contextTokens: 0, contextWindow: 32000}})}\n\n`});
      if (path === '/api/office/tasks/task_member') return route.fulfill({json: {task: tasks[0], history: {before: '', session: {id: 'run_task', project: {path: projectPath, name: '演示项目'}, title: '实现搜索空状态', model: 'test/model', status: 'running', messages: [], activity: [], thoughts: [], runStats: [], createdAt: now, updatedAt: now}}, artifacts: [], messages: []}});
      return route.fulfill({json: {sessions: [], models: []}});
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('http://127.0.0.1:5236');
    await page.getByRole('button', {name: '办公室', exact: true}).click();
    await page.locator('canvas[data-office-renderer="three"]').waitFor();
    await page.getByRole('button', {name: '任务', exact: true}).click();
    await page.getByRole('tab', {name: /团队/}).click();
    await page.locator('[data-member-id]').filter({hasText: '设计师'}).click();
    await page.getByRole('heading', {name: '与设计师对话'}).waitFor();
    await page.getByRole('button', {name: '交给TA执行'}).click();
    const dialog = page.getByRole('dialog', {name: '交给设计师一个任务'});
    await dialog.getByLabel('任务目标').fill('实现搜索空状态并补测试');
    await dialog.getByLabel('标题（可选）').fill('实现搜索空状态');
    await dialog.getByText('交接当前讨论').waitFor();
    await dialog.getByText(/当前为安全模式/).waitFor();
    await dialog.getByRole('button', {name: '确认并开始'}).click();
    await page.getByRole('heading', {name: '实现搜索空状态'}).waitFor();
    assert.equal(assignment.memberId, memberId);
    assert.equal(assignment.chatSessionId, 'run_chat');
    assert.equal(assignment.model, 'test/model');
    await page.getByText('指定成员执行（单任务）').waitFor();
    await page.getByText('设计师 · 测试模型').waitFor();
    await page.getByText('正在理解你的需求…').waitFor();
    await page.getByRole('button', {name: '返回任务列表'}).click();
    await page.getByRole('tab', {name: /团队/}).click();
    await page.locator('[data-member-id]').filter({hasText: '设计师'}).click();
    await page.getByRole('button', {name: '已有任务进行中'}).waitFor();
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
