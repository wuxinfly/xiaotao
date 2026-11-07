import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
test('agent displays streamed chunks before completion without duplicates', {timeout: 60000}, async () => {
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5199','--strictPort'],{stdio:['ignore','pipe','pipe']}); let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in')) resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch(process.env.XIAOTAO_TEST_EXECUTABLE ? {executablePath:process.env.XIAOTAO_TEST_EXECUTABLE,headless:true} : {channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
  const page=await browser.newPage();
  const stamp='2026-01-01T00:00:00Z';
  const session={id:'stream-test',title:'流式会话',model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:stamp,updatedAt:stamp,messages:[],activity:[]};
  const runId='turn_stream';
  await page.addInitScript(runId=>{
   const original=window.fetch;
   window.fetch=function(input,init) {
    if(String(input).endsWith('/messages') && init?.method==='POST') {
     const msg=(id,role,content,run='')=>({id,role,content,runId:run,createdAt:'2026-01-01T00:00:00Z'});
     const encoder=new TextEncoder(), event=data=>encoder.encode('data: '+JSON.stringify(data)+'\n\n');
     return Promise.resolve(new Response(new ReadableStream({start(controller){
      controller.enqueue(event({type:'session',message:msg('user','user','你好'),status:'running'}));
      controller.enqueue(event({type:'run_stats',runStats:{runId,userMessageId:'user',startedAt:'2026-01-01T00:00:00Z',filesRead:0,filesWritten:0,toolCalls:0,contextTokens:12}}));
      controller.enqueue(event({type:'message_delta',message:{...msg('draft-before-tool','assistant','工具前暂存',runId),createdAt:'2026-01-01T00:00:01Z'}}));
      controller.enqueue(event({type:'activity',activity:{id:'step',runId,kind:'read_file',title:'阅读文件',detail:'README.md',status:'completed',startedAt:'2026-01-01T00:00:01Z',endedAt:'2026-01-01T00:00:02Z'}}));
      controller.enqueue(event({type:'message_delta',message:{...msg('answer','assistant','第一段'.repeat(80)+'最新尾部',runId),createdAt:'2026-01-01T00:00:03Z'}}));
      window.releaseAgentStream=()=>{controller.enqueue(event({type:'message',message:{...msg('answer','assistant','第一段第二段',runId),createdAt:'2026-01-01T00:00:03Z'}}));controller.enqueue(event({type:'run_stats',runStats:{runId,userMessageId:'user',startedAt:'2026-01-01T00:00:00Z',endedAt:'2026-01-01T00:00:04Z',filesRead:1,filesWritten:0,toolCalls:1,contextTokens:12}}));controller.enqueue(event({type:'status',status:'completed'}));controller.close();};
     }}),{headers:{'Content-Type':'text/event-stream'}}));
    }
    return original.call(this,input,init);
   };
  },runId);
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/sessions') return route.fulfill({json:{sessions:[session]}});
   if(path.endsWith('/history')) return route.fulfill({json:{session,before:''}});
   if(path==='/api/agent/access-mode') return route.fulfill({json:{mode:'safe'}});
   if(path==='/api/dashboard/events') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
   return route.fulfill({json:{models:[],accounts:[],sessions:[]}});
  });
  await page.goto('http://127.0.0.1:5199');
  await page.locator('.agent-session-select[title="流式会话"]').click();
  const editor=page.getByRole('textbox',{name:'给小涛发消息'}); await editor.fill('你好'); await editor.press('Enter');
  await page.locator('.agent-run-timeline').waitFor();
  assert.equal(await page.locator('.agent-message.role-assistant').count(),0,'streamed text stays out of the conversation while tools can still follow');
  assert.equal(await page.getByText('工具前暂存',{exact:true}).count(),0,'tool narration does not flash in the conversation');
  assert.equal(await page.getByText('第一段第二段',{exact:true}).count(),0);
  const bubble = page.locator('.agent-run-bubble');
  const bubbleText = bubble.locator('.agent-result-viewport');
  assert.equal(await bubble.evaluate(element => getComputedStyle(element).animationName),'agent-stream-flow');
  assert.ok(await bubbleText.evaluate(element => element.scrollTop > 0),'two-line bubble follows the newest text');
  assert.ok(await bubbleText.evaluate(element => element.scrollHeight > element.clientHeight));
  const lineHeight = await bubbleText.evaluate(element => parseFloat(getComputedStyle(element.firstElementChild).lineHeight));
  await page.waitForTimeout(300);
  const textHeight = await bubbleText.evaluate(element => element.getBoundingClientRect().height);
  assert.ok(Math.abs(textHeight - lineHeight * 2) <= 1,'stream text is clamped to two lines');
  await page.emulateMedia({reducedMotion:'reduce'});
  assert.equal(await bubble.evaluate(element => getComputedStyle(element).animationName),'none');
  await page.emulateMedia({reducedMotion:'no-preference'});
  await page.evaluate(()=>window.releaseAgentStream()); await page.getByText('第一段第二段',{exact:true}).waitFor();
  assert.equal(await page.locator('.agent-result-viewport.is-expanded').count(),1);
  await page.waitForFunction(()=>document.querySelector('.agent-process-collapse.is-collapsed'));
  assert.match(await page.locator('.agent-final-meta').textContent(),/共思考/);
 } finally {await browser?.close();server.kill();}
});
