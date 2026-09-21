import { createHash, randomUUID } from 'node:crypto';
import { lstat, mkdir, open, readFile, rename, unlink } from 'node:fs/promises';
import { homedir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SOURCE = fileURLToPath(new URL('./xiaotao-codex/', import.meta.url));
const NAME = 'xiaotao-codex';
const OWNER = 'xiaotao-ai-workflow/codex-adapter';
const FILES = ['.codex-plugin/plugin.json', 'hooks/hooks.json', 'scripts/session-start.mjs'];
const ENTRY_PATH = `./.codex/plugins/${NAME}`;
const MARKER = '.xiaotao-source.json';

async function optionalText(file) {
  try { return await readFile(file, 'utf8'); }
  catch (error) { if (error.code === 'ENOENT') return null; throw error; }
}

async function optionalStat(file) {
  try { return await lstat(file); }
  catch (error) { if (error.code === 'ENOENT') return null; throw error; }
}

async function directory(target) {
  const info = await optionalStat(target);
  if (info && (!info.isDirectory() || info.isSymbolicLink())) {
    throw new Error(`Expected a real directory: ${target}`);
  }
  await mkdir(target, { recursive: true });
}

async function atomicText(file, content) {
  const temporary = `${file}.tmp-${randomUUID()}`;
  const handle = await open(temporary, 'wx');
  try {
    await handle.writeFile(content);
    await handle.close();
    await rename(temporary, file);
  } finally {
    await handle.close();
    await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw error; });
  }
}

// Prepares a personal marketplace source. Activation and hook trust stay with Codex.
export async function installLocal({ homeDir = homedir(), sourceDir = SOURCE } = {}) {
  const codexDir = path.join(homeDir, '.codex');
  const pluginRoot = path.join(codexDir, 'plugins');
  const pluginDir = path.join(pluginRoot, NAME);
  const marketplace = path.join(homeDir, '.agents/plugins/marketplace.json');
  const sourceFiles = await Promise.all(FILES.map(file => readFile(path.join(sourceDir, file), 'utf8')));
  const manifest = JSON.parse(sourceFiles[0]);
  if (manifest.name !== NAME) throw new Error('Unexpected plugin identity');
  // Changing the source changes the installable version; do not edit installed caches.
  const digest = createHash('sha256').update(JSON.stringify(sourceFiles)).digest('hex').slice(0, 16);
  manifest.version = `${manifest.version.split('+')[0]}+codex.${digest}`;
  sourceFiles[0] = `${JSON.stringify(manifest, null, 2)}\n`;

  for (const dir of [homeDir, path.join(homeDir, '.agents'), path.dirname(marketplace)]) {
    await directory(dir);
  }
  const lockPath = path.join(path.dirname(marketplace), '.xiaotao-codex-install.lock');
  const lock = await open(lockPath, 'wx');
  try {
    const catalogStat = await optionalStat(marketplace);
    if (catalogStat && (!catalogStat.isFile() || catalogStat.isSymbolicLink())) {
      throw new Error('Personal marketplace must be a regular file');
    }
    const before = await optionalText(marketplace);
    const catalog = before === null ? {
      name: 'personal', interface: { displayName: 'Personal' }, plugins: [],
    } : JSON.parse(before);
    if (!catalog || typeof catalog.name !== 'string' || !/^[A-Za-z0-9_-]+$/.test(catalog.name)
      || !Array.isArray(catalog.plugins)) throw new Error('Invalid personal marketplace');
    const matches = catalog.plugins.filter(entry => entry?.name === NAME);
    if (matches.length > 1 || (matches.length === 1
      && (matches[0].source?.source !== 'local' || matches[0].source?.path !== ENTRY_PATH))) {
      throw new Error('Existing xiaotao-codex marketplace entry points elsewhere; left unchanged');
    }
    await directory(codexDir);
    await directory(pluginRoot);
    const targetStat = await optionalStat(pluginDir);
    if (targetStat) {
      if (!targetStat.isDirectory() || targetStat.isSymbolicLink()) throw new Error('Invalid plugin destination');
      const marker = JSON.parse(await optionalText(path.join(pluginDir, MARKER)) || 'null');
      if (marker?.owner !== OWNER) throw new Error('Plugin destination is not managed by XiaoTao');
    }
    await directory(pluginDir);
    for (const dir of ['.codex-plugin', 'hooks', 'scripts']) await directory(path.join(pluginDir, dir));
    for (let i = 0; i < FILES.length; i++) await atomicText(path.join(pluginDir, FILES[i]), sourceFiles[i]);
    await atomicText(path.join(pluginDir, MARKER), `${JSON.stringify({ owner: OWNER })}\n`);
    if (matches.length === 0) {
      catalog.plugins.push({
        name: NAME, source: { source: 'local', path: ENTRY_PATH },
        policy: { installation: 'AVAILABLE', authentication: 'ON_INSTALL' }, category: 'Productivity',
      });
      if (await optionalText(marketplace) !== before) {
        throw new Error('Marketplace changed during preparation; retry after reviewing it');
      }
      await atomicText(marketplace, `${JSON.stringify(catalog, null, 2)}\n`);
    }
    return { pluginDir, marketplace, marketplaceName: catalog.name, version: manifest.version };
  } finally {
    await lock.close();
    await unlink(lockPath);
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    if (process.argv.length !== 2) throw new Error('Usage: node adapters/codex/install-local.mjs');
    const result = await installLocal();
    console.log(`Prepared XiaoTao Codex ${result.version}\nSource: ${result.pluginDir}\nMarketplace: ${result.marketplace}`);
    console.log('Refresh the desktop app and install/reinstall xiaotao-codex from your personal marketplace.');
    console.log('Review and trust its SessionStart hook in Codex, then start a new conversation.');
    console.log('Projects must separately have XiaoTao installed with: node ./bin/xiaotao.js init <project> --tools codex');
    console.log('This command does not activate the plugin, grant hook trust, or enable automatic checkpoints.');
  } catch (error) { console.error(`XiaoTao Codex: ${error.message}`); process.exitCode = 1; }
}
