import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

export async function sourceBrowser(port, run) {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1100,height:850}, reducedMotion:'reduce'});
    const errors = []; page.on('pageerror', error => errors.push(error.message));
    const state = {sources: [], requests: [], saves: [], tests: [], modelReads:0, failTest:false};
    const catalog = () => state.sources.filter(source => source.enabled).flatMap(source => source.models.map(model => ({...model, model:model.id, label:model.label, sourceId:source.id, sourceLabel:source.label})));
    await page.route('**/api/**', async route => {
      const req = route.request(), path = new URL(req.url()).pathname, method = req.method();
      state.requests.push({path, method});
      if (path.startsWith('/api/agent/models')) {state.modelReads++; return route.fulfill({json:{models:catalog()}});}
      if (path === '/api/model-sources' && method === 'GET') return route.fulfill({json:{sources:state.sources}});
      if (path === '/api/model-sources' && method === 'POST' || path.match(/^\/api\/model-sources\/[^/]+$/) && method === 'PUT') {
        const input = req.postDataJSON(); state.saves.push(input);
        const id = method === 'PUT' ? decodeURIComponent(path.split('/').at(-1)) : `source-${state.sources.length+1}`;
        const source = {...input, id, kind:'manual', hasAPIKey:true, models:input.models.map((model, index) => ({...model, id:model.id || `${id}-route-${index}`}))};
        delete source.apiKey;
        state.sources = [...state.sources.filter(item => item.id !== id), source];
        return route.fulfill({json:source});
      }
      if (path.endsWith('/test')) {state.tests.push(path); return route.fulfill(state.failTest ? {status:400,json:{error:{message:'上游拒绝测试'}}} : {json:{ok:true}});}
      if (path.startsWith('/api/model-sources/') && method === 'DELETE') {state.sources=state.sources.filter(source => source.id !== path.split('/').at(-1)); return route.fulfill({json:{ok:true}});}
      if (path === '/api/proxy/key') return route.fulfill({json:{key:'local-gateway-key'}});
      return route.fulfill({json:{sessions:[], projects:[], accounts:[], models:[], mode:'full'}});
    });
    await page.goto(`http://127.0.0.1:${port}`);
    const navigate = detail => page.evaluate(detail => window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail})), detail);
    await run({page, state, errors, navigate});
  } finally {await browser?.close(); server.kill();}
}

export async function fillSource(page, label, upstream = 'org/shared') {
  await page.getByRole('button', {name:'添加接口', exact:true}).first().click();
  await page.getByLabel('来源名称', {exact:true}).fill(label);
  await page.getByLabel('API URL', {exact:true}).fill('https://example.test/v1/chat/completions');
  await page.getByLabel('API Key', {exact:true}).fill('upstream-secret');
  await page.getByLabel('模型 1 上游模型 ID', {exact:true}).fill(upstream);
  await page.getByLabel('模型 1 显示名', {exact:true}).fill('Shared model');
}
export async function saveSource(page) {await page.getByRole('button', {name:'保存接口', exact:true}).click(); await page.getByRole('dialog').waitFor({state:'hidden'});}
