import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { doctorInstallation, installClaude } from '../cli/install.js';
import { handle } from '../adapters/claude-code/xiaotao-claude/hooks/context.mjs';
import { acknowledge, progressContext } from '../adapters/claude-code/xiaotao-claude/scripts/progress-check.mjs';
import { run } from '../cli/index.js';
import { PassThrough } from 'node:stream';

const packageRoot = path.resolve(import.meta.dirname, '..');

async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-claude-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const projectRoot = path.join(root, 'project'); await mkdir(projectRoot);
  return { root, projectRoot, packageRoot, environment: { XIAOTAO_HOME: path.join(root, 'home') } };
}

function terminal() {
  const input = new PassThrough(); const output = new PassThrough(); const error = new PassThrough();
  let stdout = ''; let stderr = '';
  output.on('data', chunk => { stdout += chunk; }); error.on('data', chunk => { stderr += chunk; });
  return { input, output, error, get stdout() { return stdout; }, get stderr() { return stderr; } };
}

test('Claude Code prepares a local marketplace, skill, and doctor files', async t => {
  const c = await fixture(t);
  const result = await installClaude(c);
  assert.ok(result.checks.every(check => check.ok), JSON.stringify(result.checks));
  const market = JSON.parse(await readFile(result.marketplace, 'utf8'));
  assert.equal(market.plugins[0].name, 'xiaotao-claude');
  const plugin = path.join(result.marketplaceRoot, market.plugins[0].source);
  const manifest = JSON.parse(await readFile(path.join(plugin, '.claude-plugin/plugin.json'), 'utf8'));
  assert.equal(manifest.version, result.version);
  const hooks = JSON.parse(await readFile(path.join(plugin, 'hooks/hooks.json'), 'utf8'));
  assert.deepEqual(Object.keys(hooks.hooks), ['SessionStart', 'UserPromptSubmit', 'Stop', 'PreCompact']);
  assert.match(await readFile(path.join(plugin, 'agents/xiaotao-memory-worker.md'), 'utf8'), /tools: Read, Grep, Glob/);
  assert.match(await readFile(path.join(c.environment.XIAOTAO_HOME, '.claude/skills/xiaotao/SKILL.md'), 'utf8'), /name: xiaotao/);
  assert.ok((await doctorInstallation(c.projectRoot, c.environment)).checks.some(check => check.code === 'claude_activation_unverified'));

  await rm(path.join(plugin, 'hooks/context.mjs'));
  assert.equal((await doctorInstallation(c.projectRoot, c.environment)).ok, false);
  await installClaude(c);
  assert.equal((await doctorInstallation(c.projectRoot, c.environment)).ok, true);
});

test('Claude hooks stay quiet outside enrolled projects and inject the session rules', async t => {
  const c = await fixture(t);
  assert.deepEqual(await handle({ cwd: c.projectRoot, hook_event_name: 'SessionStart' }), {});
  await installClaude(c);
  const started = await handle({ cwd: c.projectRoot, hook_event_name: 'SessionStart', source: 'startup' });
  assert.match(started.hookSpecificOutput.additionalContext, /save_progress\.py/);
  assert.match(started.hookSpecificOutput.additionalContext, /xiaotao-claude:xiaotao-memory-worker/);
  assert.deepEqual(await handle({ cwd: c.projectRoot, hook_event_name: 'Stop' }), {});
  const malformed = spawnSync(process.execPath, [path.join(packageRoot, 'adapters/claude-code/xiaotao-claude/hooks/context.mjs')], { input: 'bad json', encoding: 'utf8' });
  assert.equal(malformed.status, 0); assert.deepEqual(JSON.parse(malformed.stdout), {});
});

test('Claude progress hooks open a turn, block once, and respect stop_hook_active', async t => {
  const c = await fixture(t);
  await installClaude(c);
  const opened = await progressContext({ cwd: c.projectRoot, hook_event_name: 'UserPromptSubmit', session_id: 'session-a', prompt: 'implement it' });
  assert.match(opened.hookSpecificOutput.additionalContext, /--save/);
  assert.match(opened.hookSpecificOutput.additionalContext, /turn-/);
  const blocked = await progressContext({ cwd: c.projectRoot, hook_event_name: 'Stop', session_id: 'session-a', stop_hook_active: false });
  assert.equal(blocked.decision, 'block');
  assert.match(blocked.reason, /--ack/);
  const reminded = await progressContext({ cwd: c.projectRoot, hook_event_name: 'Stop', session_id: 'session-a', stop_hook_active: false });
  assert.match(reminded.systemMessage, /已提醒一次/);
  assert.equal(await progressContext({ cwd: c.projectRoot, hook_event_name: 'Stop', session_id: 'session-a', stop_hook_active: true }), null);
  assert.equal(await progressContext({ cwd: c.projectRoot, hook_event_name: 'Stop', session_id: 'session-b' }), null);
  const compact = await progressContext({ cwd: c.projectRoot, hook_event_name: 'PreCompact', session_id: 'session-a', trigger: 'manual' });
  assert.equal(compact.decision, 'block');
  assert.match(compact.reason, /保存回执/);
  assert.equal(await progressContext({ cwd: c.projectRoot, hook_event_name: 'PreCompact', session_id: 'session-a', trigger: 'manual' }), null);
  assert.equal(await progressContext({ cwd: c.projectRoot, hook_event_name: 'PreCompact', session_id: 'session-a', trigger: 'auto' }), null);
});

