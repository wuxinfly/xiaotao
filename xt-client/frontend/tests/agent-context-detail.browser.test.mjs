import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('context detail popover shows breakdown and actual usage, handles missing fields and keyboard escape', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5218', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
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
    const now = new Date().toISOString();

    const stats = {
      runId: 'turn_detail', userMessageId: 'u', startedAt: now, endedAt: now,
      filesRead: 1, filesWritten: 0, toolCalls: 1, modelRequests: 2,
      inputKnown: true, inputTokens: 4200, outputTokens: 850,
      cacheKnown: true, cachedTokens: 3100,
      contextTokens: 3500, contextWindow: 32000,
      breakdown: {
        systemTokens: 800,
        toolsTokens: 600,
        messagesTokens: 500,
        toolResultsTokens: 1000,
        referenceTokens: 600,
        imageTokens: 1500,
        hasImages: true,
        totalTokens: 5000,
      },
    };

    const session = {
      id: 'session_detail', title: '上下文详情测试', model: 'test-model', status: 'completed',
      project: {path: 'E:/fixture', name: '测试项目'}, createdAt: now, updatedAt: now,
      messages: [
        {id: 'u', role: 'user', content: '帮我查看代码', createdAt: now, runId: 'turn_detail'},
        {id: 'a', role: 'assistant', content: '处理完毕。', createdAt: now, runId: 'turn_detail'},
      ],
      activity: [], thoughts: [], runStats: [stats],
    };

    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json: {sessions: [session]}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'test-model', label: '测试模型 32K', contextWindow: 32000}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (path.endsWith('/history')) return route.fulfill({json: {session, before: ''}});
      return route.fulfill({json: {}});
    });

    await page.goto('http://127.0.0.1:5218');
    await page.locator('.agent-session-select').filter({hasText: '上下文详情测试'}).click();

    const meterBtn = page.locator('button.agent-context-meter');
    await meterBtn.waitFor();
    assert.match(await meterBtn.textContent(), /ctx\s*11%/, 'context meter displays percentage');

    // Click to open popover
    await meterBtn.click();
    const popover = page.locator('.agent-context-detail-popover');
    await popover.waitFor();

    // Verify sections
    const text = await popover.textContent();
    assert.match(text, /上下文与用量详情/, 'title rendered');
    assert.match(text, /当前模型请求上下文/, 'estimate section rendered');
    assert.match(text, /估算/, 'estimate badge rendered');
    assert.match(text, /本轮任务累计实际用量/, 'actual section rendered');
    assert.match(text, /实际/, 'actual badge rendered');

    // Verify breakdown categories
    assert.match(text, /系统与项目指令/, 'system category');
    assert.match(text, /工具定义/, 'tools category');
    assert.match(text, /对话消息/, 'messages category');
    assert.match(text, /工具调用与输出/, 'toolResults category');
    assert.match(text, /引用对话/, 'reference category');
    assert.match(text, /图片估算/, 'image category');
    assert.match(text, /包含图片附件/, 'image note rendered');

    // Verify actual metrics
    assert.match(text, /4,200/, 'actual input tokens formatted');
    assert.match(text, /850/, 'actual output tokens formatted');
    assert.match(text, /3,100/, 'cached tokens formatted');
    assert.match(text, /2 次/, 'model requests formatted');

    // Test keyboard Escape to close popover
    await page.keyboard.press('Escape');
    await popover.waitFor({state: 'hidden'});
    assert.equal(await popover.isVisible(), false, 'popover closed on Escape');

    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});

test('context detail popover gracefully handles missing contextWindow and unknown cache', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5219', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
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
    const now = new Date().toISOString();

    const stats = {
      runId: 'turn_unknown', userMessageId: 'u', startedAt: now, endedAt: now,
      filesRead: 0, filesWritten: 0, toolCalls: 0, modelRequests: 1,
      inputKnown: false, outputTokens: 0,
      cacheKnown: false,
      contextTokens: 1200, contextWindow: 0,
    };

    const session = {
      id: 'session_unknown', title: '未知窗口测试', model: 'raw-model', status: 'completed',
      project: {path: 'E:/fixture', name: '测试项目'}, createdAt: now, updatedAt: now,
      messages: [
        {id: 'u', role: 'user', content: '测试未知窗口', createdAt: now, runId: 'turn_unknown'},
      ],
      activity: [], thoughts: [], runStats: [stats],
    };

    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json: {sessions: [session]}});
      if (path === '/api/agent/models') return route.fulfill({json: {models: [{model: 'raw-model', label: '未知模型'}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json: {mode: 'safe'}});
      if (path.endsWith('/history')) return route.fulfill({json: {session, before: ''}});
      return route.fulfill({json: {}});
    });

    await page.goto('http://127.0.0.1:5219');
    await page.locator('.agent-session-select').filter({hasText: '未知窗口测试'}).click();

    const meterBtn = page.locator('button.agent-context-meter');
    await meterBtn.waitFor();
    assert.match(await meterBtn.textContent(), /ctx\s*1\.2K/, 'compact tokens displayed when window is unknown');

    await meterBtn.click();
    const popover = page.locator('.agent-context-detail-popover');
    await popover.waitFor();

    const text = await popover.textContent();
    assert.match(text, /当前模型窗口上限未配置或未知/, 'graceful fallback text for unknown window');
    assert.match(text, /—/, 'unknown cache and input marked with dash');

    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
