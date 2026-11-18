import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { doctorInstallation, globalUpdateNotice, installCodex, installScenes, readGlobalInstallation, readInstallation, updateScenes } from '../cli/install.js';

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-install-test-'));
  const packageRoot = path.join(root, 'package'); const projectRoot = path.join(root, 'project'); const home = path.join(root, 'home');
  await mkdir(path.join(packageRoot, 'xiaotao', 'scripts'), { recursive: true }); await mkdir(projectRoot, { recursive: true });
  await writeFile(path.join(packageRoot, 'package.json'), JSON.stringify({ name: '@wxfly/xiaotao', version: '0.3.0' }));
  await writeFile(path.join(packageRoot, 'xiaotao', 'SKILL.md'), '# XiaoTao\n');
  await writeFile(path.join(packageRoot, 'xiaotao', 'scripts', 'timeline_catalog.py'), [
    'import json',
    "print(json.dumps({'timezone': 'UTC', 'occurred_at': '2026-11-14T00:00:00Z', 'date': '2026-11-14'}))",
  ].join('\n') + '\n');
  return { root, packageRoot, projectRoot, environment: { XIAOTAO_HOME: home } };
}

test('installs once at user level and writes project metadata', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const result = await installScenes({ ...context, sceneIds: ['skills'] });
  const target = path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao');
  assert.equal(await readFile(path.join(target, 'SKILL.md'), 'utf8'), '# XiaoTao\n');
  assert.deepEqual((await readInstallation(context.projectRoot)).scenes, ['skills']);
  assert.equal(result.checks.every((check) => check.ok), true, JSON.stringify(result.checks));
});

test('a second project reuses the same global installation', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  await installScenes({ ...context, sceneIds: ['skills'] });
  const secondProject = path.join(context.root, 'second-project'); await mkdir(secondProject);
  await installScenes({ packageRoot: context.packageRoot, projectRoot: secondProject, sceneIds: ['skills'], environment: context.environment });
  assert.equal((await readInstallation(secondProject)).targets.skills.path, path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao'));
});

test('update restores managed files and preserves unrelated files', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  await installScenes({ ...context, sceneIds: ['skills'] });
  const target = path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao');
  await writeFile(path.join(target, 'notes.md'), 'keep me\n'); await writeFile(path.join(context.packageRoot, 'xiaotao', 'SKILL.md'), '# Updated\n');
  await updateScenes(context);
  assert.equal(await readFile(path.join(target, 'SKILL.md'), 'utf8'), '# Updated\n');
  assert.equal(await readFile(path.join(target, 'notes.md'), 'utf8'), 'keep me\n');
});

test('refuses to overwrite an unmanaged user-level directory without force', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const target = path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao'); await mkdir(target, { recursive: true }); await writeFile(path.join(target, 'SKILL.md'), '# User skill\n');
  await assert.rejects(installScenes({ ...context, sceneIds: ['skills'] }), /not managed by XiaoTao/);
  await installScenes({ ...context, sceneIds: ['skills'], force: true });
  assert.equal(await readFile(path.join(target, 'SKILL.md'), 'utf8'), '# XiaoTao\n');
});

test('doctor reports a deleted global skill and version changes', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  await installScenes({ ...context, sceneIds: ['skills'] });
  await writeFile(path.join(context.packageRoot, 'package.json'), JSON.stringify({ name: '@wxfly/xiaotao', version: '0.3.1' }));
  assert.deepEqual(await globalUpdateNotice(context), { from: '0.3.0', to: '0.3.1' });
  await rm(path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao/SKILL.md'));
  assert.equal((await doctorInstallation(context.projectRoot, context.environment)).ok, false);
});

