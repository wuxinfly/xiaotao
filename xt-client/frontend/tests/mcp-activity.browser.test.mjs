import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('MCP activities identify browser actions and expose bounded plain text details', {timeout:60000}, async()=>{
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5196','--strictPort'],{stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',d=>{if(d.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1100,height:850},reducedMotion:'reduce'});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const stamp=n=>new Date(Date.UTC(2026,10,5,12,0,n)).toISOString();
  const session={id:'mcp-activity',title:'浏览器验收',model:'test',project:{path:'E:/fixture',name:'测试'},status:'completed',createdAt:stamp(0),updatedAt:stamp(5),messages:[{id:'u',role:'user',content:'打开网页并检查控制台',createdAt:stamp(0)},{id:'a',role:'assistant',content:'检查完成',createdAt:stamp(5)}],activity:[
   {id:'open',kind:'mcp_0',title:'打开网页',detail:'url: https://example.com',tool:'chrome-devtools/new_page',input:'{\n  "url": "https://example.com"\n}',output:'Example Domain\n<script>not executable</script>',status:'completed',startedAt:stamp(1),endedAt:stamp(2)},
   {id:'console',kind:'mcp_1',title:'检查控制台消息',tool:'chrome-devtools/list_console_messages',input:'{}',output:'Connection failed',status:'failed',startedAt:stamp(2),endedAt:stamp(3)},
   {id:'legacy',kind:'read_file',title:'阅读文件',detail:'README.md',status:'completed',startedAt:stamp(3),endedAt:stamp(4)}
  ]};
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/sessions')return route.fulfill({json:{sessions:[session]}});
   if(path==='/api/agent/sessions/mcp-activity/history')return route.fulfill({json:{session,before:''}});
   if(path==='/api/agent/sessions/mcp-activity')return route.fulfill({json:session});
   return route.fulfill({json:{models:[{model:'test',label:'test'}],sessions:[],accounts:[],providers:[],mode:'safe'}});
  });
  await page.goto('http://127.0.0.1:5196');
  await page.locator('.agent-session-select').filter({hasText:'浏览器验收'}).click();
  await page.locator('.agent-process-summary').click();
  const panel=page.locator('.agent-process-popover.is-visible');
  await panel.getByText('打开网页',{exact:true}).waitFor();
  await panel.getByText('url: https://example.com',{exact:true}).waitFor();
  assert.equal(await panel.locator('details[open]').count(),0);
  await panel.getByLabel('查看打开网页的参数与结果').click();
  await panel.getByText('chrome-devtools/new_page',{exact:true}).waitFor();
  assert.ok(await panel.locator('pre').first().innerText().then(text=>text.includes('https://example.com')));
  assert.equal(await panel.locator('script').count(),0,'result must render as plain text');
  await panel.getByLabel('查看检查控制台消息的参数与结果').click();
  await panel.getByText('失败原因',{exact:true}).waitFor();
  await panel.getByText('Connection failed',{exact:true}).waitFor();
  await panel.getByText('阅读文件',{exact:true}).waitFor();
  assert.equal(await panel.locator('details').count(),2,'legacy activity stays compatible');
  await panel.evaluate(el=>{el.scrollTop=0;});
  await page.screenshot({path:join(tmpdir(),'xiaotao-mcp-activity.png')});
  for(const width of [1100,390]) {
   await page.setViewportSize({width,height:850});
   const box=await panel.boundingBox();assert.ok(box.x>=0 && box.x+box.width<=width,'process panel outside viewport');
   for(const pre of await panel.locator('pre').all()) {const b=await pre.boundingBox();assert.ok(b.x+b.width<=width,'result outside viewport');}
  }
  await page.keyboard.press('Escape');
  await page.getByRole('button',{name:'切换到浅海亮色主题'}).click();
  await page.locator('.agent-process-summary').click();
  await page.screenshot({path:join(tmpdir(),'xiaotao-mcp-activity-light.png')});
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});
