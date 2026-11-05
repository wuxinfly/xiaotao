import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

test('model pricing appears in account catalog, Agent settings and picker', {timeout:60000}, async () => {
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5198','--strictPort'],{stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',d=>{if(d.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1100,height:850},reducedMotion:'reduce'});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const models=[
   {id:'paid',displayName:'Paid',priceFactor:0.5,isFree:false},
   {id:'promo',displayName:'Promo',priceFactor:0.1,isFree:true},
   {id:'zero',displayName:'Zero',priceFactor:0},
   {id:'unknown',displayName:'Unknown'}
  ];
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname;
   if(path==='/api/agent/models')return route.fulfill({json:{models:models.map(m=>({...m,model:'qoder/'+m.id,label:m.displayName}))}});
   if(path==='/api/proxy/models')return route.fulfill({json:{models,source:'live'}});
   if(path==='/api/accounts')return route.fulfill({json:{accounts:[{id:'q',uid:'fixture',provider:'qoder',region:'cn',nickname:'Fixture',enabled:true,addedAt:1,usageTokens:0,usageCalls:0}]}});
   if(path==='/api/providers')return route.fulfill({json:{providers:[{id:'qoder',displayName:'Qoder',regions:['cn'],billing:'credit'}]}});
   return route.fulfill({json:{sessions:[],accounts:[],providers:[],models:[],mode:'full',key:'fixture'}});
  });
  await page.goto('http://127.0.0.1:5198');
  await page.getByRole('button',{name:'打开 Agent 设置'}).click();
  await page.locator('.agent-settings-group-toggle').filter({hasText:'Qoder'}).click();
  const choices=page.locator('.agent-model-choice');
  await choices.filter({hasText:'Paid'}).waitFor();
  assert.equal(await choices.filter({hasText:'Paid'}).locator('.model-price-badge').textContent(),'0.5×');
  assert.equal(await choices.filter({hasText:'Promo'}).locator('.model-price-badge').textContent(),'免费');
  assert.equal(await choices.filter({hasText:'Zero'}).locator('.model-price-badge').textContent(),'免费');
  assert.equal(await choices.filter({hasText:'Unknown'}).locator('.model-price-badge').count(),0);
  await page.locator('.agent-settings-group-head').getByRole('button',{name:'全选',exact:true}).click();
  await page.getByRole('button',{name:'保存设置'}).click();
  await page.getByRole('button',{name:'选择模型',exact:true}).click();
  await page.locator('.agent-model-menu .model-price-badge').first().waitFor();
  assert.equal(await page.locator('.agent-model-menu .model-price-badge').count(),3);
  await page.keyboard.press('Escape');
  await page.evaluate(()=>window.dispatchEvent(new CustomEvent('xiaotao:navigate',{detail:'accounts'})));
  await page.getByTitle('查看 Qoder 模型目录',{exact:true}).click();
  await page.locator('.model-catalog-row').first().waitFor();
  await page.locator('.model-catalog-row .model-price-badge').first().waitFor();
  assert.equal(await page.locator('.model-catalog-row .model-price-badge').count(),3);
  for(const width of [1100,540]){
   await page.setViewportSize({width,height:850});
   for(const badge of await page.locator('.model-catalog-row .model-price-badge').all()){
    const b=await badge.boundingBox();assert.ok(b.x>=0&&b.x+b.width<=width,'badge outside viewport');
   }
  }
  await page.screenshot({path:join(tmpdir(),'xiaotao-model-pricing.png')});
  await page.keyboard.press('Escape');
  await page.getByRole('button',{name:'切换到浅海亮色主题'}).click();
  await page.getByTitle('查看 Qoder 模型目录',{exact:true}).click();
  await page.locator('.model-catalog-row .model-price-badge').first().waitFor();
  assert.equal(await page.locator('.model-catalog-row .model-price-badge').count(),3);
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});
