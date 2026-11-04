import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
test('agent displays streamed chunks before completion without duplicates', {timeout: 60000}, async () => {
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5193','--strictPort'],{stdio:['ignore','pipe','pipe']}); let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in')) resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch(process.env.XIAOTAO_TEST_EXECUTABLE ? {executablePath:process.env.XIAOTAO_TEST_EXECUTABLE,headless:true} : {channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
  const page=await browser.newPage();
  const stamp='2026-01-01T00:00:00Z';
  const session={id:'stream-test',title:'流式会话',model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:stamp,updatedAt:stamp,messages:[],activity:[]};
  await page.addInitScript(()=>{
   const original=window.fetch;
   window.fetch=function(input,init) {
    if(String(input).endsWith('/messages') && init?.method==='POST') {
     const msg=(id,role,content)=>({id,role,content,createdAt:'2026-01-01T00:00:00Z'});
     const encoder=new TextEncoder(), event=data=>encoder.encode('data: '+JSON.stringify(data)+'\n\n');
     return Promise.resolve(new Response(new ReadableStream({start(controller){
      controller.enqueue(event({type:'session',message:msg('user','user','你好'),status:'running'}));
      controller.enqueue(event({type:'message_delta',message:msg('answer','assistant','第一段')}));
      window.releaseAgentStream=()=>{controller.enqueue(event({type:'message_delta',message:msg('answer','assistant','第二段')}));controller.enqueue(event({type:'message',message:msg('answer','assistant','第一段第二段')}));controller.enqueue(event({type:'status',status:'completed'}));controller.close();};
     }}),{headers:{'Content-Type':'text/event-stream'}}));
    }
    return original.call(this,input,init);
   };
  });
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/sessions') return route.fulfill({json:{sessions:[session]}});
   if(path.endsWith('/history')) return route.fulfill({json:{session,before:''}});
   if(path==='/api/agent/access-mode') return route.fulfill({json:{mode:'safe'}});
   if(path==='/api/dashboard/events') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
   return route.fulfill({json:{models:[],accounts:[],sessions:[]}});
  });
  await page.goto('http://127.0.0.1:5193');
  await page.locator('.agent-session-select[title="流式会话"]').click();
  const editor=page.getByRole('textbox',{name:'给小涛发消息'}); await editor.fill('你好'); await editor.press('Enter');
  await page.getByText('第一段',{exact:true}).waitFor(); assert.equal(await page.getByText('第一段第二段',{exact:true}).count(),0);
  await page.evaluate(()=>window.releaseAgentStream()); await page.getByText('第一段第二段',{exact:true}).waitFor();
  assert.equal(await page.locator('.agent-message.role-assistant').count(),1);
 } finally {await browser?.close();server.kill();}
});
