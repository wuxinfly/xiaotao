import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { doctorInstallation, globalUpdateNotice, installScenes, readInstallation, updateScenes } from '../cli/install.js';

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-install-test-'));
  const packageRoot = path.join(root, 'package'); const projectRoot = path.join(root, 'project'); const home = path.join(root, 'home');
  await mkdir(path.join(packageRoot, 'xiaotao'), { recursive: true }); await mkdir(projectRoot, { recursive: true });
  await writeFile(path.join(packageRoot, 'package.json'), JSON.stringify({ name: 'xiaotao-ai-workflow', version: '0.3.0' }));
  await writeFile(path.join(packageRoot, 'xiaotao', 'SKILL.md'), '# XiaoTao\n');
  return { root, packageRoot, projectRoot, environment: { XIAOTAO_HOME: home } };
}

test('installs once at user level and writes project metadata', async (t) => {
  const context = await fixture(); t.after(() => rm(context.root, { recursive: true, force: true }));
  const result = await installScenes({ ...context, sceneIds: ['skills'] });
  const target = path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao');
  assert.equal(await readFile(path.join(target, 'SKILL.md'), 'utf8'), '# XiaoTao\n');
  assert.deepEqual((await readInstallation(context.projectRoot)).scenes, ['skills']);
  assert.equal(result.checks.every((check) => check.ok), true);
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
  await writeFile(path.join(context.packageRoot, 'package.json'), JSON.stringify({ name: 'xiaotao-ai-workflow', version: '0.3.1' }));
  assert.deepEqual(await globalUpdateNotice(context), { from: '0.3.0', to: '0.3.1' });
  await rm(path.join(context.environment.XIAOTAO_HOME, '.agents/skills/xiaotao/SKILL.md'));
  assert.equal((await doctorInstallation(context.projectRoot, context.environment)).ok, false);
});