test('doctor runs the installed memory command read-only and reports failures', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  await installScenes({ ...context, sceneIds: ['skills'] });
  const diagnosis = await doctorInstallation(context.projectRoot, context.environment);
  assert.equal(diagnosis.checks.find((check) => check.code === 'memory_runtime_ready').ok, true);

  const target = path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao');
  await writeFile(path.join(target, 'scripts', 'timeline_catalog.py'), "raise SystemExit('broken memory command')\n");
  const broken = await doctorInstallation(context.projectRoot, context.environment);
  const failure = broken.checks.find((check) => check.code === 'memory_runtime_invalid');
  assert.equal(broken.ok, false);
  assert.match(failure.message, /broken memory command/);

  await rm(path.join(target, 'scripts', 'timeline_catalog.py'));
  const missing = await doctorInstallation(context.projectRoot, context.environment);
  assert.match(missing.checks.find((check) => check.code === 'memory_runtime_invalid').message, /is missing/);
});

test('doctor explains a missing Python executable', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const command = async () => {
    const error = new Error('spawn python ENOENT');
    error.code = 'ENOENT';
    throw error;
  };
  const result = await installScenes({ ...context, sceneIds: ['skills'], command });
  const failure = result.checks.find((check) => check.code === 'memory_runtime_invalid');
  assert.equal(failure.ok, false);
  assert.match(failure.message, /XIAOTAO_PYTHON/);
});

test('none does not erase a previous shared installation record', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  await installScenes({ ...context, sceneIds: ['skills'] });
  const before = await readGlobalInstallation(context.environment);
  const secondProject = path.join(context.root, 'none-project'); await mkdir(secondProject);
  await installScenes({ packageRoot: context.packageRoot, projectRoot: secondProject, sceneIds: [], environment: context.environment });
  assert.deepEqual(await readGlobalInstallation(context.environment), before);
});

test('reads BOM metadata and gives old metadata a migration message', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const configDir = path.join(context.projectRoot, '.xiaotao'); await mkdir(configDir);
  await writeFile(path.join(configDir, 'installation.json'), `\uFEFF${JSON.stringify({ schema_version: 1, tools: ['codex'] })}`);
  const diagnosis = await doctorInstallation(context.projectRoot, context.environment);
  assert.match(diagnosis.checks.at(-1).message, /Run xiaotao init again/);
  await assert.rejects(updateScenes(context), /old XiaoTao installation metadata/);
});

test('installs and verifies DSH through an injected command runner', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const script = path.join(context.packageRoot, 'adapters/deepseek-harness/scripts'); await mkdir(script, { recursive: true }); await writeFile(path.join(script, 'install-local.mjs'), '');
  const profile = path.join(context.environment.XIAOTAO_HOME, '.dsh', 'profiles', 'web');
  const adapter = path.join(profile, 'node_modules', '@xiaotao-ai', 'dsh-adapter');
  await mkdir(path.join(adapter, 'lib'), { recursive: true });
  await mkdir(path.join(adapter, 'lib', 'core', 'scripts'), { recursive: true });
  await writeFile(path.join(adapter, 'package.json'), JSON.stringify({ name: '@xiaotao-ai/dsh-adapter', main: 'lib/index.js' }));
  await writeFile(path.join(adapter, 'lib', 'index.js'), 'export default {};\n');
  await writeFile(path.join(adapter, 'lib', 'core', 'SKILL.md'), '# XiaoTao\n');
  await writeFile(path.join(adapter, 'lib', 'core', 'scripts', 'timeline_catalog.py'), [
    'import json',
    "print(json.dumps({'timezone': 'UTC', 'occurred_at': '2026-11-14T00:00:00Z', 'date': '2026-11-14'}))",
  ].join('\n') + '\n');
  const calls = [];
  const command = async (name, args) => {
    calls.push([name, args]);
    const stdout = args.includes('context')
      ? JSON.stringify({ timezone: 'UTC', occurred_at: '2026-11-14T00:00:00Z', date: '2026-11-14' })
      : name === 'dsh' ? '@xiaotao-ai/dsh-adapter' : '';
    return { stdout, stderr: '' };
  };
  const result = await installScenes({ ...context, sceneIds: ['dsh'], command });
  assert.equal(result.checks.every((check) => check.ok), true, JSON.stringify(result.checks));
  assert.deepEqual(calls.map(([name]) => name), [process.execPath, 'dsh', process.platform === 'win32' ? 'python' : 'python3']);
});