const today = new Date().toISOString().slice(0, 10);
const dailyTarget = `.xiaotao/memory/timeline/years/${today.replaceAll('-', '/')}/details/topic.md`;

async function put(root, file, content) {
  await mkdir(path.dirname(path.join(root, file)), { recursive: true });
  await writeFile(path.join(root, file), content);
}

async function saveDaily(root, revision = 0) {
  const updated = new Date().toISOString();
  const record = {
    schema_version: 1, record_id: `daily-${today}-topic`, topic_id: 'topic', date: today, timezone: 'UTC',
    title: '本轮工作', summary: '已改，尚未验证', details: '', source_refs: [], revision,
    updated_at: updated, updated_by: 'xiao-tao/test', occurred_at: updated,
  };
  const front = `---\nrevision: ${revision}\nupdated_at: ${updated}\nupdated_by: xiao-tao/test\n---\n`;
  await put(root, dailyTarget, `${front}\n\`\`\`xiaotao-daily-record\n${JSON.stringify(record)}\n\`\`\`\n`);
}

test('saved receipt is verified by file bytes and a later change blocks again', async t => {
  const c = await fixture(t); await installClaude(c);
  const event = { cwd: c.projectRoot, session_id: 'session-a', hook_event_name: 'UserPromptSubmit', prompt: 'finish it' };
  await progressContext(event);
  await saveDaily(c.projectRoot);
  await acknowledge({ ...event, status: 'saved', path: dailyTarget });
  assert.equal(await progressContext({ ...event, hook_event_name: 'Stop', stop_hook_active: false }), null);
  await saveDaily(c.projectRoot, 1);
  assert.equal((await progressContext({ ...event, hook_event_name: 'Stop', stop_hook_active: false })).decision, 'block');
});

test('skip and failed receipts control the reminder without writing project memory', async t => {
  const c = await fixture(t); await installClaude(c);
  const event = { cwd: c.projectRoot, session_id: 'session-a', hook_event_name: 'UserPromptSubmit', prompt: 'chat only' };
  await progressContext(event);
  await acknowledge({ ...event, status: 'skipped', reason: 'no-new-facts' });
  assert.equal(await progressContext({ ...event, hook_event_name: 'Stop', stop_hook_active: false }), null);
  await progressContext({ ...event, prompt: 'next turn' });
  await acknowledge({ ...event, status: 'failed', reason: 'save-failed' });
  assert.match((await progressContext({ ...event, hook_event_name: 'Stop', stop_hook_active: false })).systemMessage, /未保存/);
  await assert.rejects(acknowledge({ ...event, status: 'skipped', reason: 'arbitrary' }), /invalid_reason/);
});

test('installed hook manifest runs the exec-form command with a plugin root', async t => {
  const c = await fixture(t);
  const result = await installClaude(c);
  const hooks = JSON.parse(await readFile(path.join(result.pluginDir, 'hooks/hooks.json'), 'utf8'));
  const handler = hooks.hooks.UserPromptSubmit[0].hooks[0];
  assert.equal(handler.command, 'node');
  const args = handler.args.map(arg => arg.replace('${CLAUDE_PLUGIN_ROOT}', result.pluginDir));
  const output = execFileSync(process.execPath, args, {
    input: JSON.stringify({ cwd: c.projectRoot, session_id: 'session-a', hook_event_name: 'UserPromptSubmit', prompt: 'run' }),
    encoding: 'utf8',
  });
  assert.equal(JSON.parse(output).hookSpecificOutput.hookEventName, 'UserPromptSubmit');
});

test('xiaotao claude prepares the marketplace and supports JSON output', async t => {
  const c = await fixture(t);
  const io = terminal();
  assert.equal(await run(['claude', c.projectRoot], { ...io, environment: c.environment }), 0);
  assert.match(io.stdout, /Claude Code 插件来源/);
  assert.match(io.stdout, /\/plugin marketplace add/);
  const json = terminal();
  assert.equal(await run(['claude', c.projectRoot, '--json'], { ...json, environment: c.environment }), 0);
  assert.equal(JSON.parse(json.stdout).ok, true);
  const unsupported = terminal();
  assert.equal(await run(['init', '--json'], { ...unsupported, environment: c.environment }), 2);
  assert.match(unsupported.stderr, /--json is only valid/);
  assert.equal((await doctorInstallation(c.projectRoot, c.environment)).ok, true);
});