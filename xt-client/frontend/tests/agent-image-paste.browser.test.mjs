import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFElEQVR42mP8z8Dwn4GBgYGJAQoAHgQCAft6VQAAAABJRU5ErkJggg==', 'base64');

test('Pasted images preview, respect model capability and render in history', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5194', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => { if (data.toString().includes('ready in')) resolve(); })), once(server, 'exit').then(() => { throw Error('Vite exited'); })]);
    browser = await chromium.launch(process.env.XIAOTAO_TEST_BROWSER_PATH
      ? {executablePath: process.env.XIAOTAO_TEST_BROWSER_PATH, headless: true, args: ['--no-sandbox']}
      : {channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 1100, height: 820}});
    const models = [
      {model: 'vision/model', label: 'Vision', supportsImages: true},
      {model: 'text/model', label: 'Text'},
    ];
    const sessions = [{
      id: 'session-image', project: {path: '', name: '默认工作目录', isDefault: true}, title: '图片会话', model: 'vision/model', status: 'completed',
      messages: [
        {id: 'u-image', role: 'user', content: '看看这张图', runId: 'image-run', images: [{id: 'img1', mimeType: 'image/png', size: png.length, url: '/test-image.png', width: 2, height: 2, createdAt: '2026-01-01T00:00:00Z'}], createdAt: '2026-01-01T00:00:00Z'},
        {id: 'a-image', role: 'assistant', content: '图片已收到。', runId: 'image-run', createdAt: '2026-01-01T00:00:01Z'},
      ],
      activity: [], thoughts: [], runStats: [], hasImages: true, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:01Z',
    }];
    await page.route('**/api/**', route => {
      if (new URL(route.request().url()).pathname === '/api/agent/sessions') {
        return route.fulfill({json: {sessions}});
      }
      return route.fulfill({json: {sessions: [], models: [], accounts: [], providers: []}});
    });
    await page.route('**/api/agent/models**', route => route.fulfill({json: {models}}));
    await page.route('**/api/agent/sessions/session-image/history**', route => route.fulfill({json: {session: sessions[0], before: ''}}));
    await page.route('**/api/agent/access-mode', route => route.fulfill({json: {mode: 'safe'}}));
    await page.route('**/test-image.png', route => route.fulfill({body: png, contentType: 'image/png'}));
    await page.addInitScript(() => localStorage.setItem('xiaotao.agent.enabledModels', JSON.stringify(['vision/model', 'text/model'])));
    await page.goto('http://127.0.0.1:5194');
    await page.getByRole('button', {name: /Agent/}).first().click().catch(() => {});
    const editor = page.getByRole('textbox', {name: '给小涛发消息'});
    await editor.waitFor();
    await editor.evaluate((element, base64) => {
      const raw = Uint8Array.from(atob(base64), char => char.charCodeAt(0));
      const file = new File([raw], 'screenshot.png', {type: 'image/png'});
      const data = new DataTransfer();
      data.items.add(file);
      element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}));
    }, png.toString('base64'));
    const draft = page.locator('.agent-composer-image img');
    await draft.waitFor();
    assert.equal(await draft.getAttribute('src').then(value => value?.startsWith('data:image/png')), true);
    assert.equal(await page.locator('.agent-composer-image').count(), 1);
    assert.equal(await page.getByRole('button', {name: '发送消息'}).isEnabled(), true);
    await page.getByRole('button', {name: '选择模型'}).click();
    await page.getByRole('menuitemradio', {name: /Text/}).click();
    await page.getByText('当前模型不支持图片').waitFor();
    assert.equal(await page.getByRole('button', {name: '发送消息'}).isDisabled(), true);
    await page.getByRole('button', {name: '移除图片 1'}).click();
    assert.equal(await page.locator('.agent-composer-image').count(), 0);

    await page.getByRole('button', {name: '切换会话排列方式'}).click();
    await page.getByRole('button', {name: '按会话查看'}).click();
    await page.locator('.agent-session-select').click();
    await page.locator('.agent-context-title strong').filter({hasText:'图片会话'}).waitFor();
    await page.locator('.agent-model').filter({hasText:'Vision'}).waitFor();
    await page.getByRole('button', {name: '选择模型'}).click();
    await page.getByRole('menuitemradio', {name: /Text/}).click();
    await page.getByText('会话包含图片，当前模型不支持').waitFor();
    assert.equal(await page.getByRole('button', {name: '发送消息'}).isDisabled(), true);
  } finally { await browser?.close(); server.kill(); }
});
