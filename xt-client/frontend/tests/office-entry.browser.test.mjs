import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {readFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {test} from 'node:test';

const path = 'E:\\work\\office-entry-test';
const appearance = {shape:'round',primaryColor:'#36a9c9',secondaryColor:'#173b55'};
function team(count=1,onboarding='completed') {return {members:Array.from({length:count},(_,i)=>({id:`role_${i}`,name:`成员${i}`,responsibility:'产品设计',capabilities:['讨论'],scope:'project',model:{modelId:'test/model',contextWindow:128000,maxOutputTokens:8192},appearance})),globalRoles:[],projectRoles:[],onboarding,globalRevision:0,projectRevision:0,teamRevision:0};}
function snapshot(members) {return {project:{path,name:'新项目'},accessMode:'safe',tasks:[],members,artifacts:[],capabilities:{midRunMessages:true,queuedMessages:true,approval:true,restartResume:false,sameProjectIsolation:true,commandArtifacts:false,artifactTools:['write_file'],notes:[]}};}

test('onboarding, saved-team isolation and scene recovery', {timeout:90000}, async () => {
  const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5234','--strictPort'],{stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
    browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true,args:['--enable-unsafe-swiftshader']});
    const context=await browser.newContext({viewport:{width:1440,height:1000}});
    let current=team(0,'pending'), modelRequests=0, failModel=false;
    await context.route('**/api/**',route=>{
      const url=new URL(route.request().url()), pathname=url.pathname, method=route.request().method();
      if(pathname==='/api/agent/projects')return route.fulfill({json:{projects:[{path,name:'新项目',saved:true}]}});
      if(pathname==='/api/agent/models')return route.fulfill({json:{models:[{model:'test/model',label:'测试模型',contextWindow:128000,maxOutputTokens:8192}]}});
      if(pathname==='/api/office/team'&&method==='GET')return route.fulfill({json:current});
      if(pathname==='/api/office/team/templates')return route.fulfill({json:[{id:'product',name:'产品实现小队',description:'清晰分工，自由讨论',roles:[{name:'设计师',responsibility:'需求与界面',capabilities:['设计'],model:{modelId:'',contextWindow:0,maxOutputTokens:0},appearance}]}]});
      if(pathname==='/api/office/team/bootstrap')return route.fulfill({json:current=team()});
      if(pathname==='/api/office/team/skip-onboarding')return route.fulfill({json:current=team(0,'skipped')});
      if(pathname==='/api/office/snapshot')return route.fulfill({json:snapshot([{id:'coordinator',name:'小涛',role:'coordinator',status:'idle'},...current.members.map(member=>({...member,status:'idle',role:'project'}))])});
      return route.fulfill({json:{sessions:[],projects:[],accounts:[],models:[]}});
    });
    await context.route('**/models/office.glb',route=>{modelRequests++; return failModel?route.abort('failed'):route.continue();});
    const page=await context.newPage();
    await page.goto('http://127.0.0.1:5234');
    const dock=page.locator('.agent-sidebar .workspace-dock');
    const office=dock.getByRole('button',{name:'办公室',exact:true});
    const bounds=await office.boundingBox();
    const sidebar=await page.locator('.agent-sidebar').boundingBox();
    assert.ok(bounds.x>=sidebar.x && bounds.x+bounds.width<=sidebar.x+sidebar.width);
    await office.click();
    await page.locator('[data-entry-phase=ready]').waitFor();
    await page.locator('.workspace-page-view').evaluate(async element => {
      await Promise.all(element.getAnimations().map(animation => animation.finished));
    });
    const surface = page.locator('.office-3d-canvas');
    const roomBounds = await surface.boundingBox();
    const content = await page.getByRole('main', {name: 'office'}).boundingBox();
    assert.deepEqual(roomBounds, content, 'the 3D surface fills the workspace below the title bar');
    await page.getByRole('button', {name: '任务', exact: true}).click();
    await page.getByRole('tab', {name: /团队/}).waitFor();
    assert.deepEqual(await surface.boundingBox(), roomBounds, 'opening the drawer must keep the 3D viewport in place');
    await page.keyboard.press('Escape');
    await page.getByRole('button',{name:'组建小队',exact:true}).click();
    await page.getByRole('heading',{name:'为你的项目，组建一支小队。'}).waitFor();
    await page.setViewportSize({width: 800, height: 500});
    const scrollArea = page.locator('.office-onboarding-modal .app-modal-body');
    await scrollArea.evaluate(element => {element.scrollTop = element.scrollHeight;});
    assert.ok(await scrollArea.evaluate(element => element.scrollTop > 0));
    await page.getByRole('button',{name:'稍后设置'}).click();
    await page.setViewportSize({width: 1440, height: 1000});
    await page.locator('[data-entry-phase=ready]').waitFor();
    await page.getByRole('button',{name:'组建小队',exact:true}).waitFor({state:'detached'});
    assert.equal(await page.locator('canvas[data-office-renderer="three"]').count(),1);
    await page.reload();
    await page.locator('.workspace-dock').getByRole('button',{name:'办公室',exact:true}).click();
    await page.locator('[data-entry-phase=ready]').waitFor();
    if (!await page.getByRole('tab', {name: /团队/}).isVisible()) await page.getByRole('button', {name: '任务', exact: true}).click();
    await page.getByRole('tab', {name: /团队/}).click();
    await page.locator('.workspace-team').getByRole('button',{name:'管理团队',exact:true}).first().click();
    await page.keyboard.press('Escape');
    current=team(0,'pending');
    failModel=true;
    await page.reload();
    await page.locator('.workspace-dock').getByRole('button',{name:'办公室',exact:true}).click();
    await page.getByText('办公室场景暂时未能准备完成').waitFor();
    await page.getByRole('button',{name:'组建小队',exact:true}).click();
    await page.getByRole('button',{name:'创建我的小队'}).click();
    assert.equal(current.onboarding,'completed');
    await page.getByRole('button', {name: '任务', exact: true}).click();
    await page.getByRole('tab', {name: /团队/}).click();
    await page.getByRole('region', {name: '团队成员列表'}).getByRole('heading',{name:'项目团队'}).waitFor();
    failModel=false;
    await page.getByRole('button', {name: '收起协作面板'}).click();
    await page.getByRole('button',{name:'重新加载'}).click();
    await page.locator('canvas[data-office-renderer="three"]').waitFor();
    await page.locator('[data-entry-phase=ready]').waitFor();
    assert.ok(modelRequests>=2);
    const priorRequests=modelRequests;
    assert.equal(await page.getByRole('heading', {name: '办公室', exact: true}).count(), 0);
    assert.equal(await page.getByText('夜晚', {exact: true}).count(), 0);
    const stage = page.locator('.office-3d-stage');
    const darkBackground = await stage.evaluate(element => getComputedStyle(element).backgroundImage);
    await page.locator('.workspace-sidebar').waitFor({state:'hidden'});
    await page.screenshot({path:join(tmpdir(),'xiaotao-office-entry.png')});
    await page.getByRole('button',{name:'更多',exact:true}).click();
    await page.getByRole('menuitem',{name:'切换到浅海主题'}).click();
    await page.locator('html[data-theme=light]').waitFor();
    assert.equal(await page.getByText('白天', {exact: true}).count(), 0);
    assert.notEqual(await stage.evaluate(element => getComputedStyle(element).backgroundImage), darkBackground);
    await page.screenshot({path:join(tmpdir(),'xiaotao-office-day.png')});
    assert.equal(modelRequests,priorRequests);
  } finally {await browser?.close();server.kill();}
});

