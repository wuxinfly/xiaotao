import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { chromium } from 'playwright';

test('compact HUD swaps to a narrow peek and keeps enter/exit motion interruptible', { timeout: 60000 }, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5197', '--strictPort'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => { if (data.toString().includes('ready in')) resolve(); })), once(server, 'exit').then(() => { throw Error('Vite exited before ready'); })]);
    browser = await chromium.launch({ channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true });
    const page = await browser.newPage({ viewport: { width: 260, height: 170 } });
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/dashboard/events') return route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' });
      if (path === '/api/dashboard') return route.fulfill({ json: { version: 'fixture', versionSeq: 1, launch: { startedAt: 1793678400, uptimeSeconds: 60, batchId: 'fixture' }, currentSession: { totalTokens: 3000, promptTokens: 2000, completionTokens: 1000, cachedTokens: 500 }, today: { totalTokens: 3000, promptTokens: 2000, completionTokens: 1000, cachedTokens: 500, totalRequests: 2 }, cumulative: { totalTokens: 3000, promptTokens: 2000, completionTokens: 1000, cachedTokens: 500, totalRequests: 2 }, recentRequests: [], activeRequests: [], activeCount: 0, heatmap: { startDate: '2026-01-01', endDate: '2026-11-07', trackingStartDate: '2026-01-01', days: {} }, storage: { status: 'healthy' } } });
      return route.fulfill({ json: { accounts: [] } });
    });
    await page.goto('http://127.0.0.1:5197/hud.html');
    const hud = page.locator('.request-hud');
    await hud.waitFor();
    const size = await hud.evaluate(element => ({ width: element.getBoundingClientRect().width, height: element.getBoundingClientRect().height }));
    assert.equal(size.width, 260);
    assert.equal(size.height, 170);
    assert.equal(await hud.getAttribute('data-collapsed'), null);
    assert.ok(await page.getByText('今日用量', { exact: false }).isVisible());

    await page.evaluate(() => window.xtHudCollapsed?.(true));
    assert.equal(await hud.getAttribute('data-collapsed'), 'true');
    assert.equal(await page.locator('.request-hud-collapsed-mark').evaluate(element => getComputedStyle(element).display), 'grid');
    assert.equal(await page.locator('.request-hud-header').evaluate(element => getComputedStyle(element).visibility), 'hidden');
    assert.equal(await page.getByText('今日用量', { exact: false }).evaluate(element => getComputedStyle(element).visibility), 'hidden');
    assert.ok(!(await page.getByText('今日用量', { exact: false }).isVisible()));

    await page.evaluate(() => window.xtHudCollapsed?.(false));
    assert.equal(await hud.getAttribute('data-collapsed'), null);
    assert.ok(await page.getByText('今日用量', { exact: false }).isVisible());

    await page.emulateMedia({ reducedMotion: 'reduce' });
    assert.equal(await hud.evaluate(element => getComputedStyle(element).transitionDuration), '0s');
    const screenshot = join(tmpdir(), 'xiaotao-hud-compact.png');
    await page.screenshot({ path: screenshot });
  } finally {
    await browser?.close();
    server.kill();
  }
});
