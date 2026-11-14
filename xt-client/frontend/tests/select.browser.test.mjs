import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('shared select interactions', {timeout: 90000}, async t => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5189', '--strictPort'], {cwd: process.cwd(), stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([
      new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})),
      once(server, 'exit').then(() => {throw Error('Vite exited before ready');}),
    ]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});

    async function open(query = '') {
      const page = await browser.newPage({viewport: {width: 1100, height: 760}});
      const errors = [];
      page.on('pageerror', error => errors.push(error.message));
      await page.goto(`http://127.0.0.1:5189/tests/select.fixture.html${query}`);
      await page.locator('#basic-value').waitFor();
      return {page, errors};
    }
    const visiblePanel = page => page.locator('.app-select-panel.is-visible');
    const activeText = page => page.evaluate(() => document.activeElement?.textContent?.trim());

    await t.test('keyboard, escape, outside click and selection', async () => {
      const {page, errors} = await open();
      const trigger = page.getByRole('button', {name: '基础下拉'});
      await trigger.focus();
      await page.keyboard.press('ArrowDown');
      await page.waitForFunction(() => Boolean(document.querySelector('.app-select-panel.is-visible')));
      await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Beta');
      await page.keyboard.press('ArrowDown');
      assert.equal(await activeText(page), 'Gamma');
      await page.keyboard.press('Enter');
      await page.waitForFunction(() => document.getElementById('basic-value')?.textContent === 'gamma');
      assert.equal(await trigger.getAttribute('aria-expanded'), 'false');
      assert.ok(await trigger.evaluate(el => el === document.activeElement), 'focus should return to trigger after picking');

      await trigger.press('Enter');
      await page.waitForFunction(() => Boolean(document.querySelector('.app-select-panel.is-visible')));
      await page.keyboard.press('Escape');
      await page.waitForFunction(() => !document.querySelector('.app-select-panel.is-visible'));
      assert.equal(await page.locator('#basic-value').textContent(), 'gamma');
      assert.ok(await trigger.evaluate(el => el === document.activeElement), 'focus should return to trigger after escape');

      await trigger.click();
      await page.waitForFunction(() => Boolean(document.querySelector('.app-select-panel.is-visible')));
      await page.mouse.click(960, 320);
      await page.waitForFunction(() => !document.querySelector('.app-select-panel.is-visible'));
      assert.equal(await page.locator('#basic-value').textContent(), 'gamma');
      assert.deepEqual(errors, []);
      await page.close();
    });

    await t.test('search filter, empty states, disabled trigger and rapid toggle', async () => {
      const {page, errors} = await open();
      const trigger = page.getByRole('button', {name: '搜索下拉'});
      await trigger.click();
      const input = page.getByRole('textbox', {name: '搜索选项'});
      await input.waitFor();
      await page.waitForFunction(() => document.activeElement?.getAttribute('aria-label') === '搜索选项');
      await input.fill('09');
      await page.waitForFunction(() => document.querySelectorAll('.app-select-panel.is-visible [role="option"]').length === 1);
      await input.fill('zzz');
      await page.getByText('没有匹配项').waitFor();
      await input.fill('');
      await page.waitForFunction(() => document.querySelectorAll('.app-select-panel.is-visible [role="option"]').length === 24);
      await input.fill('05');
      await input.press('Enter');
      await page.waitForFunction(() => document.getElementById('search-value')?.textContent === 'model-5');

      const disabled = page.getByRole('button', {name: '禁用下拉'});
      assert.equal(await disabled.isDisabled(), true);
      await disabled.dispatchEvent('click');
      assert.equal(await disabled.getAttribute('aria-expanded'), 'false');

      await page.getByRole('button', {name: '空下拉'}).click();
      await page.getByText('暂无可选项').waitFor();
      await page.keyboard.press('Escape');

      const basic = page.getByRole('button', {name: '基础下拉'});
      await basic.click();
      await basic.click();
      await basic.click();
      await page.waitForFunction(() => Boolean(document.querySelector('.app-select-panel.is-visible')?.parentElement));
      assert.equal(await basic.getAttribute('aria-expanded'), 'true');
      await basic.click();
      assert.equal(await basic.getAttribute('aria-expanded'), 'false');
      assert.deepEqual(errors, []);
      await page.close();
    });

    await t.test('modal stacking, escape layering, viewport flip, themes and motion', async () => {
      const {page, errors} = await open();
      await page.getByRole('button', {name: '打开弹窗'}).click();
      await page.getByRole('dialog').waitFor();
      const inner = page.getByRole('button', {name: '弹窗下拉'});
      await inner.click();
      await visiblePanel(page).waitFor();
      const beta = page.getByRole('option', {name: 'Beta'});
      const betaBox = await beta.boundingBox();
      assert.ok(betaBox, 'option should have a box');
      const onTop = await page.evaluate(({x, y}) => Boolean(document.elementFromPoint(x, y)?.closest('.app-select-panel')), {x: betaBox.x + betaBox.width / 2, y: betaBox.y + betaBox.height / 2});
      assert.ok(onTop, 'select panel should render above the modal');
      await page.keyboard.press('Escape');
      await page.waitForFunction(() => !document.querySelector('.app-select-panel.is-visible'));
      assert.equal(await page.getByRole('dialog').count(), 1, 'escape in select must not close the modal');
      assert.ok(await inner.evaluate(el => el === document.activeElement), 'focus should return to the select trigger');
      assert.equal(await page.locator('#modal-value').textContent(), 'alpha');
      await inner.click();
      await page.getByRole('option', {name: 'Beta'}).click();
      await page.waitForFunction(() => document.getElementById('modal-value')?.textContent === 'beta');
      await page.locator('.app-modal-footer').getByRole('button', {name: '关闭'}).click();
      await page.waitForFunction(() => !document.querySelector('[role="dialog"]'));

      const bottom = page.getByRole('button', {name: '底部下拉'});
      await bottom.click();
      const panel = visiblePanel(page);
      await panel.waitFor();
      const panelBox = await panel.boundingBox();
      const triggerBox = await bottom.boundingBox();
      assert.ok(panelBox.y + panelBox.height <= triggerBox.y + 1, 'panel should flip above when viewport space below is insufficient');
      assert.ok(panelBox.y >= 0 && panelBox.y + panelBox.height <= 760, 'panel should stay inside the viewport');
      assert.deepEqual(errors, []);
      await page.close();
    });

    await t.test('light theme and reduced motion', async () => {
      const light = await open('?theme=light');
      assert.equal(await light.page.evaluate(() => document.documentElement.dataset.theme), 'light');
      const lightTrigger = light.page.getByRole('button', {name: '基础下拉'});
      assert.equal(await lightTrigger.evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(241, 247, 249)');
      await lightTrigger.click();
      await visiblePanel(light.page).waitFor();
      const selected = light.page.getByRole('option', {name: 'Beta'});
      assert.equal(await selected.evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(213, 235, 238)');
      assert.deepEqual(light.errors, []);
      await light.page.close();

      const reduced = await open();
      await reduced.page.emulateMedia({reducedMotion: 'reduce'});
      const trigger = reduced.page.getByRole('button', {name: '基础下拉'});
      assert.equal(await trigger.evaluate(el => getComputedStyle(el).transitionDuration), '0s');
      await trigger.click();
      const panel = visiblePanel(reduced.page);
      await panel.waitFor();
      assert.equal(await panel.evaluate(el => getComputedStyle(el).transform), 'none');
      assert.deepEqual(reduced.errors, []);
      await reduced.page.close();
    });

    console.log('select fixture: keyboard, escape layering, outside click, search, states, flip, modal stacking, themes and reduced motion passed.');
  } finally {
    await browser?.close();
    server.kill();
  }
});
