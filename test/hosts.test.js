import assert from 'node:assert/strict';
import test from 'node:test';

import { HOSTS, detectHosts, parseToolList } from '../cli/hosts.js';

test('declares the three MVP hosts and canonical skill destinations', () => {
  assert.deepEqual(Object.keys(HOSTS), ['codex', 'claude', 'opencode']);
  assert.equal(HOSTS.codex.skillDir, '.agents/skills/xiaotao');
  assert.equal(HOSTS.claude.skillDir, '.claude/skills/xiaotao');
  assert.equal(HOSTS.opencode.skillDir, '.opencode/skills/xiaotao');
});

test('parses comma-separated tools without duplicates', () => {
  assert.deepEqual(parseToolList('codex,claude,codex'), ['codex', 'claude']);
  assert.deepEqual(parseToolList('all'), ['codex', 'claude', 'opencode']);
  assert.deepEqual(parseToolList('none'), []);
  assert.throws(() => parseToolList('cursor'), /Unknown AI tool: cursor/);
});

test('detects existing host directories', async () => {
  const seen = [];
  const exists = async (relativePath) => {
    seen.push(relativePath);
    return relativePath === '.claude' || relativePath === '.codex';
  };

  assert.deepEqual(await detectHosts(exists), ['codex', 'claude']);
  assert.ok(seen.includes('.agents/skills'));
  assert.ok(seen.includes('.codex'));
  assert.ok(seen.includes('.claude'));
});
