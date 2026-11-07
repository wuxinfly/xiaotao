import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('formal answer streams monotonically, survives tools, history, stop and later runs', {timeout: 60000}, async () => {
 const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5197', '--strictPort'], {stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1200,height:800}});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const at=n=>new Date(Date.UTC(2026,10,7,0,0,n)).toISOString();
  const session={id:'answer',title:'回答阶段',model:'test',project:{path:'E:/fixture',name:'测试'},status:'completed',messages:[],activity:[],thoughts:[],runStats:[],createdAt:at(0),updatedAt:at(0)};
  const stats={runId:'r1',userMessageId:'u1',phase:'working',startedAt:at(1),filesRead:1,filesWritten:0,toolCalls:1,contextTokens:50};
  await page.addInitScript(()=>{
   localStorage.setItem('xiaotao.agent.enabledModels',JSON.stringify(['test']));
   const native=window.fetch;
   window.fetch=(url,init)=>{
    if(String(url).endsWith('/messages') && init?.method==='POST')return Promise.resolve(new Response(new ReadableStream({start(controller){window.answerStream=controller;}}),{headers:{'Content-Type':'text/event-stream'}}));
    return native(url,init);
   };
  });
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/sessions')return route.fulfill({json:{sessions:[session,{...session,id:'other',title:'另一个会话'}]}});
   if(path.endsWith('/history'))return route.fulfill({json:{session:path.includes('/other/') ? {...session,id:'other',title:'另一个会话',messages:[],runStats:[]} : session,before:''}});
   if(path==='/api/agent/models')return route.fulfill({json:{models:[{model:'test',label:'测试模型'}]}});
   if(path.endsWith('/access-mode'))return route.fulfill({json:{mode:'full'}});
   return route.fulfill({json:{}});
  });
  await page.goto('http://127.0.0.1:5197');
  await page.locator('.agent-session-select').filter({hasText:'回答阶段'}).click();
  const emit=async events=>{await page.evaluate(events=>{for(const event of events)window.answerStream.enqueue(new TextEncoder().encode('data: '+JSON.stringify({...event,sessionId:'answer'})+'\n\n'));},events);};
  const send=async()=>{await page.locator('.tiptap').fill('检查');await page.getByRole('button',{name:'发送消息',exact:true}).click();await page.waitForFunction(()=>window.answerStream);};
  await send();
  await emit([{type:'session',status:'running',message:{id:'u1',role:'user',runId:'r1',content:'检查',createdAt:at(1)}},{type:'run_stats',runStats:stats},{type:'message_delta',message:{id:'draft',role:'assistant',runId:'r1',content:'中间说明',createdAt:at(2)}}]);
  await page.getByText('中间说明',{exact:true}).waitFor();
  assert.equal(await page.locator('.role-assistant').count(),0);
  const answer={id:'draft',role:'assistant',runId:'r1',content:'**检查完成**\n\n- 第一项\n- 第二项\n\n'+Array.from({length:30},(_,i)=>'段落 '+i+'：详细结论。').join('\n\n'),createdAt:at(3)};
  await emit([{type:'activity',activity:{id:'read',runId:'r1',kind:'read_file',title:'阅读文件',status:'completed',startedAt:at(2)}},{type:'message_delta',message:answer}]);
  const bubble=page.locator('.agent-run-bubble');
  await bubble.locator('strong').waitFor();
  assert.equal(await bubble.locator('li').count(),2);
  const node=await bubble.elementHandle();
  const viewport=bubble.locator('.agent-result-viewport');
  await page.waitForTimeout(300);
  assert.ok(await viewport.evaluate(el=>el.clientHeight<=47 && el.scrollHeight>el.clientHeight));
  await page.emulateMedia({reducedMotion:'reduce'});
  assert.equal(await viewport.evaluate(el=>getComputedStyle(el).transitionDuration),'0s');
  await page.emulateMedia({reducedMotion:'no-preference'});
  await emit([{type:'answer_end',message:answer},{type:'run_stats',runStats:{...stats,phase:'completed',endedAt:at(8)}},{type:'status',runId:'r1',status:'completed'}]);
  await page.evaluate(()=>window.answerStream.close());
  await page.locator('.agent-final-meta').waitFor();
  await page.waitForTimeout(400);
  assert.ok(await node.evaluate(el=>el.isConnected),'completion expands the same DOM bubble');
  assert.ok(await viewport.evaluate(el=>el.clientHeight>=el.scrollHeight-1),'complete answer is fully expanded');
  const summary=page.locator('.agent-final-meta');
  assert.ok((await summary.boundingBox()).y >= (await bubble.boundingBox()).y+(await bubble.boundingBox()).height);
  assert.equal(await page.locator('.agent-process-collapse').getAttribute('aria-hidden'),'true');
  await summary.getByRole('button').click();
  await page.getByRole('dialog',{name:'本轮思考过程'}).getByText('阅读文件',{exact:true}).waitFor();
  await page.keyboard.press('Escape');
  for(const width of [1200,390]) {
    await page.setViewportSize({width,height:800});
    assert.ok(await page.locator('.agent-feed').evaluate(el=>el.scrollWidth<=el.clientWidth+1));
    assert.ok(await summary.evaluate(el=>el.clientHeight<35),'summary stays on one line');
  }
  await page.setViewportSize({width:1200,height:800});
  await page.screenshot({path:process.env.TEMP+'/xiaotao-agent-result.png',fullPage:true});
  session.messages=[{id:'u1',role:'user',runId:'r1',content:'检查',createdAt:at(1)},answer];
  session.runStats=[{...stats,phase:'completed',endedAt:at(8)}];
  await page.reload();
  await page.locator('.agent-session-select').filter({hasText:'回答阶段'}).click();
  await page.locator('.agent-result-viewport.is-expanded').waitFor();
  assert.equal(await page.locator('.agent-run-bubble strong').textContent(),'检查完成');
  for(const status of ['failed','canceled']) {
    await send();
    const runId='r-'+status;
    await emit([{type:'session',status:'running',message:{id:'u-'+status,role:'user',runId,content:'中断',createdAt:at(10)}},{type:'run_stats',runStats:{...stats,runId,userMessageId:'u-'+status}},{type:'message_delta',message:{id:'partial-'+status,role:'assistant',runId,content:'部分文字',createdAt:at(11)}},{type:'message',message:{id:'partial-'+status,role:'assistant',runId,content:'部分文字',createdAt:at(11)}},{type:'run_stats',runStats:{...stats,runId,phase:status,endedAt:at(12)}},{type:'status',runId,status}]);
    await page.evaluate(()=>window.answerStream.close());
    const run=page.locator('[data-run-id="'+runId+'"]');
    await run.locator('.agent-final-meta').waitFor();
    assert.match(await run.textContent(),status==='failed' ? /任务失败/ : /任务已停止/);
    assert.match(await run.locator('.agent-run-bubble').textContent(),/部分文字/);
    await page.waitForFunction(()=>document.querySelector('.agent-send')?.getAttribute('aria-label')==='发送消息');
  }
  session.messages=[{id:'wide',role:'user',content:'query'+ 'x'.repeat(3000)+'\n\n```go\n'+ 'error'.repeat(600)+'\n```',createdAt:at(30)}];
  session.runStats=[];
  await page.reload();
  await page.locator('.agent-session-select').filter({hasText:'回答阶段'}).click();
  await page.locator('[data-message-id="wide"] pre').waitFor();
  for(const width of [1200,620]) {
   await page.setViewportSize({width,height:800});
   const layout=await page.locator('[data-message-id="wide"]').evaluate(el=>{
    const feed=el.closest('.agent-feed'), content=el.querySelector('.agent-message-content'), pre=el.querySelector('pre');
    return {feedFits:feed.scrollWidth<=feed.clientWidth+1,contentFits:content.getBoundingClientRect().right<=feed.getBoundingClientRect().right+1,codeScrolls:pre.scrollWidth>pre.clientWidth};
   });
   assert.deepEqual(layout,{feedFits:true,contentFits:true,codeScrolls:true});
  }
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});
