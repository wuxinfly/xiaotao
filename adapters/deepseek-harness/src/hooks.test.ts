import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtemp, rm, writeFile, mkdir, readFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import {
  AutoCheckpointCoordinator,
  registerLifecycleHooks,
  loadBoundedRuntimeContext,
  injectSessionRuntimeContext,
} from './hooks'
import type { LifecycleHandlers, TurnStopPayload } from './hooks'

interface ListenerRecord {
  name: string
  listener: (payload: unknown) => unknown
  options: { global?: boolean }
}

/** Minimal fake Cordis context capturing `ctx.on` calls and log warnings. */
function makeCtx() {
  const listeners: ListenerRecord[] = []
  const warnings: unknown[][] = []
  return {
    on(name: string, listener: (payload: unknown) => unknown, options?: { global?: boolean }) {
      listeners.push({ name, listener, options: options ?? {} })
      return () => false
    },
    logger: {
      warn: (...args: unknown[]) => {
        warnings.push(args)
      },
    },
    listeners,
    warnings,
  }
}

const payload: TurnStopPayload = { agent: {} as never, turn: 1, signal: new AbortController().signal }

function buildCatalog(projectRoot: string): void {
  const script = path.resolve(process.cwd(), '../../xiaotao/scripts/memory_catalog.py')
  const candidates = process.platform === 'win32' ? ['python', 'py', 'python3'] : ['python3', 'python']
  for (const python of candidates) {
    const result = spawnSync(python, [script, '--project-root', projectRoot, 'build'], {
      encoding: 'utf8',
      windowsHide: true,
    })
    if (result.status === 0) return
  }
  throw new Error('test fixture could not build Memory Catalog')
}

function autoFixture(contextWindow = 1000) {
  const events: any[] = [
    { seq: 0, type: 'user/message', data: { source: { kind: 'user' } } },
    { seq: 1, type: 'assistant/message', data: { usage: { inputTokens: 750, outputTokens: 100 } } },
  ]
  const steered: any[] = []
  const injected: any[] = []
  const agent = {
    session: { header: { cwd: process.cwd() }, events, requestContext: () => ({ contextWindow }) },
    steer(message: any) {
      steered.push(message)
      events.push({ seq: events.length, type: 'user/message', data: message })
    },
    inject(message: any) {
      injected.push(message)
    },
  }
  return { events, steered, injected, agent: agent as never }
}

test('registerLifecycleHooks registers nothing when no handler is given', () => {
  const ctx = makeCtx()
  const handlers: LifecycleHandlers = {}
  registerLifecycleHooks(ctx as never, handlers)
  assert.equal(ctx.listeners.length, 0)
})

test('registerLifecycleHooks listens on agent/turn-stopping with global scope', () => {
  const ctx = makeCtx()
  registerLifecycleHooks(ctx as never, { onTurnStopping: () => {} })
  assert.equal(ctx.listeners.length, 1)
  assert.equal(ctx.listeners[0].name, 'agent/turn-stopping')
  assert.equal(ctx.listeners[0].options.global, true)
})

test('registerLifecycleHooks listens on agent/session-start with global scope', () => {
  const ctx = makeCtx()
  let called = false
  registerLifecycleHooks(ctx as never, {
    onSessionStart: () => {
      called = true
    },
  })
  assert.equal(ctx.listeners.length, 1)
  assert.equal(ctx.listeners[0].name, 'agent/session-start')
  assert.equal(ctx.listeners[0].options.global, true)
  ctx.listeners[0].listener({ agent: {} as never })
  assert.equal(called, true)
})

test('the listener returns a promise that is resolved only after the handler settles', async () => {
  const ctx = makeCtx()
  let finished = false
  registerLifecycleHooks(ctx as never, {
    onTurnStopping: async () => {
      await new Promise((r) => setTimeout(r, 20))
      finished = true
    },
  })
  const returned = ctx.listeners[0].listener(payload)
  assert.ok(returned instanceof Promise, 'listener must return a Promise for the serial dispatcher to await')
  assert.equal(finished, false, 'handler must not have completed synchronously')
  await returned
  assert.equal(finished, true)
})

