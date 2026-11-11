import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('plugin catalog, account and usage controls fit the card', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5202', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', d => {if (d.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited');})]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const context = await browser.newContext({viewport: {width: 1280, height: 850}});
    const names = ['GLM-5.3 Flash', 'DeepSeek V4.1 Flash', 'MiMo V2.6 Flash', 'GLM-5.3 Flash (Max)', 'DeepSeek V4.1 Flash (Max)', 'Kimi K2.8 Preview'];
    let modelSettings;
    let accounts = [{id: 'a', name: '已授权甲'}, {id: 'b', name: '已授权乙'}];
    let order;
    let pluginVersion = '0.1.0';
    let capabilities = [];
    let enabled = true;
    await context.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/dashboard/events') return route.fulfill({contentType: 'text/event-stream', body: ': fixture\n\n'});
      if (path.endsWith('/xiaotao.trae/models')) {
        modelSettings = route.request().postDataJSON();
        return route.fulfill({json: {ok: true}});
      }
      if (path === '/api/source-plugins/packages') return route.fulfill({json: {packages: [{path: 'C:\\plugins\\trae', manifest: {id: 'xiaotao.trae', name: 'Trae 国内版', version: '0.2.0', entrypoint: 'provider.exe', capabilities: ['browser-login']}, installedVersion: pluginVersion, updateAvailable: pluginVersion !== '0.2.0', enabled}]}});
      if (path === '/api/source-plugins') return route.fulfill({json: {plugins: [{id: 'xiaotao.trae', enabled, accounts, modelSettings, manifest: {name: 'Trae 国内版', version: pluginVersion, entrypoint: 'provider.exe', capabilities}, models: Array.from({length: 34}, (_, i) => ({key: 'model-' + (i + 1), label: names[i] ?? '模型 ' + (i + 1), priceFactor: i === 0 ? 0.31 : undefined, isFree: i === 1 ? true : undefined}))}]}});
      if (path === '/api/source-plugins/legacy-trae') return route.fulfill({json: {accounts: [{id: '1', name: '测试旧账号', uid: '用户 1001'}]}});
      if (path.endsWith('/enabled')) {
        enabled = route.request().postDataJSON().enabled;
        return route.fulfill({json: {ok: true}});
      }
      if (path === '/api/source-plugins/update') {
        pluginVersion = '0.2.0';
        capabilities = ['browser-login'];
        return route.fulfill({json: {ok: true}});
      }
      if (path.endsWith('/account-order')) {
        order = route.request().postDataJSON().ids;
        accounts = order.map(id => accounts.find(account => account.id === id));
        return route.fulfill({json: {ok: true}});
      }
      if (path.endsWith('/action')) {
        const {method, accountId} = route.request().postDataJSON();
        if (method === 'login/browser') {
          accounts = [...accounts, {id: 'c', name: '新授权账号'}];
          return route.fulfill({json: {ok: true}});
        }
        assert.equal(accountId, 'b');
        return route.fulfill({json: {value: method === 'quota/get' ? {items: [{label: '总额度', used: 49, total: 3100, unit: 'credit'}, {label: '签到奖励', used: 0, total: 100, unit: 'credit'}, {label: '签到奖励', used: 0, total: 100, unit: 'credit'}]} : {supported: true, checkedIn: true, dailyCredit: 200}}});
      }
      return route.fulfill({json: {sessions: [], accounts: [], providers: [], models: [], key: 'fixture', agent: {currentSessionId: '', defaultModels: {}, defaultModelSettings: {}, enabledModels: [], modelConfigs: [], settings: null}, settings: {models: [], defaultModels: {}, enabledModels: []}}});
    });
    const page = await context.newPage();
    await page.goto('http://127.0.0.1:5202');
    await page.locator('.workspace-dock').getByRole('button', {name: '模型接口'}).click();
    const panel = page.getByRole('region', {name: '模型来源插件'});
    await panel.getByRole('heading', {name: 'Trae 国内版'}).waitFor();
    await panel.getByText('发现 v0.2.0').waitFor();
    await panel.getByRole('button', {name: '安装插件', exact: true}).click();
    await panel.getByText('发现本机插件包').waitFor();
    await panel.getByRole('button', {name: '停用并更新'}).click();
    await panel.getByText('已更新到 v0.2.0').waitFor();
    const flashChip = () => panel.locator('.plugin-model-chip').filter({hasText: 'GLM-5.3 Flash'}).first();
    await flashChip().waitFor();
    assert.equal(await panel.getByText('模型 34', {exact: true}).count(), 0);
    await panel.getByRole('searchbox', {name: '搜索模型'}).fill('model-34');
    await panel.getByText('模型 34', {exact: true}).waitFor();
    await panel.getByRole('searchbox', {name: '搜索模型'}).clear();
    await panel.getByRole('button', {name: '显示其余 22 个'}).click();
    await panel.getByText('模型 34', {exact: true}).waitFor();
    await panel.getByRole('button', {name: '收起列表'}).click();
    await panel.getByRole('button', {name: '全不选'}).click();
    await flashChip().click();
    await panel.getByRole('textbox', {name: '上游模型 ID'}).fill('org/Custom-Model');
    await panel.getByRole('button', {name: '添加模型'}).click();
    await panel.getByRole('button', {name: '保存模型'}).click();
    await panel.getByText('模型设置已保存').waitFor();
    assert.deepEqual(modelSettings, {selectedKeys: ['model-1', 'manual:org/Custom-Model'], customIds: ['org/Custom-Model']});
    await panel.getByRole('searchbox', {name: '搜索模型'}).clear();
    await panel.getByRole('button', {name: '仅显示已选'}).click();
    const chips = await panel.locator('.plugin-model-chip').allTextContents();
    assert.equal(chips.length, 2);
    assert.match(chips[0], /^GLM-5\.3 Flash/);
    assert.match(chips[0], /0\.31×$/);
    assert.equal(chips[1], 'org/Custom-Model');
    const accountRows = panel.locator('.plugin-account-row');
    await accountRows.filter({hasText: '已授权乙'}).locator('.plugin-account-grip').dragTo(accountRows.filter({hasText: '已授权甲'}));
    await panel.getByText('账号顺序已保存').waitFor();
    assert.deepEqual(order, ['b', 'a']);
    await panel.getByRole('button', {name: '添加另一个账号'}).click();
    await panel.getByText('新授权账号').waitFor();
    const first = accountRows.filter({hasText: '已授权乙'});
    await first.getByRole('button', {name: '查询配额'}).click();
    await panel.getByText('49 / 3,100 积分 · 已用 1.6%').waitFor();
    await first.getByRole('button', {name: '展开额度明细'}).click();
    await panel.getByText('签到奖励').waitFor();
    assert.equal(await panel.locator('.plugin-quota-name small').filter({hasText: '×2'}).count(), 1);
    await first.getByRole('button', {name: '签到状态'}).click();
    await panel.getByText('今日已签到 · 每日 200 积分').waitFor();
    assert.equal(await panel.locator('pre').count(), 0);
    await panel.getByText('49 / 3,100 积分 · 已用 1.6%').waitFor();
    await panel.getByText('测试旧账号').waitFor();
    if (process.env.XIAOTAO_PLUGIN_PREVIEW) await panel.screenshot({path: process.env.XIAOTAO_PLUGIN_PREVIEW});
    await page.setViewportSize({width: 420, height: 800});
    assert.equal(await panel.getByRole('button', {name: '添加另一个账号'}).isVisible(), true);
    assert.equal(await panel.evaluate(element => element.scrollWidth <= element.clientWidth), true);
  } finally {await browser?.close(); server.kill();}
});
