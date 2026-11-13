import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {mkdtemp, readFile} from 'node:fs/promises';
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
    let current=team(0,'pending'), modelRequests=0, failModel=true;
    await context.route('**/api/**',route=>{
      const url=new URL(route.request().url()), pathname=url.pathname, method=route.request().method();
      if(pathname==='/api/agent/projects')return route.fulfill({json:{projects:[{path,name:'新项目',saved:true}]}});
      if(pathname==='/api/agent/models')return route.fulfill({json:{models:[{model:'test/model',label:'测试模型',contextWindow:128000,maxOutputTokens:8192}]}});
      if(pathname==='/api/office/team'&&method==='GET')return route.fulfill({json:current});
      if(pathname==='/api/office/team/templates')return route.fulfill({json:[{id:'product',name:'产品实现小队',description:'清晰分工，自由讨论',roles:[{name:'设计师',responsibility:'需求与界面',capabilities:['设计'],model:{modelId:'',contextWindow:0,maxOutputTokens:0},appearance}]}]});
      if(pathname==='/api/office/team/bootstrap')return route.fulfill({json:current=team()});
      if(pathname==='/api/office/team/skip-onboarding')return route.fulfill({json:current=team(0,'skipped')});
      if(pathname==='/api/office/snapshot')return route.fulfill({json:snapshot(current.members)});
      return route.fulfill({json:{sessions:[],projects:[],accounts:[],models:[]}});
    });
    await context.route('**/models/office.glb',route=>{modelRequests++; return failModel?route.abort('failed'):route.continue();});
    const page=await context.newPage();
    await page.goto('http://127.0.0.1:5234');
    await page.locator('.workspace-dock').getByRole('button',{name:'办公室',exact:true}).click();
    await page.getByRole('heading',{name:'为你的项目，组建一支小队。'}).waitFor();
    await page.getByRole('button',{name:'稍后设置'}).click();
    await page.getByRole('heading',{name:'项目团队'}).waitFor();
    assert.equal(await page.locator('canvas').count(),0);
    await page.reload();
    await page.locator('.workspace-dock').getByRole('button',{name:'办公室',exact:true}).click();
    await page.getByRole('heading',{name:'项目团队'}).waitFor();
    await page.getByRole('button',{name:'管理团队',exact:true}).click();
    await page.keyboard.press('Escape');
    current=team(0,'pending');
    await page.reload();
    await page.locator('.workspace-dock').getByRole('button',{name:'办公室',exact:true}).click();
    await page.getByRole('button',{name:'创建我的小队'}).click();
    await page.getByText('办公室场景暂时未能准备完成').waitFor();
    assert.equal(current.onboarding,'completed');
    await page.getByRole('heading',{name:'项目团队'}).waitFor();
    failModel=false;
    await page.getByRole('button',{name:'重新加载'}).click();
    await page.locator('canvas[data-office-renderer="three"]').waitFor();
    await page.locator('[data-entry-phase=ready]').waitFor();
    assert.ok(modelRequests>=2);
    await page.screenshot({path:join(tmpdir(),'xiaotao-office-entry.png')});
  } finally {await browser?.close();server.kill();}
});

test('published local GLB has self-contained nodes and valid local indices',async()=>{
  const buffer=await readFile(new URL('../public/models/office.glb',import.meta.url));
  assert.equal(buffer.readUInt32LE(0),0x46546c67);
  const length=buffer.readUInt32LE(12);
  const gltf=JSON.parse(buffer.subarray(20,20+length).toString());
  assert.ok(gltf.nodes.some(node=>node.name==='OfficeRoom'));
  assert.ok(gltf.nodes.some(node=>node.name==='QCharacter'));
  for(const accessor of gltf.accessors.filter(item=>item.type==='SCALAR')) {
    const view=gltf.bufferViews[accessor.bufferView], start=20+length+8+(view.byteOffset||0)+(accessor.byteOffset||0);
    const positionAccessor=gltf.accessors[gltf.meshes[0].primitives[0].attributes.POSITION];
    for(let i=0;i<accessor.count;i++) assert.ok(buffer.readUInt32LE(start+i*4)<positionAccessor.count);
  }
  assert.match(gltf.buffers[0].uri,/^data:application\/octet-stream;base64,/);
});
