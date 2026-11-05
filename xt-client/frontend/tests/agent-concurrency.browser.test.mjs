import assert from 'node:assert/strict';
import {test} from 'node:test';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('sessions stream independently while switching, drafting and stopping', {timeout:60000}, async () => {
  const stamp = new Date().toISOString();
  const sessions = ['alpha','beta'].map(id => ({id,title:id,model:'test',project:{path:'E:/fixture',name:'并发测试'},status:'completed',messages:[],activity:[],createdAt:stamp,updatedAt:stamp}));
  sessions[1].messages = Array.from({length:30},(_,i)=>({id:'history-'+i,role:i%2?'assistant':'user',content:'历史内容 '.repeat(30),createdAt:stamp}));
  const streams = new Map();
  const approvals = [];
  const emit = (id,event) => streams.get(id)?.write(`data: ${JSON.stringify({...event,sessionId:id})}\n\n`);
  const finish = (id,status) => {
    sessions.find(item => item.id === id).status = status;
    emit(id,{type:'status',status}); streams.get(id)?.end(); streams.delete(id);
  };
  const fixture = createServer(async (req,res) => {
    const path = new URL(req.url,'http://fixture').pathname;
    const json = value => {res.setHeader('Content-Type','application/json');res.end(JSON.stringify(value));};
    if(path === '/api/agent/sessions' && req.method === 'GET') return json({sessions});
    const detail = path.match(/^\/api\/agent\/sessions\/([^/]+)$/);
    if (detail && req.method === 'GET') return json(sessions.find(item => item.id === detail[1]));
    const match = path.match(/\/sessions\/([^/]+)\/(history|messages|cancel|approval)$/);
    if(match) {
      const [,id,action] = match;
      const session = sessions.find(item => item.id === id);
      if(action === 'approval') {let raw='';for await(const chunk of req) raw+=chunk;approvals.push({session:id,...JSON.parse(raw)});return json({ok:true});}
      if(action === 'history') return json({session,before:''});
      if(action === 'cancel') {finish(id,'canceled');return json({ok:true});}
      let raw='';for await(const chunk of req) raw+=chunk;
      session.status='running';
      const message={id:id+'-user',role:'user',content:JSON.parse(raw).content,createdAt:stamp};
      session.messages.push(message);
      res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});
      streams.set(id,res);
      emit(id,{type:'session',status:'running',message});
      return;
    }
    return json({sessions:[],models:[{model:'test',label:'test'}],accounts:[],mode:'safe'});
  });
  fixture.listen(0,'127.0.0.1');await once(fixture,'listening');
  const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5193','--strictPort'],{env:{...process.env,XIAOTAO_DEV_API:`http://127.0.0.1:${fixture.address().port}`},stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in')) resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
    browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
    const page=await browser.newPage({viewport:{width:1200,height:850},reducedMotion:'reduce'});
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await page.addInitScript(()=>localStorage.setItem('xiaotao.agent.enabledModels','["test"]'));
    await page.goto('http://127.0.0.1:5193');
    const editor=page.locator('.tiptap');
    const select=async id=>{await page.locator('.agent-session-select').filter({hasText:id}).click();await page.waitForFunction(id=>document.querySelector('.agent-context-title strong')?.textContent===id,id);};
    await select('alpha');await editor.fill('run alpha');await editor.press('Enter');
    await page.getByRole('button',{name:'停止任务'}).waitFor();
    await page.locator('.agent-session-select').filter({hasText:'alpha'}).getByRole('img',{name:'运行中',exact:true}).waitFor();
    await select('beta');await editor.fill('beta draft');
    await page.locator('.agent-feed').evaluate(el=>{el.scrollTop=100;el.dispatchEvent(new Event('scroll'));});
    await select('alpha');await select('beta');assert.equal(await editor.innerText(),'beta draft');
    await page.waitForTimeout(100);
    assert.ok(Math.abs(await page.locator('.agent-feed').evaluate(el=>el.scrollTop)-100)<5,'restores scroll position');
    await editor.fill('run beta');await editor.press('Enter');
    await page.getByRole('button',{name:'停止任务'}).waitFor();
    assert.equal(streams.size,2);
    emit('alpha',{type:'approval',approval:{id:'approval-alpha',tool:'run_command',detail:'alpha command'}});
    emit('beta',{type:'approval',approval:{id:'approval-beta',tool:'run_command',detail:'beta command'}});
    await page.getByRole('alertdialog').filter({hasText:'alpha command'}).waitFor();
    await page.getByRole('button',{name:'允许执行',exact:true}).click();
    await page.getByRole('alertdialog').filter({hasText:'beta command'}).waitFor();
    await page.getByRole('button',{name:'允许执行',exact:true}).click();
    await page.waitForTimeout(100);
    assert.deepEqual(approvals.map(item=>[item.session,item.approved]),[['alpha',true],['beta',true]]);
    const message={id:'alpha-answer',role:'assistant',content:'alpha background reply',createdAt:stamp};
    emit('alpha',{type:'message',message});sessions[0].messages.push(message);
    await select('alpha');await page.getByText('alpha background reply',{exact:true}).waitFor();
    await select('beta');assert.equal(await page.getByText('alpha background reply',{exact:true}).count(),0);
    await page.getByRole('button',{name:'停止任务'}).click();
    await page.waitForFunction(()=>!document.querySelector('button[aria-label="停止任务"]'));
    assert.ok(streams.has('alpha'));assert.ok(!streams.has('beta'));
    await select('alpha');await page.getByRole('button',{name:'新会话',exact:true}).click();
    await editor.fill('new draft');
    finish('alpha','completed');
    await page.waitForTimeout(200);assert.equal(await editor.innerText(),'new draft');
    await page.locator('.agent-session-select').filter({hasText:'alpha'}).getByRole('img',{name:'回答完成，未查看'}).waitFor();
    assert.equal(await page.locator('.agent-session-select').filter({hasText:'beta'}).locator('.agent-session-indicator').count(),0,'canceled sessions have no unread marker');
    await select('beta');await page.getByRole('button',{name:'新会话',exact:true}).click();assert.equal(await editor.innerText(),'new draft');
    await page.reload();
    await page.locator('.agent-session-select').filter({hasText:'alpha'}).getByRole('img',{name:'回答完成，未查看'}).waitFor();
    await select('alpha');
    await page.waitForFunction(()=>!document.querySelector('.agent-session-indicator'));
    await editor.fill('fail alpha');await editor.press('Enter');
    await page.getByRole('button',{name:'停止任务'}).waitFor();
    await select('beta');finish('alpha','failed');
    await page.locator('.agent-session-select').filter({hasText:'alpha'}).getByRole('img',{name:'执行失败，未查看'}).waitFor();
    await select('alpha');
    await page.waitForFunction(()=>!document.querySelector('.agent-session-indicator'));
    assert.deepEqual(errors,[]);
  } finally {
    for(const res of streams.values()) res.end();
    await browser?.close();server.kill();fixture.closeAllConnections();await new Promise(resolve=>fixture.close(resolve));
  }
});
