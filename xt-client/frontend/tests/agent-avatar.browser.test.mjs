import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('assistant prose and tools share one avatar per user turn', {timeout:60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5192', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1100,height:800}, reducedMotion:'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const stamp = n => new Date(Date.UTC(2026,10,3,0,0,n)).toISOString();
    let turnSeq = 0, lastUser = '', turn = '';
    const nextTurn = () => {turnSeq++; turn = 'turn-'+turnSeq; lastUser = 'u'+turnSeq;};
    const message = (n, role) => {
      if (role === 'user') nextTurn();
      return {id:'m'+n, role, runId:turn, content:role === 'user' ? '检查一下' : '再摸一遍，看看有没有变化。', createdAt:stamp(n)};
    };
    const activity = n => ({id:'a'+n, runId:turn, kind:'run_command', title:'运行命令', detail:'Get-Content VERSION', status:'completed', startedAt:stamp(n), endedAt:stamp(n+1)});
    const stats = (item, ended) => {
      const runs = [], seen = new Set();
      for (const entry of [...item.messages, ...item.activity]) {
        if (!entry.runId || seen.has(entry.runId)) continue;
        seen.add(entry.runId);
        const times = [...item.messages, ...item.activity].filter(other => other.runId === entry.runId).map(other => other.createdAt || other.startedAt).sort();
        runs.push({runId:entry.runId, userMessageId:item.messages.find(other => other.role === 'user' && other.runId === entry.runId)?.id, startedAt:times[0], endedAt:ended ? times.at(-1) : undefined, phase:ended ? (item.status === 'failed' ? 'failed' : item.status === 'canceled' ? 'canceled' : 'completed') : 'working', filesRead:0, filesWritten:0, toolCalls:1, modelRequests:1});
      }
      return runs;
    };
    const cases = [
      {id:'failed', status:'failed', messages:[message(0,'user'), message(1,'assistant')], activity:[activity(2)], error:'模型调用失败: RESOURCE_LIMIT', avatars:1},
      {id:'completed', status:'completed', messages:[message(0,'user'), message(4,'assistant')], activity:[activity(2)], avatars:1},
      {id:'text', status:'completed', messages:[message(0,'user'), message(1,'assistant')], activity:[], avatars:1},
      {id:'interleaved', status:'completed', messages:[message(0,'user'), message(1,'assistant'), message(4,'assistant'), message(8,'assistant')], activity:[activity(2), activity(6)], avatars:1},
      {id:'multi-error', status:'failed', messages:[message(0,'user'), message(1,'assistant'), message(4,'assistant')], activity:[activity(2), activity(6)], error:'模型调用失败: RESOURCE_LIMIT', avatars:1},
      {id:'retry', status:'completed', messages:[message(0,'user'), message(1,'assistant'), message(4,'user'), message(5,'assistant')], activity:[activity(2)], avatars:2},
      {id:'canceled', status:'canceled', messages:[message(0,'user'), message(1,'assistant')], activity:[activity(2)], avatars:1},
    ];
    const sessions = cases.map(item => ({...item, title:item.id, model:'test', project:{path:'E:/fixture',name:'头像回归'}, createdAt:stamp(0), updatedAt:stamp(10), runStats:stats(item, item.status !== 'running')}));
    // All API calls stay in synthetic fixtures; never contact a running client.
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json:{sessions}});
      const match = path.match(/\/sessions\/([^/]+)\/history$/);
      if (match) return route.fulfill({json:{session:sessions.find(item => item.id === match[1]), before:''}});
      return route.fulfill({json:{sessions:[], models:[], accounts:[]}});
    });
    await page.goto('http://127.0.0.1:5192');
    for (const item of cases) {
      await page.locator('.agent-session-select').filter({hasText:item.id}).click();
      await page.waitForFunction(id => document.querySelector('.agent-context-title strong')?.textContent === id, item.id);
      assert.equal(await page.locator('.agent-feed .agent-message-avatar').count(), item.avatars, item.id);
      if (item.status === 'failed') {
        assert.ok(await page.locator('.agent-feed .agent-run-notice').last().textContent().then(text => text.includes('任务失败')), 'failed turn renders the terminal notice');
        await page.setViewportSize({width:540,height:800});
        const avatar = await page.locator('.agent-feed .agent-message-avatar').first().boundingBox();
        const feed = await page.locator('.agent-feed').boundingBox();
        assert.ok(avatar.x >= feed.x, 'shared avatar must remain inside the narrow feed');
        await page.setViewportSize({width:1100,height:800});
      }
    }
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
