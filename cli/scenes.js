import { stat } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);

export function resolveUserHome(environment = process.env) {
  return path.resolve(environment.XIAOTAO_HOME || os.homedir());
}

export function resolveDshHome(environment = process.env) {
  return path.resolve(environment.DSH_HOME || path.join(resolveUserHome(environment), '.dsh'));
}

async function exists(target) {
  try {
    await stat(target);
    return true;
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw error;
  }
}

async function hasDsh() {
  try {
    await execFileAsync('dsh', ['--version'], { windowsHide: true });
    return true;
  } catch {
    return false;
  }
}

export const SCENES = Object.freeze({
  dsh: Object.freeze({
    id: 'dsh',
    name: 'DSH',
    description: '安装为 DSH profile 插件',
    resolveTarget: (environment) => ({ kind: 'dsh-profile', profile: 'web', dsh_home: resolveDshHome(environment) }),
    detect: hasDsh,
  }),
  skills: Object.freeze({
    id: 'skills',
    name: 'Codex / 通用 Agent Skills',
    description: '安装到用户级 .agents/skills',
    resolveTarget: (environment) => ({ kind: 'user-skills', path: path.join(resolveUserHome(environment), '.agents', 'skills', 'xiaotao') }),
    detect: async () => true,
  }),
  claude: Object.freeze({
    id: 'claude',
    name: 'Claude Code',
    description: '安装到用户级 .claude/skills',
    resolveTarget: (environment) => ({ kind: 'user-skills', path: path.join(resolveUserHome(environment), '.claude', 'skills', 'xiaotao') }),
    detect: async (environment) => exists(path.join(resolveUserHome(environment), '.claude')),
  }),
});

export async function detectScenes(environment = process.env) {
  const detected = [];
  for (const scene of Object.values(SCENES)) {
    if (await scene.detect(environment)) detected.push(scene.id);
  }
  return detected;
}
