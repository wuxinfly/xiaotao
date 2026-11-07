import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {test} from 'node:test';
import {chromium} from 'playwright';

test('thinking preview and completed icon summary stay compact in both themes', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5201', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 1100, height: 800}, reducedMotion: 'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const stamp = seconds => new Date(Date.UTC(2026, 10, 7, 0, 0, seconds)).toISOString();
    const base = {
      id: 'summary', project: {path: 'E:/fixture', name: '测试项目'}, title: '摘要布局', model: 'test',
      status: 'running', error: '', createdAt: stamp(0), updatedAt: stamp(10), messages: [],
      activity: [], thoughts: [], runStats: [],
    };
    const user = {id: 'u', role: 'user', content: '检查这个样式', createdAt: stamp(0), runId: 'turn_visual'};
    const thought = {id: 't1', runId: 'turn_visual', kind: 'understanding', text: '先确认布局和交互细节。', status: 'completed', createdAt: stamp(1)};
    const activity = {id: 'a1', runId: 'turn_visual', kind: 'read_file', title: '阅读文件', detail: 'AgentPage.tsx', status: 'completed', startedAt: stamp(2), endedAt: stamp(3)};
    const liveStats = {runId: 'turn_visual', userMessageId: 'u', startedAt: stamp(1), filesRead: 1, filesWritten: 0, toolCalls: 1, modelRequests: 1, contextTokens: 12000};
    const answer = {id: 'a', role: 'assistant', content: '布局检查完成。', createdAt: stamp(4), runId: 'turn_visual'};
    const completeStats = {...liveStats, endedAt: stamp(9), phase: 'completed', modelRequests: 3, outputSpeed: 18.6, outputTokens: 64, firstTokenMs: 900};
    let complete = false;
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json: {sessions: [base]}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test', label: '测试模型'}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json: {mode: 'full'}});
      if (path.endsWith('/history')) return route.fulfill({json: {session: complete
        ? {...base, status: 'completed', messages: [user, answer], activity: [activity], thoughts: [thought], runStats: [completeStats]}
        : {...base, messages: [user], activity: [activity], thoughts: [thought], runStats: [liveStats]}, before: ''}});
      return route.fulfill({json: {}});
    });
    await page.goto('http://127.0.0.1:5201');
    await page.locator('.agent-session-select').filter({hasText: '摘要布局'}).click();
    await page.locator('.agent-run-scroll').waitFor();
    const liveTrace = await page.locator('.agent-run-scroll').evaluate(element => element.getBoundingClientRect().height);
    assert.ok(liveTrace < 96, 'one checkpoint and one activity use content height, not a fixed four-row box');
    assert.equal(await page.getByRole('button', {name: '查看思考过程'}).count(), 1);
    const openIcon = await page.getByRole('button', {name: '查看思考过程'}).boundingBox();
    const traceBox = await page.locator('.agent-run-trace').boundingBox();
    assert.ok(openIcon.x + openIcon.width >= traceBox.x + traceBox.width - 8, `drawer icon sits inside the trace top-right (icon=${JSON.stringify(openIcon)}, trace=${JSON.stringify(traceBox)})`);
    assert.ok(openIcon.y < traceBox.y + 12, `drawer icon aligns with the trace top (icon=${JSON.stringify(openIcon)}, trace=${JSON.stringify(traceBox)})`);
    await page.screenshot({path: process.env.TEMP + '/xiaotao-agent-live-compact.png', fullPage: true});

    complete = true;
    await page.reload();
    await page.locator('.agent-session-select').filter({hasText: '摘要布局'}).click();
    await page.locator('.agent-final-meta').waitFor();
    assert.equal(await page.locator('.agent-final-stat').count(), 6, 'only nonzero duration/first-token/read/tools/requests/speed icons render');
    assert.equal(await page.locator('.agent-final-stat[aria-label^="写入"]').count(), 0);
    const writeIcon = page.locator('.agent-final-stat[aria-label^="输出速度"]');
    await writeIcon.hover();
    await page.getByRole('tooltip').waitFor();
    assert.match(await page.getByRole('tooltip').textContent(), /18\.6 tok\/s/);
    await page.screenshot({path: process.env.TEMP + '/xiaotao-agent-summary-dark.png', fullPage: true});
    await page.getByRole('button', {name: '切换到浅海亮色主题'}).click();
    await page.waitForTimeout(100);
    await page.screenshot({path: process.env.TEMP + '/xiaotao-agent-summary-light.png', fullPage: true});
    for (const width of [1100, 540, 390]) {
      await page.setViewportSize({width, height: 800});
      const summary = await page.locator('.agent-final-meta-wrap').evaluate(element => {
        const box = element.getBoundingClientRect();
        return {left: box.left, right: box.right, top: box.top, bottom: box.bottom};
      });
      assert.ok(summary.left >= 0 && summary.right <= width, `summary stays inside ${width}px viewport`);
      assert.ok(await page.locator('.agent-feed').evaluate(element => element.scrollWidth <= element.clientWidth + 1), `no horizontal overflow at ${width}px`);
    }
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
