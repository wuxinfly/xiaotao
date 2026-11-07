import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('selected reply text can be quoted, removed, and sent with or without a question', {timeout:60000}, async () => {
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5194','--strictPort'],{stdio:['ignore','pipe','pipe']}); let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in')) resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch(process.env.XIAOTAO_TEST_EXECUTABLE ? {executablePath:process.env.XIAOTAO_TEST_EXECUTABLE,headless:true} : {channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1200,height:800}});
  const stamp='2026-01-01T00:00:00Z'; const turn='turn-quote-1';
  // 真实会话里每条回复都带轮次统计，界面会把 assistant 消息折叠进 RunTimeline（.agent-run-bubble），
  // 不再渲染 .agent-message.role-assistant。这里必须带上 runStats，否则测到的只是线上不存在的旧形状。
  const messages=[{id:'source',role:'assistant',runId:turn,content:'需要先重启服务，然后检查状态。',createdAt:stamp}];
  const runStats=[{runId:turn,userMessageId:'u0',startedAt:stamp,endedAt:stamp,filesRead:1,filesWritten:0,toolCalls:2,contextTokens:10,contextWindow:1000}];
  const session={id:'quote-test',title:'引用测试',model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:stamp,updatedAt:stamp,messages,activity:[],runStats};
  const sent=[];
  await page.route('**/api/**',route=>{
   const req=route.request(), path=new URL(req.url()).pathname;
   if(path==='/api/agent/sessions') return route.fulfill({json:{sessions:[{...session,messages:[]}]}});
   if(path.endsWith('/history')) return route.fulfill({json:{session,before:''}});
   if(path.endsWith('/messages') && req.method()==='POST') {
    const body=req.postDataJSON(); sent.push(body.content); const message={id:'u'+sent.length,role:'user',content:body.content,createdAt:new Date(Date.now()+sent.length*1000).toISOString()}; messages.push(message);
    const events=[{type:'session',message,status:'running'},{type:'status',status:'completed'}];
    return route.fulfill({contentType:'text/event-stream',body:events.map(e=>'data: '+JSON.stringify(e)+'\n\n').join('')});
   }
   if(path==='/api/agent/access-mode') return route.fulfill({json:{mode:'safe'}});
   if(path==='/api/dashboard/events') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
   return route.fulfill({json:{models:[],accounts:[],sessions:[]}});
  });
  await page.goto('http://127.0.0.1:5194'); await page.locator('.agent-session-select[title="引用测试"]').click();
  await page.locator('.agent-run-bubble-text.agent-message-content').waitFor();
  assert.equal(await page.locator('.agent-message.role-assistant').count(), 0, '真实路径下不应再渲染旧消息行');
  async function selectQuote(start, end) {
   await page.locator('.agent-run-bubble-text.agent-message-content p').first().evaluate((el, offsets)=>{
    const text=el.firstChild, range=document.createRange(), selection=window.getSelection();
    range.setStart(text,offsets[0]); range.setEnd(text,offsets[1]); selection.removeAllRanges(); selection.addRange(range);
    el.dispatchEvent(new MouseEvent('mouseup',{bubbles:true}));
   }, [start,end]);
   await page.getByRole('button',{name:'引用',exact:true}).click();
  }
  await selectQuote(0,7); await page.getByLabel('引用的文字',{exact:true}).waitFor();
  await page.locator('.agent-quote-marker').first().waitFor();
  await page.locator('.agent-quote-marker').first().click();
  await page.getByLabel('回复',{exact:true}).fill('为什么要重启？');
  await page.getByRole('button',{name:'完成',exact:true}).click();
  await selectQuote(8,14);
  assert.equal(await page.locator('.agent-quote-chip').count(),2);
  await page.waitForFunction(()=>document.querySelectorAll('.agent-quote-marker').length===2);
  await page.locator('.agent-quote-marker').nth(1).click();
  await page.getByLabel('引用文字',{exact:true}).fill('检查服务状态');
  await page.getByLabel('回复',{exact:true}).fill('如何检查？');
  await page.getByRole('button',{name:'完成',exact:true}).click();
  const editor=page.getByRole('textbox',{name:'给小涛发消息'}); await editor.fill('请分别回答'); await page.getByRole('button',{name:'发送消息'}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.agent-message.role-user').length===1);
  assert.match(sent[0],/引用小涛的回复 1：\n\n> 需要先重启服务\n\n我的回复：为什么要重启？/);
  assert.match(sent[0],/引用小涛的回复 2：\n\n> 检查服务状态\n\n我的回复：如何检查？\n\n请分别回答/);
  assert.equal(await page.locator('.agent-message.role-user blockquote').count(),2);
  assert.equal(await page.locator('.agent-quote-marker').count(),0);
  assert.equal(await page.locator('.agent-quote-highlight').count(),0);
  await page.waitForFunction(()=>!document.querySelector('.agent-send.is-stop'));
  await selectQuote(0,7); await page.getByRole('button',{name:'移除引用 1'}).click();
  assert.equal(await page.locator('.agent-quote-chip').count(),0);
  await selectQuote(0,7); await page.getByRole('button',{name:'发送消息'}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.agent-message.role-user').length===2);
  assert.equal(sent[1], '引用小涛的回复 1：\n\n> 需要先重启服务');

 } finally {await browser?.close();server.kill();}
});
