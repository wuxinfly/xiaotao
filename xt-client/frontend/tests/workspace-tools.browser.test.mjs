import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {test} from 'node:test';
import {chromium} from 'playwright';
import {officePane, workspaceToggle} from './office-test-navigation.mjs';

const now = '2026-11-25T00:00:00+08:00';
const project = {path: 'E:/workspace-fixture', name: '工作区项目', saved: true};
const session = {
  id: 'main-workspace', title: '工作区标签验收', project, model: 'test/model', status: 'completed',
  createdAt: now, updatedAt: now, messages: [], activity: [], thoughts: [], runStats: [],
};
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lK3V1wAAAABJRU5ErkJggg==', 'base64');

test('workspace tabs add a file manager and an addressable browser', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5258', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw new Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 1400, height: 900}, reducedMotion: 'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let openedUrl = '';
    let openedProject = '';
    await page.route('https://example.com/**', route => route.fulfill({contentType: 'text/html', body: '<h1>example page</h1>'}));
    await page.route('**/api/**', route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      const query = new URL(request.url()).searchParams;
      if (path === '/api/agent/sessions') return route.fulfill({json: {sessions: [session]}});
      if (path === '/api/agent/sessions/main-workspace/history') return route.fulfill({json: {session, before: ''}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test/model', label: '测试模型'}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (path === '/api/agent/projects') return route.fulfill({json: {projects: [project]}});
      if (path === '/api/agent/references/sessions') return route.fulfill({json: {sessions: []}});
      if (path === '/api/office/snapshot') return route.fulfill({json: {project, accessMode: 'safe', tasks: [], members: [], artifacts: [], capabilities: {notes: []}}});
      if (path === '/api/office/conversations/main-workspace') return route.fulfill({json: []});
      if (path === '/api/workspace/files') {
        const dir = query.get('path') || '';
        const entries = dir === 'src'
          ? [{name: 'app.ts', path: 'src/app.ts', dir: false, size: 32, modified: now}]
          : dir === ''
            ? [
              {name: 'src', path: 'src', dir: true, size: 0, modified: now},
              {name: 'README.md', path: 'README.md', dir: false, size: 18, modified: now},
              {name: 'logo.png', path: 'logo.png', dir: false, size: png.length, modified: now},
            ]
            : [];
        return route.fulfill({json: {root: project.path, path: dir, parent: '', truncated: false, entries}});
      }
      if (path === '/api/workspace/file') {
        const file = query.get('path');
        if (file === 'README.md') return route.fulfill({json: {path: file, name: 'README.md', size: 18, modified: now, kind: 'text', truncated: false, content: '# 小涛\n你好'}});
        if (file === 'logo.png') return route.fulfill({json: {path: file, name: 'logo.png', size: png.length, modified: now, kind: 'image', truncated: false}});
        return route.fulfill({json: {path: file, name: 'app.ts', size: 32, modified: now, kind: 'text', truncated: false, content: 'export const app = true;'}});
      }
      if (path === '/api/workspace/file/raw') return route.fulfill({status: 200, contentType: 'image/png', body: png});
      if (path === '/api/system/open-url') {
        openedUrl = request.postDataJSON().url;
        return route.fulfill({json: {ok: true}});
      }
      if (path === '/api/agent/project/open') {
        openedProject = request.postDataJSON().path;
        return route.fulfill({json: {ok: true}});
      }
      return route.fulfill({json: {}});
    });
    await page.goto('http://127.0.0.1:5258');
    await page.getByRole('button', {name: session.title, exact: true}).click();
    await page.locator('.agent-context-title strong').getByText(session.title, {exact: true}).waitFor();
    await workspaceToggle(page).click();
    const pane = officePane(page);
    await pane.locator('.office-live-page').waitFor();

    const add = pane.getByRole('button', {name: '打开文件管理或浏览器'});
    await add.click();
    const menu = page.getByRole('menu', {name: '打开工具'});
    await menu.getByRole('menuitem', {name: /文件管理/}).waitFor();
    await menu.getByRole('menuitem', {name: /浏览器/}).click();
    const browserTab = pane.getByRole('button', {name: '浏览器', exact: true});
    await browserTab.waitFor();
    await page.getByRole('textbox', {name: '浏览器地址'}).fill('example.com');
    await page.getByRole('textbox', {name: '浏览器地址'}).press('Enter');
    const iframe = pane.locator('.workspace-browser-view iframe');
    await iframe.waitFor();
    assert.equal(await iframe.getAttribute('src'), 'https://example.com/');
    await page.frameLocator('.workspace-browser-view iframe').getByText('example page').waitFor();
    assert.equal(await pane.locator('.office-live-page').isVisible(), false);

    await page.getByRole('textbox', {name: '浏览器地址'}).fill('javascript:alert(1)');
    await page.getByRole('textbox', {name: '浏览器地址'}).press('Enter');
    await pane.getByRole('alert').getByText('只支持 http 或 https 地址').waitFor();
    await pane.getByRole('button', {name: '在系统浏览器中打开'}).click();
    await page.getByRole('button', {name: '刷新网页'}).click();
    assert.equal(openedUrl, 'https://example.com/');

    await add.click();
    await menu.getByRole('menuitem', {name: /文件管理/}).click();
    const filesTab = pane.getByRole('button', {name: '文件管理', exact: true});
    await filesTab.waitFor();
    await pane.locator('.workspace-file-row').filter({hasText: 'README.md'}).click();
    await pane.locator('.workspace-text-preview').getByText('你好').waitFor();
    await pane.getByRole('button', {name: '返回文件列表'}).click();
    await pane.locator('.workspace-file-row').filter({hasText: 'src'}).first().click();
    await pane.locator('.workspace-file-row').filter({hasText: 'app.ts'}).click();
    await pane.locator('.workspace-text-preview').getByText('export const app = true;').waitFor();
    await pane.getByRole('button', {name: '在资源管理器中打开项目'}).click();
    assert.equal(openedProject, project.path);

    await pane.getByRole('button', {name: '关闭浏览器'}).click();
    assert.equal(await browserTab.count(), 0);
    await pane.getByRole('button', {name: '办公室', exact: true}).click();
    await pane.locator('.office-live-page').waitFor();
    assert.equal(await pane.locator('.office-live-page').isVisible(), true);
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