test('a rejecting handler is swallowed and logged, not re-thrown', async () => {
  const ctx = makeCtx()
  registerLifecycleHooks(ctx as never, {
    onTurnStopping: async () => {
      throw new Error('boom')
    },
  })
  const returned = ctx.listeners[0].listener(payload)
  await returned // must resolve, not reject
  assert.equal(ctx.warnings.length, 1)
  assert.match(String(ctx.warnings[0][0]), /onTurnStopping listener failed/)
})

test('a synchronously throwing handler is swallowed and logged', async () => {
  const ctx = makeCtx()
  registerLifecycleHooks(ctx as never, {
    onTurnStopping: () => {
      throw new Error('sync boom')
    },
  })
  await ctx.listeners[0].listener(payload)
  assert.equal(ctx.warnings.length, 1)
  assert.match(String(ctx.warnings[0][0]), /onTurnStopping listener failed/)
})

test('the serial lifecycle wait is bounded and aborts the handler signal', async () => {
  const ctx = makeCtx()
  let aborted = false
  registerLifecycleHooks(ctx as never, {
    onTurnStopping: ({ signal }) => new Promise<void>(() => {
      signal.addEventListener('abort', () => { aborted = true })
    }),
  }, { timeoutMs: 50 })
  const started = Date.now()
  await ctx.listeners[0].listener(payload)
  assert.ok(Date.now() - started < 500)
  assert.equal(aborted, true)
  assert.match(String(ctx.warnings[0][0]), /timed out after 50ms/)
})

test('automatic checkpoint triggers from measured pressure and records a bounded plugin prompt', () => {
  const f = autoFixture()
  const coordinator = new AutoCheckpointCoordinator({ pressureThreshold: 0.7 })
  const decision = coordinator.evaluateAndTrigger({ agent: f.agent, turn: 3,
    signal: new AbortController().signal })
  assert.equal(decision, 'triggered')
  assert.equal(f.steered.length, 1)
  assert.equal(f.steered[0].source.plugin, 'xiaotao-auto-checkpoint/turn-3')
  assert.match(f.steered[0].content[0].text, /不是 pre-compaction/)
  assert.match(f.steered[0].content[0].text, /先 inspect/)
})

test('automatic checkpoint stays quiet without measurable pressure or below threshold', () => {
  const missing = autoFixture(0)
  const coordinator = new AutoCheckpointCoordinator({ pressureThreshold: 0.8 })
  assert.equal(coordinator.evaluateAndTrigger({ agent: missing.agent, turn: 1,
    signal: new AbortController().signal }), 'pressure-unknown')
  const low = autoFixture(2000)
  assert.equal(coordinator.evaluateAndTrigger({ agent: low.agent, turn: 1,
    signal: new AbortController().signal }), 'below-threshold')
  assert.equal(missing.steered.length + low.steered.length, 0)
})

test('automatic checkpoint prefers DSH projected pressure and excludes output in usage fallback', () => {
  const projected = autoFixture(10_000)
  const projectedAgent = projected.agent as unknown as { ctx?: unknown }
  projectedAgent.ctx = { get: (name: string) => name === 'sessionProjections' ? {
    snapshot: () => ({ values: { contextPressure: { projectedTokens: 850, contextWindow: 1000 } } }),
  } : undefined } as never
  const coordinator = new AutoCheckpointCoordinator({ pressureThreshold: 0.8 })
  assert.equal(coordinator.evaluateAndTrigger({ agent: projected.agent, turn: 1,
    signal: new AbortController().signal }), 'triggered')

  const fallback = autoFixture(1000)
  fallback.events[1].data.usage = { inputTokens: 650, outputTokens: 500 }
  assert.equal(coordinator.evaluateAndTrigger({ agent: fallback.agent, turn: 1,
    signal: new AbortController().signal }), 'below-threshold')
})

