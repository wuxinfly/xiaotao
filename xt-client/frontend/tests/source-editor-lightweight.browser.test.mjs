import assert from 'node:assert/strict';
import {test} from 'node:test';
import {fillSource, saveSource, sourceBrowser} from './source-browser-fixture.mjs';

test('source editor previews the final URL, reveals the key, and aligns token limits', {timeout:60000}, () => sourceBrowser(5273, async ({page,state,errors,navigate}) => {
  await navigate('accounts');
  await fillSource(page, 'Lightweight editor');
  const url = page.getByLabel('API URL', {exact:true});
  await page.getByText('将请求：https://example.test/v1/chat/completions').waitFor();
  await url.fill('https://example.test/v1/');
  await page.getByText('将请求：https://example.test/v1/chat/completions').waitFor();
  await url.fill('https://example.test/v1/chat/completions/');
  await page.getByText('将请求：https://example.test/v1/chat/completions').waitFor();
  await url.fill('https://example.test/v1?key=1');
  await assert.equal(await page.locator('.source-url-preview').count(), 0);

  await url.fill('https://example.test/v1');
  const key = page.getByLabel('API Key', {exact:true});
  await assert.equal(await key.getAttribute('type'), 'password');
  await page.getByRole('button', {name:'显示 API Key'}).click();
  await assert.equal(await key.getAttribute('type'), 'text');
  await assert.equal(await key.inputValue(), 'upstream-secret');
  await page.getByRole('button', {name:'隐藏 API Key'}).click();
  await assert.equal(await key.getAttribute('type'), 'password');

  assert.equal(await page.getByRole('button', {name:'上游格式'}).innerText(), 'OpenAI Chat Completions');
  await assert.ok(await page.getByText('必填', {exact:true}).count() >= 3);
  await assert.ok(await page.getByText('选填', {exact:true}).count() >= 4);
  await assert.equal(await page.getByLabel('模型 1 上下文窗口').getAttribute('min'), '1024');
  await assert.equal(await page.getByLabel('模型 1 最大输出').getAttribute('min'), '16');
  await assert.ok(await page.locator('.source-token-field small').filter({hasText:'tokens'}).count() >= 2);

  await page.getByLabel('模型 1 上下文窗口').fill('128');
  await page.getByRole('button', {name:'保存接口', exact:true}).click();
  await page.getByRole('alert').filter({hasText:'模型 1：上下文窗口至少为 1024 tokens'}).waitFor();
  await assert.equal(state.saves.length, 0);
  await page.getByLabel('模型 1 上下文窗口').fill('4096');
  await page.getByLabel('模型 1 最大输出').fill('1');
  await page.getByRole('button', {name:'保存接口', exact:true}).click();
  await page.getByRole('alert').filter({hasText:'模型 1：最大输出至少为 16 tokens'}).waitFor();
  await assert.equal(state.saves.length, 0);
  await page.getByLabel('模型 1 最大输出').fill('4096');
  await page.getByRole('button', {name:'保存接口', exact:true}).click();
  await page.getByRole('alert').filter({hasText:'模型 1：最大输出须小于上下文窗口'}).waitFor();
  await assert.equal(state.saves.length, 0);

  await page.getByLabel('模型 1 最大输出').fill('256');
  await saveSource(page);
  await assert.equal(state.sources.length, 1);
  await assert.equal(state.saves[0].models[0].maxOutputTokens, 256);
  await assert.deepEqual(errors, []);
}));

test('discovering models opens a searchable dialog and changes only the editor draft', {timeout:60000}, () => sourceBrowser(5274, async ({page,state,errors,navigate}) => {
  const {tmpdir} = await import('node:os');
  const {join} = await import('node:path');
  const models = [
    {id:'org/shared', display_name:'Shared model'},
    {id:'auto/deepseek-v4.1-flash-1017', display_name:'DeepSeek V4.1 Flash 1017'},
    {id:'group/glm-5.3-flash', display_name:'GLM 5.3 Flash'},
    {id:'auto/kimi-k3.1-pro', display_name:'Kimi K3.1 Pro'},
    {id:'group/minimax-m3.1-flash', display_name:'MiniMax M3.1 Flash'},
    {id:'team/very-long-model-identifier-for-narrow-frame-layout-without-horizontal-overflow'},
  ];
  let requests = 0;
  await page.route('**/api/model-sources/source-1/discover', route => {
    assert.equal(Object.hasOwn(route.request().postDataJSON(), 'apiKey'), false);
    requests++;
    if (requests === 1) return route.fulfill({status:502,json:{error:{message:'测试服务暂不可用'}}});
    return route.fulfill({json:{models:requests === 4 ? [] : models}});
  });
  await navigate('accounts');
  await fillSource(page, '团队模型服务');
  await saveSource(page);
  await page.getByRole('button', {name:'编辑', exact:true}).click();
  const editor = page.getByRole('dialog', {name:'编辑模型接口'});
  const trigger = editor.getByRole('button', {name:'获取可用模型'});
  const picker = page.getByRole('dialog', {name:'获取可用模型'});
  const search = picker.getByRole('searchbox', {name:'搜索模型'});
  const add = picker.getByRole('button', {name:'添加选中的模型'});
  await editor.screenshot({path:join(tmpdir(),'xiaotao-source-editor.png')});
  await trigger.click();
  await picker.getByRole('alert').getByText('测试服务暂不可用').waitFor();
  assert.equal(await editor.count(), 0, 'covered editor is hidden from accessibility');
  await picker.getByRole('button', {name:'重试'}).click();
  await search.waitFor();
  const existing = picker.getByRole('checkbox', {name:/Shared model/});
  assert.ok(await existing.isDisabled());
  assert.ok(await existing.isChecked());
  await picker.getByRole('checkbox', {name:/DeepSeek/}).check();
  await picker.screenshot({path:join(tmpdir(),'xiaotao-source-catalog.png')});
  await search.fill('group/GLM');
  await picker.getByRole('checkbox', {name:/GLM 5.3 Flash/}).check();
  assert.equal(await picker.getByRole('checkbox').count(), 1);
  await search.fill('no matching model');
  await picker.getByText('没有匹配的模型。').waitFor();
  await page.keyboard.press('Escape');
  await editor.waitFor();
  await page.waitForFunction(() => document.activeElement?.textContent === '获取可用模型');
  assert.equal(await editor.getByLabel(/上游模型 ID/).count(), 1);
  assert.equal(state.saves.length, 1);
  await page.setViewportSize({width:370,height:580});
  await trigger.click();
  await picker.getByRole('checkbox', {name:/DeepSeek/}).check();
  await search.fill('group/GLM');
  await picker.getByRole('checkbox', {name:/GLM 5.3 Flash/}).check();
  await search.clear();
  assert.ok(await picker.locator('.source-picker-list').evaluate(list => list.scrollWidth === list.clientWidth));
  await add.click();
  await editor.getByLabel('模型 3 上游模型 ID').waitFor();
  assert.equal(await editor.getByLabel('模型 2 上游模型 ID').inputValue(), models[1].id);
  assert.equal(await editor.getByLabel('模型 3 上游模型 ID').inputValue(), models[2].id);
  assert.equal(state.saves.length, 1);
  await trigger.click();
  await picker.getByText('上游没有返回模型，可以手动填写模型 ID。').waitFor();
  await picker.getByRole('button', {name:'取消'}).click();
  await saveSource(page);
  assert.equal(state.saves[1].models.length, 3);
  assert.equal(Object.hasOwn(state.saves[1], 'apiKey'), false);
  assert.deepEqual(errors, []);
}));
