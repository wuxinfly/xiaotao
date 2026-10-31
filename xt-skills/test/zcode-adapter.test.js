import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { installScenes, updateScenes, doctorInstallation } from '../cli/install.js';
import { handle } from '../adapters/zcode/plugin/hooks/context.mjs';
import { detectScenes } from '../cli/scenes.js';
const packageRoot = path.resolve(import.meta.dirname, '..');
async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-zcode-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const projectRoot = path.join(root, 'project'); await mkdir(projectRoot);
  return { root, projectRoot, packageRoot, environment: { XIAOTAO_HOME: path.join(root, 'home') } };
}
test('ZCode prepares a self-contained marketplace, updates, and detects missing hooks', async t => {
  const c = await fixture(t);
  const result = await installScenes({ ...c, sceneIds: ['zcode'] });
  assert.ok(result.checks.every(x => x.ok));
  const target = result.targets.zcode.path;
  const market = JSON.parse(await readFile(path.join(target, 'marketplace.json'), 'utf8'));
  const plugin = path.resolve(target, market.plugins[0].source);
  assert.equal(JSON.parse(await readFile(path.join(plugin, '.zcode-plugin/plugin.json'), 'utf8')).version, market.plugins[0].version);
  const hooks = JSON.parse(await readFile(path.join(plugin, 'hooks/hooks.json'), 'utf8'));
  assert.deepEqual(Object.keys(hooks.hooks), ['SessionStart', 'UserPromptSubmit', 'Stop']);
  assert.ok((await readFile(path.join(plugin, 'skills/xiaotao/scripts/save_progress.py'), 'utf8')).length);
  const child = spawnSync(process.execPath, [path.join(plugin, 'hooks/context.mjs')], { input: JSON.stringify({ cwd: c.projectRoot, hook_event_name: 'SessionStart' }), encoding: 'utf8' });
  assert.equal(child.status, 0, child.stderr);
  assert.match(JSON.parse(child.stdout).hookSpecificOutput.additionalContext, /save_progress.py/);
  await rm(path.join(plugin, 'hooks/context.mjs'));
  assert.equal((await doctorInstallation(c.projectRoot, c.environment)).ok, false);
  await updateScenes(c);
  assert.equal((await doctorInstallation(c.projectRoot, c.environment)).ok, true);
  assert.ok((await doctorInstallation(c.projectRoot, c.environment)).checks.some(x => x.code === 'zcode_activation_unverified'));
});
test('ZCode hooks skip ordinary projects, prompt for progress once, and fail open on malformed input', async t => {
  const c = await fixture(t);
  assert.deepEqual(await handle({cwd: c.projectRoot, hook_event_name:'Stop'}), {});
  await installScenes({...c, sceneIds:['zcode']});
  assert.equal((await handle({cwd:c.projectRoot, hook_event_name:'Stop'})).decision, 'block');
  assert.deepEqual(await handle({cwd:c.projectRoot, hook_event_name:'Stop', stop_hook_active:true}), {});
  assert.deepEqual(await handle({cwd:c.projectRoot, hookEventName:'Stop', stopHookActive:true}), {});
  assert.deepEqual(await handle({cwd:c.projectRoot, hook_event_name:'PreCompact'}), {});
  const result = spawnSync(process.execPath, [path.join(packageRoot, 'adapters/zcode/plugin/hooks/context.mjs')], {input:'bad json', encoding:'utf8'});
  assert.equal(result.status, 0); assert.deepEqual(JSON.parse(result.stdout), {});
});
test('ZCode install refuses unmanaged destination and is discoverable from user home', async t => {
  const c = await fixture(t);
  const target = path.join(c.environment.XIAOTAO_HOME, '.xiaotao/hosts/zcode');
  await mkdir(target, {recursive:true}); await writeFile(path.join(target, 'mine.txt'), 'keep');
  await assert.rejects(installScenes({...c, sceneIds:['zcode']}), /not managed/);
  assert.equal(await readFile(path.join(target, 'mine.txt'), 'utf8'), 'keep');
  await mkdir(path.join(c.environment.XIAOTAO_HOME, '.zcode'));
  assert.ok((await detectScenes(c.environment)).includes('zcode'));
});
