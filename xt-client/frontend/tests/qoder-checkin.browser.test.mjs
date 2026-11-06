import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('Qoder checkin: regional cards, unavailable activity, replay and scoped batch', {timeout:60000}, async () => {
 const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5199','--strictPort'],{stdio:['ignore','pipe','pipe']});
 let browser;
 try {
  await Promise.race([new Promise(resolve=>server.stdout.on('data',d=>{if(d.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
  browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
  const page=await browser.newPage({viewport:{width:1100,height:850},reducedMotion:'reduce'});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  let claimed=false;const posts=[];const statusReads=[];
  const accounts=[{id:'cn',region:'cn',nickname:'CN fixture'},{id:'global',region:'global',nickname:'Global fixture'}].map(a=>({...a,uid:a.id,provider:'qoder',enabled:true,authState:'valid',addedAt:1}));
  await page.route('**/api/**',async route=>{
   const req=route.request();const url=new URL(req.url());const path=url.pathname;
   if(req.method()==='POST')posts.push(path+url.search);
   if(path==='/api/accounts')return route.fulfill({json:{accounts}});
   if(path==='/api/providers')return route.fulfill({json:{providers:[{id:'qoder',displayName:'Qoder',regions:['cn','global'],billing:'credit'}]}});
   if(path.endsWith('/checkin')){
    const id=path.split('/')[3];
    if(req.method()==='POST') {const replayed=claimed;claimed=true;return route.fulfill({json:{ok:!replayed,checkedIn:true,extraCredits:replayed?0:37,message:'当前活动奖励已领取'}});}
    statusReads.push(id);
    return route.fulfill({json:{supported:true,active:id==='global',checkedIn:id==='global'&&claimed,message:id==='cn'?'暂无可领取活动':claimed?'当前活动奖励已领取':'有活动奖励可领取'}});
   }
   if(path==='/api/checkin/claim-all')return route.fulfill({json:{results:{cn:{ok:false,checkedIn:false,error:'fixture failure'},global:{ok:false,checkedIn:true,message:'当前活动奖励已领取'}}}});
   return route.fulfill({json:{sessions:[],models:[],accounts:[],providers:[],mode:'full',key:'fixture'}});
  });
  await page.goto('http://127.0.0.1:5199');
  await page.evaluate(()=>window.dispatchEvent(new CustomEvent('xiaotao:navigate',{detail:'accounts'})));
  const group=page.locator('.provider-group-section').filter({hasText:'Qoder'});
  await group.getByTitle('展开分组',{exact:true}).click();
  const cn=page.locator('.modern-account-card').filter({hasText:'CN fixture'});
  const global=page.locator('.modern-account-card').filter({hasText:'Global fixture'});
  await cn.getByTitle('暂无可领取活动',{exact:true}).waitFor();
  assert.equal(await cn.locator('.badge-checkin-todo').count(),0);
  await global.getByRole('button',{name:'每日签到打卡',exact:true}).click();
  await global.locator('.badge-checkin-done').waitFor();
  assert.ok(statusReads.includes('cn')&&statusReads.includes('global'));
  assert.ok(posts.includes('/api/accounts/global/checkin'));
  await global.getByRole('button',{name:'每日签到打卡',exact:true}).click();
  await page.getByText('当前活动奖励已领取',{exact:true}).first().waitFor();
  await Promise.all([page.waitForResponse(r=>r.url().includes('/api/checkin/claim-all')),group.getByRole('button',{name:'一键打卡',exact:true}).click()]);
  assert.ok(posts.includes('/api/checkin/claim-all?provider=qoder'));
  await page.getByText('1 个账号签到失败，请单独打卡查看原因',{exact:true}).waitFor();
  for(const width of [540,1100]){await page.setViewportSize({width,height:850});await global.getByRole('button',{name:'每日签到打卡',exact:true}).waitFor();}
  assert.deepEqual(errors,[]);
 } finally {await browser?.close();server.kill();}
});
