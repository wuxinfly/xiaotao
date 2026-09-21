import { stat, readFile } from 'node:fs/promises';
import path from 'node:path';
import { createInterface } from 'node:readline/promises';

import { detectHosts, HOSTS, parseToolList } from './hosts.js';
import { doctorInstallation, installHosts, updateHosts } from './install.js';

const PACKAGE_ROOT = path.resolve(import.meta.dirname, '..');

const HELP = `XiaoTao multi-host installer

Usage:
  xiaotao init [path] [--tools <list>] [--force] [--json]
  xiaotao update [path] [--json]
  xiaotao doctor [path] [--json]
  xiaotao --version

Options:
  --tools <list>  Comma-separated tools: codex, claude, opencode, all, or none
  --force         Adopt a non-empty destination not previously managed by XiaoTao
  --json          Print machine-readable JSON
  --help, -h      Show help
  --version, -V   Show version
`;

class UsageError extends Error {}

async function exists(target) {
  try {
    await stat(target);
    return true;
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
}

async function packageVersion() {
  return JSON.parse(await readFile(path.join(PACKAGE_ROOT, 'package.json'), 'utf8')).version;
}

function parseArgs(argv) {
  if (argv.length === 0 || argv[0] === '--help' || argv[0] === '-h') return { kind: 'help' };
  if (argv[0] === '--version' || argv[0] === '-V') return { kind: 'version' };

  const command = argv[0];
  if (!['init', 'update', 'doctor'].includes(command)) {
    throw new UsageError(`Unknown command: ${command}`);
  }

  const options = { kind: 'command', command, projectPath: '.', json: false, force: false };
  let pathSeen = false;
  for (let index = 1; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === '--help' || argument === '-h') return { kind: 'help' };
    if (argument === '--json') {
      options.json = true;
      continue;
    }
    if (argument === '--force') {
      if (command !== 'init') throw new UsageError('--force is only valid with xiaotao init');
      options.force = true;
      continue;
    }
    if (argument === '--tools') {
      if (command !== 'init') throw new UsageError('--tools is only valid with xiaotao init');
      const value = argv[index + 1];
      if (!value || value.startsWith('--')) throw new UsageError('--tools requires a value');
      options.tools = value;
      index += 1;
      continue;
    }
    if (argument.startsWith('-')) throw new UsageError(`Unknown option: ${argument}`);
    if (pathSeen) throw new UsageError(`Unexpected argument: ${argument}`);
    options.projectPath = argument;
    pathSeen = true;
  }
  return options;
}

async function chooseTools(projectRoot, input, output) {
  const detected = await detectHosts((relativePath) => exists(path.join(projectRoot, relativePath)));
  output.write('Select AI tools for this project:\n');
  const hostIds = Object.keys(HOSTS);
  hostIds.forEach((toolId, index) => {
    const detectedLabel = detected.includes(toolId) ? ' (detected)' : '';
    output.write(`  ${index + 1}. ${HOSTS[toolId].name}${detectedLabel}\n`);
  });
  const defaultText = detected.length > 0 ? detected.join(',') : 'none';
  const prompt = createInterface({ input, output });
  try {
    const answer = (await prompt.question(`Tools [${defaultText}]: `)).trim();
    if (answer === '') return detected;
    const selected = [];
    for (const item of answer.split(',').map((value) => value.trim()).filter(Boolean)) {
      if (/^\d+$/.test(item)) {
        const toolId = hostIds[Number(item) - 1];
        if (!toolId) throw new UsageError(`Unknown tool selection: ${item}`);
        if (!selected.includes(toolId)) selected.push(toolId);
      } else {
        for (const toolId of parseToolList(item)) {
          if (!selected.includes(toolId)) selected.push(toolId);
        }
      }
    }
    return selected;
  } finally {
    prompt.close();
  }
}

function writeJson(output, value) {
  output.write(`${JSON.stringify(value, null, 2)}\n`);
}

export async function run(argv, io = {}) {
  const input = io.input ?? process.stdin;
  const output = io.output ?? process.stdout;
  const errorOutput = io.error ?? process.stderr;

  let parsed;
  try {
    parsed = parseArgs(argv);
    if (parsed.kind === 'help') {
      output.write(HELP);
      return 0;
    }
    if (parsed.kind === 'version') {
      output.write(`${await packageVersion()}\n`);
      return 0;
    }

    const projectRoot = path.resolve(parsed.projectPath);
    if (parsed.command === 'init') {
      let toolIds;
      if (parsed.tools !== undefined) {
        toolIds = parseToolList(parsed.tools);
      } else if (input.isTTY && output.isTTY) {
        toolIds = await chooseTools(projectRoot, input, output);
      } else {
        throw new UsageError('Pass --tools <list> when xiaotao init is not running interactively.');
      }
      const metadata = await installHosts({
        projectRoot,
        packageRoot: PACKAGE_ROOT,
        toolIds,
        force: parsed.force,
      });
      if (parsed.json) {
        writeJson(output, { ok: true, action: 'init', ...metadata });
      } else if (toolIds.length === 0) {
        output.write('Initialized XiaoTao without an AI host integration.\n');
      } else {
        for (const toolId of toolIds) {
          output.write(`Installed XiaoTao for ${HOSTS[toolId].name} at ${HOSTS[toolId].skillDir}\n`);
        }
      }
      return 0;
    }

    if (parsed.command === 'update') {
      const metadata = await updateHosts({ projectRoot, packageRoot: PACKAGE_ROOT });
      if (parsed.json) writeJson(output, { ok: true, action: 'update', ...metadata });
      else output.write(`Updated XiaoTao for ${metadata.tools.length} AI tool(s).\n`);
      return 0;
    }

    const diagnosis = await doctorInstallation(projectRoot);
    if (parsed.json) {
      writeJson(output, diagnosis);
    } else {
      for (const check of diagnosis.checks) {
        output.write(`${check.ok ? 'PASS' : 'FAIL'} ${check.code}${check.tool ? ` (${check.tool})` : ''}${check.path ? `: ${check.path}` : ''}\n`);
      }
    }
    return diagnosis.ok ? 0 : 1;
  } catch (error) {
    const exitCode = error instanceof UsageError ? 2 : 1;
    if (parsed?.json) writeJson(output, { ok: false, error: error.message });
    else errorOutput.write(`Error: ${error.message}\n`);
    return exitCode;
  }
}
