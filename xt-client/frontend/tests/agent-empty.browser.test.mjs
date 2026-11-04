import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

test('empty Agent state shows only the hero mascot and greeting', {timeout:60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5193', '--strictPort'], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => {if (data.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited before ready');})]);
    browser = await chromium.launch({channel:process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless:true});
    const page = await browser.newPage({viewport:{width:1280,height:900}, reducedMotion:'reduce'});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/agent/sessions') return route.fulfill({json:{sessions:[]}});
      return route.fulfill({json:{models:[], accounts:[], mode:'full'}});
    });

    await page.goto('http://127.0.0.1:5193');
    const image = page.locator('.agent-empty-mark img');
    await image.waitFor();
    assert.equal(await image.getAttribute('src'), '/agent/mascot-hero.png');
    assert.equal(await image.evaluate(element => element.complete && element.naturalWidth > 0), true);
    assert.equal(await page.locator('.agent-empty h1').textContent(), '今天想做什么？');
    assert.equal(await page.locator('.agent-empty .agent-kicker, .agent-empty .agent-empty-copy, .agent-empty .agent-suggestions').count(), 0);

    for (const viewport of [{width:1280,height:900},{width:700,height:700},{width:540,height:700}]) {
      await page.setViewportSize(viewport);
      const boxes = await page.locator('.agent-empty-mark, .agent-empty h1').evaluateAll(elements => elements.map(element => element.getBoundingClientRect().toJSON()));
      assert.equal(boxes.length, 2);
      for (let index = 1; index < boxes.length; index += 1) {
        assert.ok(boxes[index].top >= boxes[index - 1].bottom - 0.5, `empty content overlaps at ${viewport.width}px: ${index - 1} -> ${index}`);
      }
      const hero = boxes[0];
      const feed = await page.locator('.agent-feed').boundingBox();
      assert.ok(hero.left >= feed.x && hero.right <= feed.x + feed.width, 'hero must stay inside the feed');
      assert.ok(hero.width / hero.height > 1.75 && hero.width / hero.height < 1.85, 'hero must preserve its aspect ratio');
    }

    assert.equal(await page.locator('.agent-composer').isVisible(), true);
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
