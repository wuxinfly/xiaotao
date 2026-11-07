import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

// The live bar under a running turn must show elapsed time, first-token
// latency, produced tokens and output speed at the same time.
test('live stats bar shows elapsed, first token, tokens and speed while running', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5207', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 1100, height: 800}});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const startedAt = new Date(Date.now() - 4200).toISOString();
    const liveStats = {
      runId: 'turn_live', userMessageId: 'u', startedAt,
      filesRead: 0, filesWritten: 0, toolCalls: 1, modelRequests: 1,
      firstTokenMs: 1500, inputKnown: true, inputTokens: 900, liveTokens: 320, liveSpeed: 42.5, contextTokens: 900,
    };
    const session = {
      id: 'live', title: '实时指标', model: 'test', status: 'running',
      project: {path: 'E:/fixture', name: '测试项目'}, createdAt: startedAt, updatedAt: startedAt,
      messages: [{id: 'u', role: 'user', content: '跑一下', createdAt: startedAt, runId: 'turn_live'}],
      activity: [{id: 'a1', runId: 'turn_live', kind: 'read_file', title: '阅读文件', detail: 'README.md', status: 'completed', startedAt, endedAt: startedAt}],
      thoughts: [], runStats: [liveStats],
    };
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json: {sessions: [session]}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test', label: '测试模型'}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (path.endsWith('/history')) return route.fulfill({json: {session, before: ''}});
      return route.fulfill({json: {}});
    });
    await page.goto('http://127.0.0.1:5207');
    await page.locator('.agent-session-select').filter({hasText: '实时指标'}).click();
    const bar = page.locator('.agent-live-stats');
    await bar.waitFor();
    assert.equal(await bar.locator('> span').count(), 5, 'live bar renders both token counts');
    assert.match(await bar.textContent(), /首 token 1\.5s/, 'first-token latency is visible');
    assert.match(await bar.textContent(), /320 tok/, 'produced tokens are visible');
    assert.match(await bar.locator('[title^="累计输入"]').textContent(), /900 tok/);
    assert.equal(await bar.locator('[title^="累计输入"] .lucide-arrow-up').count(), 1);
    assert.equal(await bar.locator('[title^="累计输出"] .lucide-arrow-down').count(), 1);
    assert.match(await bar.textContent(), /42\.5\s*tok\/s/, 'output speed is visible');
    const first = await bar.textContent();
    await page.waitForTimeout(1200);
    const second = await bar.textContent();
    assert.notEqual(first, second, 'elapsed time keeps moving while the run is streaming');
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
