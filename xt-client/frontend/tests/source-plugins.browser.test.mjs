import assert from 'node:assert/strict';
import {test} from 'node:test';
import {sourceBrowser} from './source-browser-fixture.mjs';

test('local plugin installation, model refresh and disable', {timeout: 60000}, () => sourceBrowser(5272, async ({page, navigate, errors}) => {
  let plugin = null;
  let refreshed = false;
  await page.route('**/api/source-plugins**', async route => {
    const req = route.request(), path = new URL(req.url()).pathname;
    if (req.method() === 'GET') {
      if (path.endsWith('/legacy-trae')) return route.fulfill({json: {accounts: []}});
      if (path.endsWith('/packages')) return route.fulfill({json: {packages: plugin ? [] : [{
        path: 'C:\\plugins\\trae',
        manifest: {id: 'xiaotao.trae', name: 'Trae 国内版', version: '0.2.0', entrypoint: 'provider.exe', capabilities: ['browser-login']},
        updateAvailable: false, enabled: false,
      }]}});
      return route.fulfill({json: {plugins: plugin ? [plugin] : []}});
    }
    const input = req.postDataJSON();
    if (path.endsWith('/install')) {
      plugin = {id: 'xiaotao.trae', enabled: false, manifest: {name: 'Trae 国内版', version: '0.2.0', entrypoint: 'provider.exe', capabilities: ['browser-login']}, models: []};
    } else if (path.endsWith('/enabled')) {
      plugin.enabled = input.enabled;
    } else if (path.endsWith('/action') && input.method === 'models/refresh') {
      plugin.models = [{key: 'model', label: 'Example'}];
      refreshed = true;
    }
    return route.fulfill({json: {ok: true}});
  });
  await navigate('accounts');
  const panel = page.getByRole('region', {name: '模型来源插件'});
  await panel.getByRole('button', {name: '安装插件'}).click();
  await panel.getByText('发现本机插件包').waitFor();
  await panel.locator('.plugin-package').getByRole('button', {name: '安装', exact: true}).click();
  await panel.getByText('已安装 Trae 国内版 v0.2.0，请核对后启用').waitFor();
  await panel.getByRole('switch', {name: '启用 Trae 国内版'}).click();
  await panel.getByText('已启用', {exact: true}).waitFor();
  await panel.getByRole('button', {name: '刷新目录'}).click();
  await panel.getByText('已选 1 / 1').waitFor();
  assert.equal(refreshed, true);
  await panel.getByRole('switch', {name: '启用 Trae 国内版'}).click();
  await panel.getByText('已停用', {exact: true}).waitFor();
  assert.deepEqual(errors, []);
}));