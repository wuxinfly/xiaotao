import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('local Skills drafts, scanning, toggles, errors, themes and narrow layouts', {timeout:60000}, async () => {
  const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5198','--strictPort'],{stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
    browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
    const page=await browser.newPage({viewport:{width:1100,height:900},reducedMotion:'reduce'});
    const errors=[]; page.on('pageerror',error=>errors.push(error.message));
    let config={directories:[],disabledSkills:[]},failSave=false,failRead=true,scans=0,saves=0;
    const id='a'.repeat(64);
    const catalog=settings=>({settings,skills:settings.directories.some(dir=>!dir.disabled)?[{id,name:'review',description:'Review the code and report concrete findings.',path:'C:\\Skills\\review\\SKILL.md',directory:settings.directories[0].path,enabled:!settings.disabledSkills.includes(id)}]:[],issues:settings.directories.length?[{path:'C:\\Skills\\broken\\SKILL.md',message:'缺少 YAML front matter'}]:[]});
    await page.route('**/api/**',route=>{
      const req=route.request(),path=new URL(req.url()).pathname;
      if(path==='/api/agent/skills/scan'){scans++;return route.fulfill({json:catalog(req.postDataJSON())});}
      if(path==='/api/agent/skills') {
        if(req.method()==='GET') return failRead?route.fulfill({status:500,json:{error:{message:'配置暂不可读'}}}):route.fulfill({json:catalog(config)});
        if(failSave)return route.fulfill({status:400,json:{error:{message:'磁盘写入失败'}}});
        config=req.postDataJSON();saves++;return route.fulfill({json:catalog(config)});
      }
      if(path==='/api/agent/mcp')return route.fulfill({json:{mcpServers:{},source:'empty'}});
      return route.fulfill({json:{sessions:[],models:[],accounts:[],providers:[],mode:'safe'}});
    });
    await page.goto('http://127.0.0.1:5198');
    await page.getByRole('button',{name:'打开 Agent 设置'}).click();
    await page.getByRole('button',{name:'Skills 技能',exact:true}).click();
    await page.getByRole('alert').filter({hasText:'配置暂不可读'}).waitFor();
    failRead=false;
    await page.getByRole('button',{name:'重试读取技能配置'}).click();
    await page.getByText('还没有添加本地技能',{exact:true}).waitFor();
    await page.getByText('始终启用',{exact:true}).waitFor();
    await page.getByRole('button',{name:'添加目录',exact:true}).click();
    await page.getByLabel('技能目录路径 1',{exact:true}).fill('C:\\Skills');
    await page.getByRole('button',{name:'常用模型',exact:true}).click();
    await page.getByRole('button',{name:'Skills 技能',exact:true}).click();
    assert.equal(await page.getByLabel('技能目录路径 1',{exact:true}).inputValue(),'C:\\Skills');
    await page.getByRole('button',{name:'扫描目录',exact:true}).click();
    await page.getByText('已发现 1 个技能',{exact:true}).waitFor();
    assert.deepEqual(config.directories,[],'draft scan must not persist');
    await page.getByText('缺少 YAML front matter',{exact:true}).waitFor();
    await page.getByLabel('启用 review',{exact:true}).uncheck();
    await page.getByText('0 个可用',{exact:true}).waitFor();
    await page.getByRole('button',{name:'保存 Skills 配置'}).click();
    await page.getByText('已保存，下一次发送消息生效。',{exact:true}).waitFor();
    assert.deepEqual(config.disabledSkills,[id]);assert.equal(saves,1);assert.ok(scans>=2);
    failSave=true;
    await page.getByLabel('技能目录路径 1',{exact:true}).fill('C:\\OtherSkills');
    await page.getByRole('button',{name:'保存 Skills 配置'}).click();
    await page.getByRole('alert').filter({hasText:'磁盘写入失败'}).waitFor();
    assert.equal(await page.getByLabel('技能目录路径 1',{exact:true}).inputValue(),'C:\\OtherSkills');
    await page.getByRole('button',{name:'扫描目录',exact:true}).click();
    await page.getByText('已发现 1 个技能',{exact:true}).waitFor();
    for(const width of [1100,390]) {
      await page.setViewportSize({width,height:900});
      const box=await page.locator('.skills-card').boundingBox();
      assert.ok(box.x>=0&&box.x+box.width<=width,'skill card overflows viewport');
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
    }
    await page.screenshot({path:join(tmpdir(),'xiaotao-skills-settings.png')});
    await page.getByRole('button',{name:'关闭',exact:true}).click();
    await page.getByRole('dialog').waitFor({state:'hidden'});
    await page.setViewportSize({width:1100,height:900});
    await page.emulateMedia({reducedMotion:'no-preference'});
    await page.getByRole('button',{name:'切换到浅海亮色主题'}).click();
    await page.getByRole('button',{name:'打开 Agent 设置'}).click();
    await page.getByRole('button',{name:'Skills 技能',exact:true}).click();
    await page.getByLabel('技能目录路径 1',{exact:true}).waitFor();
    assert.equal(await page.getByLabel('技能目录路径 1',{exact:true}).inputValue(),'C:\\Skills','reopen loads saved config');
    assert.equal(await page.getByLabel('启用 review',{exact:true}).isChecked(),false);
    await page.getByLabel('技能目录路径 1',{exact:true}).focus();
    await page.keyboard.press('Tab');
    assert.ok(await page.locator(':focus').count(),'keyboard focus lost');
    await page.screenshot({path:join(tmpdir(),'xiaotao-skills-settings-light.png')});
    assert.deepEqual(errors,[]);
  } finally {await browser?.close();server.kill();}
});
