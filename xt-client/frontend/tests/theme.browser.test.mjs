import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('theme persists, covers portals and synchronizes the HUD', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5191', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if(data.toString().includes('ready in')) resolve();})), once(server,'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const context = await browser.newContext({viewport:{width:1280,height:850}, reducedMotion:'reduce'});
    const errors=[];
    context.on('page', page => page.on('pageerror', error => errors.push(error.message)));
    // Synthetic data only; all APIs are intercepted, including SSE.
    const tokens={totalTokens:3000,promptTokens:2000,completionTokens:1000,cachedTokens:500};
    await context.route('**/api/**', route => {
      const path=new URL(route.request().url()).pathname;
      if(path === '/api/dashboard/events') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      if(path === '/api/dashboard') return route.fulfill({json:{version:'fixture',versionSeq:1,launch:{startedAt:1793678400,uptimeSeconds:60,batchId:'fixture'},currentSession:tokens,today:{...tokens,totalRequests:2},cumulative:{...tokens,totalRequests:2},recentRequests:[],activeRequests:[],activeCount:0,heatmap:{startDate:'2026-01-01',endDate:'2026-11-03',trackingStartDate:'2026-01-01',days:{}},storage:{status:'healthy'}}});
      return route.fulfill({json:{sessions:[],accounts:[],providers:[],models:[],key:'fixture-key'}});
    });
    const page=await context.newPage();
    const screenshot = name => page.screenshot({path:join(tmpdir(), `xiaotao-light-${name}.png`)});
    await page.goto('http://127.0.0.1:5191');
    await page.getByRole('button',{name:'切换到浅海亮色主题'}).focus();
    await page.keyboard.press('Enter');
    assert.equal(await page.locator('html').getAttribute('data-theme'),'light');
    assert.equal(await page.locator('.agent-main-panel').evaluate(el=>getComputedStyle(el).backgroundImage),'none');
    await page.reload();
    await page.getByRole('button',{name:'切换到深海暗色主题'}).waitFor();
    await screenshot('agent');
    await page.getByRole('button',{name:'打开 Agent 设置'}).click();
    assert.equal(await page.locator('.app-modal').evaluate(el=>getComputedStyle(el).backgroundColor),'rgb(251, 254, 255)');
    await page.keyboard.press('Escape');
    await page.evaluate(()=>window.dispatchEvent(new CustomEvent('xiaotao:navigate',{detail:'dashboard'})));
    await page.locator('.hero-tokens').waitFor();
    assert.equal(await page.locator('.hero-tokens').evaluate(el=>getComputedStyle(el).color),'rgb(32, 63, 76)');
    await screenshot('dashboard');
    await page.evaluate(()=>window.dispatchEvent(new CustomEvent('xiaotao:navigate',{detail:'accounts'})));
    await page.getByRole('button',{name:'添加账号',exact:true}).first().click();
    assert.equal(await page.locator('.dialog-title').evaluate(el=>getComputedStyle(el).color),'rgb(32, 63, 76)');
    await screenshot('dialog');
    await page.keyboard.press('Escape');
    const hud=await context.newPage();
    await hud.goto('http://127.0.0.1:5191/hud.html');
    assert.equal(await hud.locator('html').getAttribute('data-theme'),'light');
    await page.getByRole('button',{name:'切换到深海暗色主题'}).click();
    await hud.waitForFunction(()=>document.documentElement.dataset.theme==='dark');
    assert.equal(await page.locator('html').getAttribute('data-theme'),'dark');
    await page.getByRole('button',{name:'切换到浅海亮色主题'}).click();
    for(const width of [540,850]) {
      await page.setViewportSize({width,height:800});
      const toggle=await page.locator('.theme-toggle').boundingBox();
      const nav=await page.locator('.titlebar nav').boundingBox();
      assert.ok(toggle.x>=nav.x+nav.width,'theme toggle must not overlap navigation');
    }
    const blocked=await browser.newContext();
    await blocked.addInitScript(()=>{Storage.prototype.getItem=()=>{throw new Error('blocked storage')};Storage.prototype.setItem=()=>{throw new Error('blocked storage')};});
    await blocked.route('**/api/**',r=>r.fulfill({json:{sessions:[],models:[]}}));
    const fallback=await blocked.newPage();
    await fallback.goto('http://127.0.0.1:5191/hud.html');
    await fallback.evaluate(async()=>{const {saveTheme}=await import('/src/lib/theme.ts'); saveTheme('light');});
    assert.equal(await fallback.locator('html').getAttribute('data-theme'),'light');
    assert.deepEqual(errors,[]);
  } finally {
    await browser?.close();
    server.kill();
  }
});