test('published local GLB has self-contained nodes and valid local indices',async()=>{
  const buffer=await readFile(new URL('../public/models/office.glb',import.meta.url));
  assert.equal(buffer.readUInt32LE(0),0x46546c67);
  const length=buffer.readUInt32LE(12);
  const gltf=JSON.parse(buffer.subarray(20,20+length).toString());
  assert.ok(gltf.nodes.some(node=>node.name==='OfficeRoom'));
  assert.ok(gltf.nodes.some(node=>node.name==='QCharacter'));
  for(const name of ['OfficeArchive','OfficeDesk','CoordinatorDesk','OfficeChair','OfficeFloorExtension','LeftThigh','RightShin']) {
    assert.ok(gltf.nodes.some(node=>node.name===name), name);
  }
  for(const shape of ['square','triangle','hexagon']) assert.ok(gltf.nodes.some(node=>node.name===`QCharacter-${shape}`));
  for(const mesh of gltf.meshes) {
    for(const primitive of mesh.primitives) {
      const accessor=gltf.accessors[primitive.indices], view=gltf.bufferViews[accessor.bufferView];
      const start=20+length+8+(view.byteOffset||0);
      const positions=gltf.accessors[primitive.attributes.POSITION].count;
      for(let i=0;i<accessor.count;i++) assert.ok(buffer.readUInt32LE(start+i*4)<positions);
    }
  }
  assert.equal(gltf.buffers[0].uri,undefined);
  assert.equal(buffer.length, buffer.readUInt32LE(8));
});
