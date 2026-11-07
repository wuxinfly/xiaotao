import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('browser activation shows up as its own activity', {timeout:60000}, async()=>{
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5198','--strictPort'],{stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',d=>{if(d.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1100,height:850},reducedMotion:'reduce'});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const stamp=n=>new Date(Date.UTC(2026,10,7,12,0,n)).toISOString();
  const session={id:'mcp-lazy',title:'按需启用',model:'test',project:{path:'E:/fixture',name:'测试'},status:'completed',createdAt:stamp(0),updatedAt:stamp(5),messages:[{id:'u',role:'user',content:'打开网页',createdAt:stamp(0)},{id:'a',role:'assistant',content:'完成',createdAt:stamp(5)}],runStats:[{runId:'turn_1',userMessageId:'u',startedAt:stamp(0),endedAt:stamp(5),filesRead:0,filesWritten:0,toolCalls:2,contextTokens:0}],activity:[
   {id:'enable',runId:'turn_1',kind:'enable_mcp_tools',title:'启用浏览器工具',detail:'按需启动 MCP 服务',status:'completed',startedAt:stamp(1),endedAt:stamp(2)},
   {id:'open',runId:'turn_1',kind:'mcp_0',title:'打开网页',detail:'url: https://example.com',tool:'chrome-devtools/new_page',status:'completed',startedAt:stamp(2),endedAt:stamp(3)}
  ]};
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/sessions')return route.fulfill({json:{sessions:[session]}});
   if(path==='/api/agent/sessions/mcp-lazy/history')return route.fulfill({json:{session,before:''}});
   if(path==='/api/agent/sessions/mcp-lazy')return route.fulfill({json:session});
   return route.fulfill({json:{models:[{model:'test',label:'test'}],sessions:[],accounts:[],providers:[],mode:'safe'}});
  });
  await page.goto('http://127.0.0.1:5198');
  await page.locator('.agent-session-select').filter({hasText:'按需启用'}).click();
  await page.getByLabel('查看思考过程').first().click();
  const drawer=page.locator('.agent-thought-drawer');
  await drawer.getByText('启用浏览器工具',{exact:false}).waitFor();
  await drawer.getByText('按需启动 MCP 服务',{exact:false}).waitFor();
  await drawer.getByText('打开网页',{exact:false}).waitFor();
  await page.screenshot({path:join(tmpdir(),'xiaotao-mcp-lazy.png')});
  for(const width of [1100,390]) {
   await page.setViewportSize({width,height:850});
   const box=await drawer.boundingBox();assert.ok(box.x>=0 && box.x+box.width<=width,'thought drawer outside viewport');
  }
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});
