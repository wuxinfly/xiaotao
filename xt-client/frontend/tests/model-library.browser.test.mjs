import assert from 'node:assert/strict';
import {test} from 'node:test';
import {mkdirSync} from 'node:fs';
import {sourceBrowser} from './source-browser-fixture.mjs';

test('Agent model library supports batch selection, search, parameters and searchable utility assignment', {timeout:60000}, () => sourceBrowser(5228, async ({page,state,errors,navigate}) => {
  state.sources.push(
    {id:'a',label:'团队接口',enabled:true,models:[{id:'internal-a',label:'Qwen 旗舰',upstreamModelId:'org/qwen',contextWindow:262144,maxOutputTokens:8192,supportsImages:true,supportsReasoning:true,reasoningEfforts:['low','high']},{id:'internal-b',label:'DeepSeek',upstreamModelId:'deepseek-chat',contextWindow:65536,maxOutputTokens:4096}]},
    {id:'b',label:'日常接口',enabled:true,models:[{id:'internal-c',label:'轻量助手',upstreamModelId:'helper',contextWindow:32768,maxOutputTokens:2048},{id:'internal-d',label:'Qwen 旗舰',upstreamModelId:'org/qwen',contextWindow:131072,maxOutputTokens:8192}]},
  );
  await navigate('agent');
  await page.getByRole('button',{name:'打开 Agent 设置'}).click();
  const dialog=page.getByRole('dialog');
  await page.getByRole('checkbox',{name:'Qwen 旗舰 · 团队接口'}).waitFor();
  assert.equal(await page.locator('.model-library-card').count(),4);
  await page.getByLabel('搜索模型',{exact:true}).fill('日常接口');
  assert.equal(await page.locator('.model-library-card').count(),2);
  await page.getByRole('checkbox',{name:'轻量助手 · 日常接口'}).click();
  assert.equal(await page.locator('.model-library-card').count(),2,'selecting a model keeps the library open');
  await page.getByRole('button',{name:'清除模型搜索'}).click();
  await page.getByLabel('筛选模型来源').selectOption('a');
  await page.getByRole('button',{name:/添加当前结果/}).click();
  assert.equal(await page.getByRole('checkbox',{checked:true}).count(),2);
  await page.getByLabel('筛选模型来源').selectOption('');
  await page.getByRole('button',{name:/常用模型 3/,exact:true}).click();
  assert.equal(await page.locator('.model-library-card').count(),3);
  await page.getByRole('button',{name:'调整 Qwen 旗舰 参数'}).click();
  await page.getByLabel('Qwen 旗舰 上下文窗口').fill('65536');
  await page.getByLabel('Qwen 旗舰 最大输出').fill('4096');
  await page.getByLabel('Qwen 旗舰 思考等级').selectOption('high');
  await page.getByRole('button',{name:'调整 Qwen 旗舰 参数'}).click();
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(()=>!!document.activeElement?.closest('[inert]')),false);
  await page.getByRole('button',{name:'调整 Qwen 旗舰 参数'}).click();
  mkdirSync('../../work/ui',{recursive:true});
  await page.screenshot({path:'../../work/ui/model-library-desktop.png'});
  for (const width of [700,540,375]) {
    await page.setViewportSize({width,height:850});
    const rect=await dialog.boundingBox();assert.ok(rect.x>=0&&rect.x+rect.width<=width);
    assert.equal(await dialog.evaluate(node=>node.scrollWidth>node.clientWidth),false);
    assert.equal(await page.locator('.model-library').evaluate(node=>node.scrollWidth>node.clientWidth),false);
  }
  await page.screenshot({path:'../../work/ui/model-library-mobile.png'});
  await page.setViewportSize({width:1280,height:900});
  await page.getByRole('button',{name:/默认模型/}).click();
  await page.getByRole('combobox',{name:'AI 重命名模型',exact:true}).click();
  await page.getByLabel('搜索AI 重命名模型').fill('轻量');
  await page.getByRole('option',{name:/轻量助手/}).click();
  await page.getByRole('combobox',{name:'行为复盘模型',exact:true}).click();
  await page.getByLabel('搜索行为复盘模型').fill('Qwen');
  assert.equal(await page.getByRole('option').count(),3);
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('End');
  assert.equal(await page.getByRole('option').last().evaluate(node=>node===document.activeElement),true);
  await page.keyboard.press('Escape');
  assert.equal(await page.getByRole('combobox',{name:'行为复盘模型',exact:true}).getAttribute('aria-expanded'),'false');
  assert.equal(await dialog.isVisible(),true,'Escape closes only the inline picker');
  await page.screenshot({path:'../../work/ui/model-defaults.png'});
  await page.getByLabel('AI 重命名最大输出').fill('32768');
  await page.getByRole('button',{name:'保存设置',exact:true}).click();
  await page.getByRole('alert').filter({hasText:'最大输出必须小于上下文窗口'}).waitFor();
  await page.getByLabel('AI 重命名最大输出').fill('2048');
  await page.getByRole('button',{name:'保存设置',exact:true}).click();
  const saved=await page.evaluate(()=>({models:JSON.parse(localStorage.getItem('xiaotao.agent.enabledModels')),configs:JSON.parse(localStorage.getItem('xiaotao.agent.modelConfigs')),defaults:JSON.parse(localStorage.getItem('xiaotao.agent.defaultModels'))}));
  assert.deepEqual(saved.models,['internal-c','internal-a','internal-b']);
  assert.equal(saved.configs.find(model=>model.model==='internal-a').reasoningEffort,'high');
  assert.equal(saved.configs.find(model=>model.model==='internal-a').contextWindow,65536);
  assert.equal(saved.defaults.title,'internal-c');
  await page.getByRole('button',{name:'打开 Agent 设置'}).click();
  await page.getByRole('button',{name:/常用模型.*执行模型/}).click();
  await page.getByRole('checkbox',{name:'轻量助手 · 日常接口'}).click();
  await page.getByRole('button',{name:'取消',exact:true}).click();
  assert.deepEqual(await page.evaluate(()=>JSON.parse(localStorage.getItem('xiaotao.agent.enabledModels'))),saved.models);
  await page.getByRole('button',{name:'打开 Agent 设置'}).click();
  await page.getByRole('button',{name:/常用模型.*执行模型/}).click();
  assert.equal(await page.getByRole('checkbox',{name:'轻量助手 · 日常接口'}).getAttribute('aria-checked'),'true');
  assert.deepEqual(errors,[]);
}));
