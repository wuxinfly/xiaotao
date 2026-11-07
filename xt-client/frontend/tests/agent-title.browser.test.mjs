import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('rename dialog generates a title without saving until confirmed', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5189', '--strictPort'], {cwd: process.cwd(), stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => { if (data.toString().includes('ready in')) resolve(); })),
      once(server, 'exit').then(() => { throw Error('Vite exited before ready'); }),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 900, height: 700}});
    const now = new Date().toISOString();
    const session = {id: 'title_session', title: '原来的标题', model: 'test', status: 'completed', project: {path: 'E:/fixture', name: '测试项目'}, createdAt: now, updatedAt: now, messages: [], activity: []};
    let generated = 0, renamed = '';
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test', label: '测试模型'}]}});
      if (url.pathname === '/api/agent/access-mode') return route.fulfill({json: {mode: 'full'}});
      if (url.pathname === '/api/agent/sessions' && route.request().method() === 'GET') return route.fulfill({json: {sessions: [session]}});
      if (url.pathname === '/api/agent/sessions/title_session/title' && route.request().method() === 'POST') {
        generated++;
        return route.fulfill({json: {title: 'AI 生成的新标题'}});
      }
      if (url.pathname === '/api/agent/sessions/title_session' && route.request().method() === 'PATCH') {
        renamed = route.request().postDataJSON().title;
        return route.fulfill({json: {...session, title: renamed}});
      }
      if (url.pathname === '/api/agent/sessions/title_session/history') return route.fulfill({json: {session, before: ''}});
      return route.fulfill({json: {}});
    });
    await page.goto('http://127.0.0.1:5189');
    await page.getByRole('button', {name: '会话操作 原来的标题'}).click();
    await page.getByRole('menuitem', {name: '重命名'}).click();
    const dialog = page.getByRole('dialog', {name: '重命名会话'});
    const input = dialog.getByLabel('会话名称');
    await input.waitFor();
    assert.equal(await input.inputValue(), '原来的标题');
    await dialog.getByRole('button', {name: 'AI生成标题'}).click();
    await page.waitForFunction(() => document.querySelector('.agent-rename-modal input')?.value === 'AI 生成的新标题');
    assert.equal(generated, 1);
    assert.equal(renamed, '', 'generation must not persist before save');
    await page.emulateMedia({reducedMotion: 'reduce'});
    for (const width of [900, 360]) {
      await page.setViewportSize({width, height: 700});
      const generatedButton = dialog.getByRole('button', {name: 'AI生成标题'});
      const inputBox = await input.boundingBox();
      const buttonBox = await generatedButton.boundingBox();
      const modalBox = await dialog.boundingBox();
      assert.ok(inputBox && buttonBox && modalBox, 'rename controls must be visible');
      assert.ok(inputBox.x + inputBox.width <= buttonBox.x, `input and generate button must not overlap at ${width}px`);
      assert.ok(buttonBox.x + buttonBox.width <= modalBox.x + modalBox.width, `generate button must stay inside dialog at ${width}px`);
    }
    await dialog.getByRole('button', {name: '保存'}).click();
    await page.waitForFunction(() => !document.querySelector('.agent-rename-modal'));
    assert.equal(renamed, 'AI 生成的新标题');
    assert.equal(await dialog.count(), 0);
    console.log('AI title generation fills the rename input, stays unsaved, and persists on confirmation.');
  } finally {
    await browser?.close();
    server.kill();
  }
});