test('automatic checkpoint deduplicates a trigger and observes cooldown after commit', () => {
  const f = autoFixture()
  const coordinator = new AutoCheckpointCoordinator({ pressureThreshold: 0.7, cooldownTurns: 2 })
  assert.equal(coordinator.evaluateAndTrigger({ agent: f.agent, turn: 3,
    signal: new AbortController().signal }), 'triggered')
  assert.equal(coordinator.evaluateAndTrigger({ agent: f.agent, turn: 3,
    signal: new AbortController().signal }), 'no-new-progress')

  f.events.push({ seq: f.events.length, type: 'tool/call', data: { turn: 3, callId: 'checkpoint-1',
    name: 'xiaotao_checkpoint', arguments: JSON.stringify({ operation: 'save' }) } })
  f.events.push({ seq: f.events.length, type: 'tool/result', data: { message: { content: [{
    type: 'tool-result', toolCallId: 'checkpoint-1', content: [{ type: 'text', text: '{"status":"committed"}' }],
  }] } } })
  f.events.push({ seq: f.events.length, type: 'user/message', data: { source: { kind: 'user' } } })
  f.events.push({ seq: f.events.length, type: 'assistant/message', data: {
    usage: { inputTokens: 700, outputTokens: 80 } } })
  assert.equal(coordinator.evaluateAndTrigger({ agent: f.agent, turn: 4,
    signal: new AbortController().signal }), 'cooldown')

  f.events.push({ seq: f.events.length, type: 'user/message', data: { source: { kind: 'user' } } })
  assert.equal(coordinator.evaluateAndTrigger({ agent: f.agent, turn: 5,
    signal: new AbortController().signal }), 'triggered')
  assert.equal(f.steered.length, 2)
})

test('automatic checkpoint rejects unsafe trigger configuration', () => {
  assert.throws(() => new AutoCheckpointCoordinator({ pressureThreshold: 0.99 }), /pressureThreshold/)
  assert.throws(() => new AutoCheckpointCoordinator({ cooldownTurns: 0 }), /cooldownTurns/)
  const f = autoFixture()
  const controller = new AbortController()
  controller.abort()
  assert.equal(new AutoCheckpointCoordinator().evaluateAndTrigger({ agent: f.agent, turn: 1,
    signal: controller.signal }), 'cancelled')
})

test('loadBoundedRuntimeContext returns null when no active work exists', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-empty-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  // No index file
  assert.equal(await loadBoundedRuntimeContext(tmp), null)

  // Empty entries
  await mkdir(path.join(tmp, '.xiaotao/memory'), { recursive: true })
  await writeFile(path.join(tmp, '.xiaotao/memory/index.json'), JSON.stringify({
    schema_version: 1,
    entries: [],
    pending_followups: [],
  }))
  assert.equal(await loadBoundedRuntimeContext(tmp), null)

  // Orphaned index.json without any authoritative sources on disk
  await writeFile(path.join(tmp, '.xiaotao/memory/index.json'), JSON.stringify({
    schema_version: 1,
    entries: [
      { memory_id: 'task-orphan', title: 'Orphan task', record_type: 'task', status: 'active' },
    ],
    pending_followups: [],
  }))
  assert.equal(await loadBoundedRuntimeContext(tmp), null)
})

