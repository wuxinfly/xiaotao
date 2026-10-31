import { createHash } from 'node:crypto';
import { mkdir, open, readFile, realpath, rename, stat, unlink } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { safeRelative, validDelegationEnvelope, validDelegationPacket } from './delegation-contract.mjs';
import {
  packetWithinWorker,
  parseWorkerSnapshotYaml,
} from './worker-snapshot-contract.mjs';

const CONFIG = '.xiaotao/installation.json';
const PENDING = '.xiaotao/runtime/codex/pending-delegation.json';
const MAX_INPUT = 64 * 1024;
const MAX_PACKET = 32 * 1024;
const MAX_INSTRUCTION = 16 * 1024;
const MAX_CONTEXT = 32 * 1024;

function inside(root, target) {
  const relative = path.relative(root, target);
  return relative === '' || (!relative.startsWith(`..${path.sep}`)
    && relative !== '..' && !path.isAbsolute(relative));
}

async function regularInside(root, relative, maxBytes, required = true) {
  if (!safeRelative(relative)) return null;
  try {
    const target = await realpath(path.join(root, ...relative.split('/')));
    const info = await stat(target);
    return inside(root, target) && info.isFile() && info.size <= maxBytes ? target : null;
  } catch (error) {
    if (!required && (error.code === 'ENOENT' || error.code === 'ENOTDIR')) return path.join(root, ...relative.split('/'));
    if (error.code === 'ENOENT' || error.code === 'ENOTDIR') return null;
    throw error;
  }
}

async function boundedJson(file, maxBytes) {
  const info = await stat(file);
  if (!info.isFile() || info.size > maxBytes) throw new Error('bounded_json_invalid');
  return JSON.parse(await readFile(file, 'utf8'));
}

async function safeDestination(root, relative) {
  if (!safeRelative(relative)) return false;
  let candidate = path.resolve(root, ...relative.split('/'));
  if (!inside(root, candidate)) return false;
  while (candidate !== root) {
    try { return inside(root, await realpath(candidate)); }
    catch (error) {
      if (error.code !== 'ENOENT' && error.code !== 'ENOTDIR') throw error;
      candidate = path.dirname(candidate);
    }
  }
  return true;
}

async function projectRoot(cwd) {
  if (typeof cwd !== 'string' || !path.isAbsolute(cwd)) return null;
  let root = await realpath(cwd);
  for (let depth = 0; depth < 64; depth++) {
    const configFile = await regularInside(root, CONFIG, 16 * 1024);
    if (configFile) {
      const metadata = await boundedJson(configFile, 16 * 1024);
      return metadata.package === 'xiaotao-ai-workflow' && metadata.schema_version === 1 ? root : null;
    }
    try {
      await stat(path.join(root, '.git'));
      return null;
    } catch (error) { if (error.code !== 'ENOENT') throw error; }
    try {
      await stat(path.join(root, '.xiaotao'));
      return null;
    } catch (error) { if (error.code !== 'ENOENT') throw error; }
    const parent = path.dirname(root);
    if (parent === root) return null;
    root = parent;
  }
  return null;
}

function packetWorkerFromPath(relative) {
  const match = /^\.xiaotao\/(?:tasks\/[^/]+|memory\/temporary\/active\/[^/]+)\/workers\/([^/]+)\/runs\/[^/]+\/delegation\.json$/.exec(relative);
  return match?.[1] || null;
}

function snapshotPathForPacket(relative) {
  const match = /^(\.xiaotao\/(?:tasks\/[^/]+|memory\/temporary\/active\/[^/]+)\/workers\/[^/]+)\/runs\/[^/]+\/delegation\.json$/.exec(relative);
  return match ? `${match[1]}/spec.yaml` : null;
}

async function instructionMaterial(root, packet) {
  const coreRoot = await regularInside(root, '.agents/skills/xiaotao/SKILL.md', MAX_INSTRUCTION);
  if (!coreRoot) throw new Error('project_core_unavailable');
  const coreDirectory = path.dirname(coreRoot);
  const sections = [];
  let total = 0;
  for (const resolved of packet.instructions.resolved) {
    const base = resolved.source_scope === 'core' ? coreDirectory : root;
    const digest = createHash('sha256');
    const contents = [];
    for (const relative of resolved.source_paths) {
      const file = await regularInside(base, relative, MAX_INSTRUCTION);
      if (!file) throw new Error('instruction_source_invalid');
      const content = await readFile(file);
      total += content.length;
      if (total > MAX_CONTEXT) throw new Error('instruction_context_exceeded');
      digest.update(relative).update('\0').update(content).update('\0');
      contents.push(content.toString('utf8'));
    }
    if (digest.digest('hex') !== resolved.sha256) throw new Error('instruction_digest_mismatch');
    sections.push(`## ${resolved.ref}\n${contents.join('\n')}`);
  }
  return sections.join('\n\n');
}

