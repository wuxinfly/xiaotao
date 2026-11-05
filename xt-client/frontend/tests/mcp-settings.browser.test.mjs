import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('MCP settings save drafts, test connections and keep errors editable', {timeout:60000}, async () => {
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5197','--strictPort'],{stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',d=>{if(d.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1100,height:850},reducedMotion:'reduce'});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  let config={mcpServers:{},source:'empty'}, failSave=false, failTest=false;
  await page.route('**/api/**',route=>{
   const req=route.request(),path=new URL(req.url()).pathname;
   if(path==='/api/agent/mcp/test')return failTest ? route.fulfill({status:400,json:{error:{message:'连接失败: fixture error'}}}) : route.fulfill({json:{tools:['new_page','list_console_messages']}});
   if(path==='/api/agent/mcp'){
    if(req.method()==='GET')return route.fulfill({json:config});
    if(failSave)return route.fulfill({status:400,json:{error:{message:'磁盘写入失败'}}});
    config={...req.postDataJSON(),source:'local'};return route.fulfill({json:{ok:true}});
   }
   return route.fulfill({json:{sessions:[],models:[],accounts:[],providers:[],mode:'safe'}});
  });
  await page.goto('http://127.0.0.1:5197');
  await page.getByRole('button',{name:'打开 Agent 设置'}).click();
  await page.getByRole('button',{name:'MCP 工具',exact:true}).click();
  await page.getByText('还没有 MCP 服务',{exact:true}).waitFor();
  await page.getByRole('button',{name:'添加 Chrome 浏览器工具'}).click();
  assert.equal(await page.getByLabel('启动命令',{exact:true}).inputValue(),'npx.cmd');
  await page.getByRole('button',{name:'测试连接',exact:true}).click();
  await page.getByText('连接成功 · 2 个工具',{exact:true}).waitFor();
  assert.deepEqual(config.mcpServers,{},'test must not save');
  await page.getByRole('button',{name:'常用模型',exact:true}).click();
  await page.getByRole('button',{name:'MCP 工具',exact:true}).click();
  assert.equal(await page.getByLabel('启动命令',{exact:true}).inputValue(),'npx.cmd','tab switch preserves draft');
  await page.getByLabel('启用服务',{exact:true}).uncheck();
  await page.getByRole('button',{name:'保存 MCP 配置'}).click();
  await page.getByText('已保存，下一次任务生效。',{exact:true}).waitFor();
  assert.equal(config.mcpServers['chrome-devtools'].disabled,true);
  assert.deepEqual(config.mcpServers['chrome-devtools'].args,['-y','chrome-devtools-mcp@latest']);
  failSave=true;
  await page.getByLabel('启动命令',{exact:true}).fill('missing-command');
  await page.getByRole('button',{name:'保存 MCP 配置'}).click();
  await page.getByRole('alert').filter({hasText:'磁盘写入失败'}).waitFor();
  assert.equal(await page.getByLabel('启动命令',{exact:true}).inputValue(),'missing-command');
  failTest=true;
  await page.getByRole('button',{name:'测试连接',exact:true}).click();
  await page.getByRole('alert').filter({hasText:'fixture error'}).waitFor();
  for(const width of [1100,390]) {
   await page.setViewportSize({width,height:850});
   const b=await page.locator('.mcp-card').boundingBox();
   assert.ok(b.x>=0 && b.x+b.width<=width,'card outside viewport');
  }
  await page.screenshot({path:join(tmpdir(),'xiaotao-mcp-settings.png')});
  await page.getByRole('button',{name:'关闭',exact:true}).click();
  await page.getByRole('dialog').waitFor({state:'hidden'});
  await page.getByRole('button',{name:'切换到浅海亮色主题'}).click();
  await page.getByRole('button',{name:'打开 Agent 设置'}).click();
  await page.getByRole('button',{name:'MCP 工具',exact:true}).click();
  await page.getByLabel('启动命令',{exact:true}).waitFor();
  assert.equal(await page.getByLabel('启动命令',{exact:true}).inputValue(),'npx.cmd','reopen loads saved config');
  await page.screenshot({path:join(tmpdir(),'xiaotao-mcp-settings-light.png')});
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});
