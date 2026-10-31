// Stop and PreCompact check explicit daily receipts; neither summarizes chats nor writes progress.
import { createHash, randomUUID } from 'node:crypto';
import { lstat, mkdir, open, readFile, realpath, rename, unlink } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { homedir } from 'node:os';
import { spawn } from 'node:child_process';

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
  if (!identity(event.session_id) || (event.hook_event_name !== 'PreCompact' && !identity(event.turn_id))) return null;
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

const WORK_TARGET = /^\.xiaotao\/(?:tasks\/(?:archive\/)?(?<task>[^/]+)\/progress\.md|memory\/temporary\/(?:active|archive)\/(?<temporary>[^/]+)\/current\.md)$/;
const DAILY_TARGET = /^\.xiaotao\/memory\/timeline\/years\/(?<year>\d{4})\/(?<month>\d{2})\/(?<day>\d{2})\/details\/(?<topic>[a-z0-9][a-z0-9_-]{0,79})\.md$/;

async function savedState(root, relative, startedAt, work = false) {
  const match = typeof relative === 'string' && (work ? WORK_TARGET : DAILY_TARGET).exec(relative);
  if (!match) throw new Error('invalid_progress_target');
  const bytes = await bounded(await safePath(root, relative));
  const text = bytes.toString('utf8');
  const front = text.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/)?.[1];
  const revision = front?.match(/^revision: (\d+)\s*$/m)?.[1];
  const updated = front?.match(/^updated_at: (.+)\s*$/m)?.[1]?.trim();
  const actor = front?.match(/^updated_by: (.+)$/m)?.[1]?.trim();
  if (!revision || !actor || !Number.isFinite(Date.parse(updated))
    || Date.parse(updated) < startedAt - 2000 || Date.parse(updated) > Date.now() + 2000
    || !Number.isSafeInteger(Number(revision))) throw new Error('progress_not_saved_this_turn');
  let record;
  if (!work) {
    const blocks = [...text.matchAll(/^```xiaotao-daily-record\r?\n([\s\S]*?)^```\s*$/gm)];
    if (blocks.length !== 1) throw new Error('invalid_daily_record');
    record = JSON.parse(blocks[0][1]);
    const day = `${match.groups.year}-${match.groups.month}-${match.groups.day}`;
    if (record.schema_version !== 1 || record.date !== day || record.topic_id !== match.groups.topic
      || record.record_id !== `daily-${day}-${match.groups.topic}` || record.revision !== Number(revision)
      || record.updated_at !== updated || record.updated_by !== actor
      || typeof record.title !== 'string' || !record.title.trim()
      || typeof record.summary !== 'string' || !record.summary.trim()
      || typeof record.timezone !== 'string' || !Number.isFinite(Date.parse(record.occurred_at))) throw new Error('invalid_daily_record');
    // Compare the record's IANA local date without trusting a caller-provided path.
    const local = new Intl.DateTimeFormat('en-CA', { timeZone: record.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(record.occurred_at));
    const part = type => local.find(p => p.type === type)?.value;
    if (`${part('year')}-${part('month')}-${part('day')}` !== day) throw new Error('invalid_daily_date');
  }
  return { state: { path: relative, sha256: digest(bytes), revision: Number(revision) }, record };
}

async function savedReceipt(root, relative, startedAt, workPaths = []) {
  const { state, record } = await savedState(root, relative, startedAt);
  if (!Array.isArray(workPaths) || workPaths.length > 1 || workPaths.some(p => typeof p !== 'string')) throw new Error('invalid_work_paths');
  const workStates = [];
  if (record.work_ref) {
    const { kind, id, path: original } = record.work_ref;
    const expected = typeof original === 'string' && WORK_TARGET.exec(original);
    const actual = workPaths.length === 1 && WORK_TARGET.exec(workPaths[0]);
    if (!['task', 'temporary'].includes(kind) || !expected || expected.groups[kind] !== id
      || !actual || actual.groups[kind] !== id) throw new Error('linked_work_receipt_missing');
    const saved = await savedState(root, workPaths[0], startedAt, true);
    workStates.push(saved.state);
  } else if (workPaths.length) throw new Error('unrelated_work_receipt');
  return { ...state, work_states: workStates };
}

async function receiptValid(root, record) {
  if (record?.pending_save) return false;
  const receipt = record?.receipt;
  if (receipt?.status === 'skipped' && ['no-new-facts', 'user-opt-out', 'no-current-work'].includes(receipt.reason)) return true;
  if (receipt?.status !== 'saved') return false;
  try {
    if (receipt.verified_at !== undefined && (!Number.isFinite(receipt.verified_at) || receipt.verified_at < record.started_at)) return false;
    const current = await savedReceipt(root, receipt.path, receipt.verified_at === undefined ? record.started_at : 0, receipt.work_states?.map(s => s.path) ?? []);
    return current.sha256 === receipt.sha256 && current.revision === receipt.revision
      && JSON.stringify(current.work_states) === JSON.stringify(receipt.work_states ?? []);
  } catch { return false; }
}

