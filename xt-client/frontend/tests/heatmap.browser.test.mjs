import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('edge heatmap cells and provider headers keep their full visual bounds', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5194', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if(data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1440,height:900}, reducedMotion:'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const tokens = {totalTokens:3000,promptTokens:2000,completionTokens:1000,cachedTokens:500};
    const account = {id:'trae:fixture',provider:'trae',region:'cn',name:'测试账号',enabled:true};
    // Synthetic data only; all APIs are intercepted, including SSE.
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/dashboard/events') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      if (path === '/api/dashboard') return route.fulfill({json:{version:'fixture',versionSeq:1,launch:{startedAt:1793678400,uptimeSeconds:60,batchId:'fixture'},currentSession:tokens,today:{...tokens,totalRequests:2},cumulative:{...tokens,totalRequests:2},recentRequests:[],activeRequests:[],activeCount:0,heatmap:{startDate:'2025-11-04',endDate:'2026-11-04',trackingStartDate:'2025-11-04',days:{'2026-11-04':{totalTokens:3000,promptTokens:2000,completionTokens:1000,cachedTokens:500,totalRequests:2,hasMissingUsage:false}}},storage:{status:'healthy'}}});
      if (path === '/api/accounts') return route.fulfill({json:{accounts:[account],providers:[{id:'trae',displayName:'Trae 国内版'}]}});
      return route.fulfill({json:{sessions:[],accounts:[account],providers:[{id:'trae',displayName:'Trae 国内版'}],models:[],key:'fixture-key'}});
    });
    await page.goto('http://127.0.0.1:5194');
    await page.evaluate(()=>window.dispatchEvent(new CustomEvent('xiaotao:navigate',{detail:'dashboard'})));
    const today = page.locator('.day-cell.is-today');
    await today.waitFor();

    const assertInsideMatrix = async label => {
      const bounds = await page.evaluate(() => {
        const matrix = document.querySelector('.calendar-matrix').getBoundingClientRect();
        const cell = document.querySelector('.day-cell.is-today').getBoundingClientRect();
        return {matrix:{left:matrix.left,right:matrix.right,top:matrix.top,bottom:matrix.bottom},cell:{left:cell.left,right:cell.right,top:cell.top,bottom:cell.bottom}};
      });
      assert.ok(bounds.cell.left >= bounds.matrix.left, `${label}: left edge is clipped`);
      assert.ok(bounds.cell.right <= bounds.matrix.right, `${label}: right edge is clipped`);
      assert.ok(bounds.cell.top >= bounds.matrix.top, `${label}: top edge is clipped`);
      assert.ok(bounds.cell.bottom <= bounds.matrix.bottom, `${label}: bottom edge is clipped`);
    };

    await today.hover();
    await assertInsideMatrix('hover');
    await page.mouse.move(0, 0);
    await today.focus();
    await assertInsideMatrix('focus');

    await page.evaluate(()=>window.dispatchEvent(new CustomEvent('xiaotao:navigate',{detail:'accounts'})));
    const group = page.locator('.provider-group-section').first();
    await group.waitFor();
    if (await group.evaluate(el => el.classList.contains('is-collapsed'))) {
      await group.locator('.group-collapse-btn').click();
    }
    await group.hover();
    assert.equal(await group.locator('.group-left').evaluate(el => getComputedStyle(el).backgroundColor), 'rgba(0, 0, 0, 0)');
    assert.ok(await group.locator('.group-right').evaluate(el => parseFloat(getComputedStyle(el).borderLeftWidth) > 0));
    const actionHeights = await group.locator('.group-right button').evaluateAll(buttons => buttons.map(button => Math.round(button.getBoundingClientRect().height)));
    assert.equal(new Set(actionHeights).size, 1, 'header actions should share one height');
    const headerBounds = await page.evaluate(() => {
      const section = document.querySelector('.provider-group-section').getBoundingClientRect();
      const header = document.querySelector('.provider-group-header').getBoundingClientRect();
      return {sectionLeft:section.left,sectionRight:section.right,headerLeft:header.left,headerRight:header.right};
    });
    assert.ok(headerBounds.headerLeft > headerBounds.sectionLeft);
    assert.ok(headerBounds.headerRight < headerBounds.sectionRight);
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
