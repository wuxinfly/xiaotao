import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { run } from '../cli/index.js';

function terminal(answer = '') {
  const input = new PassThrough(); const output = new PassThrough(); const error = new PassThrough();
  input.isTTY = true; output.isTTY = true; input.end(answer);
  let stdout = ''; output.on('data', (chunk) => { stdout += chunk; });
  return { input, output, error, get stdout() { return stdout; } };
}

test('init asks for hosts and doctor checks the project record', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-cli-test-')); const home = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-home-test-'));
  t.after(() => Promise.all([rm(projectRoot, { recursive: true, force: true }), rm(home, { recursive: true, force: true })]));
  const io = terminal('2\n');
  assert.equal(await run(['init', projectRoot], { ...io, environment: { XIAOTAO_HOME: home } }), 0);
  assert.match(io.stdout, /Codex/);
  assert.match(await readFile(path.join(home, '.agents/skills/xiaotao/SKILL.md'), 'utf8'), /name: xiaotao/);
  const diagnosis = terminal();
  assert.equal(await run(['doctor', projectRoot], { ...diagnosis, environment: { XIAOTAO_HOME: home } }), 0);
});

test('init rejects non-interactive use', async () => {
  const input = new PassThrough(); const output = new PassThrough(); const error = new PassThrough();
  assert.equal(await run(['init'], { input, output, error, environment: {} }), 2);
});
