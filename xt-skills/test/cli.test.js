import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { run } from '../cli/index.js';

function terminal(answer = '') {
  const input = new PassThrough(); const output = new PassThrough(); const error = new PassThrough();
  input.isTTY = true; output.isTTY = true; input.end(answer);
  let stdout = ''; let stderr = ''; output.on('data', (chunk) => { stdout += chunk; }); error.on('data', (chunk) => { stderr += chunk; });
  return { input, output, error, get stdout() { return stdout; }, get stderr() { return stderr; } };
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

test('rejects JSON for commands that do not produce JSON', async () => {
  const io = terminal();
  assert.equal(await run(['init', '--json'], { ...io, environment: {} }), 2);
  assert.match(io.stderr, /--json is only valid/);
});

test('update accepts --yes without a TTY and checks old metadata before prompting', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-cli-test-')); const home = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-home-test-'));
  t.after(() => Promise.all([rm(projectRoot, { recursive: true, force: true }), rm(home, { recursive: true, force: true })]));
  const init = terminal('2\n');
  assert.equal(await run(['init', projectRoot], { ...init, environment: { XIAOTAO_HOME: home } }), 0);
  await writeFile(path.join(home, '.xiaotao', 'managed-installation.json'), JSON.stringify({ package_version: '0.2.0' }));
  const input = new PassThrough(); const output = new PassThrough(); const error = new PassThrough();
  assert.equal(await run(['update', projectRoot, '--yes'], { input, output, error, environment: { XIAOTAO_HOME: home } }), 0);
  await mkdir(path.join(projectRoot, '.xiaotao'), { recursive: true });
  await writeFile(path.join(projectRoot, '.xiaotao', 'installation.json'), JSON.stringify({ schema_version: 1 }));
  const old = terminal('n\n');
  assert.equal(await run(['update', projectRoot], { ...old, environment: { XIAOTAO_HOME: home } }), 1);
  assert.doesNotMatch(old.stdout, /共享安装将从/);
});
