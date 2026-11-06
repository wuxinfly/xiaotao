import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/features/accounts/quotaUtils.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } });
const { getSummaryItem, calculateQuotaSummary } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const pack = (total, remaining = total, label = '资源包') => ({ label, unit: 'credit', total, remaining, used: total - remaining });

test('WorkBuddy sums all resource packs and subscription balances', () => {
  const items = [pack(100), pack(500), pack(1500)];
  const summary = calculateQuotaSummary({ status: 'ok', items });
  assert.equal(summary.remainingText, '2,100');
  assert.equal(summary.limitText, '2,100 积分');
  assert.equal(summary.percent, 100);
  const partial = getSummaryItem([pack(100, 0, '订阅套餐'), pack(500, 250)]);
  assert.equal(partial.remaining, 250);
  assert.equal(partial.used, 350);
});

test('explicit totals win; missing values and unrelated quota windows stay intact', () => {
  const total = pack(700, 400, '总额度');
  assert.equal(getSummaryItem([pack(100), total]), total);
  assert.equal(getSummaryItem([pack(100), { ...pack(500), remaining: null }]).remaining, null);
  const windows = [{ label: '5小时', unit: 'percent', total: 100 }, { label: '周', unit: 'percent', total: 100 }];
  assert.equal(getSummaryItem(windows), windows[0]);
  assert.equal(getSummaryItem([pack(100, 50, '订阅额度'), pack(500, 250, '加购包')]).remaining, 300);
});
