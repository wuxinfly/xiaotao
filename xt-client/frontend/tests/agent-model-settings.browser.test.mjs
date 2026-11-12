import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('Agent settings add execution models and configure utility models', {timeout:60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5221', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({...(process.env.XIAOTAO_TEST_EXECUTABLE ? {executablePath:process.env.XIAOTAO_TEST_EXECUTABLE} : {channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge'}), headless:true});
    const page = await browser.newPage({viewport:{width:1280,height:900}, reducedMotion:'reduce'});
    const errors = [], created = [], sent = [], titles = [];
    page.on('pageerror', error => errors.push(error.message));
    const createdAt = new Date().toISOString();
    const session = {
      id:'settings-session', title:'参数测试', model:'qoder/qwen',
      modelSettings:{contextWindow:65536, maxOutputTokens:4096},
      status:'completed', project:{path:'E:/fixture',name:'测试项目'},
      messages:[], activity:[], thoughts:[], runStats:[], createdAt, updatedAt:createdAt,
    };
    await page.route('**/api/**', route => {
      const request = route.request(), path = new URL(request.url()).pathname;
      if (path === '/api/agent/sessions' && request.method() === 'POST') {
        created.push(request.postDataJSON());
        return route.fulfill({json:session});
      }
      if (path === '/api/agent/sessions') return route.fulfill({json:{sessions:[]}});
      if (path === '/api/agent/models') return route.fulfill({json:{models:[{model:'qoder/qwen',label:'Qwen',sourceId:'source1',sourceLabel:'My source',contextWindow:262144,maxOutputTokens:8192},{model:'qoder/helper',label:'Helper',sourceId:'source1',sourceLabel:'My source',contextWindow:32768,maxOutputTokens:2048}]}});
      if (path === '/api/agent/access-mode') return route.fulfill({json:{mode:'full'}});
      if (path.endsWith('/title')) {
        titles.push(request.postDataJSON());
        return route.fulfill({json:{title:'AI 标题'}});
      }
      if (path.endsWith('/messages')) {
        sent.push(request.postDataJSON());
        return route.fulfill({
          status:200,
          headers:{'Content-Type':'text/event-stream; charset=utf-8'},
          body:'event: error\ndata: {"type":"error","sessionId":"settings-session","error":"fixture stop"}\n\n',
        });
      }
      if (path.endsWith('/history')) return route.fulfill({json:{session,before:''}});
      return route.fulfill({json:{sessions:[],accounts:[],providers:[],models:[],mode:'full',key:'fixture'}});
    });

    await page.goto('http://127.0.0.1:5221');
    await page.getByRole('button', {name:'打开 Agent 设置'}).click();
    await page.getByRole('checkbox',{name:'Qwen · My source',exact:true}).click();
    await page.getByRole('checkbox',{name:'Helper · My source',exact:true}).click();
    await page.getByRole('button',{name:'调整 Qwen 参数'}).click();
    await page.getByRole('button',{name:'调整 Helper 参数'}).click();
    const contextInput = page.getByLabel('Qwen 上下文窗口');
    const outputInput = page.getByLabel('Qwen 最大输出');
    assert.equal(await contextInput.inputValue(), '262144');
    await contextInput.fill('65536');
    await outputInput.fill('4096');
    for (const viewport of [{width:1280,height:900},{width:700,height:760},{width:540,height:760}]) {
      await page.setViewportSize(viewport);
      const inputs = await page.locator('.model-library-parameters.is-open .agent-model-execution label').evaluateAll(elements => elements.map(element => element.getBoundingClientRect().toJSON()));
      assert.equal(inputs.length, 4);
      assert.ok(inputs[0].right <= viewport.width - 9, `context input overflows at ${viewport.width}px`);
      assert.ok(inputs[1].right <= viewport.width - 9, `output input overflows at ${viewport.width}px`);
    }
    await page.setViewportSize({width:1280,height:900});
    await page.screenshot({path:'../../work/ui/agent-model-settings-regression.png', fullPage:true});
    await page.getByRole('button', {name:/默认模型/}).click();
    for (const [label, name] of [['AI 重命名模型','Helper'],['行为复盘模型','Helper'],['上下文压缩模型','Qwen']]) {
      await page.getByRole('combobox',{name:label,exact:true}).click();
      await page.getByRole('listbox',{name:label,exact:true}).getByRole('option',{name:new RegExp(name)}).click();
    }
    await page.screenshot({path:'../../work/ui/agent-default-models-regression.png', fullPage:true});
    await page.getByRole('button', {name:'保存设置', exact:true}).click();
    const defaults = await page.evaluate(() => ({
      models: JSON.parse(localStorage.getItem('xiaotao.agent.defaultModels')),
      settings: JSON.parse(localStorage.getItem('xiaotao.agent.defaultModelSettings')),
    }));
    assert.deepEqual(defaults.models, {title:'qoder/helper', review:'qoder/helper', compaction:'qoder/qwen'});
    assert.deepEqual(defaults.settings, {});
    await page.locator('.agent-composer .agent-editor').click();
    await page.keyboard.type('测试参数');
    await page.locator('.agent-send').click();
    await page.waitForTimeout(100);
    assert.equal(created.length, 1);
    assert.deepEqual(created[0].modelSettings, {contextWindow:65536,maxOutputTokens:4096});
    assert.equal(sent.length, 1);
    assert.deepEqual(sent[0].modelSettings, {contextWindow:65536,maxOutputTokens:4096});
    assert.match(sent[0].model, /^qoder\/qwen$/);
    assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('xiaotao.agent.modelConfigs'))[0].maxOutputTokens), 4096);
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
