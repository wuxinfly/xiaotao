import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('workspace navigation is docked left and theme lives in the more menu', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5199', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const context = await browser.newContext({viewport: {width: 1280, height: 850}, reducedMotion: 'reduce'});
    const tokens = {totalTokens: 3000, promptTokens: 2000, completionTokens: 1000, cachedTokens: 500};
    await context.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/dashboard/events') return route.fulfill({contentType: 'text/event-stream', body: ': fixture\n\n'});
      if (path === '/api/dashboard') return route.fulfill({json: {version: 'fixture', versionSeq: 1, launch: {startedAt: 1793678400, uptimeSeconds: 60, batchId: 'fixture'}, currentSession: tokens, today: {...tokens, totalRequests: 2}, cumulative: {...tokens, totalRequests: 2}, recentRequests: [], activeRequests: [], activeCount: 0, heatmap: {startDate: '2026-01-01', endDate: '2026-11-03', trackingStartDate: '2026-01-01', days: {}}, storage: {status: 'healthy'}}});
      return route.fulfill({json: {
        sessions: [],
        accounts: [],
        providers: [],
        models: [],
        key: 'fixture-key',
        agent: {currentSessionId: '', defaultModels: {title: '', review: '', compression: ''}, defaultModelSettings: {}, enabledModels: [], modelConfigs: [], settings: null},
        settings: {models: [], defaultModels: {}, enabledModels: []},
      }});
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('http://127.0.0.1:5199');
    await page.locator('.workspace-dock').waitFor();

    assert.equal(await page.locator('.titlebar nav').count(), 0, 'top navigation must be removed');
    assert.equal(await page.locator('.theme-toggle').count(), 0, 'theme control must leave the title bar');
    const footer = await page.locator('.workspace-dock').boundingBox();
    assert.ok(footer.x < 20 && footer.y + footer.height > 800, 'workspace navigation must sit at the lower-left edge');
    assert.equal(await page.locator('.workspace-dock').evaluate(element => getComputedStyle(element).position), 'fixed');
    assert.ok(parseFloat(await page.locator('.agent-session-list').evaluate(element => getComputedStyle(element).paddingBottom)) >= footer.height, 'session history must reserve space above the dock');
    const footerButtons = page.locator('.workspace-dock .workspace-nav button');
    assert.deepEqual(await footerButtons.allTextContents(), ['小涛 Agent', '仪表盘', '账号与配额']);
    await page.locator('.workspace-dock').getByRole('button', {name: '仪表盘', exact: true}).click();
    await page.locator('.dashboard-page').waitFor();
    await page.locator('.workspace-dock').getByRole('button', {name: '小涛 Agent', exact: true}).click();
    await page.locator('.agent-workspace').waitFor();

    await page.locator('.workspace-dock').getByRole('button', {name: '账号与配额', exact: true}).click();
    await page.getByText('管理不同接入来源的账号与模型调用额度').waitFor();
    const side = await page.locator('.workspace-sidebar').boundingBox();
    assert.ok(side.x < 20 && side.height > 700, 'model pages keep the same left navigation rail');
    await page.locator('.workspace-dock').getByRole('button', {name: '小涛 Agent', exact: true}).click();
    await page.locator('.agent-workspace').waitFor();

    await page.getByRole('button', {name: '更多', exact: true}).click();
    await page.getByRole('menuitem', {name: '切换到浅海主题'}).click();
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'light');
    await page.getByRole('menuitem', {name: '切换到深海主题'}).waitFor();
    await page.screenshot({path: join(tmpdir(), 'xiaotao-workspace-navigation.png')});

    await page.setViewportSize({width: 540, height: 800});
    assert.equal(await page.locator('.agent-sidebar').evaluate(element => getComputedStyle(element).display), 'flex');
    assert.ok(await page.locator('.workspace-dock').getByRole('button', {name: '仪表盘', exact: true}).isVisible());
    const trigger = await page.getByRole('button', {name: '更多', exact: true}).boundingBox();
    assert.ok(trigger.x + trigger.width <= 540, 'more menu remains in the narrow title bar');
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
