import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

// Isolated fixtures only: intercept every API request, never reach a user's service.
test('large sidebar and paged history interactions', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5188', '--strictPort'], {cwd:process.cwd(), stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if(data.toString().includes('ready in')) resolve();})), once(server,'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1200,height:800}});
    const errors=[]; page.on('pageerror', error=>errors.push(error.message));
    const stamp = n => new Date(Date.UTC(2026,0,1,0,0,n)).toISOString();
    const sessions=Array.from({length:10000}, (_,i)=>({id:'s'+i,title:'会话 '+i,model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:stamp(0),updatedAt:stamp(10000-i),messages:[],activity:[]}));
    let failOlder=true;
    let holdOlder=false, releaseOlder;
    let fullReads=0;
    await page.route('**/api/**', async route=> {
      const url=new URL(route.request().url());
      if(url.pathname==='/api/agent/models') return route.fulfill({json:{models:[{model:'test',label:'测试模型'}]}});
      if(url.pathname==='/api/agent/access-mode') return route.fulfill({json:{mode:'full'}});
      if(url.pathname==='/api/agent/sessions') return route.fulfill({json:{sessions}});
      if (/\/sessions\/s\d+$/.test(url.pathname) && route.request().method()==='GET') fullReads++;
      if (url.pathname.endsWith('/messages') && route.request().method()==='POST') {
        const events=[{type:'session',message:{id:'live-user',role:'user',content:'新的问题',createdAt:stamp(101)},status:'running'},{type:'message',message:{id:'live-answer',role:'assistant',content:'新的回复',createdAt:stamp(102)}},{type:'status',status:'completed'}];
        return route.fulfill({contentType:'text/event-stream',body:events.map(event=>'data: '+JSON.stringify(event)+'\n\n').join('')});
      }
      const match=url.pathname.match(/\/sessions\/(s\d+)\/history$/);
      if(match) {
        const older=!!url.searchParams.get('before');
        if(older && failOlder) {failOlder=false;return route.fulfill({status:500,json:{error:{message:'fixture failure'}}});}
        if (older && holdOlder) await new Promise(resolve=>{releaseOlder=resolve;});
        const messages=Array.from({length:older?20:80},(_,i)=>({id:'m'+(i+(older?0:20)),role:i%2?'assistant':'user',content:'历史消息 '+(i+(older?0:20))+'\n\n```js\nconsole.log("hello");\n```',createdAt:stamp(i+(older?0:20))}));
        return route.fulfill({json:{session:{...sessions[Number(match[1].slice(1))],messages},before:older?'':'20:0'}});
      }
      return route.fulfill({json:{}});
    });
    await page.goto('http://127.0.0.1:5188');
    await page.locator('.agent-group-label').filter({hasText:'测试项目'}).waitFor();
    assert.ok(await page.locator('.agent-session-row').count()<35,'10k sessions must stay windowed');
    await page.locator('.agent-group-label').filter({hasText:'测试项目'}).click();
    assert.equal(await page.locator('.agent-session-row').count(),0,'collapsed children must unmount');
    await page.locator('.agent-group-label').filter({hasText:'测试项目'}).click();
    const nav=page.getByRole('navigation',{name:'会话历史'});
    await nav.focus(); await page.keyboard.press('End');
    await page.waitForFunction(()=>document.activeElement?.getAttribute('title')==='会话 9999');
    assert.ok(await page.locator('.agent-session-row').count()<35);
    await page.keyboard.press('Home');
    await page.waitForFunction(()=>document.activeElement?.classList.contains('agent-group-label'));
    await page.keyboard.press('ArrowDown');
    await page.waitForFunction(()=>document.activeElement?.getAttribute('title')==='会话 0');
    await page.keyboard.press('Enter');
    await page.locator('.agent-message').last().waitFor();
    assert.equal(await page.locator('.agent-message').count(),80);
    await page.waitForFunction(()=>{const el=document.querySelector('.agent-feed');return el.scrollHeight-el.scrollTop-el.clientHeight<5;});
    const jump = page.getByRole('button', {name:'回到底部', exact:true});
    assert.equal(await jump.count(), 0, 'bottom button is inaccessible when already at bottom');
    await page.locator('.agent-feed').evaluate(el=>{el.scrollTop-=400;});
    await jump.waitFor();
    const jumpBox = await jump.boundingBox(), composerBox = await page.locator('.agent-composer').boundingBox();
    assert.ok(Math.abs(jumpBox.x+jumpBox.width/2-composerBox.x-composerBox.width/2)<2, 'button is centered above composer');
    assert.ok(jumpBox.y+jumpBox.height<composerBox.y, 'button stays above composer');
    await jump.focus(); await page.keyboard.press('Enter');
    await page.waitForFunction(()=>{const el=document.querySelector('.agent-feed');return el.scrollHeight-el.scrollTop-el.clientHeight<5;});
    await page.waitForFunction(()=>document.querySelector('.agent-scroll-bottom').getAttribute('aria-hidden')==='true');
    await page.emulateMedia({reducedMotion:'reduce'});
    await page.locator('.agent-feed').evaluate(el=>{el.scrollTop-=400;});
    await jump.click();
    await page.waitForFunction(()=>{const el=document.querySelector('.agent-feed');return el.scrollHeight-el.scrollTop-el.clientHeight<5;});
    await page.locator('.agent-feed').evaluate(el=>{el.scrollTop=0;});
    await page.getByRole('button',{name:'加载失败，点击重试'}).waitFor();
    const before=await page.locator('.agent-message').first().boundingBox();
    await page.getByRole('button',{name:'加载失败，点击重试'}).click();
    await page.waitForFunction(()=>document.querySelectorAll('.agent-message').length===100);
    const after=await page.locator('.agent-message').nth(20).boundingBox();
    assert.ok(Math.abs(after.y-before.y)<3,`prepend shifted old message: ${before.y} -> ${after.y}`);
    const editor=page.getByRole('textbox',{name:'给小涛发消息'});
    const firstNode=await page.locator('.agent-message-content').first().elementHandle();
    await editor.fill('新的问题');
    assert.ok(await firstNode.evaluate(el=>el.isConnected),'typing should preserve old message DOM');
    await editor.press('Enter');
    await page.getByText('新的回复',{exact:true}).waitFor();
    assert.equal(await page.locator('.agent-message').count(),102,'sending should retain loaded history');
    assert.equal(await page.getByRole('button',{name:'加载更早的消息'}).count(),0,'fully loaded history should remain complete');
    await page.locator('.agent-session-select').filter({hasText:'会话 1'}).first().click();
    await page.waitForFunction(()=>document.querySelectorAll('.agent-message').length===80);
    await page.waitForFunction(()=>{const el=document.querySelector('.agent-feed');return el.scrollHeight-el.scrollTop-el.clientHeight<5;});
    holdOlder=true;
    await page.locator('.agent-feed').evaluate(el=>{el.scrollTop=0;});
    await page.getByRole('button',{name:'正在加载历史…'}).waitFor();
    await page.locator('.agent-session-select').filter({hasText:'会话 2'}).first().click();
    await page.waitForFunction(()=>document.querySelector('.agent-context-title strong')?.textContent==='会话 2');
    releaseOlder();
    await page.waitForLoadState('networkidle');
    assert.equal(await page.locator('.agent-message').count(),80,'stale older response must not enter another session');
    await page.emulateMedia({reducedMotion:'reduce'});
    await page.getByRole('button',{name:'会话操作 会话 2',exact:true}).click();
    await page.getByRole('menuitem',{name:'重命名'}).waitFor();
    await page.keyboard.press('Escape');
    assert.equal(fullReads,0,'UI must not request full session history');
    assert.deepEqual(errors,[]);
    console.log('10,000 fixture sessions: bounded rows; collapse, keyboard, menus, retry, 80→100 prepend and switch-to-bottom passed.');
  } finally {
    await browser?.close();
    server.kill();
  }
});
