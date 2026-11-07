import assert from 'node:assert/strict';
import {test} from 'node:test';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {chromium} from 'playwright';

// The normalisation module is exercised directly in the browser: it needs a
// real canvas to decode, scale and re-encode, and it must not depend on the
// rest of the Agent page.
test('Pasted images are scaled and re-encoded before upload', {timeout: 60000}, async () => {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5197', '--strictPort'], {stdio: ['ignore', 'pipe', 'pipe']});
  let browser;
  try {
    await Promise.race([new Promise(resolve => server.stdout.on('data', data => { if (data.toString().includes('ready in')) resolve(); })), once(server, 'exit').then(() => { throw Error('Vite exited'); })]);
    browser = await chromium.launch({channel: process.env.XIAOTAO_TEST_BROWSER || 'msedge', headless: true});
    const page = await browser.newPage({viewport: {width: 1100, height: 820}});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('http://127.0.0.1:5197');

    const result = await page.evaluate(async () => {
      const {normaliseImage, scaledSize} = await import('/src/features/agent/imageNormalise.ts');

      const build = async (width, height, paint) => {
        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;
        paint(canvas.getContext('2d'), width, height);
        const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/png'));
        return {blob, file: new File([blob], 'shot.png', {type: 'image/png'})};
      };

      const screenshot = async (width, height) => build(width, height, (context, w, h) => {
        const gradient = context.createLinearGradient(0, 0, w, h);
        gradient.addColorStop(0, '#123a5e');
        gradient.addColorStop(1, '#7fb2d9');
        context.fillStyle = gradient;
        context.fillRect(0, 0, w, h);
        for (let row = 0; row < 40; row += 1) {
          context.fillStyle = row % 2 ? '#ffffff' : '#e8eef4';
          context.fillRect(60, 80 + row * 28, w - 120, 20);
          context.fillStyle = '#20303c';
          for (let column = 0; column < 90; column += 1) {
            context.fillRect(70 + column * 25, 82 + row * 28, 4 + ((row * 7 + column * 3) % 14), 16);
          }
        }
        context.fillStyle = '#c94f4f';
        context.fillRect(w - 600, h - 300, 420, 160);
      });

      const transparent = async (width, height) => build(width, height, (context, w, h) => {
        context.clearRect(0, 0, w, h);
        context.fillStyle = '#c94f4f';
        context.fillRect(10, 10, 200, 200);
      });

      // A large screenshot must be scaled down and end up smaller.
      const large = await screenshot(2400, 1200);
      const largeResult = await normaliseImage(large.file);

      // A small image must be left alone.
      const small = await screenshot(400, 300);
      const smallResult = await normaliseImage(small.file);

      // A transparent PNG must stay PNG so it is not flattened onto black.
      const alpha = await transparent(800, 600);
      const alphaResult = await normaliseImage(alpha.file);

      return {
        large: {
          originalBytes: large.blob.size,
          bytes: largeResult.size,
          width: largeResult.width,
          height: largeResult.height,
          mimeType: largeResult.mimeType,
          scaled: largeResult.scaled,
          dataLength: largeResult.data.length,
        },
        small: {originalBytes: small.blob.size, bytes: smallResult.size, width: smallResult.width, height: smallResult.height, scaled: smallResult.scaled},
        alpha: {mimeType: alphaResult.mimeType, width: alphaResult.width, height: alphaResult.height},
        scaledSize: scaledSize(2400, 1200),
        unchangedSize: scaledSize(800, 600),
      };
    });

    // The long edge is capped and the aspect ratio is preserved.
    assert.equal(result.large.width, 1568, 'width must scale to the limit');
    assert.equal(result.large.height, 784, 'height must keep the aspect ratio');
    assert.equal(result.large.scaled, true);
    assert.ok(result.large.bytes < result.large.originalBytes, `payload must shrink: ${result.large.bytes} vs ${result.large.originalBytes}`);
    assert.ok(result.large.dataLength > 0, 'image data must be present');

    // A small image keeps its size and is not re-encoded.
    assert.equal(result.small.scaled, false);
    assert.equal(result.small.width, 400);
    assert.equal(result.small.height, 300);

    // Transparency is preserved.
    assert.equal(result.alpha.mimeType, 'image/png', 'transparent images must stay PNG');

    // The pure helper agrees with the observed behaviour.
    assert.deepEqual(result.scaledSize, {width: 1568, height: 784});
    assert.deepEqual(result.unchangedSize, {width: 800, height: 600});

    assert.deepEqual(errors, []);
  } finally {
    await browser?.close();
    server.kill();
  }
});
