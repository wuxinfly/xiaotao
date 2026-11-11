import assert from 'node:assert/strict';
import {test} from 'node:test';
import {sourceBrowser} from './source-browser-fixture.mjs';

test('local plugin installation, credential import, model refresh and disable', {timeout:60000}, () => sourceBrowser(5272, async ({page, navigate, errors}) => {
  const plugins = [];
  const actions = [];
  await page.route('**/api/source-plugins**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname;
    if (req.method() === 'GET') return route.fulfill({json:path.endsWith('legacy-trae') ? {accounts:[]} : {plugins}});
    const input = req.postDataJSON();
    if (path.endsWith('/install')) plugins.push({id:'xiaotao.trae', enabled:false, manifest:{name:'Trae 国内版', version:'0.1', entrypoint:'provider.exe'}, models:[]});
    else if (path.endsWith('/enabled')) plugins[0].enabled = input.enabled;
    else {actions.push(input); if (input.method === 'models/refresh') plugins[0].models = [{key:'model', label:'Example'}];}
    return route.fulfill({json:{ok:true}});
  });
  await navigate('accounts');
  const panel = page.getByRole('region', {name:'模型来源插件'});
  await panel.getByLabel('本地插件包目录').fill('C:\\plugins\\trae');
  await panel.getByRole('button', {name:'安装包（暂不执行）'}).click();
  await panel.getByText('provider.exe', {exact:true}).waitFor();
  await panel.getByRole('button', {name:'启用', exact:true}).click();
  await panel.getByLabel('Trae 刷新令牌').fill('synthetic-test-token');
  await panel.getByRole('button', {name:'导入登录'}).click();
  await page.waitForFunction(() => document.querySelector('input[type="password"]')?.value === '');
  await panel.getByRole('button', {name:'刷新模型',exact:true}).click();
  await panel.getByText(/1 个模型/).waitFor();
  assert.equal(actions[0].method,'login/import');
  assert.equal(actions[1].method,'models/refresh');
  await panel.getByRole('button', {name:'停用',exact:true}).click();
  await panel.getByRole('button', {name:'启用',exact:true}).waitFor();
  assert.deepEqual(errors,[]);
}));
