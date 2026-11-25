import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {test} from 'node:test';
import {openOffice, workspaceToggle} from './office-test-navigation.mjs';

const path = 'E:\\work\\office-entry-test';
const appearance = {shape:'round',primaryColor:'#36a9c9',secondaryColor:'#173b55'};
function team(count=1,onboarding='completed') {return {members:Array.from({length:count},(_,i)=>({id:`role_${i}`,name:`成员${i}`,responsibility:'产品设计',capabilities:['讨论'],scope:'project',model:{modelId:'test/model',contextWindow:128000,maxOutputTokens:8192},appearance})),globalRoles:[],projectRoles:[],onboarding,globalRevision:0,projectRevision:0,teamRevision:0};}
function snapshot(members, tasks) {return {project:{path,name:'新项目'},accessMode:'safe',tasks,members,artifacts:[],capabilities:{midRunMessages:true,queuedMessages:true,approval:true,restartResume:false,sameProjectIsolation:true,commandArtifacts:false,artifactTools:['write_file'],notes:[]}};}

test('office desk wall, onboarding, theme and responsive layout', {timeout:90000}, async () => {
  const server=spawn(process.execPath,['node_modules/vite/bin/vite.js','--host','127.0.0.1','--port','5234','--strictPort'],{stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve=>server.stdout.on('data',data=>{if(data.toString().includes('ready in'))resolve();})),once(server,'exit').then(()=>{throw Error('Vite exited');})]);
    browser=await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER||'msedge',headless:true});
    const context=await browser.newContext({viewport:{width:1440,height:1000}});
    let current=team(0,'pending');
    const tasks = [];
    await context.route('**/api/**',route=>{
      const url=new URL(route.request().url()), pathname=url.pathname, method=route.request().method();
      if(pathname==='/api/agent/projects')return route.fulfill({json:{projects:[{path,name:'新项目',saved:true}]}});
      if(pathname==='/api/agent/project')return route.fulfill({json:{path,name:'新项目',saved:true}});
      if(pathname==='/api/agent/models')return route.fulfill({json:{models:[{model:'test/model',label:'测试模型',contextWindow:128000,maxOutputTokens:8192}]}});
      if(pathname==='/api/office/team'&&method==='GET')return route.fulfill({json:current});
      if(pathname==='/api/office/team/templates')return route.fulfill({json:[{id:'product',name:'产品实现小队',description:'清晰分工，自由讨论',roles:[{name:'设计师',responsibility:'需求与界面',capabilities:['设计'],model:{modelId:'',contextWindow:0,maxOutputTokens:0},appearance}]}]});
      if(pathname==='/api/office/team/bootstrap')return route.fulfill({json:current=team()});
      if(pathname==='/api/office/team/skip-onboarding')return route.fulfill({json:current=team(0,'skipped')});
      if(pathname==='/api/office/snapshot')return route.fulfill({json:snapshot([{id:'coordinator',name:'小涛',role:'coordinator',status:'idle'},...current.members.map(member=>({...member,status:member.status || 'idle',role:'project'}))], tasks)});
      return route.fulfill({json:{sessions:[],projects:[],accounts:[],models:[]}});
    });
    const page=await context.newPage();
    await page.goto('http://127.0.0.1:5234');
    const bounds=await workspaceToggle(page).boundingBox();
    const header=await page.locator('.agent-context-bar').boundingBox();
    assert.ok(bounds.x > header.x + header.width / 2 && bounds.x + bounds.width <= header.x + header.width);
    const pane=await openOffice(page, '新项目');
    const wall = pane.getByRole('region', {name:'办公室工位'});
    await wall.getByRole('button', {name:/小涛/}).waitFor();
    const content = await pane.locator('.office-live-page').boundingBox();
    const home = await page.getByRole('main', {name: 'agent'}).boundingBox();
    assert.ok(content.width < home.width && content.x > home.x, 'the office opens on the right of the home workspace');
    await page.getByRole('textbox', {name:'给小涛发消息'}).waitFor();
    await pane.getByRole('button', {name:'成员', exact:true}).click();
    await pane.getByRole('region', {name:'团队成员列表'}).waitFor();
    await page.keyboard.press('Escape');

    await wall.getByRole('button', {name:'组建小队'}).click();
    await page.getByRole('heading', {name:'为你的项目，组建一支小队。'}).waitFor();
    await page.setViewportSize({width:800,height:500});
    const scrollArea = page.locator('.office-onboarding-modal .app-modal-body');
    await scrollArea.evaluate(element => {element.scrollTop = element.scrollHeight;});
    assert.ok(await scrollArea.evaluate(element => element.scrollTop > 0));
    await page.getByRole('button', {name:'稍后设置'}).click();
    await wall.getByRole('button', {name:'组建小队'}).waitFor({state:'detached'});
    assert.equal(current.onboarding, 'skipped');

    current = team(0,'pending');
    await page.reload();
    await page.setViewportSize({width:1500,height:920});
    await openOffice(page, '新项目');
    await wall.getByRole('button', {name:'组建小队'}).click();
    await page.getByRole('button', {name:'创建我的小队'}).click();
    await wall.getByRole('button', {name:/成员0/}).waitFor();
    assert.equal(current.onboarding, 'completed');
    assert.equal(await pane.locator('canvas').count(), 0);

    tasks.push({id:'review-task', title:'检查构建输出', status:'waiting', executor:{name:'小满', modelLabel:'测试模型'}, approval:{id:'approval-1', detail:'执行构建检查'}});
    const combined = team(3);
    current = {...combined, members:combined.members.map((member, index) => ({
      ...member,
      name: ['阿岚','小满','阿青'][index],
      responsibility: ['前端开发','测试验证','交互设计'][index],
      status: ['running','waiting','idle'][index],
      taskId: ['build-task','review-task',undefined][index],
      taskTitle: ['修复历史分页','检查构建输出',undefined][index],
      appearance: {...appearance, shape:['hexagon','square','triangle'][index], primaryColor:['#3aaa9a','#6b8bb3','#986d9a'][index]},
    }))};
    await page.reload();
    await openOffice(page, '新项目');
    await wall.getByRole('button', {name:/阿岚/}).waitFor();
    await wall.getByRole('region', {name:'待你处理'}).getByRole('button', {name:'检查构建输出 · 等你确认'}).waitFor();
    const grid = wall.locator('.office-desk-grid');
    const columns = () => grid.evaluate(element => getComputedStyle(element).gridTemplateColumns.split(' ').length);
    assert.equal(await columns(), 2);
    const scene = pane.locator('.workspace-scene');
    const background = () => scene.evaluate(element => getComputedStyle(element).backgroundColor);
    const darkBackground = await background();
    const preview = await pane.boundingBox();
    await page.screenshot({path:join(tmpdir(), 'xiaotao-office-2d.png'), animations:'disabled', clip:{...preview,height:Math.min(510, preview.height)}});
    await page.getByRole('button',{name:'更多',exact:true}).click();
    await page.getByRole('menuitem',{name:'切换到浅海主题'}).click();
    await page.locator('html[data-theme=light]').waitFor();
    assert.notEqual(await background(), darkBackground);
    await page.setViewportSize({width:390,height:780});
    assert.equal(await columns(), 1);
    assert.ok(await scene.evaluate(element => element.scrollWidth <= element.clientWidth));
    await pane.screenshot({path:join(tmpdir(), 'xiaotao-office-2d-light.png'), animations:'disabled'});
  } finally {await browser?.close();server.kill();}
});
