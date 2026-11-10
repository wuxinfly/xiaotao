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
    assert.deepEqual(await footerButtons.allTextContents(), ['仪表盘', '模型接口']);
    assert.equal(await page.getByRole('button', {name: '小涛 Agent', exact: true}).count(), 0, 'agent is the default page and must not have a dock entry');
    await page.locator('.agent-workspace').waitFor();
    await page.locator('.workspace-dock').getByRole('button', {name: '仪表盘', exact: true}).click();
    await page.locator('.dashboard-page').waitFor();
    assert.equal(await page.locator('.workspace-sidebar').count(), 0, 'secondary pages must not reserve the left sidebar');
    assert.equal(await page.locator('.workspace-dock').count(), 0, 'secondary pages must hide workspace navigation');
    const titlebar = await page.locator('.titlebar').boundingBox();
    const dashboardBack = await page.getByRole('button', {name: '返回小涛', exact: true}).boundingBox();
    assert.ok(dashboardBack.x < 20 && dashboardBack.y >= titlebar.y && dashboardBack.y + dashboardBack.height <= titlebar.y + titlebar.height, 'back button must sit at the upper-left of the title bar');
    assert.equal(await page.locator('.workspace-back-row').count(), 0, 'secondary pages must not keep a content back row');
    const dashboardView = await page.locator('.workspace-page-view').boundingBox();
    assert.ok(dashboardView.x < 1 && dashboardView.width >= 1200, 'dashboard must use the full content width');
    await page.screenshot({path: join(tmpdir(), 'xiaotao-dashboard-secondary.png')});
    await page.getByRole('button', {name: '返回小涛', exact: true}).click();
    await page.locator('.agent-workspace').waitFor();
    assert.equal(await page.locator('.workspace-dock .workspace-nav button.is-active').count(), 0);

    await page.locator('.workspace-dock').getByRole('button', {name: '模型接口', exact: true}).click();
    await page.getByText('填写自己的 API 地址、密钥和模型，供小涛调用。').waitFor();
    assert.equal(await page.locator('.workspace-sidebar').count(), 0);
    assert.equal(await page.locator('.workspace-dock').count(), 0);
    const accountsView = await page.locator('.workspace-page-view').boundingBox();
    assert.ok(accountsView.x < 1 && accountsView.width >= 1200, 'accounts page must use the full content width');
    await page.screenshot({path: join(tmpdir(), 'xiaotao-accounts-secondary.png')});
    await page.getByRole('button', {name: '返回小涛', exact: true}).click();
    await page.locator('.agent-workspace').waitFor();

    await page.getByRole('button', {name: '更多', exact: true}).click();
    await page.getByRole('menuitem', {name: '切换到浅海主题'}).click();
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'light');
    await page.getByRole('menuitem', {name: '切换到深海主题'}).waitFor();
    await page.screenshot({path: join(tmpdir(), 'xiaotao-workspace-navigation.png')});

    await page.setViewportSize({width: 540, height: 800});
    assert.equal(await page.locator('.agent-sidebar').evaluate(element => getComputedStyle(element).display), 'flex');
    assert.ok(await page.locator('.workspace-dock').getByRole('button', {name: '仪表盘', exact: true}).isVisible());
    await page.locator('.workspace-dock').getByRole('button', {name: '模型接口', exact: true}).click();
    await page.getByRole('button', {name: '返回小涛', exact: true}).waitFor();
    assert.equal(await page.locator('.workspace-dock').count(), 0);
    await page.setViewportSize({width: 420, height: 800});
    const narrowBack = await page.getByRole('button', {name: '返回小涛', exact: true}).boundingBox();
    assert.ok(narrowBack.x < 20, 'back button remains at the upper-left in narrow windows');
    await page.getByRole('button', {name: '返回小涛', exact: true}).click();
    await page.locator('.agent-workspace').waitFor();
    const trigger = await page.getByRole('button', {name: '更多', exact: true}).boundingBox();
    assert.ok(trigger.x + trigger.width <= 420, 'more menu remains in the narrow title bar');
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