test('loadBoundedRuntimeContext returns bounded runtime context when active work exists', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-active-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  const task1 = path.join(tmp, '.xiaotao/tasks/task-1')
  const task2 = path.join(tmp, '.xiaotao/tasks/task-2')
  const task3 = path.join(tmp, '.xiaotao/tasks/task-3')
  const task4 = path.join(tmp, '.xiaotao/tasks/task-4')
  const temp1 = path.join(tmp, '.xiaotao/memory/temporary/active/temp-1')
  const ltDir = path.join(tmp, '.xiaotao/memory/long-term/entries')
  const fuDir = path.join(tmp, '.xiaotao/memory/followups/pending')

  await mkdir(task1, { recursive: true })
  await mkdir(task2, { recursive: true })
  await mkdir(task3, { recursive: true })
  await mkdir(task4, { recursive: true })
  await mkdir(temp1, { recursive: true })
  await mkdir(ltDir, { recursive: true })
  await mkdir(fuDir, { recursive: true })

  const taskYaml = (id: string, objective: string) => `id: ${id}
objective: ${objective}
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`
  await writeFile(path.join(task1, 'task.yaml'), taskYaml('task-1', 'Task 1'))
  await writeFile(path.join(task2, 'task.yaml'), taskYaml('task-2', 'Task 2'))
  await writeFile(path.join(task3, 'task.yaml'), taskYaml('task-3', 'Task 3'))
  await writeFile(path.join(task4, 'task.yaml'), taskYaml('task-4', 'Task 4'))
  await writeFile(path.join(temp1, 'meta.yaml'), `id: temp-1
topic: Temporary 1
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`)
  await writeFile(path.join(ltDir, 'lt-1.md'), `---
revision: 1
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
---

\`\`\`xiaotao-memory-entry
{"entry_id":"lt-1","title":"Long-term 1","memory_kind":"principle","content":"Rule","source_refs":[".xiaotao/tasks/task-1/task.yaml"],"status":"active"}
\`\`\`
`)
  await writeFile(path.join(fuDir, 'f-1.yaml'), `followup_id: f-1
title: Follow-up 1
status: pending
created_at: 2026-09-12T10:00:00Z
source_refs:
  - .xiaotao/tasks/task-1/task.yaml
`)
  buildCatalog(tmp)

  const result = await loadBoundedRuntimeContext(tmp)
  assert.ok(result)
  assert.match(result, /# Memory Overview \(Runtime Context\)/)
  assert.match(result, /## Active Tasks \(4\)/)
  assert.match(result, /task-1/)
  assert.match(result, /task-3/)
  assert.match(result, /另外 1 项活动任务已省略/)
  assert.match(result, /## Active Temporary Memory \(1\)/)
  assert.match(result, /temp-1/)
  assert.match(result, /## Pending Follow-ups \(1\)/)
  assert.match(result, /f-1/)
  assert.match(result, /## Long-term Memory \(1 项已索引\)/)
})

test('injectSessionRuntimeContext queues non-waking bounded context on session start', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-inject-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  const taskDir = path.join(tmp, '.xiaotao/tasks/task-active')
  await mkdir(taskDir, { recursive: true })
  await writeFile(path.join(taskDir, 'task.yaml'), `id: task-active
objective: Work in progress
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`)
  buildCatalog(tmp)

  const f = autoFixture()
  ;(f.agent as unknown as { session: { header: { cwd: string } } }).session.header.cwd = tmp
  const injected = await injectSessionRuntimeContext({ agent: f.agent })
  assert.equal(injected, true)
  assert.equal(f.steered.length, 0, 'session startup must not wake an idle agent')
  assert.equal(f.injected.length, 1)
  const injectedMsg = f.injected[0]
  assert.equal(injectedMsg.source?.kind, 'plugin')
  assert.equal(injectedMsg.source?.plugin, 'xiaotao-runtime-context')
  assert.match(injectedMsg.content[0].text, /task-active/)
  assert.match(injectedMsg.content[0].text, /Work in progress/)
  assert.match(injectedMsg.content[0].text, /## XiaoTao 工作规则/)
  assert.match(injectedMsg.content[0].text, /唯一身份是“小涛”/)
  assert.match(injectedMsg.content[0].text, /不要自称执行者、Worker、Agent 或 XiaoTao 工作流/)
  assert.match(injectedMsg.content[0].text, /必须先加载 XiaoTao Skill/)
  assert.match(injectedMsg.content[0].text, /不得因为可以搜索代码而跳过记忆搜索/)
  assert.match(injectedMsg.content[0].text, /最多 `show` 3 条相关记忆/)
})

test('injectSessionRuntimeContext binds each session to its own project cwd', async (t) => {
  const hostRoot = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-host-'))
  const projectA = path.join(hostRoot, 'project-a')
  const projectB = path.join(hostRoot, 'project-b')
  t.after(() => rm(hostRoot, { recursive: true, force: true }))

  for (const [root, id] of [[projectA, 'task-a'], [projectB, 'task-b']] as const) {
    const taskDir = path.join(root, '.xiaotao/tasks', id)
    await mkdir(taskDir, { recursive: true })
    await writeFile(path.join(taskDir, 'task.yaml'), `id: ${id}\nobjective: ${id}\nstatus: active\ncreated_at: 2026-09-14T00:00:00Z\nupdated_at: 2026-09-14T00:00:00Z\nupdated_by: old-zhou/test\nrevision: 1\n`)
    buildCatalog(root)
  }

  const sessionA = autoFixture()
  ;(sessionA.agent as unknown as { session: { header: { cwd: string } } }).session.header.cwd = projectA
  const sessionB = autoFixture()
  ;(sessionB.agent as unknown as { session: { header: { cwd: string } } }).session.header.cwd = projectB

  assert.equal(await injectSessionRuntimeContext({ agent: sessionA.agent }), true)
  assert.equal(await injectSessionRuntimeContext({ agent: sessionB.agent }), true)
  assert.equal(sessionA.steered.length + sessionB.steered.length, 0)
  assert.match(sessionA.injected[0].content[0].text, /task-a/)
  assert.doesNotMatch(sessionA.injected[0].content[0].text, /task-b/)
  assert.match(sessionB.injected[0].content[0].text, /task-b/)
  assert.doesNotMatch(sessionB.injected[0].content[0].text, /task-a/)
})

test('injectSessionRuntimeContext does not fall back to the DSH launch cwd', async () => {
  const f = autoFixture()
  ;(f.agent as unknown as { session: { header: { cwd?: string } } }).session.header.cwd = undefined
  assert.equal(await injectSessionRuntimeContext({ agent: f.agent }), false)
  assert.equal(f.steered.length, 0)
  assert.equal(f.injected.length, 0)
})

test('injectSessionRuntimeContext still injects memory-first routing when XiaoTao has no active work', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-routing-only-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))
  await mkdir(path.join(tmp, '.xiaotao'), { recursive: true })

  const f = autoFixture()
  ;(f.agent as unknown as { session: { header: { cwd: string } } }).session.header.cwd = tmp
  assert.equal(await injectSessionRuntimeContext({ agent: f.agent }), true)
  assert.equal(f.steered.length, 0)
  assert.equal(f.injected.length, 1)
  assert.match(f.injected[0].content[0].text, /## XiaoTao 工作规则/)
  assert.match(f.injected[0].content[0].text, /唯一身份是“小涛”/)
  assert.doesNotMatch(f.injected[0].content[0].text, /# Memory Overview/)
})

test('loadBoundedRuntimeContext discovers recoverable checkpoint and returns context even without active tasks', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-checkpoint-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  // Checkpoint file under .xiaotao/tasks/task-chk/references/checkpoints/req-chk-1.json
  const checkpointPayload = {
    schema_version: 1,
    request_id: 'req-chk-1',
    project: 'test-project',
    kind: 'task',
    target_id: 'task-chk',
    session_id: 'session-dsh',
    base_revision: 1,
    base_hash: 'a'.repeat(64),
    input_hash: 'b'.repeat(64),
    source_hash: 'c'.repeat(64),
    proposal: 'proposal content',
    proposal_hash: 'd'.repeat(64),
    snapshot: {
      objective: 'Suspended task checkpoint',
      confirmed: ['step 1 done'],
      rejected: [],
      in_progress: ['step 2'],
      next: ['step 3'],
      open_questions: [],
      source_refs: ['.xiaotao/tasks/task-chk/progress.md'],
    },
  }
  const chkDir = path.join(tmp, '.xiaotao/tasks/task-chk/references/checkpoints')
  await mkdir(chkDir, { recursive: true })
  await writeFile(path.join(chkDir, 'req-chk-1.json'), JSON.stringify(checkpointPayload, null, 2))

  // 1. Pending request: status: pending and proposed_revision: 2
  const result = await loadBoundedRuntimeContext(tmp)
  assert.ok(result)
  assert.match(result, /# Memory Overview \(Runtime Context\)/)
  assert.match(result, /当前检测到项目存在活动工作/)
  assert.match(result, /## Recoverable Checkpoint/)
  assert.match(result, /Recoverable checkpoint: yes/)
  assert.match(result, /status: pending/)
  assert.match(result, /scope: task/)
  assert.match(result, /binding: task-chk/)
  assert.match(result, /proposed_revision: 2/)

  // 2. Verified committed checkpoint: status: committed and revision: 2
  await writeFile(path.join(chkDir, 'req-chk-1.committed.json'), JSON.stringify({
    request_id: 'req-chk-1',
    revision: 2,
  }))

  const committedResult = await loadBoundedRuntimeContext(tmp)
  assert.ok(committedResult)
  assert.match(committedResult, /## Recoverable Checkpoint/)
  assert.match(committedResult, /status: committed/)
  assert.match(committedResult, /revision: 2/)
  assert.doesNotMatch(committedResult, /proposed_revision/)
})

test('loadBoundedRuntimeContext does not rebuild a missing index during session start', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-rebuild-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  const taskDir = path.join(tmp, '.xiaotao/tasks/task-live')
  await mkdir(taskDir, { recursive: true })
  await writeFile(path.join(taskDir, 'task.yaml'), `id: task-live
objective: Live active task
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`)

  const result = await loadBoundedRuntimeContext(tmp)
  assert.ok(result)
  assert.match(result, /# Memory Overview \(Runtime Context\)/)
  assert.match(result, /当前检测到项目存在活动工作/)
  assert.match(result, /启动时没有可用的 Catalog 快照/)
  assert.match(result, /## Active Tasks \(0\)/)

  // SessionStart must not create the derived index.
  const indexPath = path.join(tmp, '.xiaotao/memory/index.json')
  await assert.rejects(readFile(indexPath, 'utf8'), /ENOENT/)
})

test('loadBoundedRuntimeContext uses a valid cached snapshot without scanning new sources', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-stale-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  const oldTaskDir = path.join(tmp, '.xiaotao/tasks/task-cached')
  await mkdir(oldTaskDir, { recursive: true })
  await writeFile(path.join(oldTaskDir, 'task.yaml'), `id: task-cached
objective: Cached active task
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`)
  buildCatalog(tmp)

  const newTaskDir = path.join(tmp, '.xiaotao/tasks/task-live-new')
  await mkdir(newTaskDir, { recursive: true })
  await writeFile(path.join(newTaskDir, 'task.yaml'), `id: task-live-new
objective: Brand new active task
status: active
created_at: 2026-09-12T11:00:00Z
updated_at: 2026-09-12T11:00:00Z
updated_by: old-zhou/test
revision: 1
`)

  const result = await loadBoundedRuntimeContext(tmp)
  assert.ok(result)
  assert.match(result, /启动时使用现有 Catalog 快照/)
  assert.match(result, /task-cached/)
  assert.doesNotMatch(result, /task-live-new/)

  const cached = JSON.parse(await readFile(path.join(tmp, '.xiaotao/memory/index.json'), 'utf8'))
  assert.ok(cached.entries.some((e: any) => e.memory_id === 'task-cached'))
  assert.ok(!cached.entries.some((e: any) => e.memory_id === 'task-live-new'))
})

test('loadBoundedRuntimeContext with fs still avoids rebuilding during session start', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-fs-rebuild-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  const taskDir = path.join(tmp, '.xiaotao/tasks/task-with-fs')
  await mkdir(taskDir, { recursive: true })
  await writeFile(path.join(taskDir, 'task.yaml'), `id: task-with-fs
objective: Active task tested with fs parameter
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`)

  // Pass dummy fs object
  const dummyFs: any = {
    async resolve(p: string) { return { targetKey: p } },
    async stat() { return undefined },
    async readText() { return '' },
    async writeText() { return { targetKey: '' } },
  }

  const result = await loadBoundedRuntimeContext(tmp, dummyFs)
  assert.ok(result)
  assert.match(result, /# Memory Overview \(Runtime Context\)/)
  assert.match(result, /当前检测到项目存在活动工作/)
  assert.match(result, /启动时没有可用的 Catalog 快照/)

  // The optional fs service does not permit startup to create the index.
  const indexPath = path.join(tmp, '.xiaotao/memory/index.json')
  await assert.rejects(readFile(indexPath, 'utf8'), /ENOENT/)
})

test('loadBoundedRuntimeContext does not consume stale index when refresh/rebuild fails and outputs degraded warning', async (t) => {
  const tmp = await mkdtemp(path.join(os.tmpdir(), 'dsh-runtime-context-fail-deg-'))
  t.after(() => rm(tmp, { recursive: true, force: true }))

  const taskDir = path.join(tmp, '.xiaotao/tasks/task-dsh-current')
  await mkdir(taskDir, { recursive: true })
  await writeFile(path.join(taskDir, 'task.yaml'), `id: task-dsh-current
objective: Real current DSH task
status: active
created_at: 2026-09-12T10:00:00Z
updated_at: 2026-09-12T10:00:00Z
updated_by: old-zhou/test
revision: 1
`)

  // Stale index containing obsolete task
  await mkdir(path.join(tmp, '.xiaotao/memory'), { recursive: true })
  await writeFile(path.join(tmp, '.xiaotao/memory/index.json'), JSON.stringify({
    schema_version: 1,
    generated_at: '2026-09-01T00:00:00Z',
    source_digest: 'obsolete',
    entries: [
      { memory_id: 'task-stale', title: 'Stale task that must not be consumed', record_type: 'task', status: 'active' },
    ],
    pending_followups: [],
  }))

  // Authoritative committed checkpoint exists
  const chkDir = path.join(taskDir, 'references/checkpoints')
  await mkdir(chkDir, { recursive: true })
  await writeFile(path.join(chkDir, 'req-dsh-deg-1.committed.json'), JSON.stringify({
    request_id: 'req-dsh-deg-1',
    revision: 1,
  }))

  // Force Python overview/rebuild to fail
  process.env.XIAOTAO_FORCE_PYTHON_FAIL = '1'
  t.after(() => { delete process.env.XIAOTAO_FORCE_PYTHON_FAIL })

  const result = await loadBoundedRuntimeContext(tmp)
  assert.ok(result)

  // Stale task from index.json MUST NOT be consumed
  assert.doesNotMatch(result, /task-stale/)
  assert.doesNotMatch(result, /Stale task that must not be consumed/)

  // Explicit degraded warning MUST be output
  assert.match(result, /警告：检测到项目存在 XiaoTao 权威工作源，但启动时没有可用的 Catalog 快照/)

  // Authoritative checkpoint MUST still be presented
  assert.match(result, /## Recoverable Checkpoint/)
  assert.match(result, /Recoverable checkpoint: yes/)
  assert.match(result, /status: committed/)
  assert.match(result, /binding: task-dsh-current/)
  assert.match(result, /revision: 1/)
})
