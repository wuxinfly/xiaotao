// Stop checks explicit turn receipts; it never interprets transcripts or writes Core progress.
import { createHash, randomUUID } from 'node:crypto';
import { lstat, mkdir, open, readFile, realpath, rename, unlink } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const LIMIT = 32 * 1024;
const digest = value => createHash('sha256').update(value).digest('hex');
const identity = value => typeof value === 'string' && value.length > 0 && value.length <= 256;

async function safePath(root, relative, createParents = false) {
  if (path.isAbsolute(relative) || relative.includes('\\') || relative.split('/').some(p => !p || p === '.' || p === '..')) throw new Error('unsafe_path');
  let current = root;
  const parts = relative.split('/');
  for (let i = 0; i < parts.length; i++) {
    current = path.join(current, parts[i]);
    let info;
    try { info = await lstat(current); }
    catch (e) {
      if (e.code !== 'ENOENT') throw e;
      if (i < parts.length - 1 && createParents) { await mkdir(current); info = await lstat(current); }
      else if (i === parts.length - 1) return current;
      else throw e;
    }
    if (info.isSymbolicLink() || (i < parts.length - 1 && !info.isDirectory())) throw new Error('unsafe_path');
  }
  return current;
}

async function bounded(file) {
  const info = await lstat(file);
  if (!info.isFile() || info.isSymbolicLink() || info.size > LIMIT) throw new Error('invalid_file');
  const content = await readFile(file);
  if (content.length > LIMIT) throw new Error('file_budget_exceeded');
  return content;
}

async function project(cwd) {
  if (typeof cwd !== 'string' || !path.isAbsolute(cwd)) return null;
  let root = await realpath(cwd);
  for (let depth = 0; depth < 64; depth++) {
    try {
      const config = JSON.parse(await bounded(await safePath(root, '.xiaotao/installation.json')));
      return config.package === 'xiaotao-ai-workflow' && [1, 2].includes(config.schema_version) ? root : null;
    } catch (e) { if (e.code !== 'ENOENT') throw e; }
    try { await lstat(path.join(root, '.git')); return null; } catch (e) { if (e.code !== 'ENOENT') throw e; }
    const parent = path.dirname(root);
    if (parent === root) break;
    root = parent;
  }
  return null;
}

async function atomic(file, record) {
  const temporary = `${file}.${randomUUID()}.tmp`;
  try { await open(temporary, 'wx').then(async h => { try { await h.writeFile(JSON.stringify(record)); } finally { await h.close(); } }); await rename(temporary, file); }
  finally { await unlink(temporary).catch(e => { if (e.code !== 'ENOENT') throw e; }); }
}

async function withRecord(event, fn) {
  if (!identity(event.session_id) || !identity(event.turn_id)) return null;
  const root = await project(event.cwd);
  if (!root) return null;
  const file = await safePath(root, `.xiaotao/runtime/codex/progress/${digest(event.session_id)}.json`, true);
  const lockPath = await safePath(root, `.xiaotao/runtime/codex/progress/${digest(event.session_id)}.lock`);
  const lock = await open(lockPath, 'wx');
  try {
    let record = null;
    try { record = JSON.parse(await bounded(file));
      if (record.schema_version !== 1 || !identity(record.session_id) || !identity(record.turn_id)
        || !Number.isFinite(record.started_at) || typeof record.reminded !== 'boolean') throw new Error('invalid_record'); } catch (e) { if (e.code !== 'ENOENT') throw e; }
    return await fn({ root, file, record });
  } finally { await lock.close(); await unlink(lockPath); }
}

async function savedState(root, relative, startedAt) {
  if (typeof relative !== 'string' || !/^\.xiaotao\/(?:tasks\/(?:archive\/)?[^/]+\/progress\.md|memory\/temporary\/(?:active|archive)\/[^/]+\/current\.md)$/.test(relative)) throw new Error('invalid_progress_target');
  const bytes = await bounded(await safePath(root, relative));
  const front = bytes.toString('utf8').match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/)?.[1];
  const revision = front?.match(/^revision: (\d+)\s*$/m)?.[1];
  const updated = front?.match(/^updated_at: (.+)\s*$/m)?.[1]?.trim();
  // Validate the mutable Current State, not a final chat summary.
  if (!revision || !front?.match(/^updated_by: .+$/m) || !Number.isFinite(Date.parse(updated))
    || Date.parse(updated) < startedAt - 2000 || Date.parse(updated) > Date.now() + 2000
    || !Number.isSafeInteger(Number(revision))) throw new Error('progress_not_saved_this_turn');
  return { path: relative, sha256: digest(bytes), revision: Number(revision) };
}

