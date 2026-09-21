import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
  doctorInstallation,
  installHosts,
  readInstallation,
  updateHosts,
} from '../cli/install.js';

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-install-test-'));
  const packageRoot = path.join(root, 'package');
  const projectRoot = path.join(root, 'project');
  await mkdir(path.join(packageRoot, 'xiaotao', 'references'), { recursive: true });
  await mkdir(projectRoot, { recursive: true });
  await writeFile(
    path.join(packageRoot, 'package.json'),
    JSON.stringify({ name: 'xiaotao-ai-workflow', version: '0.1.0' }),
  );
  await writeFile(path.join(packageRoot, 'xiaotao', 'SKILL.md'), '# XiaoTao\n');
  await writeFile(path.join(packageRoot, 'xiaotao', 'references', 'memory.md'), '# Memory\n');
  return { root, packageRoot, projectRoot };
}

test('installs the portable skill for every MVP host and records ownership', async (t) => {
  const context = await fixture();
  t.after(() => rm(context.root, { recursive: true, force: true }));

  const result = await installHosts({
    projectRoot: context.projectRoot,
    packageRoot: context.packageRoot,
    toolIds: ['codex', 'claude', 'opencode'],
  });

  assert.deepEqual(result.tools, ['codex', 'claude', 'opencode']);
  for (const relativeDir of [
    '.agents/skills/xiaotao',
    '.claude/skills/xiaotao',
    '.opencode/skills/xiaotao',
  ]) {
    assert.equal(await readFile(path.join(context.projectRoot, relativeDir, 'SKILL.md'), 'utf8'), '# XiaoTao\n');
    const marker = JSON.parse(
      await readFile(path.join(context.projectRoot, relativeDir, '.xiaotao-managed.json'), 'utf8'),
    );
    assert.equal(marker.package, 'xiaotao-ai-workflow');
    assert.equal(marker.version, '0.1.0');
  }

  const metadata = await readInstallation(context.projectRoot);
  assert.equal(metadata.schema_version, 1);
  assert.deepEqual(metadata.tools, ['codex', 'claude', 'opencode']);
});

test('update refreshes managed files and preserves user-authored files', async (t) => {
  const context = await fixture();
  t.after(() => rm(context.root, { recursive: true, force: true }));

  await installHosts({
    projectRoot: context.projectRoot,
    packageRoot: context.packageRoot,
    toolIds: ['claude'],
  });
  const target = path.join(context.projectRoot, '.claude', 'skills', 'xiaotao');
  await writeFile(path.join(target, 'notes.md'), 'keep me\n');
  await writeFile(path.join(context.packageRoot, 'xiaotao', 'SKILL.md'), '# XiaoTao updated\n');

  await updateHosts({ projectRoot: context.projectRoot, packageRoot: context.packageRoot });

  assert.equal(await readFile(path.join(target, 'SKILL.md'), 'utf8'), '# XiaoTao updated\n');
  assert.equal(await readFile(path.join(target, 'notes.md'), 'utf8'), 'keep me\n');
});

test('refuses to overwrite an unmanaged destination without force', async (t) => {
  const context = await fixture();
  t.after(() => rm(context.root, { recursive: true, force: true }));
  const target = path.join(context.projectRoot, '.agents', 'skills', 'xiaotao');
  await mkdir(target, { recursive: true });
  await writeFile(path.join(target, 'SKILL.md'), '# User skill\n');

  await assert.rejects(
    installHosts({
      projectRoot: context.projectRoot,
      packageRoot: context.packageRoot,
      toolIds: ['codex'],
    }),
    /not managed by XiaoTao/,
  );
  assert.equal(await readFile(path.join(target, 'SKILL.md'), 'utf8'), '# User skill\n');
});

test('doctor reports healthy installs and missing managed files', async (t) => {
  const context = await fixture();
  t.after(() => rm(context.root, { recursive: true, force: true }));
  await installHosts({
    projectRoot: context.projectRoot,
    packageRoot: context.packageRoot,
    toolIds: ['opencode'],
  });

  assert.equal((await doctorInstallation(context.projectRoot)).ok, true);
  await rm(path.join(context.projectRoot, '.opencode', 'skills', 'xiaotao', 'SKILL.md'));
  const diagnosis = await doctorInstallation(context.projectRoot);
  assert.equal(diagnosis.ok, false);
  assert.ok(diagnosis.checks.some((check) => check.code === 'skill_missing' && !check.ok));
});
