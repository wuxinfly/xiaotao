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

  await assert.equal(await page.locator('.source-static-field input').count(), 0);
  await assert.match(await page.locator('.source-static-field strong').innerText(), /OpenAI Chat Completions/);
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
