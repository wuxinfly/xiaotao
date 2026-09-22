import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { Readable, Writable } from 'node:stream';

import {
  buildEphemeralReminder,
  loadBoundedRuntimeContext,
  run,
  ScanBudgetError,
} from '../adapters/antigravity/xiaotao-antigravity/scripts/pre-invocation.mjs';
import {
  isReadOnlyToolList,
  mapDelegationToSubagent,
  sanitizeSubagentName,
} from '../adapters/antigravity/xiaotao-antigravity/scripts/subagent-bridge.mjs';
import { installLocal } from '../adapters/antigravity/install-local.mjs';

async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'xiaotao-ag-test-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  return root;
}

async function put(root, file, text) {
  const full = path.join(root, file);
  await mkdir(path.dirname(full), { recursive: true });
  await writeFile(full, text);
}

async function invokeHook(input) {
  let stdout = '';
  let stderr = '';
  const inStream = Readable.from([JSON.stringify(input)]);
  const outStream = new Writable({
    write(chunk, encoding, done) {
      stdout += chunk.toString();
      done();
    },
  });
  const errStream = new Writable({
    write(chunk, encoding, done) {
      stderr += chunk.toString();
      done();
    },
  });
  await run(inStream, outStream, errStream);
  return {
    stdout: stdout.trim() ? JSON.parse(stdout.trim()) : {},
    stderr,
  };
}

test('Antigravity PreInvocation hook ignores non-initial turns', async () => {
  const res = await invokeHook({
    invocationNum: 2,
    workspacePaths: ['/some/path'],
  });
  assert.deepEqual(res.stdout, {});
});

test('Antigravity PreInvocation hook ignores non-XiaoTao workspaces', async (t) => {
  const root = await fixture(t);
  const res = await invokeHook({
    invocationNum: 1,
    workspacePaths: [root],
  });
  assert.deepEqual(res.stdout, {});
});

test('Antigravity PreInvocation hook injects runtime context and Core Guard on first turn', async (t) => {
  const root = await fixture(t);
  await put(root, '.xiaotao/installation.json', JSON.stringify({ package: 'xiaotao-ai-workflow' }));
  await put(root, '.xiaotao/tasks/task-1/task.yaml', 'id: task-1\nstatus: active\nobjective: 优化首屏加载性能\n');
  await put(
    root,
    '.xiaotao/tasks/task-1/handoffs/handoff-1.json',
    JSON.stringify({
      status: 'completed',
      detailed_result: { summary: '完成性能瓶颈采样分析' },
    })
  );
  await put(
    root,
    '.xiaotao/checkpoints/chk-1.json',
    JSON.stringify({ request_id: 'chk-1', status: 'active', target_id: 'task-1' })
  );
  await put(root, '.xiaotao/memory/followups/pending/fu-1.yaml', 'id: fu-1\nstatus: pending\n');

  const res = await invokeHook({
    invocationNum: 1,
    workspacePaths: [root],
  });

  assert.ok(res.stdout.injectSteps, 'expected injectSteps');
  assert.equal(res.stdout.injectSteps.length, 1);
  const msg = res.stdout.injectSteps[0].ephemeralMessage;
  assert.match(msg, /小涛 \(XiaoTao\) 运行时锚点与 Core Guard/);
  assert.match(msg, /task-1\(优化首屏加载性能\)/);
  assert.match(msg, /完成性能瓶颈采样分析/);
  assert.match(msg, /可恢复检查点: 1 个/);
  assert.match(msg, /待跟进事项: 1 个/);
  assert.match(msg, /enable_write_tools: false/);

  // Check token budget (approximate tokens by chars / 3)
  assert.ok(msg.length < 1500, `reminder too long: ${msg.length} chars`);
});

test('Antigravity PreInvocation hook gracefully handles scan budget exceeded', async (t) => {
  const root = await fixture(t);
  await put(root, '.xiaotao/installation.json', JSON.stringify({ package: 'xiaotao-ai-workflow' }));

  // Exceed MAX_METADATA_BYTES (16KB)
  const hugeText = 'id: huge-task\nstatus: active\nobjective: ' + 'x'.repeat(20 * 1024) + '\n';
  await put(root, '.xiaotao/tasks/huge/task.yaml', hugeText);

  const res = await invokeHook({
    invocationNum: 1,
    workspacePaths: [root],
  });

  assert.ok(res.stdout.injectSteps);
  const msg = res.stdout.injectSteps[0].ephemeralMessage;
  assert.match(msg, /超出扫描预算，已安全降级为 Bare Core 模式/);
});

test('Subagent Bridge correctly enforces read-only tool isolation', () => {
  assert.equal(isReadOnlyToolList(['view_file', 'grep_search', 'find_by_name']), true);
  assert.equal(isReadOnlyToolList(['view_file', 'write_to_file']), false);
  assert.equal(isReadOnlyToolList(['run_command']), false);

  const readOnlyPacket = {
    worker_id: 'code-scout',
    objective: '调查代码调用链',
    effective_tools: ['view_file', 'grep_search'],
    effective_instructions: [{ id: 'practice:investigation' }],
  };
  const readOnlySpec = {
    id: 'code-scout',
    display_name: '代码调查员',
  };

  const mapped = mapDelegationToSubagent(readOnlyPacket, readOnlySpec);
  assert.equal(mapped.toolIsolationEnforced, true);
  assert.equal(mapped.defineSubagentArgs.enable_write_tools, false);
  assert.equal(mapped.defineSubagentArgs.name, 'code-scout');
  assert.equal(mapped.defineSubagentArgs.description, '代码调查员');
  assert.match(mapped.defineSubagentArgs.system_prompt, /宿主已关闭写权限/);
  assert.equal(mapped.invokeSubagentArgs.Role, '代码调查员');
  assert.equal(mapped.invokeSubagentArgs.Prompt, '调查代码调用链');

  // Execution worker
  const execPacket = {
    worker_id: 'builder',
    objective: '实现特性',
    effective_tools: ['write_to_file', 'run_command'],
  };
  const execMapped = mapDelegationToSubagent(execPacket, { display_name: '代码构建员' });
  assert.equal(execMapped.toolIsolationEnforced, false);
  assert.equal(execMapped.defineSubagentArgs.enable_write_tools, true);
});

test('Subagent Bridge sanitizes worker names', () => {
  assert.equal(sanitizeSubagentName('My Worker #1'), 'my_worker__1');
  assert.equal(sanitizeSubagentName('scout.v2'), 'scout.v2');
  assert.equal(sanitizeSubagentName(''), 'xiaotao_worker');
});

test('installLocal successfully writes Antigravity plugin and Core Skill', async (t) => {
  const home = await fixture(t);
  const res = await installLocal({ homeDir: home });
  assert.ok(res.targetDir.includes('xiaotao-antigravity'));

  const manifest = JSON.parse(await readFile(path.join(res.targetDir, 'plugin.json'), 'utf8'));
  assert.equal(manifest.name, 'xiaotao-antigravity');

  const hooks = JSON.parse(await readFile(path.join(res.targetDir, 'hooks.json'), 'utf8'));
  assert.ok(hooks['xiaotao-recovery'].PreInvocation);

  const marker = JSON.parse(await readFile(path.join(res.targetDir, '.xiaotao-source.json'), 'utf8'));
  assert.equal(marker.owner, 'xiaotao-ai-workflow/antigravity-adapter');

  const skillContent = await readFile(path.join(res.targetDir, 'skills/xiaotao/SKILL.md'), 'utf8');
  assert.match(skillContent, /name: xiaotao/);
});
