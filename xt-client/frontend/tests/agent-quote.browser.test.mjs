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
  const stamp='2026-01-01T00:00:00Z'; const messages=[{id:'source',role:'assistant',content:'需要先重启服务，然后检查状态。',createdAt:stamp}];
  const session={id:'quote-test',title:'引用测试',model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:stamp,updatedAt:stamp,messages,activity:[]};
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
  await page.locator('.agent-message.role-assistant .agent-message-content').waitFor();
  async function selectQuote() {
   await page.locator('.agent-message.role-assistant .agent-message-content p').first().evaluate(el=>{
    const text=el.firstChild, range=document.createRange(), selection=window.getSelection();
    range.setStart(text,0); range.setEnd(text,7); selection.removeAllRanges(); selection.addRange(range);
    el.dispatchEvent(new MouseEvent('mouseup',{bubbles:true}));
   });
   await page.getByRole('button',{name:'引用',exact:true}).click();
  }
  await selectQuote(); await page.getByLabel('引用的文字',{exact:true}).waitFor();
  await page.getByRole('button',{name:'移除引用的文字'}).click(); assert.equal(await page.getByLabel('引用的文字',{exact:true}).count(),0);
  await selectQuote(); const editor=page.getByRole('textbox',{name:'给小涛发消息'}); await editor.fill('为什么？'); await page.getByRole('button',{name:'发送消息'}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.agent-message.role-user').length===1);
  assert.match(sent[0],/引用小涛的回复：\n\n> 需要先重启服务\n\n为什么？/);
  assert.equal(await page.locator('.agent-message.role-user blockquote').first().innerText(),'需要先重启服务');
  await page.waitForFunction(()=>!document.querySelector('.agent-send.is-stop'));
  await selectQuote(); await page.getByRole('button',{name:'发送消息'}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.agent-message.role-user').length===2);
  assert.equal(sent[1], '引用小涛的回复：\n\n> 需要先重启服务');
 } finally {await browser?.close();server.kill();}
});
