import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

const projectPath = 'E:\\work\\demo-project';
const secondProjectPath = 'E:\\work\\second-project';

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
      {id: 'xiaotao', name: '小涛', role: 'coordinator', status: 'idle', todayTasks: 1},
      {id: 'designer', name: '设计师', role: 'designer', status: 'idle', todayTasks: 0},
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
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const context = await browser.newContext({viewport: {width: 1280, height: 900}, reducedMotion: 'reduce'});
    const requests = [];
    const posts = [];
    const chats = {};
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
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test/model', label: '测试模型', sourceLabel: '本地', contextWindow: 128000}]}});
      if (path === '/api/office/snapshot') {
        const board = url.searchParams.get('projectPath') === secondProjectPath
          ? {...snapshot([]), project: {path: secondProjectPath, name: '另一个项目'}}
          : snapshot(tasks, artifacts);
        return route.fulfill({json: board});
      }
      const chatRoute = path.match(/^\/api\/office\/members\/([^/]+)\/chat(?:\/(messages|history))?$/);
      if (chatRoute) {
        const [, member, action] = chatRoute;
        if (action === 'messages') {
          chats[member] = req.postDataJSON().content;
          return route.fulfill({json: {memberId: member, sessionId: member, status: 'completed'}});
        }
        if (action === 'history') return route.fulfill({json: history([
          {id: `${member}-user`, role: 'user', content: chats[member], createdAt: '2026-11-12T09:00:01Z'},
          {id: `${member}-reply`, role: 'assistant', content: '我们先把想法整理成一份交接。', createdAt: '2026-11-12T09:00:02Z'},
        ])});
        return route.fulfill({json: {memberId: member, sessionId: chats[member] ? member : '', status: 'idle'}});
      }
      if (path === '/api/office/tasks/task_1' && method === 'GET') {
        return route.fulfill({json: {
          task: tasks[0],
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
        return route.fulfill({json: {artifact, available: true, content: artifact.id === 'art_1' ? 'export function Search() {}' : '# 搜索页记录'}});
      }
      return route.fulfill({json: {sessions: [], projects: [], accounts: [], models: []}});
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('http://127.0.0.1:5231');
    await page.locator('.workspace-dock').waitFor();
    await page.locator('.workspace-dock').getByRole('button', {name: '办公室', exact: true}).click();
    await page.locator('.office-page').waitFor();

    await page.getByRole('heading', {name: 'Agent 办公室'}).waitFor();
    await page.getByText('真实运行状态').waitFor();
    assert.equal(await page.getByText('上次更新').count(), 0);
    assert.equal(await page.getByRole('region', {name: '任务', exact: true}).count(), 0);
    assert.equal(await page.getByRole('dialog').count(), 0);
    const shelf = page.getByRole('button', {name: '打开项目资料馆'});
    const roomRect = await page.locator('.sea-office').boundingBox();
    const shelfRect = await shelf.boundingBox();
    assert.ok(shelfRect.x + shelfRect.width / 2 >= roomRect.x + roomRect.width * .7, 'the library must sit on the right side of the room');
    await shelf.hover();
    assert.equal(await shelf.evaluate(node => getComputedStyle(node).borderColor !== 'rgba(139, 210, 207, 0.36)'), true, 'the library must highlight on hover');
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office-dark.png')});
    assert.equal(await page.getByText('参与 23 次').count(), 0, 'prototype demo numbers must not appear');

    const designer = page.getByRole('button', {name: /设计师，产品设计/});
    await page.emulateMedia({reducedMotion: 'no-preference'});
    const actor = designer.locator('..');
    const initialPosition = await actor.evaluate(node => getComputedStyle(node).transform);
    await page.waitForTimeout(700);
    assert.notEqual(await actor.evaluate(node => getComputedStyle(node).transform), initialPosition, 'employees must move when motion is enabled');
    await designer.evaluate(node => node.click());
    const stoppedPosition = await actor.evaluate(node => getComputedStyle(node).transform);
    await page.waitForTimeout(300);
    assert.equal(await actor.evaluate(node => getComputedStyle(node).transform), stoppedPosition, 'employees must remain still after being selected');
    await page.getByRole('menu', {name: '设计师的操作'}).getByRole('menuitem', {name: '继续走动'}).click();
    await page.waitForTimeout(300);
    assert.notEqual(await actor.evaluate(node => getComputedStyle(node).transform), stoppedPosition, 'employees must resume walking');
    await page.emulateMedia({reducedMotion: 'reduce'});
    await designer.click();
    assert.equal(await designer.evaluate(node => node.closest('.office-actor').classList.contains('is-paused')), true, 'clicking an employee must stop the walk animation');
    const designerActions = page.getByRole('menu', {name: '设计师的操作'});
    await designerActions.getByRole('menuitem', {name: '对话'}).click();
    const designerChat = page.getByRole('dialog', {name: '与设计师对话'});
    await designerChat.waitFor();
    assert.equal(await designerChat.evaluate(node => node.parentElement.parentElement === document.body), true);
    await designerChat.getByLabel('模型').waitFor();
    await designerChat.getByText('128,000 tokens').waitFor();
    await page.getByLabel('给设计师发消息').fill('我想调整搜索页面');
    await page.locator('.member-chat').getByRole('button', {name: '发送', exact: true}).click();
    await page.getByText('我们先把想法整理成一份交接。').waitFor();
    assert.equal(chats.designer, '我想调整搜索页面');
    await page.getByRole('button', {name: '查看交接并安排'}).click();
    assert.equal(await page.locator('.office-create-modal textarea').inputValue(), '我们先把想法整理成一份交接。');
    await page.keyboard.press('Escape');
    await page.getByRole('heading', {name: '交给小涛一个任务'}).waitFor({state: 'detached'});
    await designerChat.waitFor();
    await page.keyboard.press('Escape');
    await designerChat.waitFor({state: 'detached'});
    await page.getByRole('button', {name: /小涛，办公室协调/}).click();
    await page.getByRole('menu', {name: '小涛的操作'}).getByRole('menuitem', {name: '对话'}).click();
    const coordinatorChat = page.getByRole('dialog', {name: '与小涛对话'});
    await coordinatorChat.waitFor();
    assert.equal(await page.getByText('我们先把想法整理成一份交接。').count(), 0, 'employee histories must stay separate');

    await page.keyboard.press('Escape');
    await coordinatorChat.waitFor({state: 'detached'});
    await page.getByRole('button', {name: /小涛，任务执行/}).click();
    await page.getByRole('menu', {name: '小涛的操作'}).getByRole('menuitem', {name: '查看任务'}).click();
    await page.getByRole('dialog', {name: '任务详情'}).waitFor();
    await page.getByRole('button', {name: '查看全部任务'}).click();
    const menu = page.getByRole('dialog', {name: '办公室菜单'});
    await menu.getByLabel('项目').waitFor();
    await menu.getByText('上次更新').waitFor();
    await menu.locator('.office-task-card').first().click();
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
    const taskDialog = page.getByRole('dialog', {name: '任务详情'});
    await taskDialog.waitFor();
    await page.keyboard.press('Escape');
    await taskDialog.waitFor({state: 'detached'});
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
    await page.keyboard.press('Escape');
    await library.waitFor({state: 'detached'});
    artifacts.length = 0;
    await page.getByRole('button', {name: /办公室菜单/}).click();
    await menu.getByRole('button', {name: '刷新', exact: true}).click();
    await page.keyboard.press('Escape');
    await shelf.getByText('0 本资料 · 打开目录').waitFor();
    await menu.waitFor({state: 'detached'});
    await page.getByRole('button', {name: '办公室菜单', exact: true}).click();
    await menu.getByLabel('项目').selectOption(secondProjectPath);
    await page.getByText('工作空间 / 另一个项目').waitFor();
    await menu.getByText('这个项目还没有任务。').waitFor();
    await page.keyboard.press('Escape');
    await menu.waitFor({state: 'detached'});

    // Polling must stop once the office page is left.
    const before = requests.filter(item => item.path === '/api/office/snapshot').length;
    await page.getByRole('button', {name: '返回小涛', exact: true}).click();
    await page.locator('.agent-workspace').waitFor();
    await page.waitForTimeout(3200);
    assert.equal(requests.filter(item => item.path === '/api/office/snapshot').length, before, 'office polling continued after leaving the page');

    await page.locator('.workspace-dock').getByRole('button', {name: '办公室', exact: true}).click();
    await page.locator('.office-page').waitFor();
    await page.getByRole('button', {name: '更多', exact: true}).click();
    await page.getByRole('menuitem', {name: '切换到浅海主题'}).click();
    const heading = await page.getByRole('heading', {name: 'Agent 办公室'}).evaluate(element => getComputedStyle(element).color);
    assert.notEqual(heading, 'rgb(232, 247, 250)', 'office heading must follow the light theme');
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office.png')});
    await page.setViewportSize({width: 380, height: 600});
    assert.equal(await page.locator('.office-page').evaluate(node => node.scrollWidth <= node.clientWidth), true);
    await page.getByRole('button', {name: /设计师，产品设计/}).click();
    await page.getByRole('menu', {name: '设计师的操作'}).getByRole('menuitem', {name: '对话'}).click();
    await designerChat.getByLabel('给设计师发消息').waitFor();
    await page.screenshot({path: join(tmpdir(), 'xiaotao-office-chat-narrow.png')});
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