test('installs and verifies Antigravity plugin and skill', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const agSource = path.join(context.packageRoot, 'adapters/antigravity/xiaotao-antigravity');
  await mkdir(agSource, { recursive: true });
  await writeFile(path.join(agSource, 'plugin.json'), '{"name":"xiaotao-antigravity"}');
  await writeFile(path.join(agSource, 'hooks.json'), '{"PreInvocation":[]}');
  const result = await installScenes({ ...context, sceneIds: ['antigravity'] });
  assert.equal(result.checks.every((check) => check.ok), true);
  const target = path.join(context.environment.XIAOTAO_HOME, '.gemini/config/plugins/xiaotao-antigravity');
  assert.equal(await readFile(path.join(target, 'skills/xiaotao/SKILL.md'), 'utf8'), '# XiaoTao\n');
  assert.equal(await readFile(path.join(target, 'plugin.json'), 'utf8'), '{"name":"xiaotao-antigravity"}');
});

test('codex install writes Skill, metadata and an independent CLI marketplace', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const source = path.join(context.packageRoot, 'adapters/codex/xiaotao-codex');
  for (const file of [
    '.codex-plugin/plugin.json',
    'hooks/hooks.json',
    'scripts/session-start.mjs',
    'scripts/progress-check.mjs',
    'scripts/handoff-contract.mjs',
    'scripts/subagent-start.mjs',
    'scripts/delegation-contract.mjs',
    'scripts/worker-snapshot-contract.mjs',
  ]) {
    await mkdir(path.dirname(path.join(source, file)), { recursive: true });
    await writeFile(path.join(source, file), file.endsWith('.json') ? '{"name":"xiaotao-codex","version":"0.6.0"}' : '// fixture');
  }
  const result = await installCodex({ ...context, prepare: true });
  assert.equal(result.installed, false);
  assert.equal(result.pluginId, 'xiaotao-codex@xiaotao');
  assert.equal((await readInstallation(context.projectRoot)).scenes.includes('skills'), true);
  const catalog = JSON.parse(await readFile(path.join(context.environment.XIAOTAO_HOME, '.xiaotao/plugins/codex-marketplace/.agents/plugins/marketplace.json'), 'utf8'));
  assert.equal(catalog.name, 'xiaotao');
  assert.equal(catalog.plugins[0].source.path, './plugins/xiaotao-codex');
});

test('codex install invokes the CLI marketplace and plugin add flow', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const source = path.join(context.packageRoot, 'adapters/codex/xiaotao-codex');
  for (const file of [
    '.codex-plugin/plugin.json',
    'hooks/hooks.json',
    'scripts/session-start.mjs',
    'scripts/progress-check.mjs',
    'scripts/handoff-contract.mjs',
    'scripts/subagent-start.mjs',
    'scripts/delegation-contract.mjs',
    'scripts/worker-snapshot-contract.mjs',
  ]) {
    await mkdir(path.dirname(path.join(source, file)), { recursive: true });
    await writeFile(path.join(source, file), file.endsWith('.json') ? '{"name":"xiaotao-codex","version":"0.6.1"}' : '// fixture');
  }
  const calls = [];
  const command = async (name, args) => {
    calls.push([name, args]);
    const stdout = args.includes('context')
      ? JSON.stringify({ timezone: 'UTC', occurred_at: '2026-11-14T00:00:00Z', date: '2026-11-14' })
      : args[1] === 'add' ? JSON.stringify({ pluginId: 'xiaotao-codex@xiaotao' }) : '{}';
    return { stdout, stderr: '' };
  };
  const result = await installCodex({ ...context, command });
  assert.equal(result.installed, true);
  assert.equal(result.ok, true);
  assert.deepEqual(calls.map(([name]) => name), [process.platform === 'win32' ? 'python' : 'python3', 'codex', 'codex']);
  assert.equal(calls[0][1][0], '-B');
  assert.deepEqual(calls[1][1].slice(0, 3), ['plugin', 'marketplace', 'add']);
  assert.deepEqual(calls[2][1], ['plugin', 'add', 'xiaotao-codex@xiaotao', '--json']);
});

