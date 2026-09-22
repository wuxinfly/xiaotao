import assert from 'node:assert/strict';
import path from 'node:path';
import test from 'node:test';
import { resolveDshHome, resolveUserHome, SCENES } from '../cli/scenes.js';

test('resolves every user-level target from XIAOTAO_HOME', () => {
  const environment = { XIAOTAO_HOME: '/tmp/xiaotao-home' };
  assert.equal(resolveUserHome(environment), path.resolve('/tmp/xiaotao-home'));
  assert.equal(SCENES.skills.resolveTarget(environment).path, path.resolve('/tmp/xiaotao-home/.agents/skills/xiaotao'));
  assert.equal(SCENES.claude.resolveTarget(environment).path, path.resolve('/tmp/xiaotao-home/.claude/skills/xiaotao'));
  assert.equal(resolveDshHome(environment), path.resolve('/tmp/xiaotao-home/.dsh'));
});
