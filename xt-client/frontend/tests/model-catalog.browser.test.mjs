import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('model views read shared data; buttons refresh all sources or the current source', {timeout:60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5199', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => { if (data.toString().includes('ready in')) resolve(); })),
      once(server, 'exit').then(() => { throw Error('Vite exited'); }),
    ]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({reducedMotion:'reduce'});
    const errors = []; page.on('pageerror', error => errors.push(error.message));
    const accounts = [
      {id:'q1', provider:'qoder', region:'cn', nickname:'Q1'},
      {id:'q2', provider:'qoder', region:'global', nickname:'Q2'},
      {id:'w1', provider:'workbuddy', region:'cn', nickname:'W1'},
    ].map(account => ({...account, uid:account.id, enabled:true, addedAt:1, usageTokens:0, usageCalls:0}));
    const versions = {q1:1, q2:1, w1:1};
    const requests = [];
    let failRefresh = false;
    const catalog = id => [{
      id: 'shared',
      displayName: `${id.toUpperCase()} v${versions[id]}`,
      priceFactor: versions[id],
      maxInputTokens: id === 'q1' ? 131072 : 0,
    }];
    const aggregated = () => [accounts[0], accounts[2]].map(account => ({model:`${account.provider}/shared`, label:catalog(account.id)[0].displayName, priceFactor:versions[account.id]}));
    await page.addInitScript(() => localStorage.setItem('xiaotao.agent.modelCatalog', JSON.stringify([{model:'obsolete/model', label:'Obsolete directory'}])));
    await page.route('**/api/**', route => {
      const request = route.request(), url = new URL(request.url()), path = url.pathname;
      if (path.startsWith('/api/agent/models') || path === '/api/proxy/models') requests.push({path, method:request.method(), id:url.searchParams.get('accountId')});
      if (path === '/api/agent/models/refresh') {
        for (const id of Object.keys(versions)) versions[id]++;
        return route.fulfill({json:{models:aggregated(), errors:[]}});
      }
      if (path === '/api/agent/models') return route.fulfill({json:{models:aggregated()}});
      if (path === '/api/proxy/models') {
        const id = url.searchParams.get('accountId');
        if (request.method() === 'POST' && !failRefresh) versions[id]++;
        return route.fulfill({json:{models:catalog(id), source:failRefresh ? 'stale' : 'cached', error:failRefresh ? 'offline' : ''}});
      }
      if (path === '/api/accounts') return route.fulfill({json:{accounts}});
      if (path === '/api/providers') return route.fulfill({json:{providers:[{id:'qoder',displayName:'Qoder',regions:['cn','global'],billing:'credit'}, {id:'workbuddy',displayName:'WorkBuddy',regions:['cn'],billing:'credit'}]}});
      return route.fulfill({json:{sessions:[], accounts:[], providers:[], models:[], mode:'full', key:'fixture'}});
    });
    await page.goto('http://127.0.0.1:5199');
    await page.getByRole('button', {name:'打开 Agent 设置'}).click();
    await page.getByRole('button', {name:'刷新模型列表', exact:true}).waitFor();
    assert.equal(requests.filter(request => request.method === 'POST').length, 0);
    assert.equal(await page.getByText('Obsolete directory').count(), 0);
    await page.getByRole('button', {name:'刷新模型列表', exact:true}).click();
    await page.getByRole('button', {name:'刷新模型列表', exact:true}).waitFor();
    assert.equal(requests.filter(request => request.path === '/api/agent/models/refresh' && request.method === 'POST').length, 1);
    assert.deepEqual(versions, {q1:2, q2:2, w1:2});
    await page.getByRole('button', {name:'取消', exact:true}).click();
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail:'accounts'})));
    await page.getByTitle('查看 Qoder 模型目录', {exact:true}).click();
    await page.locator('.model-catalog-row').filter({hasText:'Q1 v2'}).waitFor();
    assert.equal(await page.locator('.model-catalog-row .model-catalog-context').textContent(), '上下文 128K tokens');
    assert.equal(await page.locator('.model-catalog-row .model-catalog-context').getAttribute('title'), '目录声明的最大输入上下文，以实际可用为准');
    assert.equal(requests.filter(request => request.path === '/api/proxy/models' && request.method === 'POST').length, 0);
    await page.getByRole('button', {name:'刷新列表', exact:true}).click();
    await page.locator('.model-catalog-row').filter({hasText:'Q1 v3'}).waitFor();
    assert.deepEqual(versions, {q1:3, q2:3, w1:2});
    assert.deepEqual(requests.filter(request => request.path === '/api/proxy/models' && request.method === 'POST').map(request => request.id).sort(), ['q1','q2']);
    await page.locator('.model-catalog-account select').selectOption('q2');
    await page.locator('.model-catalog-row').filter({hasText:'Q2 v3'}).waitFor();
    assert.equal(await page.locator('.model-catalog-row .model-catalog-context').textContent(), '上下文未知');
    assert.equal(await page.locator('.model-catalog-row .model-catalog-context').getAttribute('title'), '目录未提供最大输入上下文');
    assert.deepEqual(versions, {q1:3, q2:3, w1:2});
    failRefresh = true;
    await page.getByRole('button', {name:'刷新列表', exact:true}).click();
    await page.getByText('2 个账号模型目录刷新失败，已保留原有目录').waitFor();
    await page.locator('.model-catalog-row').filter({hasText:'Q2 v3'}).waitFor();
    await page.getByRole('button', {name:'关闭模型目录弹窗'}).click();
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail:'agent'})));
    await page.getByRole('button', {name:'打开 Agent 设置'}).click();
    await page.getByRole('button', {name:/添加第一个模型/}).click();
    await page.locator('.agent-settings-group-toggle').filter({hasText:'Qoder'}).click();
    await page.locator('.agent-model-choice').filter({hasText:'Q1 v3'}).waitFor();
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
