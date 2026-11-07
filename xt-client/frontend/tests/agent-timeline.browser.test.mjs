import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('agent run bubble, bounded trace, drawer and completed summary', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5191', '--strictPort'], {cwd: process.cwd(), stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 1200, height: 800}});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const stamp = seconds => new Date(Date.UTC(2026, 10, 7, 0, 0, seconds)).toISOString();
    const base = {
      id: 'timeline', project: {path: 'E:/fixture', name: '测试项目'}, title: '时序交互', model: 'test',
      status: 'completed', error: '', createdAt: stamp(0), updatedAt: stamp(20), messages: [], activity: [],
      thoughts: [], runStats: [],
    };
    const thoughts = [
      {id: 't0', runId: 'turn_1', kind: 'understanding', text: '先确认这是时序交互，不改文件。', status: 'completed', createdAt: stamp(1)},
      {id: 't1', runId: 'turn_1', kind: 'observation', text: '已确认气泡和轨迹的分层关系。', status: 'completed', createdAt: stamp(3)},
    ];
    const activity = [
      {id: 'a0', runId: 'turn_1', kind: 'read_file', title: '阅读文件', detail: 'README.md', status: 'completed', startedAt: stamp(2), endedAt: stamp(3)},
      {id: 'a1', runId: 'turn_1', kind: 'search_files', title: '搜索代码', detail: 'timeline', status: 'completed', startedAt: stamp(4), endedAt: stamp(5)},
    ];
    const stats = {runId: 'turn_1', userMessageId: 'u', startedAt: stamp(1), endedAt: stamp(9), filesRead: 2, filesWritten: 0, toolCalls: 2, modelRequests: 3, outputSpeed: 21.4, outputTokens: 72, firstTokenMs: 1200, contextTokens: 24000};
    const liveStats = {...stats, endedAt: undefined, liveTokens: 64, liveSpeed: 18.2};
    const completedStats = {...stats};
    const terminal = {
      ...base, status: 'running', updatedAt: stamp(9),
      messages: [
        {id: 'u', role: 'user', content: '检查交互', createdAt: stamp(0), runId: stats.runId},
        {id: 'mid', role: 'assistant', content: '我先看看当前状态。', createdAt: stamp(1), runId: stats.runId},
        {id: 'mid2', role: 'assistant', content: 'PowerShell 语法，我分开跑。', createdAt: stamp(4), runId: stats.runId},
        {id: 'a', role: 'assistant', content: '交互已经检查完成。', createdAt: stamp(8), runId: stats.runId},
      ],
      activity, thoughts, runStats: [liveStats],
    };
    let completionMode = false;
    let release;
    const held = new Promise(resolve => {release = resolve;});
    let sent = false;
    const summary = {...base, model: 'test', status: 'completed', updatedAt: stamp(9), messages: [], activity: [], thoughts: null, runStats: null};
    summary.status = 'running';
    let historyCalls = 0;
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname === '/api/agent/sessions') return route.fulfill({json: {sessions: [summary]}});
      if (url.pathname === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test', label: '测试模型', priceFactor: 1}]}});
      if (url.pathname === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (url.pathname === '/api/agent/sessions/timeline/history') return route.fulfill({json: {session: {...terminal, messages: terminal.messages.filter(item => item.role === 'user')}, before: ''}});
      if (url.pathname.endsWith('/history')) {
        historyCalls++;
        return route.fulfill({json: {session: completionMode ? {...terminal, status: 'completed'} : terminal, before: ''}});
      }
      if (url.pathname === '/api/agent/sessions/timeline/cancel') {
        terminal.status = 'completed';
        summary.status = 'completed';
        return route.fulfill({json: {ok: true}});
      }
      if (url.pathname === '/api/agent/sessions/timeline') return route.fulfill({json: terminal});
      if (url.pathname.endsWith('/messages') && route.request().method() === 'POST') {
        sent = true;
        await held;
        return route.fulfill({contentType: 'text/event-stream', body: [
          {type: 'thought', thought: {id: 't3', runId: stats.runId, kind: 'checkpoint', text: '检查完成，准备收尾。', status: 'completed', createdAt: stamp(7)}},
          {type: 'run_stats', runStats: stats},
          {type: 'status', status: 'completed'},
        ].map(event => 'data: ' + JSON.stringify(event) + '\n\n').join('')});
      }
      return route.fulfill({json: {}});
    });
    await page.goto('http://127.0.0.1:5191');
    await page.getByRole('button', {name: '切换会话排列方式'}).click();
    await page.getByRole('button', {name: '按会话查看'}).click();
    await page.locator('.agent-session-select').filter({hasText: '时序交互'}).waitFor();
    await page.locator('.agent-session-select').filter({hasText: '时序交互'}).click();
    const timeline = page.locator('.agent-run-timeline');
    await timeline.waitFor();
    await page.evaluate(() => localStorage.setItem('xiaotao.agent.enabledModels', JSON.stringify(['test'])));
    await page.reload();
    await page.locator('.agent-session-select').filter({hasText: '时序交互'}).waitFor();
    await page.locator('.agent-session-select').filter({hasText: '时序交互'}).click();
    await page.locator('.agent-run-timeline').waitFor();
    assert.equal(await page.locator('.agent-run-timeline > .agent-message-avatar').count(), 1, 'live trace keeps the XiaoTao avatar');
    assert.equal(await page.locator('.agent-run-timeline > .agent-message-avatar .agent-droplet').getAttribute('data-state'), 'reading', 'running tool avatar reflects the latest tool state');
    const userBox = await page.locator('.agent-message.role-user').last().boundingBox();
    const timelineBox = await page.locator('.agent-run-timeline').boundingBox();
    assert.ok(timelineBox.y > userBox.y, 'user query stays above the live trace');

    assert.equal(await page.getByText('正在思考', {exact: true}).count(), 0, 'live trace must replace the generic thinking row');
    assert.match(await page.locator('.agent-run-bubble').textContent(), /已确认气泡和轨迹/);
    assert.equal(await page.locator('.agent-message.role-assistant').filter({hasText: 'PowerShell 语法，我分开跑。'}).count(), 0, 'only final assistant rows render');
    assert.equal(await page.locator('.agent-message.role-assistant').count(), 0, 'working narration stays outside the body');
    assert.equal(await page.locator('.agent-run-scroll li').count(), 4, 'live trace renders available events without placeholder rows');
    const liveBar = page.locator('.agent-live-stats');
    await liveBar.waitFor();
    assert.equal(await liveBar.locator('> span').count(), 5, 'live bar shows elapsed, first token, input, output and speed');
    assert.match(await liveBar.textContent(), /首 token 1.2s/, 'live bar shows the first-token latency');
    assert.match(await liveBar.textContent(), /64 tok/, 'live bar shows the produced tokens');
    assert.match(await liveBar.textContent(), /18.2\s*tok\/s/, 'live bar shows the rolling output speed');
    assert.match(await page.locator('.agent-run-scroll').textContent(), /阅读文件/);
    assert.match(await page.locator('.agent-run-scroll').textContent(), /先确认这是时序交互/);
    const bubbleNode = await page.locator('.agent-run-bubble-text').elementHandle();
    const traceScroll = page.locator('.agent-run-scroll ol');
    await traceScroll.evaluate(element => {element.scrollTop = 0;});
    await page.waitForTimeout(500);
    assert.equal(await traceScroll.evaluate(element => element.scrollTop), 0, 'live trace cannot be manually scrolled');
    assert.ok(await traceScroll.evaluate(element => element.scrollHeight <= element.clientHeight), 'live trace has no overflow to scroll');
    await page.emulateMedia({reducedMotion: 'reduce'});
    assert.equal(await page.locator('.agent-run-bubble-text').evaluate(element => getComputedStyle(element).animationName), 'none');
    assert.ok(await bubbleNode.evaluate(element => element.isConnected), 'unchanged bubble should not remount');
    await page.getByRole('button', {name: '上下文用量，约 24000 tokens，上限未知'}).waitFor();
    assert.match(await page.locator('.agent-context-meter').textContent(), /ctx 24K/);
    await page.getByRole('button', {name: '查看思考过程'}).click();
    const drawer = page.getByRole('dialog', {name: '本轮思考过程'});
    await drawer.waitFor();
    assert.match(await drawer.textContent(), /已确认气泡/);
    assert.match(await drawer.textContent(), /阅读文件/);
    assert.equal(await drawer.locator('.agent-drawer-timeline li').count(), 4);
    await page.keyboard.press('Escape');
    await drawer.waitFor({state: 'detached'});
    const compactPage = await browser.newPage({viewport: {width: 1200, height: 800}});
    compactPage.on('pageerror', error => errors.push(error.message));
    await compactPage.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname === '/api/agent/sessions') return route.fulfill({json: {sessions: [summary]}});
      if (url.pathname === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test', label: '测试模型', priceFactor: 1}]}});
      if (url.pathname === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (url.pathname.endsWith('/history')) return route.fulfill({json: {session: {...terminal, messages: terminal.messages.filter(item => item.role === 'user'), thoughts: thoughts.slice(0, 1), activity: activity.slice(0, 1)}, before: ''}});
      return route.fulfill({json: {}});
    });
    await compactPage.goto('http://127.0.0.1:5191');
    await compactPage.getByRole('button', {name: '切换会话排列方式'}).click();
    await compactPage.getByRole('button', {name: '按会话查看'}).click();
    await compactPage.locator('.agent-session-select').filter({hasText: '时序交互'}).click();
    const compactTrace = compactPage.locator('.agent-run-scroll');
    await compactTrace.waitFor();
    const compactHeight = await compactTrace.evaluate(element => element.getBoundingClientRect().height);
    const fullHeight = await page.locator('.agent-run-scroll').evaluate(element => element.getBoundingClientRect().height);
    const compactRows = await compactPage.locator('.agent-run-scroll li').count();
    assert.ok(compactHeight < fullHeight, `short trace grows with content instead of reserving four rows (compact=${compactHeight}, full=${fullHeight}, rows=${compactRows})`);
    completionMode = true;
    const completedPage = await browser.newPage({viewport: {width: 1200, height: 800}});
    completedPage.on('pageerror', error => errors.push(error.message));
    await completedPage.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname === '/api/agent/sessions') return route.fulfill({json: {sessions: [{...summary, status: 'completed'}]}});
      if (url.pathname.endsWith('/history')) return route.fulfill({json: {session: {...terminal, status: 'completed', runStats: [completedStats]}, before: ''}});
      if (url.pathname === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test', label: '测试模型', priceFactor: 1}]}});
      if (url.pathname === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      return route.fulfill({json: {}});
    });
    await completedPage.goto('http://127.0.0.1:5191');
    await completedPage.getByRole('button', {name: '切换会话排列方式'}).click();
    await completedPage.getByRole('button', {name: '按会话查看'}).click();
    await completedPage.locator('.agent-session-select').filter({hasText: '时序交互'}).click();
    const completedAnswer = completedPage.locator('.agent-run-timeline.is-finished');
    await completedAnswer.waitFor();
    await completedAnswer.locator('.agent-final-meta').waitFor();
    assert.match(await completedPage.locator('.agent-message.role-user').last().textContent(), /检查交互/);
    assert.equal(await completedPage.locator('.agent-run-timeline.is-finished').count(), 1, 'only the final answer remains in the conversation');
    assert.equal(await completedPage.locator('.agent-process-collapse:not(.is-collapsed) .agent-run-scroll').count(), 0, 'completed run should remove the live trace');
    assert.equal(await completedPage.locator('.agent-process-collapse:not(.is-collapsed) .agent-run-open').count(), 0, 'completed run should remove the live trace drawer trigger');
    assert.equal(await completedPage.locator('.agent-run-result').count(), 0, 'completed run has no standalone summary block');
    assert.match(await completedAnswer.locator('.agent-final-meta').textContent(), /8 秒/);
    assert.match(await completedAnswer.locator('.agent-final-meta').textContent(), /2/, 'summary retains real nonzero counts');
    assert.equal(await completedAnswer.locator('.agent-final-stat[aria-label^="写入"]').count(), 0, 'zero-value summary items stay hidden');
    assert.equal(await completedAnswer.locator('.agent-final-stat').count(), 6, 'duration, first token, read, tools, requests and speed render as icons');
    assert.match(await completedAnswer.locator('.agent-final-stat[aria-label^="首 token"]').textContent(), /1.2 秒/, 'summary keeps the first-token latency');
    await completedAnswer.locator('.agent-final-stat[aria-label^="输出速度"]').hover();
    await completedPage.getByRole('tooltip').waitFor();
    assert.match(await completedPage.getByRole('tooltip').textContent(), /输出速度 21.4 tok\/s/);
    assert.match(await completedAnswer.textContent(), /交互已经检查完成/);
    assert.ok((await completedAnswer.evaluate(element => element.getBoundingClientRect().top)) >
      (await completedPage.locator('.agent-message.role-user').last().evaluate(element => element.getBoundingClientRect().bottom)), 'final answer follows user query');
    assert.ok((await completedPage.locator('.agent-feed').evaluate(element => element.scrollWidth)) <= 1200);
    await completedPage.setViewportSize({width: 390, height: 760});
    await completedPage.waitForFunction(() => document.querySelector('.agent-final-meta'));
    const mobileSummary = completedAnswer;
    await mobileSummary.waitFor();
    const mobileBox = await mobileSummary.boundingBox();
    assert.ok(mobileBox.x >= 0);
    assert.ok(mobileBox.x + mobileBox.width <= 390);
    assert.ok((await completedPage.locator('.agent-feed').evaluate(element => element.scrollWidth)) <= 390);
    assert.deepEqual(errors, []);
    await page.close();
    return;
  } finally {
    await browser?.close();
    server.kill();
  }
});
