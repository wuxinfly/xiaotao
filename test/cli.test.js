import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);
const repositoryRoot = path.resolve(import.meta.dirname, '..');
const cliPath = path.join(repositoryRoot, 'bin', 'xiaotao.js');

function runCli(args) {
  return execFileAsync(process.execPath, [cliPath, ...args], {
    cwd: repositoryRoot,
    windowsHide: true,
  });
}

async function rejectedCommand(promise) {
  try {
    await promise;
  } catch (error) {
    return error;
  }
  assert.fail('Expected command to fail');
}

test('prints help and package version', async () => {
  const help = await runCli(['--help']);
  assert.match(help.stdout, /xiaotao init \[path\]/);
  assert.match(help.stdout, /--tools <list>/);

  const version = await runCli(['--version']);
  assert.equal(version.stdout.trim(), '0.2.0');
});

test('init installs selected hosts non-interactively', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-cli-test-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));

  const result = await runCli(['init', projectRoot, '--tools', 'codex,opencode']);

  assert.match(result.stdout, /Installed XiaoTao for Codex/);
  assert.match(result.stdout, /Installed XiaoTao for OpenCode/);
  assert.match(await readFile(path.join(projectRoot, '.agents/skills/xiaotao/SKILL.md'), 'utf8'), /name: xiaotao/);
  assert.match(await readFile(path.join(projectRoot, '.opencode/skills/xiaotao/SKILL.md'), 'utf8'), /name: xiaotao/);
});

test('update restores managed files and doctor supports JSON output', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-cli-test-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));
  await runCli(['init', projectRoot, '--tools', 'claude']);
  const skillPath = path.join(projectRoot, '.claude/skills/xiaotao/SKILL.md');
  await rm(skillPath);

  const failedDoctor = await rejectedCommand(runCli(['doctor', projectRoot, '--json']));
  assert.equal(failedDoctor.code, 1);
  assert.equal(JSON.parse(failedDoctor.stdout).ok, false);

  await runCli(['update', projectRoot]);
  const diagnosis = await runCli(['doctor', projectRoot, '--json']);
  assert.equal(JSON.parse(diagnosis.stdout).ok, true);
  assert.match(await readFile(skillPath, 'utf8'), /name: xiaotao/);
});

test('non-interactive init requires an explicit tool selection', async (t) => {
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-cli-test-'));
  t.after(() => rm(projectRoot, { recursive: true, force: true }));

  const failure = await rejectedCommand(runCli(['init', projectRoot]));
  assert.equal(failure.code, 2);
  assert.match(failure.stderr, /Pass --tools/);
});
