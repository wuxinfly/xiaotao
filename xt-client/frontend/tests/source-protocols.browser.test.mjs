import assert from 'node:assert/strict';
import {test} from 'node:test';
import {sourceBrowser, fillSource, saveSource} from './source-browser-fixture.mjs';

test('manual source protocol is selectable, saved and retained when editing', {timeout:60000}, () => sourceBrowser(5227, async ({page, state, errors, navigate}) => {
  await navigate('accounts');
  for (const [protocol,label,path] of [['openai-responses','OpenAI Responses','responses'],['anthropic-messages','Anthropic Messages','messages']]) {
    await fillSource(page, label);
    await page.getByRole('combobox', {name:'上游格式'}).selectOption(protocol);
    await page.getByLabel('API URL', {exact:true}).fill(`https://example.test/v1/${path}`);
    await page.getByText(`将请求：https://example.test/v1/${path}`).waitFor();
    await saveSource(page);
    assert.equal(state.saves.at(-1).protocol, protocol);
    const card=page.locator('.source-card').filter({has:page.getByRole('heading',{name:label,exact:true})});
    await card.getByRole('button',{name:'编辑',exact:true}).click();
    assert.equal(await page.getByRole('combobox',{name:'上游格式'}).inputValue(),protocol);
    await page.getByRole('combobox',{name:'上游格式'}).selectOption('openai-chat');
    await page.getByLabel('API URL',{exact:true}).fill('https://example.test/v1');
    await page.getByText('将请求：https://example.test/v1/chat/completions').waitFor();
    await saveSource(page);
    assert.equal(state.saves.at(-1).protocol,'openai-chat');
    assert.equal('apiKey' in state.saves.at(-1),false);
  }
  assert.deepEqual(errors,[]);
}));
