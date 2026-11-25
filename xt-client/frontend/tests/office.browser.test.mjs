import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {officePane, openOffice, pickOfficeProject} from './office-test-navigation.mjs';

const projectPath = 'E:\\work\\demo-project';
const secondProjectPath = 'E:\\work\\second-project';
const roleId = 'role_0123456789abcdef';
const appearance = {bodyType: 'standard', skinTone: 'light', hairStyle: 'short', hairColor: '#3c2a20', topStyle: 'crewneck', topColor: '#5b7489', bottomColor: '#364250', shoesColor: '#252a30', accessory: 'none'};

function task(overrides = {}) {
  return {
    id: 'task_1',
    projectPath,
    projectName: '演示项目',
    title: '给搜索页加空状态',
    goal: '搜索结果为空时给出清晰提示。',
    status: 'running',
    executor: {kind: 'xiaotao', name: '小涛', role: 'executor', model: 'test/model', modelLabel: '测试模型', sessionId: 'run_1'},
    sessionId: 'run_1',
    stats: {toolCalls: 2, filesRead: 1, filesWritten: 1, modelRequests: 2, contextTokens: 1200, contextWindow: 64000, inputKnown: false},
    latestActivity: {kind: 'read_file', title: '阅读文件', detail: 'src/search.tsx', status: 'running', startedAt: '2026-11-12T09:00:00Z'},
    createdAt: '2026-11-12T09:00:00Z',
    updatedAt: '2026-11-12T09:00:10Z',
    ...overrides,
  };
}

function snapshot(tasks, artifacts = []) {
  return {
    project: {path: projectPath, name: '演示项目'},
    accessMode: 'safe',
    tasks,
    members: [
      {id: 'role_0123456789abcdee', name: '开发成员', role: 'project', status: 'idle', todayTasks: 1, appearance},
      {id: roleId, name: '设计师', role: 'project', status: 'idle', todayTasks: 0, appearance},
      ...tasks.filter(item => item.status === 'running').map(item => ({
        id: item.executor.sessionId, name: item.executor.name, role: 'executor', status: item.status,
        model: item.executor.model, taskId: item.id, taskTitle: item.title, taskStatus: item.status,
        todayTasks: 1, latestActivity: item.latestActivity,
      })),
    ],
    artifacts,
    capabilities: {
      midRunMessages: false, queuedMessages: true, approval: true, restartResume: false,
      sameProjectIsolation: false, commandArtifacts: false, artifactTools: ['write_file', 'str_replace'],
      notes: ['运行中的执行者不能接收新消息；消息会排队，并在本轮成功结束后自动送达。', '服务重启后运行中任务会标记为中断失败，不会自动恢复。'],
    },
  };
}

function history(messages, activity = []) {
  return {
    session: {
      id: 'run_1', project: {path: projectPath, name: '演示项目'}, title: '给搜索页加空状态', model: 'test/model',
      status: 'running', messages, activity, thoughts: [], runStats: [], createdAt: '2026-11-12T09:00:00Z', updatedAt: '2026-11-12T09:00:10Z',
    },
    before: '',
  };
}

