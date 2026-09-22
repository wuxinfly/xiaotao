import { cp, mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import path from 'node:path';
import { promisify } from 'node:util';

import { resolveUserHome, SCENES } from './scenes.js';

const execFileAsync = promisify(execFile);
const CONFIG_PATH = '.xiaotao/installation.json';
const GLOBAL_CONFIG = '.xiaotao/managed-installation.json';
const MARKER_NAME = '.xiaotao-managed.json';
const MANAGED_PACKAGE = 'xiaotao-ai-workflow';

async function pathExists(target) {
  try { await stat(target); return true; } catch (error) { if (error.code === 'ENOENT') return false; throw error; }
}
async function readJson(target) { return JSON.parse(await readFile(target, 'utf8')); }
async function isNonEmpty(target) {
  try { const value = await stat(target); return !value.isDirectory() || (await readdir(target)).length > 0; }
  catch (error) { if (error.code === 'ENOENT') return false; throw error; }
}
async function isManaged(target) {
  try { return (await readJson(path.join(target, MARKER_NAME))).package === MANAGED_PACKAGE; }
  catch (error) { if (error.code === 'ENOENT' || error instanceof SyntaxError) return false; throw error; }
}
async function packageInfo(packageRoot) {
  const manifest = await readJson(path.join(packageRoot, 'package.json'));
  const skillSource = path.join(packageRoot, 'xiaotao');
  if (manifest.name !== MANAGED_PACKAGE || typeof manifest.version !== 'string' || !(await pathExists(path.join(skillSource, 'SKILL.md')))) {
    throw new Error(`Invalid XiaoTao package at ${packageRoot}`);
  }
  return { version: manifest.version, skillSource };
}
function projectConfig(projectRoot) { return path.join(path.resolve(projectRoot), CONFIG_PATH); }
function globalConfig(environment) { return path.join(resolveUserHome(environment), GLOBAL_CONFIG); }

export async function readInstallation(projectRoot) { return readJson(projectConfig(projectRoot)); }
export async function readGlobalInstallation(environment = process.env) {
  try { return await readJson(globalConfig(environment)); } catch (error) { if (error.code === 'ENOENT') return null; throw error; }
}
export async function globalUpdateNotice({ packageRoot, environment = process.env }) {
  const current = await packageInfo(path.resolve(packageRoot));
  const previous = await readGlobalInstallation(environment);
  return previous?.package_version && previous.package_version !== current.version
    ? { from: previous.package_version, to: current.version } : null;
}

async function installDsh({ packageRoot, target, environment }) {
  const script = path.join(packageRoot, 'adapters', 'deepseek-harness', 'scripts', 'install-local.mjs');
  if (!(await pathExists(script))) throw new Error('DSH adapter installer is missing from this checkout');
  await execFileAsync(process.execPath, [script, '--profile', target.profile, '--dsh-home', target.dsh_home], {
    cwd: packageRoot, env: { ...environment, DSH_HOME: target.dsh_home }, windowsHide: true,
  });
}
async function verifyDsh(target, environment) {
  try {
    const { stdout, stderr } = await execFileAsync('dsh', ['--profile', target.profile, '--dump-config'], {
      cwd: path.join(target.dsh_home, 'profiles', target.profile), env: { ...environment, DSH_HOME: target.dsh_home }, windowsHide: true,
    });
    return `${stdout}\n${stderr}`.includes('@xiaotao-ai/dsh-adapter') || `${stdout}\n${stderr}`.includes('xiaotao-adapter');
  } catch { return false; }
}

export async function installScenes({ projectRoot, packageRoot, sceneIds, force = false, environment = process.env }) {
  const resolvedPackage = path.resolve(packageRoot);
  const { version, skillSource } = await packageInfo(resolvedPackage);
  const scenes = [...new Set(sceneIds)];
  for (const id of scenes) if (!SCENES[id]) throw new Error(`Unknown installation target: ${id}`);
  const targets = Object.fromEntries(scenes.map((id) => [id, SCENES[id].resolveTarget(environment)]));

  for (const id of scenes.filter((scene) => scene !== 'dsh')) {
    const target = targets[id].path;
    if ((await isNonEmpty(target)) && !(await isManaged(target)) && !force) {
      throw new Error(`Destination is not managed by XiaoTao: ${target}. Re-run init with --force to adopt it.`);
    }
  }
  for (const id of scenes) {
    const target = targets[id];
    if (id === 'dsh') { await installDsh({ packageRoot: resolvedPackage, target, environment }); continue; }
    await mkdir(target.path, { recursive: true });
    await cp(skillSource, target.path, { recursive: true, force: true });
    await writeFile(path.join(target.path, MARKER_NAME), `${JSON.stringify({ package: MANAGED_PACKAGE, scene: id, version }, null, 2)}\n`);
  }

  const now = new Date().toISOString();
  let previous = null;
  try { previous = await readInstallation(projectRoot); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  const metadata = { schema_version: 2, package: MANAGED_PACKAGE, package_version: version, scenes, targets, installed_at: previous?.installed_at ?? now, updated_at: now };
  await mkdir(path.dirname(projectConfig(projectRoot)), { recursive: true });
  await writeFile(projectConfig(projectRoot), `${JSON.stringify(metadata, null, 2)}\n`);
  const global = { schema_version: 1, package: MANAGED_PACKAGE, package_version: version, scenes, targets, updated_at: now };
  await mkdir(path.dirname(globalConfig(environment)), { recursive: true });
  await writeFile(globalConfig(environment), `${JSON.stringify(global, null, 2)}\n`);
  return { ...metadata, checks: (await doctorInstallation(projectRoot, environment)).checks };
}

export async function updateScenes({ projectRoot, packageRoot, environment = process.env }) {
  const metadata = await readInstallation(projectRoot);
  if (metadata.schema_version !== 2 || !Array.isArray(metadata.scenes)) throw new Error('This project uses old XiaoTao installation metadata. Run xiaotao init again.');
  return installScenes({ projectRoot, packageRoot, sceneIds: metadata.scenes, environment });
}

export async function doctorInstallation(projectRoot, environment = process.env) {
  const checks = [];
  let metadata;
  try { metadata = await readInstallation(projectRoot); checks.push({ code: 'config_present', ok: true, path: CONFIG_PATH }); }
  catch (error) { return { ok: false, checks: [{ code: 'config_missing', ok: false, path: CONFIG_PATH, message: error.code === 'ENOENT' ? 'Run xiaotao init first.' : error.message }] }; }
  if (metadata.schema_version !== 2 || !Array.isArray(metadata.scenes) || !metadata.targets) return { ok: false, checks: [...checks, { code: 'config_invalid', ok: false }] };
  for (const id of metadata.scenes) {
    const target = metadata.targets[id];
    if (!SCENES[id] || !target) { checks.push({ code: 'scene_unknown', ok: false, scene: id }); continue; }
    if (id === 'dsh') { const ok = await verifyDsh(target, environment); checks.push({ code: ok ? 'dsh_active' : 'dsh_inactive', ok, scene: id }); continue; }
    const skill = path.join(target.path, 'SKILL.md');
    const skillOk = await pathExists(skill); const markerOk = await isManaged(target.path);
    checks.push({ code: skillOk ? 'skill_present' : 'skill_missing', ok: skillOk, scene: id, path: skill });
    checks.push({ code: markerOk ? 'marker_present' : 'marker_missing', ok: markerOk, scene: id, path: path.join(target.path, MARKER_NAME) });
  }
  return { ok: checks.every((check) => check.ok), checks };
}
