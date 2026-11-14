import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

// The plugin panel used to hardcode the deep-sea palette while its text followed the
// theme variables, so the shallow-sea theme put muted text on a dark surface at
// roughly 2:1 contrast. These checks keep both themes readable.
test('plugin panel keeps readable contrast in both themes', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5206', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', d => {if (d.toString().includes('ready in')) resolve();})), once(server, 'exit').then(() => {throw Error('Vite exited');})]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const context = await browser.newContext({viewport: {width: 1280, height: 850}});
    // Synthetic data only; every API is intercepted.
    await context.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/dashboard/events') return route.fulfill({contentType: 'text/event-stream', body: ': fixture\n\n'});
      if (path === '/api/source-plugins/packages') return route.fulfill({json: {packages: [{path: 'C:\\plugins\\trae', manifest: {id: 'xiaotao.trae', name: 'Trae 国内版', version: '0.4.0', entrypoint: 'provider.exe'}, hash: '158b7fe845b81234567890abcdef1234567890abcdef1234567890abcdef123456', updateAvailable: false, enabled: false}]}});
      if (path === '/api/source-plugins/index') return route.fulfill({json: {plugins: [{id: 'xiaotao.trae', name: 'Trae 国内版', version: '0.4.0', path: 'trae', resolvedPath: 'C:\\plugins\\trae', description: '本地索引条目', capabilities: ['chat'], hashMatches: true, installed: false, enabled: false, updateAvailable: false}]}});
      if (path === '/api/source-plugins') return route.fulfill({json: {plugins: []}});
      if (path === '/api/source-plugins/legacy-trae') return route.fulfill({json: {accounts: []}});
      return route.fulfill({json: {sessions: [], accounts: [], providers: [], models: [], key: 'fixture'}});
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    for (const theme of ['light', 'dark']) {
      await page.goto('http://127.0.0.1:5206');
      await page.evaluate(async value => {const {saveTheme} = await import('/src/lib/theme.ts'); saveTheme(value);}, theme);
      await page.reload();
      await page.evaluate(() => window.dispatchEvent(new CustomEvent('xiaotao:navigate', {detail: 'accounts'})));
      const panel = page.getByRole('region', {name: '模型来源插件'});
      await panel.getByRole('button', {name: '安装插件', exact: true}).click();
      await panel.getByText('发现本机插件包').waitFor();
      const report = await page.evaluate(() => {
        const luminance = rgb => {
          const [r, g, b] = rgb.map(value => {const s = value / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;});
          return 0.2126 * r + 0.7152 * g + 0.0722 * b;
        };
        const channels = value => (value.match(/[\d.]+/g) || []).slice(0, 3).map(Number);
        const alphaOf = value => Number((value.match(/[\d.]+/g) || [])[3] ?? 1);
        const backgroundOf = element => {
          let background = [255, 255, 255];
          for (let node = element; node; node = node.parentElement) {
            const color = channels(getComputedStyle(node).backgroundColor);
            const alpha = alphaOf(getComputedStyle(node).backgroundColor);
            if (color.length !== 3) continue;
            if (alpha >= 0.95) {background = color; break;}
            background = color.map((value, index) => value * alpha + background[index] * (1 - alpha));
          }
          return background;
        };
        const found = [];
        let checked = 0;
        for (const element of document.querySelectorAll('.plugin-section *')) {
          const text = [...element.childNodes].filter(node => node.nodeType === 3).map(node => node.textContent.trim()).join('');
          if (!text) continue;
          const style = getComputedStyle(element);
          if (style.visibility === 'hidden' || style.display === 'none' || Number(style.opacity) < 0.9) continue;
          // Gradient buttons paint their own surface; their palette is a theme concern.
          if (style.backgroundImage !== 'none') continue;
          // Buttons paint their own surface, so read the element background first.
          const own = channels(style.backgroundColor);
          const ownAlpha = alphaOf(style.backgroundColor);
          const background = ownAlpha >= 0.95 ? own : backgroundOf(element);
          const foreground = channels(style.color);
          const light = luminance(foreground), dark = luminance(background);
          const ratio = (Math.max(light, dark) + 0.05) / (Math.min(light, dark) + 0.05);
          const size = parseFloat(style.fontSize);
          const bold = Number(style.fontWeight) >= 700;
          const required = (size >= 18.66 || (bold && size >= 14)) ? 3 : 4.5;
          checked++;
          if (ratio < required) found.push({selector: `${element.tagName.toLowerCase()}.${element.className}`, text: text.slice(0, 24), ratio, required, size});
        }
        return {checked, found};
      });
      assert.ok(report.checked > 10, `${theme} theme must expose plugin labels to check`);
      for (const sample of report.found) {
        assert.ok(sample.ratio >= sample.required, `${theme} theme: ${sample.selector} "${sample.text}" contrast ${sample.ratio.toFixed(2)} is below ${sample.required}`);
      }
    }
    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
