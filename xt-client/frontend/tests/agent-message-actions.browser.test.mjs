import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('message menu copies the original query and branches a finished answer', {timeout: 60000}, async () => {
 const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5198', '--strictPort'], {stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1200,height:800}});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const at=n=>new Date(Date.UTC(2026,10,9,0,0,n)).toISOString();
  const query='第一问：看看这段代码\n\n```js\nconst a = 1;\n```';
  const answer={id:'ans1',role:'assistant',runId:'r1',content:'**结论**\n\n已经看完。',createdAt:at(3)};
  const stats={runId:'r1',userMessageId:'u1',phase:'completed',startedAt:at(1),endedAt:at(8),filesRead:1,filesWritten:0,toolCalls:1};
  const source={id:'src',title:'第一问',model:'test',project:{path:'E:/fixture',name:'测试'},status:'completed',messages:[{id:'u1',role:'user',runId:'r1',content:query,createdAt:at(1)},answer],activity:[{id:'read',runId:'r1',kind:'read_file',title:'阅读文件',status:'completed',startedAt:at(2)}],thoughts:[],runStats:[stats],hasImages:false,createdAt:at(0),updatedAt:at(8)};
  const branched={...source,id:'br1',title:'第一问 · 分支',runStats:[stats]};
  let branchBody=null;
  await page.addInitScript(()=>{
   localStorage.setItem('xiaotao.agent.enabledModels',JSON.stringify(['test']));
   Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>{window.__copied=text;}}});
  });
  await page.route('**/api/**',async route=>{
   const request=route.request();
   const path=new URL(request.url()).pathname;
   if(path==='/api/agent/sessions/src/branch'&&request.method()==='POST'){branchBody=request.postDataJSON();return route.fulfill({json:branched});}
   if(path==='/api/agent/sessions')return route.fulfill({json:{sessions:branchBody?[branched,source]:[source]}});
   if(path.endsWith('/history')){const id=path.includes('/br1/')?'br1':'src';return route.fulfill({json:{session:id==='br1'?branched:source,before:''}});}
   if(path==='/api/agent/models')return route.fulfill({json:{models:[{model:'test',label:'测试模型'}]}});
   if(path.endsWith('/access-mode'))return route.fulfill({json:{mode:'full'}});
   return route.fulfill({json:{}});
  });
  await page.goto('http://127.0.0.1:5198');
  await page.locator('.agent-session-select').filter({hasText:'第一问'}).click();
  await page.locator('[data-message-id="u1"] .agent-message-content').waitFor();

  // The user query exposes copy directly instead of hiding it in a menu.
  const userRow=page.locator('[data-message-id="u1"]');
  await userRow.hover();
  await page.waitForTimeout(200);
  assert.equal(await userRow.locator('.agent-message-actions-trigger').evaluate(el=>getComputedStyle(el.parentElement).opacity),'1');
  assert.equal(await userRow.getByRole('button',{name:'消息操作'}).count(),0);
  await userRow.getByRole('button',{name:'复制提问'}).click();
  await page.getByText('已复制',{exact:true}).waitFor();
  assert.equal(await page.evaluate(()=>window.__copied),query,'copy keeps the original Markdown');
  assert.equal(await page.getByRole('menu',{name:'消息操作'}).count(),0);

  // Keyboard: the answer menu holds copy, branch and the thought trail.
  const answerActions=page.locator('.agent-final-meta-wrap .agent-message-actions-trigger');
  await answerActions.focus();
  await page.keyboard.press('Enter');
  await page.getByRole('menu',{name:'消息操作'}).waitFor();
  assert.equal(await page.getByRole('menuitem',{name:'复制'}).count(),1);
  assert.equal(await page.getByRole('menuitem',{name:'创建分支'}).count(),1);
  assert.equal(await page.getByRole('menuitem',{name:'查看思考过程'}).count(),1);
  assert.equal(await page.locator('.agent-final-meta-wrap .agent-run-open').count(),0,'no separate process button in the meta row');
  await page.getByRole('menuitem',{name:'查看思考过程'}).click();
  await page.getByRole('dialog',{name:'本轮思考过程'}).getByText('阅读文件',{exact:true}).first().waitFor();
  await page.keyboard.press('Escape');
  await page.getByRole('dialog',{name:'本轮思考过程'}).waitFor({state:'detached'});

  // Branch: the finished answer menu posts the message id and switches sessions.
  await answerActions.click();
  await page.getByRole('menuitem',{name:'创建分支'}).click();
  await page.getByText('已创建分支',{exact:true}).waitFor();
  assert.deepEqual(branchBody,{messageId:'ans1'});
  await page.locator('.agent-context-title strong').filter({hasText:'第一问 · 分支'}).waitFor();
  assert.equal(await page.locator('.agent-session-select[title="第一问 · 分支"]').count(),1);
  assert.equal(await page.locator('.agent-session-select[title="第一问"]').count(),1,'source session stays in the sidebar');
  assert.ok(await page.locator('[data-run-id="r1"] .agent-final-meta').isVisible(),'copied process records render in the branch');

  // A run that is still streaming has no answer menu to branch from.
  const running={...source,id:'run2',title:'进行中',runStats:[{...stats,runId:'r9',userMessageId:'u9',phase:'working',endedAt:undefined}],messages:[{id:'u9',role:'user',runId:'r9',content:'继续',createdAt:at(20)}]};
  await page.unroute('**/api/**');
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/sessions')return route.fulfill({json:{sessions:[running]}});
   if(path.endsWith('/history'))return route.fulfill({json:{session:running,before:''}});
   if(path==='/api/agent/models')return route.fulfill({json:{models:[{model:'test',label:'测试模型'}]}});
   if(path.endsWith('/access-mode'))return route.fulfill({json:{mode:'full'}});
   return route.fulfill({json:{}});
  });
  await page.reload();
  await page.locator('.agent-session-select').filter({hasText:'进行中'}).click();
  await page.locator('[data-run-id="r9"]').waitFor();
  assert.equal(await page.locator('[data-run-id="r9"] .agent-message-actions').count(),0);
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});