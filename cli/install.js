import { cp, mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import path from 'node:path';

import { HOSTS } from './hosts.js';

const CONFIG_PATH = '.xiaotao/installation.json';
const MARKER_NAME = '.xiaotao-managed.json';
const MANAGED_PACKAGE = 'xiaotao-ai-workflow';

function toPortablePath(value) {
  return value.split(path.sep).join('/');
}

async function pathExists(target) {
  try {
    await stat(target);
    return true;
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
}

async function readJson(target) {
  return JSON.parse(await readFile(target, 'utf8'));
}

async function isNonEmpty(target) {
  try {
    const targetStat = await stat(target);
    if (!targetStat.isDirectory()) return true;
    return (await readdir(target)).length > 0;
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
}

async function isManaged(target) {
  try {
    const marker = await readJson(path.join(target, MARKER_NAME));
    return marker.package === MANAGED_PACKAGE;
  } catch (error) {
    if (error.code === 'ENOENT' || error instanceof SyntaxError) return false;
    throw error;
  }
}

async function packageInfo(packageRoot) {
  const manifest = await readJson(path.join(packageRoot, 'package.json'));
  if (manifest.name !== MANAGED_PACKAGE || typeof manifest.version !== 'string') {
    throw new Error(`Invalid XiaoTao package metadata at ${packageRoot}`);
  }
  const skillSource = path.join(packageRoot, 'xiaotao');
  if (!(await pathExists(path.join(skillSource, 'SKILL.md')))) {
    throw new Error(`XiaoTao Skill source is missing: ${path.join(skillSource, 'SKILL.md')}`);
  }
  return { version: manifest.version, skillSource };
}

export async function readInstallation(projectRoot) {
  return readJson(path.join(path.resolve(projectRoot), CONFIG_PATH));
}

export async function installHosts({
  projectRoot,
  packageRoot,
  toolIds,
  force = false,
}) {
  const resolvedProject = path.resolve(projectRoot);
  const resolvedPackage = path.resolve(packageRoot);
  const uniqueTools = [...new Set(toolIds)];
  for (const toolId of uniqueTools) {
    if (!HOSTS[toolId]) throw new Error(`Unknown AI tool: ${toolId}`);
  }

  const { version, skillSource } = await packageInfo(resolvedPackage);
  const targets = uniqueTools.map((toolId) => ({
    toolId,
    target: path.join(resolvedProject, ...HOSTS[toolId].skillDir.split('/')),
  }));

  for (const { target } of targets) {
    if ((await isNonEmpty(target)) && !(await isManaged(target)) && !force) {
      throw new Error(`Destination is not managed by XiaoTao: ${target}. Re-run with --force to adopt it.`);
    }
  }

  const now = new Date().toISOString();
  for (const { toolId, target } of targets) {
    await mkdir(target, { recursive: true });
    await cp(skillSource, target, { recursive: true, force: true });
    await writeFile(
      path.join(target, MARKER_NAME),
      `${JSON.stringify({ package: MANAGED_PACKAGE, tool: toolId, version }, null, 2)}\n`,
    );
  }

  let previous = null;
  try {
    previous = await readInstallation(resolvedProject);
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  const tools = [...new Set([...(previous?.tools ?? []), ...uniqueTools])]
    .filter((toolId) => HOSTS[toolId]);
  const metadata = {
    schema_version: 1,
    package: MANAGED_PACKAGE,
    package_version: version,
    tools,
    destinations: Object.fromEntries(tools.map((toolId) => [toolId, HOSTS[toolId].skillDir])),
    installed_at: previous?.installed_at ?? now,
    updated_at: now,
  };
  const configTarget = path.join(resolvedProject, CONFIG_PATH);
  await mkdir(path.dirname(configTarget), { recursive: true });
  await writeFile(configTarget, `${JSON.stringify(metadata, null, 2)}\n`);

  return metadata;
}

export async function updateHosts({ projectRoot, packageRoot }) {
  const metadata = await readInstallation(projectRoot);
  if (!Array.isArray(metadata.tools)) {
    throw new Error(`Invalid XiaoTao installation metadata: tools must be an array`);
  }
  return installHosts({ projectRoot, packageRoot, toolIds: metadata.tools });
}

export async function doctorInstallation(projectRoot) {
  const resolvedProject = path.resolve(projectRoot);
  const checks = [];
  let metadata;
  try {
    metadata = await readInstallation(resolvedProject);
    checks.push({ code: 'config_present', ok: true, path: toPortablePath(CONFIG_PATH) });
  } catch (error) {
    checks.push({
      code: 'config_missing',
      ok: false,
      path: toPortablePath(CONFIG_PATH),
      message: error.code === 'ENOENT' ? 'Run xiaotao init first.' : error.message,
    });
    return { ok: false, checks };
  }

  if (!Array.isArray(metadata.tools)) {
    checks.push({ code: 'config_invalid', ok: false, message: 'tools must be an array' });
    return { ok: false, checks };
  }

  for (const toolId of metadata.tools) {
    const host = HOSTS[toolId];
    if (!host) {
      checks.push({ code: 'tool_unknown', ok: false, tool: toolId });
      continue;
    }
    const target = path.join(resolvedProject, ...host.skillDir.split('/'));
    const skillPath = path.join(target, 'SKILL.md');
    checks.push({
      code: (await pathExists(skillPath)) ? 'skill_present' : 'skill_missing',
      ok: await pathExists(skillPath),
      tool: toolId,
      path: toPortablePath(path.relative(resolvedProject, skillPath)),
    });
    checks.push({
      code: (await isManaged(target)) ? 'marker_present' : 'marker_missing',
      ok: await isManaged(target),
      tool: toolId,
      path: toPortablePath(path.relative(resolvedProject, path.join(target, MARKER_NAME))),
    });
  }

  return { ok: checks.every((check) => check.ok), checks };
}
