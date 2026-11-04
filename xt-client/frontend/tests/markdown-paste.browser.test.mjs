import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('Markdown wrapper pastes formatted content while ordinary code stays code', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5193', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => { if (data.toString().includes('ready in')) resolve(); })), once(server, 'exit').then(() => { throw Error('Vite exited'); })]);
    browser = await chromium.launch({executablePath: process.env.XIAOTAO_TEST_BROWSER_PATH || '/usr/bin/chromium', headless: true, args: ['--no-sandbox']});
    const page = await browser.newPage();
    await page.route('**/api/**', route => route.fulfill({json: {sessions: [], models: [], accounts: [], providers: []}}));
    await page.goto('http://127.0.0.1:5193');
    const editor = page.getByRole('textbox', {name: '给小涛发消息'});
    await editor.click();
    const paste = text => editor.evaluate((element, value) => { const data = new DataTransfer(); data.setData('text/plain', value); element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true})); }, text);
    await paste('```markdown\n# 标题\n\n- **一项**\n\n```js\nconst x = 1;\n```\n```');
    assert.equal(await editor.locator('h1').innerText(), '标题');
    assert.equal(await editor.locator('li strong').innerText(), '一项');
    assert.match(await editor.locator('pre code').innerText(), /const x = 1/);
    await editor.click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await editor.evaluate(element => {
      const data = new DataTransfer();
      data.setData('text/plain', '# VS Code 标题\n\n普通文字');
      data.setData('text/html', '<pre><code><span># VS Code 标题</span>\n\n<span>普通文字</span></code></pre>');
      element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}));
    });
    assert.equal(await editor.locator('h1').innerText(), 'VS Code 标题');
    assert.equal(await editor.locator('pre').count(), 0);
    assert.match(await editor.innerText(), /普通文字/);
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await editor.evaluate(element => {
      const data = new DataTransfer();
      data.setData('text/plain', '普通文字');
      data.setData('text/html', '<div style="white-space: pre;"><span>普通文字</span></div>');
      element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}));
    });
    assert.equal(await editor.locator('pre').count(), 0);
    assert.equal(await editor.innerText(), '普通文字');
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await paste('```js\nconst y = 2;\n```');
    assert.equal(await editor.locator('h1').count(), 0);
    assert.match(await editor.innerText(), /const y = 2/);
  } finally { await browser?.close(); server.kill(); }
});