async function validatePacket(root, envelope) {
  const packetFile = await regularInside(root, envelope.delegation_path, MAX_PACKET);
  if (!packetFile) throw new Error('delegation_path_invalid');
  const packet = await boundedJson(packetFile, MAX_PACKET);
  if (!validDelegationPacket(packet) || packetWorkerFromPath(envelope.delegation_path) !== packet.worker_id) {
    throw new Error('delegation_packet_invalid');
  }
  for (const ref of packet.context_refs) {
    if (!(await regularInside(root, ref.path, MAX_INSTRUCTION))) throw new Error('context_ref_invalid');
  }
  const expectedSnapshot = snapshotPathForPacket(envelope.delegation_path);
  if (!expectedSnapshot || packet.worker_snapshot_path !== expectedSnapshot) {
    throw new Error('worker_snapshot_path_invalid');
  }
  const snapshotFile = await regularInside(root, packet.worker_snapshot_path, MAX_INSTRUCTION);
  if (!snapshotFile) throw new Error('worker_snapshot_invalid');
  const snapshot = parseWorkerSnapshotYaml(await readFile(snapshotFile, 'utf8'));
  if (!packetWithinWorker(packet, snapshot)) throw new Error('worker_snapshot_mismatch');
  for (const output of [packet.result_path, packet.handoff_path]) {
    if (!(await safeDestination(root, output))) throw new Error('output_path_invalid');
  }
  return { packet, instructions: await instructionMaterial(root, packet) };
}

async function claimEnvelope(root, event, envelope) {
  const runtime = path.join(root, '.xiaotao/runtime/codex');
  const claims = path.join(runtime, 'claims');
  await mkdir(claims, { recursive: true });
  const lockPath = path.join(runtime, '.delegation-claim.lock');
  let lock;
  try { lock = await open(lockPath, 'wx'); }
  catch (error) { if (error.code === 'EEXIST') return null; throw error; }
  try {
    const current = await boundedJson(path.join(root, ...PENDING.split('/')), 16 * 1024);
    if (JSON.stringify(current) !== JSON.stringify(envelope)) return null;
    const identity = createHash('sha256').update(`${event.session_id}\0${event.agent_id}`).digest('hex').slice(0, 24);
    const claimed = path.join(claims, `${identity}.json`);
    try { await stat(claimed); return null; }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    await rename(path.join(root, ...PENDING.split('/')), claimed);
    return claimed;
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  } finally {
    await lock?.close();
    await unlink(lockPath).catch(error => { if (error.code !== 'ENOENT') throw error; });
  }
}

function formatContext(event, packet, instructions) {
  const payload = {
    worker_id: packet.worker_id,
    objective: packet.objective,
    completion_condition: packet.completion_condition,
    context_refs: packet.context_refs,
    tools: packet.tools,
    permissions: packet.permissions,
    unsupported_requirements: packet.host_adapter.unsupported_requirements,
    result_path: packet.result_path,
    handoff_path: packet.handoff_path,
  };
  return [
    '你正在作为一个有界 XiaoTao Worker 运行。以下 Delegation Packet 已与当前 Codex Session 绑定并完成一次性认领。',
    'Packet 中的工具与权限只是允许上限，不是宿主强制隔离；当前 Codex Adapter 状态为 degraded。不得使用上限外能力，也不得扩大范围或把条件动作视为已授权。',
    `宿主数据：${JSON.stringify({ agent_type: event.agent_type, permission_mode: event.permission_mode })}`,
    `Delegation Packet：${JSON.stringify(payload)}`,
    '',
    '# Resolved Required Instructions',
    instructions,
  ].join('\n');
}

export async function subagentContext(event) {
  if (event?.hook_event_name !== 'SubagentStart' || typeof event.session_id !== 'string'
    || typeof event.agent_id !== 'string' || typeof event.agent_type !== 'string') return null;
  const root = await projectRoot(event.cwd);
  if (!root) return null;
  const pendingFile = await regularInside(root, PENDING, 16 * 1024);
  if (!pendingFile) return null;
  let envelope;
  try { envelope = await boundedJson(pendingFile, 16 * 1024); } catch { return null; }
  if (!validDelegationEnvelope(envelope) || envelope.session_id !== event.session_id) return null;
  const claimed = await claimEnvelope(root, event, envelope);
  if (!claimed) return null;
  try {
    const { packet, instructions } = await validatePacket(root, envelope);
    return {
      hookSpecificOutput: {
        hookEventName: 'SubagentStart',
        additionalContext: formatContext(event, packet, instructions),
      },
    };
  } catch {
    return null;
  }
}

export async function run(input = process.stdin, output = process.stdout, errors = process.stderr) {
  try {
    const chunks = [];
    let size = 0;
    for await (const chunk of input) {
      const buffer = Buffer.from(chunk);
      size += buffer.length;
      if (size > MAX_INPUT) throw new Error('hook_input_exceeded');
      chunks.push(buffer);
    }
    const result = await subagentContext(JSON.parse(Buffer.concat(chunks).toString('utf8')));
    if (result) output.write(`${JSON.stringify(result)}\n`);
  } catch {
    errors.write('XiaoTao subagent hook skipped: invalid or unavailable delegation state.\n');
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await run();
