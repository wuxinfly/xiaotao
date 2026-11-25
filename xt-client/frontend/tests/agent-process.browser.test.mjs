import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('finishing and reloading a turn keeps a single process card', {timeout:60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5257', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  let releaseHistory;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1100,height:800}, reducedMotion:'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(() => {
      localStorage.setItem('xiaotao.agent.historyView','session');
      localStorage.setItem('xiaotao.agent.enabledModels', JSON.stringify(['test']));
    });
    const stamp = n => new Date(Date.UTC(2026,10,3,0,0,n)).toISOString();
    const runId = 'turn_fixture';
    const message = (n, role, content) => ({id:'m'+n, role, content, createdAt:stamp(n), runId});
    const activity = n => ({id:'a'+n, runId, kind:'read_file', title:'阅读文件', detail:'README.md', status:'completed', startedAt:stamp(n), endedAt:stamp(n+1)});
    const user = message(0,'user','检查项目');
    const answer = message(9,'assistant','最终检查结果');
    const steps = [activity(2), activity(5), activity(7)];
    const notes = [message(1,'assistant','先看一下项目结构。'), message(4,'assistant','接着检查关键文件。'), message(6,'assistant','最后检查构建。')];
    const runStats = {runId, userMessageId:'m0', startedAt:stamp(1), filesRead:3, filesWritten:0, toolCalls:3, modelRequests:1};
    const base = {id:'fixture', title:'过程回归', model:'test', project:{path:'E:/fixture',name:'测试项目'}, createdAt:stamp(0), updatedAt:stamp(10), messages:[], activity:[], runStats:[], status:'completed'};
    let finished = false;
    let historyRequested = false;
    const historyGate = new Promise(resolve => {releaseHistory = resolve;});
    // Intercept every API: no running client or real account is accessed.
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json:{sessions:[base]}});
      if (path.endsWith('/messages')) {
        finished = true;
        const events = [{type:'session',message:user,status:'running'}, ...steps.map(item => ({type:'activity',activity:item})), {type:'message',message:answer}, {type:'run_stats',runStats}, {type:'status',status:'completed'}];
        return route.fulfill({contentType:'text/event-stream',body:events.map(event => 'data: '+JSON.stringify(event)+'\n\n').join('')});
      }
      if (path.endsWith('/history')) {
        if (finished) {historyRequested = true; await historyGate;}
        return route.fulfill({json:{session:finished ? {...base,messages:[user,...notes,answer],activity:steps,runStats:[{...runStats,endedAt:stamp(10)}]} : base,before:''}});
      }
      return route.fulfill({json:{models:[{model:'test',label:'测试模型'}],accounts:[],sessions:[]}});
    });
    await page.goto('http://127.0.0.1:5257');
    await page.locator('.agent-session-select').filter({hasText:base.title}).click();
    await page.waitForFunction(() => document.querySelector('.agent-context-title strong')?.textContent === '过程回归');
    const editor = page.getByRole('textbox',{name:'给小涛发消息'});
    await editor.fill('检查项目');
    await editor.press('Enter');
    await page.locator('.agent-run-bubble').filter({hasText:'最终检查结果'}).waitFor();
    assert.equal(await page.locator('.agent-run-timeline').count(),1);
    const card = await page.locator('.agent-run-timeline').elementHandle();
    // At this point SSE has arrived; history containing missing prose is held.
    assert.ok(historyRequested);
    releaseHistory();
    await page.waitForFunction(() => document.querySelector('.agent-context-title small')?.textContent === 'Agent 工作台');
    assert.equal(await page.locator('.agent-run-timeline').count(),1,'history prose must not split the process');
    assert.equal(await card.evaluate(el => el.isConnected),true,'existing process card must survive reconciliation');
    assert.equal(await page.locator('.agent-feed .agent-run-bubble').count(),1,'only the final answer stays in the run bubble');
    assert.equal(await page.locator('.agent-feed .role-assistant').count(),0,'intermediate prose stays out of the conversation');
    assert.equal(await page.locator('.agent-feed .agent-message-avatar').count(),1);
    await page.locator('.agent-final-meta-wrap .agent-message-actions-trigger').click();
    await page.getByRole('menuitem',{name:'查看思考过程'}).click();
    const drawer = page.getByRole('dialog',{name:'本轮思考过程'});
    await drawer.locator('.agent-thought-event').first().waitFor();
    assert.equal(await drawer.locator('.agent-thought-event').count(),3);
    assert.deepEqual(await drawer.locator('.agent-thought-event-text').allTextContents(),['阅读文件 · README.md','阅读文件 · README.md','阅读文件 · README.md']);
    assert.equal(await page.locator('.agent-feed').getByText(notes[0].content,{exact:true}).count(),0,'intermediate prose stays out of the conversation');
    await page.keyboard.press('Escape');
    await drawer.waitFor({state:'detached'});
    await page.reload();
    await page.locator('.agent-session-select').filter({hasText:base.title}).click();
    await page.locator('.agent-run-bubble').filter({hasText:'最终检查结果'}).waitFor();
    await page.setViewportSize({width:540,height:800});
    assert.equal(await page.locator('.agent-run-timeline').count(),1,'reloaded history must use the same grouping');
    assert.equal(await page.locator('.agent-feed .role-assistant').count(),0);
    assert.deepEqual(errors,[]);
  } finally {
    releaseHistory?.();
    await browser?.close();
    server.kill();
  }
});
