import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { createInterface } from 'node:readline/promises';

import { detectScenes, SCENES } from './scenes.js';
import { doctorInstallation, globalUpdateNotice, installScenes, requireCurrentInstallation, updateScenes } from './install.js';

const PACKAGE_ROOT = path.resolve(import.meta.dirname, '..');
const HELP = `XiaoTao installer\n\nUsage:\n  xiaotao init [path] [--force]\n  xiaotao update [path] [--yes]\n  xiaotao doctor [path] [--json]\n\nRun init in a terminal. It detects available hosts and lets you choose.\n`;
class UsageError extends Error {}

function parseArgs(argv) {
  if (!argv.length || ['--help', '-h'].includes(argv[0])) return { kind: 'help' };
  if (['--version', '-V'].includes(argv[0])) return { kind: 'version' };
  if (!['init', 'update', 'doctor'].includes(argv[0])) throw new UsageError(`Unknown command: ${argv[0]}`);
  const options = { kind: 'command', command: argv[0], projectPath: '.', force: false, json: false, yes: false };
  let pathSeen = false;
  for (let index = 1; index < argv.length; index += 1) {
    const value = argv[index];
    if (value === '--force' && options.command === 'init') { options.force = true; continue; }
    if (value === '--yes' && options.command === 'update') { options.yes = true; continue; }
    if (value === '--json') {
      if (options.command !== 'doctor') throw new UsageError('--json is only valid with xiaotao doctor');
      options.json = true;
      continue;
    }
    if (value.startsWith('-')) throw new UsageError(`Unknown option: ${value}`);
    if (pathSeen) throw new UsageError(`Unexpected argument: ${value}`);
    options.projectPath = value;
    pathSeen = true;
  }
  return options;
}

async function ask(input, output, question) {
  const prompt = createInterface({ input, output });
  try { return (await prompt.question(question)).trim(); } finally { prompt.close(); }
}

async function chooseHosts(input, output, environment) {
  const detected = await detectScenes(environment);
  const ids = Object.keys(SCENES);
  output.write('检测到可用宿主：\n');
  ids.forEach((id, index) => output.write(`  ${index + 1}. ${SCENES[id].name}${detected.includes(id) ? '（可用）' : '（未检测到）'}\n`));
  const defaults = detected.length ? detected.join(',') : 'none';
  const answer = await ask(input, output, `请选择要安装的宿主（可多选，如 1,2；回车使用 ${defaults}）： `);
  if (!answer) return detected;
  if (answer.toLowerCase() === 'none') return [];
  const picked = [];
  for (const item of answer.split(',').map((value) => value.trim()).filter(Boolean)) {
    const id = /^\d+$/.test(item) ? ids[Number(item) - 1] : item.toLowerCase();
    if (!SCENES[id]) throw new UsageError(`Unknown host selection: ${item}`);
    if (!picked.includes(id)) picked.push(id);
  }
  return picked;
}

function printDiagnosis(output, diagnosis) {
  for (const check of diagnosis.checks) {
    output.write(`${check.ok ? 'PASS' : 'FAIL'} ${check.scene ?? 'project'}: ${check.code}${check.path ? ` (${check.path})` : ''}${check.message ? ` — ${check.message}` : ''}\n`);
  }
}

export async function run(argv, io = {}) {
  const input = io.input ?? process.stdin;
  const output = io.output ?? process.stdout;
  const errorOutput = io.error ?? process.stderr;
  const environment = io.environment ?? process.env;
  let parsed;
  try {
    parsed = parseArgs(argv);
    if (parsed.kind === 'help') { output.write(HELP); return 0; }
    if (parsed.kind === 'version') {
      output.write(`${JSON.parse(await readFile(path.join(PACKAGE_ROOT, 'package.json'), 'utf8')).version}\n`);
      return 0;
    }
    const projectRoot = path.resolve(parsed.projectPath);
    if (parsed.command === 'init') {
      if (!input.isTTY || !output.isTTY) throw new UsageError('xiaotao init must run in an interactive terminal.');
      const scenes = await chooseHosts(input, output, environment);
      const notice = await globalUpdateNotice({ packageRoot: PACKAGE_ROOT, environment });
      if (notice && (await ask(input, output, `共享安装将从 ${notice.from} 更新到 ${notice.to}，继续？ [y/N]： `)).toLowerCase() !== 'y') {
        throw new Error('Installation cancelled.');
      }
      const result = await installScenes({ projectRoot, packageRoot: PACKAGE_ROOT, sceneIds: scenes, force: parsed.force, environment });
      printDiagnosis(output, { checks: result.checks });
      output.write(`\nXiaoTao 已初始化。项目记录：${path.join(projectRoot, '.xiaotao', 'installation.json')}\n`);
      return result.checks.every((check) => check.ok) ? 0 : 1;
    }
    if (parsed.command === 'update') {
      await requireCurrentInstallation(projectRoot);
      const notice = await globalUpdateNotice({ packageRoot: PACKAGE_ROOT, environment });
      if (notice && !parsed.yes && input.isTTY && output.isTTY && (await ask(input, output, `共享安装将从 ${notice.from} 更新到 ${notice.to}，继续？ [y/N]： `)).toLowerCase() !== 'y') {
        throw new Error('Update cancelled.');
      }
      const result = await updateScenes({ projectRoot, packageRoot: PACKAGE_ROOT, environment });
      printDiagnosis(output, { checks: result.checks });
      return result.checks.every((check) => check.ok) ? 0 : 1;
    }
    const diagnosis = await doctorInstallation(projectRoot, environment);
    if (parsed.json) output.write(`${JSON.stringify(diagnosis, null, 2)}\n`);
    else printDiagnosis(output, diagnosis);
    return diagnosis.ok ? 0 : 1;
  } catch (error) {
    const code = error instanceof UsageError ? 2 : 1;
    if (parsed?.json) output.write(`${JSON.stringify({ ok: false, error: error.message })}\n`);
    else errorOutput.write(`Error: ${error.message}\n`);
    return code;
  }
}
