import { cp, lstat, mkdir, open, readFile, rename, unlink, writeFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ADAPTER_SOURCE = fileURLToPath(new URL('./xiaotao-antigravity/', import.meta.url));
const CORE_SOURCE = fileURLToPath(new URL('../../xiaotao/', import.meta.url));
const PLUGIN_NAME = 'xiaotao-antigravity';
const OWNER = 'xiaotao-ai-workflow/antigravity-adapter';
const MARKER = '.xiaotao-source.json';

async function optionalStat(target) {
  try {
    return await lstat(target);
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  }
}

async function directory(target) {
  const info = await optionalStat(target);
  if (info && (!info.isDirectory() || info.isSymbolicLink())) {
    throw new Error(`Expected a real directory: ${target}`);
  }
  await mkdir(target, { recursive: true });
}

export async function installLocal({
  homeDir = process.env.XIAOTAO_HOME || homedir(),
  sourceDir = ADAPTER_SOURCE,
  coreDir = CORE_SOURCE,
  force = false,
} = {}) {
  const geminiDir = process.env.GEMINI_CONFIG_DIR || path.join(homeDir, '.gemini', 'config');
  const pluginsDir = path.join(geminiDir, 'plugins');
  const targetDir = path.join(pluginsDir, PLUGIN_NAME);

  await directory(geminiDir);
  await directory(pluginsDir);

  const existing = await optionalStat(targetDir);
  if (existing) {
    if (!existing.isDirectory() || existing.isSymbolicLink()) {
      throw new Error(`Plugin destination is invalid: ${targetDir}`);
    }
    const markerPath = path.join(targetDir, MARKER);
    const markerStat = await optionalStat(markerPath);
    if (!markerStat && !force) {
      throw new Error(`Destination directory exists and is not managed by XiaoTao: ${targetDir}. Use force to overwrite.`);
    }
  }

  await directory(targetDir);
  await directory(path.join(targetDir, 'scripts'));
  await directory(path.join(targetDir, 'skills', 'xiaotao'));

  // 1. Copy plugin manifest and hooks
  await cp(path.join(sourceDir, 'plugin.json'), path.join(targetDir, 'plugin.json'), { force: true });
  await cp(path.join(sourceDir, 'hooks.json'), path.join(targetDir, 'hooks.json'), { force: true });
  await cp(path.join(sourceDir, 'scripts'), path.join(targetDir, 'scripts'), { recursive: true, force: true });

  // 2. Copy Core Skill into plugin's skills/xiaotao directory
  await cp(coreDir, path.join(targetDir, 'skills', 'xiaotao'), { recursive: true, force: true });

  // 3. Write markers
  await writeFile(
    path.join(targetDir, MARKER),
    `${JSON.stringify({ owner: OWNER, plugin: PLUGIN_NAME, installed_at: new Date().toISOString() }, null, 2)}\n`
  );
  await writeFile(
    path.join(targetDir, '.xiaotao-managed.json'),
    `${JSON.stringify({ package: 'xiaotao-ai-workflow', scene: 'antigravity', version: '0.3.0' }, null, 2)}\n`
  );

  return { targetDir, pluginName: PLUGIN_NAME };
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  installLocal({ force: process.argv.includes('--force') })
    .then((result) => {
      process.stdout.write(`Installed XiaoTao Antigravity adapter to ${result.targetDir}\n`);
    })
    .catch((error) => {
      process.stderr.write(`Installation failed: ${error.message}\n`);
      process.exit(1);
    });
}