function instruction(event) {
  return `阶段结束交付前合并保存一次，连续修改与验证不分别保存；正常保存不逐步播报。有新事实时调用可信插件脚本 progress-check.mjs --save，stdin JSON 为 {cwd,session_id,turn_id,progress:{topic_id,title,summary,details?,validation?,next_steps?,work_path?,request_id?}}。脚本自动处理日期、版本、工作进度追加、当天汇总、导航与回执；同日同项目同模块小修在 progress 中传 record_type:"minor"、稳定 project_id/module_id/group_id，可省略 topic_id；重要节点独立记录，详见 Core references/progress-saving.md。普通讨论不传 work_path，不创建 Task/Temporary。不再手工拼锁、快照或单独登记 saved。长任务中途补存使用不同 request_id；失败重试保留原 JSON，不反复换写入方法。无新事实或用户明确不保存时仅用 --ack 登记 skipped，reason 为 no-new-facts/user-opt-out/no-current-work；普通跳过不清除前轮未保存事实。脚本路径：${JSON.stringify(fileURLToPath(import.meta.url))}。本轮标识：${JSON.stringify({ cwd: event.cwd, session_id: event.session_id, turn_id: event.turn_id })}。失败如实说明；回执不扩大授权，也不能证明模型总结涵盖所有事实。`;
}

export async function acknowledge(event) {
  if (!['saved', 'skipped', 'failed'].includes(event.status)) throw new Error('invalid_status');
  const result = await withRecord(event, async ({ root, file, record }) => {
    if (record?.session_id !== event.session_id || record?.turn_id !== event.turn_id) throw new Error('turn_mismatch');
    let receipt;
    if (event.status === 'saved') receipt = { status: 'saved', ...await savedReceipt(root, event.path, record.started_at, event.work_paths) };
    else {
      const reasons = event.status === 'skipped' ? ['no-new-facts', 'user-opt-out', 'no-current-work'] : ['save-failed'];
      if (!reasons.includes(event.reason)) throw new Error('invalid_reason');
      receipt = { status: event.status, reason: event.reason };
    }
    // Ordinary skips cannot discard facts left unsaved by an earlier turn.
    const pending_save = receipt.status === 'saved' || receipt.reason === 'user-opt-out'
      ? false : Boolean(record.pending_save || record.receipt?.status === 'failed' || receipt.status === 'failed');
    await atomic(file, { ...record, receipt, pending_save });
    return { status: receipt.status };
  });
  if (!result) throw new Error('acknowledgement_unavailable');
  return result;
}

export async function progressContext(event) {
  if (event?.hook_event_name === 'PreCompact') return preCompact(event);
  if (!['UserPromptSubmit', 'Stop'].includes(event?.hook_event_name)) return null;
  if (event.hook_event_name === 'Stop' && event.stop_hook_active === true) return null;
  return withRecord(event, async ({ root, file, record }) => {
    if (event.hook_event_name === 'UserPromptSubmit') {
      if (record?.turn_id !== event.turn_id || record?.session_id !== event.session_id) {
        const pending_save = record?.session_id === event.session_id && !await receiptValid(root, record);
        await atomic(file, { schema_version: 1, session_id: event.session_id, turn_id: event.turn_id, started_at: Date.now(), reminded: false, pending_save: Boolean(pending_save) });
      }
      return { hookSpecificOutput: { hookEventName: 'UserPromptSubmit', additionalContext: instruction(event) } };
    }
    if (record?.turn_id !== event.turn_id || record?.session_id !== event.session_id) return null;
    if (!record.pending_save && record.receipt?.status === 'skipped' && ['no-new-facts', 'user-opt-out', 'no-current-work'].includes(record.receipt.reason)) return null;
    if (record.receipt?.status === 'failed' && record.receipt.reason === 'save-failed') return { systemMessage: 'XiaoTao 本轮登记了保存失败；进度未保存，请查看模型报告。' };
    if (await receiptValid(root, record)) return null;
    if (record.reminded) return { systemMessage: 'XiaoTao 阶段保存回执仍缺失或失效，本轮已提醒一次，放行；不能据此声称进度已保存。' };
    await atomic(file, { ...record, reminded: true });
    return { decision: 'block', reason: `仅补查本轮阶段进度，不继续其他工作或扩大授权。缺少有效保存/跳过回执；如需保存，按 Core 写入并回读后登记；无新事实则登记跳过；失败如实告知并登记 failed。${instruction(event)}` };
  });
}

async function preCompact(event) {
  if (!['manual', 'auto'].includes(event.trigger)) return null;
  return withRecord(event, async ({ root, file, record }) => {
    if (record?.session_id === event.session_id && await receiptValid(root, record)) return null;
    const message = 'XiaoTao 缺少有效的每日记忆保存回执。请先让小涛保存当天记忆及关联工作进度，再压缩；Hook 没有自动总结或保存。';
    // Automatic compaction can occur mid-turn at the context limit. Do not trap it
    // before the model has an opportunity to save. The warning is explicit.
    if (event.trigger === 'auto') return { systemMessage: message + '本次为自动压缩，放行并提示未保存风险。' };
    if (record?.session_id === event.session_id && record.compact_blocked) {
      return { systemMessage: message + '已暂停过一次；再次手动压缩视为用户重试，放行。' };
    }
    const current = record?.session_id === event.session_id ? record : {
      schema_version: 1, session_id: event.session_id, turn_id: 'precompact', started_at: Date.now(), reminded: false,
    };
    await atomic(file, { ...current, compact_blocked: true });
    return { continue: false, stopReason: message, systemMessage: message };
  });
}

