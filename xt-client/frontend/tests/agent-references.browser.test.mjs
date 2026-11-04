import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('agent references search, insert and send structured ids', {timeout:60000}, async () => {
  const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5198','--strictPort'],{stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in')) resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
    browser=await chromium.launch(process.env.XIAOTAO_TEST_EXECUTABLE ? {executablePath:process.env.XIAOTAO_TEST_EXECUTABLE,headless:true} : {channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
    const page=await browser.newPage({viewport:{width:1200,height:800}});
    const stamp='2026-01-01T00:00:00Z';
    const session={id:'current',title:'当前会话',model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:stamp,updatedAt:stamp,messages:[],activity:[]};
    const source={sessionId:'source',title:'登录讨论',project:{path:'E:/fixture',name:'测试项目'},updatedAt:stamp};
    let sent=null, referenceQuery='';
    await page.addInitScript(()=>{
      const original=window.fetch;
      window.fetch=function(input,init) {
        if(String(input).endsWith('/messages') && init?.method==='POST') {
          window.referenceSent=JSON.parse(String(init.body));
          const encoder=new TextEncoder(), event=data=>encoder.encode('data: '+JSON.stringify(data)+'\n\n');
          return Promise.resolve(new Response(new ReadableStream({start(controller){
            controller.enqueue(event({type:'session',message:{id:'user',role:'user',content:'接着做',references:[{id:'ref_1',sourceSessionId:'source',title:'登录讨论',summary:'摘要',lastPosition:2,sourceAvailable:true,sourceUpdatedAt:'2026-01-01T00:00:00Z',createdAt:'2026-01-01T00:00:00Z'}],createdAt:'2026-01-01T00:00:00Z'},status:'running'}));
            controller.enqueue(event({type:'message',message:{id:'answer',role:'assistant',content:'收到',createdAt:'2026-01-01T00:00:00Z'}}));
            controller.enqueue(event({type:'status',status:'completed'}));
            controller.close();
          }}),{headers:{'Content-Type':'text/event-stream'}}));
        }
        return original.call(this,input,init);
      };
    });
    await page.route('**/api/**',route=>{
      const url=new URL(route.request().url()), path=url.pathname;
      if(path==='/api/agent/sessions') return route.fulfill({json:{sessions:[session]}});
      if(path==='/api/agent/sessions/current/messages') return route.fulfill({json:{}});
      if(path.endsWith('/history')) return route.fulfill({json:{session,before:''}});
      if(path==='/api/agent/references/sessions') {
        referenceQuery=url.searchParams.get('query') || '';
        return route.fulfill({json:{sessions:[source]}});
      }
      if(path==='/api/agent/access-mode') return route.fulfill({json:{mode:'safe'}});
      if(path==='/api/agent/models') return route.fulfill({json:{models:[{model:'test',label:'测试模型'}]}});
      if(path==='/api/dashboard/events') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      return route.fulfill({json:{models:[],accounts:[],sessions:[]}});
    });
    await page.addInitScript(()=>localStorage.setItem('xiaotao.agent.enabledModels','["test"]'));
    await page.goto('http://127.0.0.1:5198');
    await page.locator('.agent-session-select[title="当前会话"]').click();
    const editor=page.getByRole('textbox',{name:'给小涛发消息'});
    const referenceResponse=page.waitForResponse(response=>response.url().includes('/api/agent/references/sessions')&&new URL(response.url()).searchParams.get('query')==='登录');
    await editor.fill('@登录');
    await referenceResponse;
    await page.waitForFunction(()=>!!document.querySelector('.agent-reference-picker'));
    await page.locator('.agent-reference-picker').waitFor({state:'visible'});
    await page.waitForFunction(()=>document.querySelector('.agent-reference-picker')?.classList.contains('is-visible'));
    assert.match(await page.locator('.agent-reference-picker').evaluate(el=>getComputedStyle(el).transition),/opacity|transform/);
    assert.equal(referenceQuery,'登录');
    const pickerGap=await page.evaluate(()=>{
      const picker=document.querySelector('.agent-reference-picker').getBoundingClientRect();
      const composer=document.querySelector('.agent-editor').getBoundingClientRect();
      return picker.bottom<=composer.top ? composer.top-picker.bottom : Math.max(0,picker.top-composer.top);
    });
    assert.ok(pickerGap<=12,`reference picker is ${pickerGap}px from the composer`);
    await page.getByRole('option',{name:/登录讨论/}).click();
    await page.waitForFunction(()=>document.querySelector('.agent-editor .agent-conversation-reference'));
    await page.waitForFunction(()=>!document.querySelector('.agent-send').disabled);
    await editor.pressSequentially(' 接着做');
    await editor.press('Enter');
    await page.getByText('收到',{exact:true}).waitFor();
    sent=await page.evaluate(()=>window.referenceSent);
    assert.deepEqual(sent.references,['source']);
    assert.ok(sent.content.includes('@登录讨论'));
    await page.getByRole('button',{name:/登录讨论/}).click();
    await page.getByRole('dialog',{name:/登录讨论/}).waitFor();
    await page.emulateMedia({reducedMotion:'reduce'});
    await page.setViewportSize({width:540,height:760});
    assert.equal(await page.locator('.agent-message-reference').evaluateAll(items=>items.every(item=>{const box=item.getBoundingClientRect(),feed=document.querySelector('.agent-feed').getBoundingClientRect();return box.left>=feed.left&&box.right<=feed.right+1;})),true);
  } finally {await browser?.close();server.kill();}
});
