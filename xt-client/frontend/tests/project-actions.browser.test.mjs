import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('project actions prefill, rename and open folders', {timeout:60000}, async () => {
  const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5196','--strictPort'],{stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in')) resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
    browser=await chromium.launch({channel:'msedge',headless:true});
    const page=await browser.newPage({viewport:{width:1200,height:800}});
    const session={id:'s1',title:'历史会话',model:'test',status:'completed',project:{path:'E:/fixture',name:'测试项目'},createdAt:new Date().toISOString(),updatedAt:new Date().toISOString(),messages:[],activity:[]};
    const savedProjects=[{...session.project,saved:true},{path:'E:/empty',name:'空项目',saved:true}];
    let opened='', renamed='', renamedPath='', deleted='';
    await page.route('**/api/**',route=>{
      const path=new URL(route.request().url()).pathname;
      if(path==='/api/agent/sessions') return route.fulfill({json:{sessions:[session]}});
      if(path==='/api/agent/projects' && route.request().method()==='GET') return route.fulfill({json:{projects:savedProjects}});
      if(path==='/api/agent/project') {const input=route.request().postDataJSON();return route.fulfill({json:{path:input.path,name:input.path==='E:/unsaved'?'未保存项目':'测试项目'}});}
      if(path==='/api/agent/project/name') {const input=route.request().postDataJSON();renamed=input.name;renamedPath=input.path;const saved=savedProjects.find(item=>item.path===input.path);if(saved) saved.name=input.name;if(session.project.path===input.path) session.project.name=input.name;return route.fulfill({json:{ok:true}});}
      if(path==='/api/agent/projects' && route.request().method()==='DELETE') {deleted=route.request().postDataJSON().path;return route.fulfill({json:{ok:true}});}
      if(path==='/api/agent/project/open') {opened=route.request().postDataJSON().path;return route.fulfill({json:{ok:true}});}
      return route.fulfill({json:{accounts:[],providers:[],models:[]}});
    });
    await page.goto('http://127.0.0.1:5196');
    const group=page.locator('.agent-session-group').filter({hasText:'测试项目'}); await group.waitFor();
    assert.equal(await group.locator('.agent-group-chevron').count(),0);
    assert.equal(await group.locator('.agent-group-actions').evaluate(el=>getComputedStyle(el).visibility),'hidden');
    await group.hover();
    await group.locator('.agent-group-label').hover();
    await page.waitForFunction(() => getComputedStyle(document.querySelector('.agent-group-label')).backgroundColor === 'rgba(0, 0, 0, 0)');
    const bounds = await group.evaluate(el => {
      const row = el.getBoundingClientRect();
      return [...el.querySelectorAll('.agent-group-actions button')].map(button => {
        const rect = button.getBoundingClientRect();
        const icon = button.querySelector('svg').getBoundingClientRect();
        return {width:rect.width, height:rect.height, right:row.right-rect.right, centered:Math.abs((icon.left+icon.right-rect.left-rect.right)/2)};
      });
    });
    for (const button of bounds) {
      assert.equal(button.width,26); assert.equal(button.height,26);
      assert.ok(button.right >= 4, 'action must stay inside the project row');
      assert.ok(button.centered < 1, 'icon must be centered');
    }
    await page.mouse.move(1100,700);
    await group.locator('.agent-group-label').focus(); await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(()=>document.activeElement?.getAttribute('aria-label')),'删除项目 测试项目');
    await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(()=>document.activeElement?.getAttribute('aria-label')),'项目操作 测试项目');
    await page.keyboard.press('Enter');
    await page.getByRole('menuitem',{name:'在资源管理器中打开'}).click();
    assert.equal(opened,'E:/fixture');
    await page.locator('.agent-project-trigger').click();
    await page.getByRole('button',{name:'选择新项目'}).click();
    await page.evaluate(() => {window.xtPickFolderOnly=()=>window.xtFolderPicked({path:'E:/unsaved'});});
    await page.getByRole('button',{name:'选择',exact:true}).click();
    await page.getByLabel('已选项目目录').waitFor();
    await page.getByRole('button',{name:'使用这个项目'}).click();
    assert.equal(await page.getByRole('button',{name:'删除项目 未保存项目'}).count(),0);
    const emptyGroup=page.locator('.agent-session-group').filter({hasText:'空项目'});
    await emptyGroup.hover(); await emptyGroup.getByRole('button',{name:'项目操作 空项目'}).click();
    await page.getByRole('menuitem',{name:'重命名项目'}).click();
    await page.getByRole('dialog', {name:'重命名项目'}).getByLabel('项目名称').fill('空项目新名');
    await page.getByRole('button',{name:'保存',exact:true}).click();
    await page.locator('.agent-group-label').filter({hasText:'空项目新名'}).waitFor();
    assert.equal(renamed,'空项目新名'); assert.equal(renamedPath,'E:/empty');
    await group.hover(); await page.getByRole('button',{name:'在 测试项目 中新建会话'}).click();
    await page.waitForFunction(()=>document.querySelector('.agent-project-context')?.textContent.includes('E:/fixture'));
    assert.equal(await page.locator('.agent-project-trigger').innerText(),'测试项目');
    await group.hover(); await page.getByRole('button',{name:'项目操作 测试项目'}).click();
    await page.getByRole('menuitem',{name:'重命名项目'}).click();
    await page.getByRole('dialog', {name:'重命名项目'}).getByLabel('项目名称').fill('重命名项目');
    await page.getByRole('button',{name:'保存',exact:true}).click();
    await page.locator('.agent-group-label').filter({hasText:'重命名项目'}).waitFor();
    assert.equal(renamed,'重命名项目');
    assert.equal(await page.locator('.agent-project-trigger').innerText(),'重命名项目');
    const renamedGroup=page.locator('.agent-session-group').filter({hasText:'重命名项目'});
    await renamedGroup.hover(); await page.getByRole('button',{name:'删除项目 重命名项目'}).click();
    await page.getByRole('alertdialog', {name:'删除项目？'}).getByRole('button',{name:'删除项目',exact:true}).click();
    assert.equal(deleted,'E:/fixture');
    await page.getByRole('button',{name:'删除项目 重命名项目'}).waitFor({state:'detached'});
    await renamedGroup.hover();
    await page.getByRole('button',{name:'项目操作 重命名项目'}).waitFor();
    await page.reload(); await page.locator('.agent-group-label').filter({hasText:'重命名项目'}).waitFor();
    await page.emulateMedia({reducedMotion:'reduce'});
    await page.locator('.agent-group-label').filter({hasText:'重命名项目'}).click();
    assert.equal(await page.locator('.agent-session-row').count(),0);
  } finally {await browser?.close();server.kill();}
});
