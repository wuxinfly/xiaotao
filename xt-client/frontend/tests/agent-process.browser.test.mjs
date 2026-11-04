import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('finishing and reloading a turn keeps a single process card', {timeout:60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5193', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  let releaseHistory;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1100,height:800}, reducedMotion:'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const stamp = n => new Date(Date.UTC(2026,10,3,0,0,n)).toISOString();
    const message = (n, role, content) => ({id:'m'+n, role, content, createdAt:stamp(n)});
    const activity = n => ({id:'a'+n, kind:'read_file', title:'阅读文件', detail:'README.md', status:'completed', startedAt:stamp(n), endedAt:stamp(n+1)});
    const user = message(0,'user','检查项目');
    const answer = message(9,'assistant','最终检查结果');
    const steps = [activity(2), activity(5), activity(7)];
    const notes = [message(1,'assistant','先看一下项目结构。'), message(4,'assistant','接着检查关键文件。'), message(6,'assistant','最后检查构建。')];
    const base = {id:'fixture', title:'过程回归', model:'test', project:{path:'E:/fixture',name:'测试项目'}, createdAt:stamp(0), updatedAt:stamp(10), messages:[], activity:[], status:'completed'};
    let finished = false;
    let historyRequested = false;
    const historyGate = new Promise(resolve => {releaseHistory = resolve;});
    // Intercept every API: no running client or real account is accessed.
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json:{sessions:[base]}});
      if (path.endsWith('/messages')) {
        finished = true;
        const events = [{type:'session',message:user,status:'running'}, ...steps.map(item => ({type:'activity',activity:item})), {type:'message',message:answer}, {type:'status',status:'completed'}];
        return route.fulfill({contentType:'text/event-stream',body:events.map(event => 'data: '+JSON.stringify(event)+'\n\n').join('')});
      }
      if (path.endsWith('/history')) {
        if (finished) {historyRequested = true; await historyGate;}
        return route.fulfill({json:{session:finished ? {...base,messages:[user,...notes,answer],activity:steps} : base,before:''}});
      }
      return route.fulfill({json:{models:[],accounts:[],sessions:[]}});
    });
    await page.goto('http://127.0.0.1:5193');
    await page.locator('.agent-session-select').filter({hasText:base.title}).click();
    await page.waitForFunction(() => document.querySelector('.agent-context-title strong')?.textContent === '过程回归');
    const editor = page.getByRole('textbox',{name:'给小涛发消息'});
    await editor.fill('检查项目');
    await editor.press('Enter');
    await page.getByText('最终检查结果',{exact:true}).waitFor();
    assert.equal(await page.locator('.agent-process-row').count(),1);
    const card = await page.locator('.agent-process-row').elementHandle();
    // At this point SSE has arrived; history containing missing prose is held.
    assert.ok(historyRequested);
    releaseHistory();
    await page.waitForFunction(() => document.querySelector('.agent-context-title small')?.textContent === 'Agent 工作台');
    assert.equal(await page.locator('.agent-process-row').count(),1,'history prose must not split the process');
    assert.equal(await card.evaluate(el => el.isConnected),true,'existing process card must survive reconciliation');
    assert.equal(await page.locator('.agent-feed .role-assistant').count(),1,'only final answer stays outside process');
    assert.equal(await page.locator('.agent-feed .agent-message-avatar').count(),1);
    assert.equal(await page.locator('.agent-process-summary').getAttribute('aria-expanded'),'false');
    await page.locator('.agent-process-summary').click();
    await page.getByText(notes[0].content,{exact:true}).waitFor();
    assert.equal(await page.locator('.agent-process-popover li').count(),6);
    assert.deepEqual(await page.locator('.agent-process-popover li').allTextContents(),[
      notes[0].content, '阅读文件README.md1 秒', notes[1].content, '阅读文件README.md1 秒', notes[2].content, '阅读文件README.md1 秒',
    ]);
    await page.keyboard.press('Escape');
    await page.reload();
    await page.locator('.agent-session-select').filter({hasText:base.title}).click();
    await page.getByText('最终检查结果',{exact:true}).waitFor();
    await page.setViewportSize({width:540,height:800});
    assert.equal(await page.locator('.agent-process-row').count(),1,'reloaded history must use the same grouping');
    assert.equal(await page.locator('.agent-feed .role-assistant').count(),1);
    assert.deepEqual(errors,[]);
  } finally {
    releaseHistory?.();
    await browser?.close();
    server.kill();
  }
});