// CLI --save uses only a user-installed Core, never a project-provided executable.
export async function saveProgress(event, options = {}) {
  const root = await project(event.cwd);
  if (!root || !identity(event.session_id) || !identity(event.turn_id)) throw new Error('save_context_unavailable');
  const available = await withRecord(event, async ({ record }) => record?.turn_id === event.turn_id && record?.session_id === event.session_id);
  if (!available) throw new Error('turn_mismatch');
  const core = options.coreRoot || process.env.XIAOTAO_CORE_ROOT
    || path.join(process.env.XIAOTAO_HOME || homedir(), '.agents', 'skills', 'xiaotao');
  const script = path.join(core, 'scripts', 'save_progress.py');
  const python = options.python || process.env.XIAOTAO_PYTHON || (process.platform === 'win32' ? 'python' : 'python3');
  let result;
  try {
    const request = { ...event.progress, request_id: event.progress?.request_id || `stage-${digest(`${event.session_id}:${event.turn_id}`)}` };
    result = await new Promise((resolve, reject) => {
      const child = spawn(python, [script, '--project-root', root], { shell: false, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
      let stdout = '', stderr = '';
      const timer = setTimeout(() => { child.kill(); reject(new Error('save_timeout; retry the same request')); }, 30000);
      child.on('error', error => { clearTimeout(timer); reject(error); });
      const collect = (key, chunk) => {
        if (key === 'stdout') stdout += chunk; else stderr += chunk;
        if (Buffer.byteLength(stdout) + Buffer.byteLength(stderr) > LIMIT) { child.kill(); clearTimeout(timer); reject(new Error('save_output_budget_exceeded')); }
      };
      child.stdout.setEncoding('utf8'); child.stderr.setEncoding('utf8');
      child.stdout.on('data', chunk => collect('stdout', chunk));
      child.stderr.on('data', chunk => collect('stderr', chunk));
      child.stdin.on('error', () => {}); // A missing Python/script is reported by error/close.
      child.on('close', code => {
        clearTimeout(timer);
        try {
          const parsed = JSON.parse(stdout);
          if (code !== 0 && parsed.status !== 'failed') throw new Error(stderr || 'save_failed');
          resolve(parsed);
        } catch (error) { reject(new Error(stderr || error.message)); }
      });
      child.stdin.end(JSON.stringify(request));
    });
    if (!['saved', 'duplicate', 'unchanged'].includes(result.status)) throw new Error(result.error || 'save_failed');
    await withRecord(event, async ({ root, file, record }) => {
      if (record?.turn_id !== event.turn_id || record?.session_id !== event.session_id) throw new Error('turn_mismatch');
      // The writer has verified the original or unchanged commit. Check its current
      // bytes again instead of touching old files merely to freshen their timestamps.
      const checked = await savedReceipt(root, result.path, 0, result.work_paths || []);
      if (checked.sha256 !== result.daily_sha256 || checked.revision !== result.revision
        || (checked.work_states.length && checked.work_states[0].sha256 !== result.work_sha256)) throw new Error('save_changed_before_receipt');
      const receipt = { status: 'saved', ...checked, verified_at: Date.now() };
      await atomic(file, { ...record, receipt, pending_save: false });
    });
    return { ...result, receipt_status: 'saved' };
  } catch (error) {
    await acknowledge({ ...event, status: 'failed', reason: 'save-failed' }).catch(() => {});
    return { ...result, status: 'failed', error: error.message, receipt_status: 'failed' };
  }
}

export async function run(input = process.stdin, output = process.stdout, errors = process.stderr, ack = false, save = false) {
  try {
    if (process.env.XIAOTAO_CODEX_PROGRESS_CHECK === '0' && !save) { if (!ack) output.write('{}\n'); return; }
    let text = '';
    for await (const chunk of input) { text += chunk; if (Buffer.byteLength(text) > LIMIT) throw new Error('input_budget_exceeded'); }
    const result = save ? await saveProgress(JSON.parse(text)) : ack ? await acknowledge(JSON.parse(text)) : await progressContext(JSON.parse(text));
    output.write(`${JSON.stringify(result ?? {})}\n`);
    if (save && result.status === 'failed') process.exitCode = 1;
  } catch (error) {
    errors.write(`XiaoTao progress check skipped: ${error.message}\n`);
    output.write('{}\n');
    if (ack || save) process.exitCode = 1; // CLI callers must observe failures.
    // Hook failures never trap a turn. A failed ack remains missing.
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await run(undefined, undefined, undefined, process.argv[2] === '--ack', process.argv[2] === '--save');
