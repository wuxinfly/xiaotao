import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/features/accounts/modelCatalogUtils.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } });
const { formatContextWindow, getContextWindowTitle } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('formatContextWindow handles missing, zero, or invalid values', () => {
  assert.equal(formatContextWindow(undefined), '上下文未知');
  assert.equal(formatContextWindow(null), '上下文未知');
  assert.equal(formatContextWindow(0), '上下文未知');
  assert.equal(formatContextWindow(-100), '上下文未知');
  assert.equal(formatContextWindow(Number.NaN), '上下文未知');
  assert.equal(getContextWindowTitle(undefined), '目录未提供最大输入上下文');
  assert.equal(getContextWindowTitle(0), '目录未提供最大输入上下文');
});

test('formatContextWindow formats typical standard powers and decimal round numbers', () => {
  // 128K variants (128000 or 131072)
  assert.equal(formatContextWindow(128000), '上下文 128K tokens');
  assert.equal(formatContextWindow(131072), '上下文 128K tokens');

  // 200K / 272K / 32K / 8K
  assert.equal(formatContextWindow(200000), '上下文 200K tokens');
  assert.equal(formatContextWindow(272000), '上下文 272K tokens');
  assert.equal(formatContextWindow(32768), '上下文 32K tokens');
  assert.equal(formatContextWindow(8192), '上下文 8K tokens');

  // 1M variants (1000000 or 1048576)
  assert.equal(formatContextWindow(1000000), '上下文 1M tokens');
  assert.equal(formatContextWindow(1048576), '上下文 1M tokens');
  assert.equal(formatContextWindow(2000000), '上下文 2M tokens');
  assert.equal(formatContextWindow(2097152), '上下文 2M tokens');

  assert.equal(getContextWindowTitle(128000), '目录声明的最大输入上下文，以实际可用为准');
});

test('formatContextWindow retains precision for irregular numbers and fractional roundings', () => {
  assert.equal(formatContextWindow(12500), '上下文 12.5K tokens');
  assert.equal(formatContextWindow(1500000), '上下文 1.5M tokens');
  assert.equal(formatContextWindow(12345), '上下文 12,345 tokens');
  assert.equal(formatContextWindow(999), '上下文 999 tokens');
});
