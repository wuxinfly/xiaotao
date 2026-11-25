import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {officePane, workspaceToggle} from './office-test-navigation.mjs';

test('workspace navigation is docked left and theme lives in the more menu', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5255', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({...(process.env.XIAOTAO_TEST_EXECUTABLE ? {executablePath: process.env.XIAOTAO_TEST_EXECUTABLE} : {channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge'}), headless: true});
    const context = await browser.newContext({viewport: {width: 1280, height: 850}, reducedMotion: 'reduce'});
    const tokens = {totalTokens: 3000, promptTokens: 2000, completionTokens: 1000, cachedTokens: 500};
    function conversation(id, title, source, estimated) {
      return {id, title, source, active: 1, latestRequestId: id,
        context: {estimated, window: 10000, parts: {messages: estimated - 500, tools: 500}, items: {tools: [{label: 'read_file', tokens: 500}]}},
        totals: {prompt: estimated, completion: 0},
        requests: [{id, model: 'same-model', startedAt: Date.now(), status: 'streaming', tokens: {}}]};
    }
    const internal = conversation('internal', '小涛任务', '小涛', 3000);
    const external = conversation('external', '外部会话', 'Responses', 6000);
    external.requests[0].tokens.prompt = 7000;
    external.requests[0].speedTokPerSec = 38.2;
    const otherActive = [1, 2].map(i => conversation(`active-${i}`, `并行会话 ${i}`, '小涛', 3000));
    function finish(session, speed) {
      return {...session, active: 0, requests: session.requests.map(request => ({...request, status: 'completed', speedTokPerSec: speed}))};
    }
    const recent = [1, 2, 3, 4].map(i => finish(conversation(`idle-${i}`, `已结束的会话 ${i}`, 'Responses', 4000), 21.5));
    const snapshot = {version: 'fixture', versionSeq: 1, launch: {startedAt: 1793678400, uptimeSeconds: 60, batchId: 'fixture'}, currentSession: tokens, today: {...tokens, totalRequests: 2}, cumulative: {...tokens, totalRequests: 2}, conversations: [internal, external, ...otherActive, ...recent], recentRequests: [], activeRequests: [], activeCount: 0, heatmap: {startDate: '2026-01-01', endDate: '2026-11-03', trackingStartDate: '2026-01-01', days: {}}, storage: {status: 'healthy'}};
    await context.addInitScript(() => {
      window.EventSource = class extends EventTarget {
        constructor() { super(); window.dashboardEvents = this; }
        close() {}
      };
    });
    await context.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/dashboard/events') return route.fulfill({contentType: 'text/event-stream', body: ': fixture\n\n'});
      if (path === '/api/dashboard') return route.fulfill({json: snapshot});
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
    await page.goto('http://127.0.0.1:5255');
    await page.locator('.workspace-dock').waitFor();

    assert.equal(await page.locator('.titlebar nav').count(), 0, 'top navigation must be removed');
    assert.equal(await page.locator('.theme-toggle').count(), 0, 'theme control must leave the title bar');
    const footer = await page.locator('.workspace-dock').boundingBox();
    assert.ok(footer.x < 20 && footer.y + footer.height > 800, 'workspace navigation must sit at the lower-left edge');
    const sidebar = await page.locator('.agent-sidebar').boundingBox();
    const history = await page.locator('.agent-session-list').boundingBox();
    assert.ok(footer.x >= sidebar.x && footer.x + footer.width <= sidebar.x + sidebar.width, 'navigation stays inside the Agent sidebar');
    assert.ok(history.y + history.height <= footer.y, 'navigation must not cover scrollable session history');
    const footerButtons = page.locator('.workspace-dock .workspace-nav button');
    assert.deepEqual(await footerButtons.allTextContents(), ['仪表盘', '模型接口']);
    const dockBackgrounds = () => footerButtons.evaluateAll(buttons => buttons.map(button => getComputedStyle(button).backgroundColor));
    assert.equal(new Set(await dockBackgrounds()).size, 1, 'all dock entries use the same resting background');
    assert.equal(await page.getByRole('button', {name: '小涛 Agent', exact: true}).count(), 0, 'agent is the default page and must not have a dock entry');
    await page.locator('.agent-workspace').waitFor();
    const workspace = workspaceToggle(page);
    const rightmostAction = page.locator('.agent-context-actions button').last();
    assert.equal(await rightmostAction.getAttribute('aria-label'), '工作区');
    await workspace.press('Enter');
    assert.equal(await workspace.getAttribute('aria-expanded'), 'true');
    const tabs = officePane(page).getByRole('group', {name: '工作区标签'});
    assert.deepEqual(await tabs.getByRole('button').allTextContents(), ['办公室']);
    assert.equal(await tabs.getByRole('button', {name: '办公室', exact: true}).getAttribute('aria-pressed'), 'true');
    await officePane(page).getByText('在小涛对话里选择项目后，就能查看它的办公室。').waitFor();
    assert.equal(await page.locator('.workspace-dock').count(), 1, 'the office pane must keep home navigation visible');
    await page.getByRole('textbox', {name: '给小涛发消息'}).waitFor();
    await officePane(page).getByRole('button', {name: '收起工作区'}).press('Enter');
    await officePane(page).waitFor({state: 'hidden'});
    assert.equal(await workspace.getAttribute('aria-expanded'), 'false');
    await page.locator('.agent-workspace').waitFor();
    assert.equal(new Set(await dockBackgrounds()).size, 1, 'returning from the office restores the resting background');
    await page.locator('.workspace-dock').getByRole('button', {name: '仪表盘', exact: true}).click();
    await page.locator('.dashboard-page').waitFor();
    assert.equal(await page.locator('.workspace-sidebar').count(), 0, 'secondary pages must not reserve the left sidebar');
    assert.equal(await page.locator('.workspace-dock').count(), 0, 'secondary pages must hide workspace navigation');
    const sessions = page.getByRole('list', {name: '会话列表'});
    const internalRow = sessions.getByRole('button', {name: /小涛任务/});
    const externalRow = sessions.getByRole('button', {name: /外部会话/});
    await internalRow.waitFor();
    assert.equal(await sessions.locator('[data-active="true"]').count(), 4);
    assert.equal(await sessions.locator('[data-active="false"]').count(), 3);
    assert.equal(await page.getByText('已结束的会话 4', {exact: true}).count(), 0);
    assert.match(await externalRow.locator('.context-speed').innerText(), /38\.2\s+tok\/s/);
    assert.equal(await internalRow.locator('.context-percent').innerText(), '30%');
    assert.equal(await externalRow.locator('.context-percent').innerText(), '70%');
    assert.equal(await internalRow.getAttribute('aria-expanded'), 'false');
    await internalRow.press('Enter');
    await externalRow.press('Enter');
    assert.equal(await internalRow.getAttribute('aria-expanded'), 'false');
    assert.equal(await externalRow.getAttribute('aria-expanded'), 'true');
    await page.locator('.context-legend').getByRole('button', {name: /工具定义/}).click();
    await page.getByRole('region', {name: '分块明细'}).getByText('read_file').waitFor();
    await page.locator('.context-legend').getByRole('button', {name: /对话消息/}).press('Enter');
    await page.getByRole('region', {name: '分块明细'}).getByText('对话消息', {exact: true}).waitFor();
    await page.setViewportSize({width: 420, height: 800});
    assert.ok(await externalRow.evaluate(row => row.getBoundingClientRect().right <= innerWidth && row.scrollWidth <= row.clientWidth));
    await page.setViewportSize({width: 1280, height: 850});
    await page.screenshot({path: join(tmpdir(), 'xiaotao-dashboard-secondary.png')});
    await page.evaluate(next => window.dashboardEvents.dispatchEvent(new MessageEvent('update', {data: JSON.stringify(next)})),
      {...snapshot, versionSeq: 2, conversations: [{...external, context: {...external.context, window: 0}}, finish(internal, 33.7), ...recent]});
    await internalRow.getByText('已结束', {exact: true}).waitFor();
    assert.match(await internalRow.locator('.context-speed').innerText(), /33\.7\s+tok\/s/);
    assert.equal(await externalRow.locator('.context-percent').innerText(), '—');
    await page.evaluate(next => window.dashboardEvents.dispatchEvent(new MessageEvent('update', {data: JSON.stringify(next)})),
      {...snapshot, versionSeq: 3, conversations: [finish(external, 40.4), finish(internal, 33.7), ...recent]});
    await page.getByText('0 个进行中').waitFor();
    assert.equal(await sessions.locator('[data-active="false"]').count(), 3);
    assert.equal(await externalRow.getAttribute('aria-expanded'), 'true');
    await page.evaluate(next => window.dashboardEvents.dispatchEvent(new MessageEvent('update', {data: JSON.stringify(next)})),
      {...snapshot, versionSeq: 4, conversations: []});
    await page.getByText('暂无会话请求').waitFor();
    const titlebar = await page.locator('.titlebar').boundingBox();
    const dashboardBack = await page.getByRole('button', {name: '返回小涛', exact: true}).boundingBox();
    assert.ok(dashboardBack.x < 20 && dashboardBack.y >= titlebar.y && dashboardBack.y + dashboardBack.height <= titlebar.y + titlebar.height, 'back button must sit at the upper-left of the title bar');
    assert.equal(await page.locator('.workspace-back-row').count(), 0, 'secondary pages must not keep a content back row');
    const dashboardView = await page.locator('.workspace-page-view').boundingBox();
    assert.ok(dashboardView.x < 1 && dashboardView.width >= 1200, 'dashboard must use the full content width');
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
    await page.waitForFunction(() => document.querySelector('.window-menu-trigger')?.getAttribute('aria-expanded') === 'false');
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
    await workspace.click();
    await officePane(page).waitFor();
    await workspace.click();
    await officePane(page).waitFor({state: 'hidden'});
    assert.equal(await workspace.getAttribute('aria-expanded'), 'false');
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