test('office page shows real task state, transcript, artifacts and queued messages', {timeout: 90000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5231', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true, args: ['--enable-unsafe-swiftshader']});
    const context = await browser.newContext({viewport: {width: 1280, height: 900}, reducedMotion: 'reduce'});
    const requests = [];
    const posts = [];
    const tasks = [task()];
    const artifacts = [
      {id: 'art_1', taskId: 'task_1', taskTitle: '给搜索页加空状态', projectPath, path: 'src/search.tsx', size: 2048, tool: 'str_replace', createdAt: '2026-11-12T09:00:05Z', updatedAt: '2026-11-12T09:00:05Z'},
      {id: 'art_2', taskId: 'task_1', taskTitle: '给搜索页加空状态', projectPath, path: 'docs/search-notes.md', size: 512, tool: 'write_file', createdAt: '2026-11-12T09:00:06Z', updatedAt: '2026-11-12T09:00:06Z'},
    ];
    const messages = [
      {id: 'msg_u1', taskId: 'task_1', content: '搜索结果为空时给个提示。', status: 'delivered', createdAt: '2026-11-12T09:00:01Z', deliveredAt: '2026-11-12T09:00:01Z'},
      {id: 'msg_u2', taskId: 'task_1', content: '记得顺手加个测试。', status: 'queued', createdAt: '2026-11-12T09:00:08Z'},
    ];
    await context.route('**/api/**', route => {
      const req = route.request();
      const url = new URL(req.url());
      const path = url.pathname;
      const method = req.method();
      requests.push({path, method});
      if (path === '/api/agent/projects') return route.fulfill({json: {projects: [
        {path: projectPath, name: '演示项目', saved: true}, {path: secondProjectPath, name: '另一个项目', saved: true},
      ]}});
      if (path === '/api/agent/project') {
        const {path: requested} = req.postDataJSON();
        return route.fulfill({json: {path: requested, name: requested === projectPath ? '演示项目' : '另一个项目', saved: true}});
      }
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test/model', label: '测试模型', sourceLabel: '本地', contextWindow: 128000}]}});
      if (path === '/api/office/team') {
        const other = url.searchParams.get('projectPath') === secondProjectPath;
        return route.fulfill({json: {members: other ? [] : snapshot([]).members, globalRoles: [], projectRoles: [], onboarding: other ? 'skipped' : 'completed', globalRevision: 0, projectRevision: 0, teamRevision: 0}});
      }
      if (path === '/api/office/snapshot') {
        const board = url.searchParams.get('projectPath') === secondProjectPath
          ? {...snapshot([]), members: [{id: 'coordinator', name: '小涛', role: 'coordinator', status: 'idle'}], project: {path: secondProjectPath, name: '另一个项目'}}
          : snapshot(tasks, artifacts);
        return route.fulfill({json: board});
      }
      if (path === '/api/office/tasks/task_1' && method === 'GET') {
        return route.fulfill({json: {
          task: tasks[0],
          delivery: {files: [{path: 'generated.md', label: '本轮观察到变化', diff: '+generated'}],
            commands: [{id: 'c1', command: 'npm test', status: 'failed', output: 'assertion failed'}],
            confirmed: '修改源文件', pending: {a2: '发布文档'}},
          history: history([
            {id: 'm1', role: 'user', content: '搜索结果为空时给个提示。', createdAt: '2026-11-12T09:00:01Z'},
            {id: 'm2', role: 'assistant', content: '我先看一下搜索页的实现。', createdAt: '2026-11-12T09:00:03Z'},
          ], [{id: 'a1', kind: 'read_file', title: '阅读文件', detail: 'src/search.tsx', status: 'completed', startedAt: '2026-11-12T09:00:04Z', endedAt: '2026-11-12T09:00:05Z'}]),
          artifacts,
          messages,
        }});
      }
      if (path === '/api/office/tasks/task_1/messages' && method === 'POST') {
        posts.push(req.postDataJSON());
        return route.fulfill({json: {id: 'msg_new', taskId: 'task_1', content: req.postDataJSON().content, status: 'queued', createdAt: '2026-11-12T09:00:20Z'}});
      }
      if (path.startsWith('/api/office/artifacts/art_')) {
        const artifact = artifacts.find(item => `/api/office/artifacts/${item.id}` === path);
        return route.fulfill({json: {artifact, available: true, content: artifact.id === 'art_1' ? 'export function Search() {}' : '# 搜索页记录\n\n- **测试清单**\n\n| 状态 |\n| --- |\n| 通过 |\n\n```js\nconst ready = true;\n```'}});
      }
      return route.fulfill({json: {sessions: [], projects: [], accounts: [], models: []}});
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('http://127.0.0.1:5231');
    await page.locator('.workspace-dock').waitFor();
    await openOffice(page, '演示项目');
    await officePane(page).getByRole('heading', {name: '演示项目', exact: true}).waitFor();
    assert.equal(await page.getByText('上次更新').count(), 0);
    assert.equal(await page.getByRole('region', {name: '任务', exact: true}).count(), 0);
    assert.equal(await page.getByRole('dialog').count(), 0);
    const taskTrigger = page.getByRole('button', {name: '任务', exact: true});
    assert.equal(await taskTrigger.getAttribute('aria-expanded'), 'false');
    const scene = page.locator('.workspace-scene');
    const fullWidth = (await scene.boundingBox()).width;
    await taskTrigger.click();
    await page.getByRole('heading', {name: '项目任务'}).waitFor();
    assert.equal((await scene.boundingBox()).width, fullWidth, 'opening the drawer must not resize the room');
    await page.keyboard.press('Escape');
    assert.equal(await taskTrigger.getAttribute('aria-expanded'), 'false');
    assert.equal(await taskTrigger.evaluate(node => node === document.activeElement), true);
    await taskTrigger.click();
    await page.locator('.workspace-task-list .office-task-card').first().waitFor();
    assert.equal(await page.getByRole('button', {name: '办公室菜单', exact: true}).count(), 0);
    const shelf = page.getByRole('button', {name: '打开项目资料馆'});
    const roomRect = await page.locator('.workspace-scene').boundingBox();
    const shelfRect = await shelf.boundingBox();
    assert.ok(shelfRect.x >= roomRect.x && shelfRect.x + shelfRect.width <= roomRect.x + roomRect.width, 'the library must remain inside the room');
    await shelf.hover();
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office-dark.png')});
    assert.equal(await page.getByText('参与 23 次').count(), 0, 'prototype demo numbers must not appear');

    assert.equal(await scene.locator('canvas').count(), 0);
    const closeDetail = async () => {
      const back = page.locator('.workspace-back');
      if (await back.isVisible()) await back.click();
    };
    await page.getByRole('tab', {name: /团队/}).click();
    await page.locator('.workspace-member-list [data-member-id]').filter({hasText: '设计师'}).waitFor();
    await page.getByRole('tab', {name: /任务/}).click();
    await page.locator('.workspace-task-list .office-task-card').first().click();
    await page.getByRole('heading', {name: '给搜索页加空状态'}).waitFor();
    await page.getByText('我先看一下搜索页的实现。').waitFor();
    await page.getByText('阅读文件').first().waitFor();
    await page.getByText('上游未返回用量；本轮上下文约 1200 / 64000（估算）').waitFor();
    await page.getByText('排队中，本轮结束后送达').waitFor();

    await page.locator('.office-artifacts .office-artifact').first().click();
    await page.getByText('export function Search() {}').waitFor();
    await page.getByRole('button', {name: '关闭', exact: true}).click();

    await page.locator('.office-message-compose textarea').fill('补充一个空状态组件');
    await page.locator('.office-message-actions').getByRole('button', {name: '发送', exact: true}).click();
    await page.getByText('消息已排队，本轮结束后送达。').waitFor();
    assert.equal(posts.length, 1);
    assert.equal(posts[0].content, '补充一个空状态组件');

    await page.getByRole('button', {name: '停止任务', exact: true}).click();
    await page.getByRole('alertdialog').waitFor();
    await page.keyboard.press('Escape');
    await page.getByRole('alertdialog').waitFor({state: 'detached'});
    await page.getByRole('heading', {name: '给搜索页加空状态'}).waitFor();
    Object.assign(tasks[0], {status: 'failed', interrupted: true});
    await page.getByRole('button', {name: '刷新', exact: true}).click();
    const receipt = page.getByRole('region', {name: '本轮交付'});
    await receipt.getByText('generated.md · 本轮观察到变化').click();
    await receipt.getByText('+generated', {exact: true}).waitFor();
    await receipt.getByText('npm test', {exact: true}).click();
    await receipt.getByText('assertion failed').waitFor();
    await receipt.getByText('结果待核对：发布文档').waitFor();
    const recovery = page.waitForRequest(req => req.method() === 'POST' && req.url().includes('/messages'));
    await receipt.getByRole('button', {name: '检查现场并继续'}).click();
    assert.deepEqual((await recovery).postDataJSON(), {resume: true});
    await closeDetail();
    await shelf.click();
    const library = page.getByRole('dialog', {name: '项目资料馆'});
    await library.getByRole('heading', {name: /docs/}).waitFor();
    await library.getByRole('heading', {name: /src/}).waitFor();
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office-library.png')});
    await library.getByLabel('查找资料').fill('search.tsx');
    assert.equal(await library.locator('.office-library-book').count(), 1, 'library search must narrow the book list');
    await library.getByRole('button', {name: '打开资料 search.tsx'}).click();
    await page.getByText('export function Search() {}').waitFor();
    await page.keyboard.press('Escape');
    await library.waitFor();
    assert.equal(await library.getByLabel('查找资料').inputValue(), 'search.tsx');
    await library.getByLabel('查找资料').fill('search-notes.md');
    const book = library.getByRole('button', {name: '打开资料 search-notes.md'});
    await book.click();
    const markdown = page.getByRole('dialog', {name: 'search-notes.md'});
    await markdown.getByRole('heading', {name: '搜索页记录'}).waitFor();
    await markdown.getByRole('table').getByRole('cell', {name: '通过'}).waitFor();
    assert.equal(await markdown.locator('pre code').textContent(), 'const ready = true;\n');
    assert.equal(await page.locator('.office-library-modal').isVisible(), true);
    assert.equal(await page.locator('.office-library-modal').locator('..').getAttribute('inert'), '');
    await page.keyboard.press('Escape');
    await library.waitFor();
    assert.equal(await library.getByLabel('查找资料').inputValue(), 'search-notes.md');
    await page.waitForFunction(() => document.activeElement?.getAttribute('aria-label') === '打开资料 search-notes.md', null, {timeout: 1500}).catch(async reason => {
      throw new Error(`${reason.message}\nFocus: ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 350))}`);
    });
    await page.keyboard.press('Escape');
    await library.waitFor({state: 'detached'});
    artifacts.length = 0;
    await pickOfficeProject(page, '另一个项目');
    await officePane(page).getByRole('heading', {name: '另一个项目', exact: true}).waitFor();
    await page.locator('.office-desk-person[data-member-id="coordinator"]').waitFor();
    await taskTrigger.click();
    await page.getByRole('tab', {name: /团队/}).click();
    await page.locator('.workspace-member-list [data-member-id="coordinator"]').waitFor();
    assert.equal(await page.locator('.workspace-member-list [data-member-id]').count(), 1);
    await pickOfficeProject(page, '演示项目');
    await officePane(page).getByRole('heading', {name: '演示项目', exact: true}).waitFor();
    await shelf.getByText('0', {exact: true}).waitFor();
    await pickOfficeProject(page, '另一个项目');
    await officePane(page).getByRole('heading', {name: '另一个项目', exact: true}).waitFor();
    await taskTrigger.click();
    await page.getByText('现在没有进行中的任务').waitFor();

    // Board polling stops when the office pane is closed.
    const before = requests.filter(item => item.path === '/api/office/snapshot').length;
    await officePane(page).getByRole('button', {name: '收起工作区'}).click();
    await page.locator('.agent-workspace').waitFor();
    await page.waitForTimeout(3200);
    assert.equal(requests.filter(item => item.path === '/api/office/snapshot').length, before, 'office polling continued after leaving the page');

    await openOffice(page, '演示项目');
    const taskButtonColor = () => taskTrigger.evaluate(element => getComputedStyle(element).color);
    const darkColor = await taskButtonColor();
    await page.getByRole('button', {name: '更多', exact: true}).click();
    await page.getByRole('menuitem', {name: '切换到浅海主题'}).click();
    assert.notEqual(await taskButtonColor(), darkColor, 'office controls follow the light theme');
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office.png')});
    await page.setViewportSize({width: 380, height: 600});
    assert.equal(await page.locator('.office-page').evaluate(node => node.scrollWidth <= node.clientWidth), true);
    await taskTrigger.click();
    await page.getByRole('tab', {name: /团队/}).click();
    await page.locator('.workspace-member-list [data-member-id]').filter({hasText: '设计师'}).waitFor();
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office-team-narrow.png')});
    await page.setViewportSize({width: 1280, height: 900});
    await page.locator('.workspace-member-list [data-member-id]').filter({hasText: '设计师'}).waitFor({state: 'visible'});
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
