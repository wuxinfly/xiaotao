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
    let opened='', renamed='';
    await page.route('**/api/**',route=>{
      const path=new URL(route.request().url()).pathname;
      if(path==='/api/agent/sessions') return route.fulfill({json:{sessions:[session]}});
      if(path==='/api/agent/project') return route.fulfill({json:session.project});
      if(path==='/api/agent/project/name') {renamed=route.request().postDataJSON().name;session.project.name=renamed;return route.fulfill({json:{ok:true}});}
      if(path==='/api/agent/project/open') {opened=route.request().postDataJSON().path;return route.fulfill({json:{ok:true}});}
      return route.fulfill({json:{accounts:[],providers:[],models:[]}});
    });
    await page.goto('http://127.0.0.1:5196');
    const group=page.locator('.agent-session-group'); await group.waitFor();
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
    assert.equal(await page.evaluate(()=>document.activeElement?.getAttribute('aria-label')),'项目操作 测试项目');
    await page.keyboard.press('Enter');
    await page.getByRole('menuitem',{name:'在资源管理器中打开'}).click();
    assert.equal(opened,'E:/fixture');
    await group.hover(); await page.getByRole('button',{name:'在 测试项目 中新建会话'}).click();
    await page.waitForFunction(()=>document.querySelector('.agent-project-context')?.textContent.includes('E:/fixture'));
    assert.equal(await page.locator('.agent-project-trigger').innerText(),'测试项目');
    await group.hover(); await page.getByRole('button',{name:'项目操作 测试项目'}).click();
    await page.getByRole('menuitem',{name:'重命名项目'}).click();
    await page.getByRole('dialog').locator('input').fill('重命名项目');
    await page.getByRole('button',{name:'保存',exact:true}).click();
    await page.locator('.agent-group-label').filter({hasText:'重命名项目'}).waitFor();
    assert.equal(renamed,'重命名项目');
    assert.equal(await page.locator('.agent-project-trigger').innerText(),'重命名项目');
    await page.reload(); await page.locator('.agent-group-label').filter({hasText:'重命名项目'}).waitFor();
    await page.emulateMedia({reducedMotion:'reduce'});
    await page.locator('.agent-group-label').filter({hasText:'重命名项目'}).click();
    assert.equal(await page.locator('.agent-session-row').count(),0);
  } finally {await browser?.close();server.kill();}
});