function instruction(event) {
  return `本轮结束前检查阶段进度：有新事实按 Core 保存并回读，无新事实或用户要求不保存则跳过；不要为回执创建业务 Task。调用插件可信脚本 progress-check.mjs --ack，通过 stdin 传 JSON（cwd、session_id、turn_id、status；saved 加 path，skipped 加 reason=no-new-facts/user-opt-out/no-current-work，failed 加 reason=save-failed 并向用户说明）。脚本路径：${JSON.stringify(fileURLToPath(import.meta.url))}。本轮标识：${JSON.stringify({ cwd: event.cwd, session_id: event.session_id, turn_id: event.turn_id })}。回执不授予权限、不批准长期记忆；不得使用聊天总结冒充保存。`;
}

export async function acknowledge(event) {
  if (!['saved', 'skipped', 'failed'].includes(event.status)) throw new Error('invalid_status');
  const result = await withRecord(event, async ({ root, file, record }) => {
    if (record?.session_id !== event.session_id || record?.turn_id !== event.turn_id) throw new Error('turn_mismatch');
    let receipt;
    if (event.status === 'saved') receipt = { status: 'saved', ...await savedState(root, event.path, record.started_at) };
    else {
      const reasons = event.status === 'skipped' ? ['no-new-facts', 'user-opt-out', 'no-current-work'] : ['save-failed'];
      if (!reasons.includes(event.reason)) throw new Error('invalid_reason');
      receipt = { status: event.status, reason: event.reason };
    }
    await atomic(file, { ...record, receipt });
    return { status: receipt.status };
  });
  if (!result) throw new Error('acknowledgement_unavailable');
  return result;
}

export async function progressContext(event) {
  if (!['UserPromptSubmit', 'Stop'].includes(event?.hook_event_name)) return null;
  if (event.hook_event_name === 'Stop' && event.stop_hook_active === true) return null;
  return withRecord(event, async ({ root, file, record }) => {
    if (event.hook_event_name === 'UserPromptSubmit') {
      if (record?.turn_id !== event.turn_id || record?.session_id !== event.session_id) {
        await atomic(file, { schema_version: 1, session_id: event.session_id, turn_id: event.turn_id, started_at: Date.now(), reminded: false });
      }
      return { hookSpecificOutput: { hookEventName: 'UserPromptSubmit', additionalContext: instruction(event) } };
    }
    if (record?.turn_id !== event.turn_id || record?.session_id !== event.session_id) return null;
    if (record.receipt?.status === 'skipped' && ['no-new-facts', 'user-opt-out', 'no-current-work'].includes(record.receipt.reason)) return null;
    if (record.receipt?.status === 'failed' && record.receipt.reason === 'save-failed') return { systemMessage: 'XiaoTao 本轮登记了保存失败；进度未保存，请查看模型报告。' };
    if (record.receipt?.status === 'saved') {
      try {
        const current = await savedState(root, record.receipt.path, record.started_at);
        if (current.sha256 === record.receipt.sha256 && current.revision === record.receipt.revision) return null;
      } catch { /* Remind once if a saved target disappeared or changed after acknowledgement. */ }
    }
    if (record.reminded) return { systemMessage: 'XiaoTao 阶段保存回执仍缺失或失效，本轮已提醒一次，放行；不能据此声称进度已保存。' };
    await atomic(file, { ...record, reminded: true });
    return { decision: 'block', reason: `仅补查本轮阶段进度，不继续其他工作或扩大授权。缺少有效保存/跳过回执；如需保存，按 Core 写入并回读后登记；无新事实则登记跳过；失败如实告知并登记 failed。${instruction(event)}` };
  });
}

export async function run(input = process.stdin, output = process.stdout, errors = process.stderr, ack = false) {
  try {
    if (process.env.XIAOTAO_CODEX_PROGRESS_CHECK === '0') { if (!ack) output.write('{}\n'); return; }
    let text = '';
    for await (const chunk of input) { text += chunk; if (Buffer.byteLength(text) > LIMIT) throw new Error('input_budget_exceeded'); }
    const result = ack ? await acknowledge(JSON.parse(text)) : await progressContext(JSON.parse(text));
    output.write(`${JSON.stringify(result ?? {})}\n`);
  } catch (error) {
    errors.write(`XiaoTao progress check skipped: ${error.message}\n`);
    output.write('{}\n');
    if (ack) process.exitCode = 1; // CLI callers must observe a failed acknowledgement.
    // Hook failures never trap a turn. A failed ack remains missing.
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await run(undefined, undefined, undefined, process.argv[2] === '--ack');
