import { test } from 'node:test'
import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'yaml'
import type { FsTarget, FsInfo, FsVersion, FsWriteOutcome } from '@deepseek-ai/dsh-fs'
import type { ToolRunContext } from '@deepseek-ai/dsh-tools'
import { CheckpointWriter, hash, type CheckpointFs, type CheckpointInput } from './checkpoint'
import { checkpointTool } from './checkpoint-tool'
import { XiaoTaoSchemaValidator } from './validate'
import { Context } from '@deepseek-ai/cordis'
import { SystemPrompt } from '@deepseek-ai/dsh-system-prompt'
import { ToolRuntime } from '@deepseek-ai/dsh-tools'
import { apply } from './index'

const project = path.resolve('checkpoint-fixture')
const backup = path.resolve('checkpoint-recovery')
const stamp = '2026-09-08T00:00:00Z'
const statePath = '.xiaotao/memory/temporary/active/test/current.md'
const metaPath = '.xiaotao/memory/temporary/active/test/meta.yaml'
const initial = `---\nrevision: 0\nupdated_at: ${stamp}\nupdated_by: user\n---\n# User notes\nKeep this text.\n`
const validator = new XiaoTaoSchemaValidator()
const ready = validator.loadAll(fileURLToPath(new URL('../../../xiaotao/references/schemas/', import.meta.url)))
const error = (code: string) => Object.assign(new Error(code), { code })

function fixture() {
  const files = new Map<string, { content: string; version: number }>()
  const key = (p: string) => path.resolve(project, p)
  const target = (p: string) => ({ targetKey: p, displayPath: p }) as FsTarget
  const versions = (n: number) => String(n) as FsVersion
  let beforeWrite: ((p: string, content: string) => void) | undefined
  let afterWrite: ((p: string, content: string) => void) | undefined
  let alias: ((p: string) => string) | undefined
  const fs: CheckpointFs = {
    async resolve(p, opts) { opts?.signal?.throwIfAborted(); const resolved = path.resolve(opts?.cwd ?? project, p); return target(alias?.(resolved) ?? resolved) },
    async stat(t, signal) {
      signal?.throwIfAborted()
      const found = files.get(String(t.targetKey))
      if (found) return { version: versions(found.version), type: 'file', size: Buffer.byteLength(found.content) } as FsInfo
      if ([...files.keys()].some((p) => p.startsWith(String(t.targetKey) + path.sep))) return { type: 'directory', size: 0 } as FsInfo
      return undefined
    },
    async readText(t, signal) { signal?.throwIfAborted(); const entry = files.get(String(t.targetKey)); if (!entry) throw error('FS_NOT_FOUND'); return entry.content },
    async writeText(t, content, expected, signal) {
      signal?.throwIfAborted()
      const p = String(t.targetKey)
      beforeWrite?.(p, content)
      const old = files.get(p)
      if (expected?.kind === 'createIfAbsent' && old) throw error('FS_NOT_OBSERVED')
      if (expected?.kind === 'replaceIfVersion' && (!old || versions(old.version) !== expected.version)) throw error('FS_STALE_VERSION')
      const next = { content, version: (old?.version ?? 0) + 1 }
      files.set(p, next)
      afterWrite?.(p, content)
      return { version: versions(next.version) } as FsWriteOutcome
    },
    contains(a, b) { const rel = path.relative(String(a.targetKey), String(b.targetKey)); return rel === '' || (!rel.startsWith('..') && !path.isAbsolute(rel)) },
    async listDir(t) { return [...files.keys()].filter((p) => p.startsWith(String(t.targetKey) + path.sep)).map((p) => ({ name: path.basename(p), type: 'file' as const, target: target(p) })) },
  }
  files.set(key(statePath), { content: initial, version: 1 })
  files.set(key(metaPath), { content: `id: test\ntopic: test\nstatus: active\ncreated_at: ${stamp}\nupdated_at: ${stamp}\nupdated_by: user\nrevision: 0\n`, version: 1 })
  const input: CheckpointInput = { kind: 'temporary', target_id: 'test', request_id: 'save_1', base_revision: 0,
    base_hash: hash(initial), snapshot: { objective: 'Continue test', confirmed: ['One verified fact'], rejected: [],
      in_progress: ['Implementation'], next: ['Run tests'], open_questions: [], source_refs: [] } }
  return { fs, files, key, input, setBefore: (fn?: typeof beforeWrite) => { beforeWrite = fn },
    setAfter: (fn?: typeof afterWrite) => { afterWrite = fn }, setAlias: (fn: typeof alias) => { alias = fn },
    async writer(recovery = false, controller = new AbortController()) {
      await ready
      const writer = new CheckpointWriter(fs, validator, { projectRoot: project, ...(recovery ? { recoveryRoot: backup } : {}) }, 'session-1', controller.signal)
      await writer.initialize(project)
      return writer
    },
  }
}

test('save preserves user text and atomically adds revision/receipt; retry after restart is a no-op', async () => {
  const f = fixture(), w = await f.writer()
  const inspected = await w.inspect('temporary', 'test')
  assert.equal(inspected.base_hash, f.input.base_hash)
  assert.equal((await w.save(f.input)).status, 'committed')
  const committed = f.files.get(f.key(statePath))!
  assert.match(committed.content, /Keep this text\./)
  assert.match(committed.content, /revision: 1/)
  const observationPath = f.key('.xiaotao/memory/temporary/active/test/references/checkpoints/save_1.committed.json')
  const observation = JSON.parse(f.files.get(observationPath)!.content)
  assert.equal(observation.completion, 'save')
  assert.ok(Number.isFinite(Date.parse(observation.committed_at)))
  const fresh = await f.writer()
  assert.equal((await fresh.retry('temporary', 'test', 'save_1')).status, 'already_committed')
  assert.deepEqual(JSON.parse(f.files.get(observationPath)!.content), observation)
  assert.deepEqual(f.files.get(f.key(statePath)), committed)
  assert.equal((await fresh.status('temporary', 'test', 'save_1')).status, 'committed')
})

test('same request ID with different input is rejected; reordered snapshot fields are idempotent', async () => {
  const f = fixture(), w = await f.writer()
  await w.save(f.input)
  await assert.rejects(w.save({ ...f.input, snapshot: { ...f.input.snapshot, objective: 'Different' } }), /request_id_reused/)
  const reordered = Object.fromEntries(Object.entries(f.input.snapshot).reverse()) as unknown as CheckpointInput['snapshot']
  assert.equal((await w.save({ ...f.input, snapshot: reordered })).status, 'already_committed')
})

test('new checkpoint repairs prior observation before replacing its receipt', async () => {
  const f = fixture(), w = await f.writer()
  f.setBefore((p) => { if (p.endsWith('.committed.json')) throw error('FS_IO_ERROR') })
  await assert.rejects(w.save(f.input))
  assert.match(f.files.get(f.key(statePath))!.content, /revision: 1/)
  f.setBefore()
  const current = f.files.get(f.key(statePath))!.content
  await (await f.writer()).save({ ...f.input, request_id: 'save_2', base_revision: 1, base_hash: hash(current) })
  assert.ok([...f.files.keys()].some((p) => p.endsWith('save_1.committed.json')))
  assert.match(f.files.get(f.key(statePath))!.content, /revision: 2/)
  assert.equal((f.files.get(f.key(statePath))!.content.match(/xiaotao-checkpoint:start/g) ?? []).length, 1)
})

test('lost write acknowledgement is reconciled from exact proposal bytes', async () => {
  const f = fixture(), w = await f.writer()
  f.setAfter((p) => { if (p === f.key(statePath)) throw error('FS_IO_ERROR') })
  await assert.rejects(w.save(f.input))
  f.setAfter()
  const old = f.files.get(f.key(statePath))!
  await (await f.writer()).retry('temporary', 'test', 'save_1')
  assert.deepEqual(f.files.get(f.key(statePath)), old)
  const observation = JSON.parse(f.files.get(f.key(
    '.xiaotao/memory/temporary/active/test/references/checkpoints/save_1.committed.json',
  ))!.content)
  assert.equal(observation.completion, 'recovery')
  assert.ok(Number.isFinite(Date.parse(observation.committed_at)))
})

test('legacy observations remain readable while malformed versioned observations fail closed', async () => {
  const f = fixture(), w = await f.writer()
  await w.save(f.input)
  const requestPath = f.key('.xiaotao/memory/temporary/active/test/references/checkpoints/save_1.json')
  const observationPath = f.key('.xiaotao/memory/temporary/active/test/references/checkpoints/save_1.committed.json')
  const record = f.files.get(requestPath)!.content
  const proposal = JSON.parse(record).proposal_hash
  const legacy = JSON.stringify({ request_id: 'save_1', record_hash: hash(record), proposal_hash: proposal, revision: 1 })
  f.files.set(observationPath, { content: legacy, version: 2 })
  assert.equal((await (await f.writer()).status('temporary', 'test', 'save_1')).status, 'committed')

  f.files.set(observationPath, { content: JSON.stringify({ ...JSON.parse(legacy), completion: 'recovery' }), version: 3 })
  await assert.rejects((await f.writer()).status('temporary', 'test', 'save_1'), /invalid_observation/)

  f.files.set(observationPath, { content: JSON.stringify({ ...JSON.parse(legacy), completion: 'recovery',
    committed_at: '2026-09-12T11:28:04' }), version: 4 })
  await assert.rejects((await f.writer()).status('temporary', 'test', 'save_1'), /invalid_observation/)
})

test('source/target write-ahead archive survives primary writes failing before request publication', async () => {
  const f = fixture(), w = await f.writer(true)
  f.setBefore((p) => { if (p.startsWith(project + path.sep)) throw error('FS_IO_ERROR') })
  let failure: unknown
  try { await w.save(f.input) } catch (e) { failure = e }
  assert.equal(w.failure(failure).recovery, 'secondary')
  assert.equal(w.failure(failure).failure_stage, 'project_request')
  assert.equal(f.files.get(f.key(statePath))!.content, initial)
  assert.ok([...f.files.keys()].some((p) => p.startsWith(backup + path.sep)))
  f.setBefore()
  assert.equal((await (await f.writer(true)).retry('temporary', 'test', 'save_1')).status, 'committed')
})

test('both storage channels unavailable never claim recoverability or change current state', async () => {
  const f = fixture(), w = await f.writer(true)
  f.setBefore(() => { throw error('FS_IO_ERROR') })
  try { await w.save(f.input); assert.fail('must fail') } catch (e) {
    assert.equal(w.failure(e).recovery, 'none')
    assert.equal(w.failure(e).failure_stage, 'secondary_write')
  }
  assert.equal(f.files.size, 2)
})

test('CAS race after source capture never overwrites the newer content', async () => {
  const f = fixture(), w = await f.writer()
  const newer = initial.replace('revision: 0', 'revision: 1') + 'New user work\n'
  f.setBefore((p) => { if (p === f.key(statePath)) f.files.set(p, { content: newer, version: 3 }) })
  await assert.rejects(w.save(f.input), /FS_STALE_VERSION/)
  assert.equal(f.files.get(f.key(statePath))!.content, newer)
  f.setBefore()
  await assert.rejects((await f.writer()).retry('temporary', 'test', 'save_1'), /conflict/)
})

test('parallel duplicate requests serialize or fail closed without double revision', async () => {
  const f = fixture(), a = await f.writer(), b = await f.writer()
  const results = await Promise.allSettled([a.save(f.input), b.save(f.input)])
  assert.ok(results.some((r) => r.status === 'fulfilled'))
  assert.match(f.files.get(f.key(statePath))!.content, /revision: 1/)
  await (await f.writer()).retry('temporary', 'test', 'save_1')
  assert.match(f.files.get(f.key(statePath))!.content, /revision: 1/)
})

test('archived targets and missing sources stop pending retry', async () => {
  const f = fixture(), w = await f.writer()
  f.files.set(f.key('evidence.txt'), { content: 'evidence', version: 1 })
  f.input.snapshot.source_refs = ['evidence.txt']
  f.setBefore((p) => { if (p === f.key(statePath)) throw error('FS_IO_ERROR') })
  await assert.rejects(w.save(f.input))
  f.setBefore()
  f.files.delete(f.key('evidence.txt'))
  await assert.rejects((await f.writer()).retry('temporary', 'test', 'save_1'), /source_unavailable/)
  const meta = f.files.get(f.key(metaPath))!
  f.files.set(f.key(metaPath), { content: meta.content.replace('status: active', 'status: archive'), version: 2 })
  await assert.rejects((await f.writer()).retry('temporary', 'test', 'save_1'), /target_not_active/)
})

test('malformed snapshots, oversized facts and invalid frontmatter do not write checkpoints', async () => {
  for (const mode of ['missing', 'large', 'frontmatter']) {
    const f = fixture(), w = await f.writer()
    if (mode === 'missing') delete (f.input.snapshot as Partial<CheckpointInput['snapshot']>).objective
    if (mode === 'large') f.input.snapshot.objective = 'x'.repeat(20000)
    if (mode === 'frontmatter') {
      const content = initial.replace('revision: 0', 'revision: 0\nrevision: 2')
      f.files.set(f.key(statePath), { content, version: 2 }); f.input.base_hash = hash(content)
    }
    await assert.rejects(w.save(f.input))
    assert.equal(f.files.size, 2)
  }
})

test('traversal, cross-target symlinks and configured recovery overlap fail closed', async () => {
  const f = fixture(), w = await f.writer()
  await assert.rejects(w.inspect('temporary', '../test'), /invalid_target/)
  f.setAlias((p) => p === f.key(statePath) ? f.key('.xiaotao/tasks/other/progress.md') : p)
  await assert.rejects(w.inspect('temporary', 'test'), /target_path_escape/)
  const bad = new CheckpointWriter(f.fs, validator, { projectRoot: project, recoveryRoot: project }, 's', new AbortController().signal)
  await assert.rejects(bad.initialize(project), /recovery_root_overlaps_project/)
})

test('corrupt immutable recovery record is not trusted', async () => {
  const f = fixture(), w = await f.writer()
  await w.save(f.input)
  const p = [...f.files.keys()].find((p) => p.endsWith('save_1.json'))!
  const r = JSON.parse(f.files.get(p)!.content)
  r.proposal += 'tampered'
  f.files.set(p, { content: JSON.stringify(r), version: 2 })
  await assert.rejects((await f.writer()).retry('temporary', 'test', 'save_1'), /checkpoint_hash_mismatch/)
})

test('transaction bundles are not silently bypassed', async () => {
  const f = fixture(), w = await f.writer()
  f.files.set(f.key('.xiaotao/transactions/t/intent.yaml'), { content: 'pending', version: 1 })
  await assert.rejects(w.save(f.input), /transaction_overlay_unsupported/)
  assert.equal(f.files.get(f.key(statePath))!.content, initial)
})

test('cancellation after commit still releases locks and permits retry', async () => {
  const f = fixture(), controller = new AbortController(), w = await f.writer(false, controller)
  f.setAfter((p) => { if (p === f.key(statePath)) controller.abort() })
  await assert.rejects(w.save(f.input))
  f.setAfter()
  for (const [p, entry] of f.files) if (p.endsWith('.lock')) assert.equal(JSON.parse(entry.content).state, 'released')
  await (await f.writer()).retry('temporary', 'test', 'save_1')
  assert.match(f.files.get(f.key(statePath))!.content, /revision: 1/)
})

test('typed native tool requires a matching live caller project and routes inspect/save/retry', async () => {
  const f = fixture(); await ready
  const tool = checkpointTool(f.fs, validator, { projectRoot: project })
  const exec = { agent: { id: 'session-2', session: { header: { cwd: project } } }, signal: new AbortController().signal } as ToolRunContext
  const result = await tool.execute({ operation: 'save', ...f.input }, exec) as { status: string }
  assert.equal(result.status, 'committed')
  assert.equal((await tool.execute({ operation: 'retry', kind: 'temporary', target_id: 'test', request_id: 'save_1' }, exec) as { status: string }).status, 'already_committed')
  const other = { ...exec, agent: { ...exec.agent, session: { header: { cwd: backup } } } } as ToolRunContext
  assert.equal((await tool.execute({ operation: 'inspect', kind: 'temporary', target_id: 'test' }, other) as { code: string }).code, 'caller_project_mismatch')
  assert.equal((await tool.execute({}, { ...exec, agent: undefined }) as { code: string }).code, 'caller_session_required')
  assert.equal(parse(f.files.get(f.key(statePath))!.content.split('---')[1]).revision, 1)
})

test('real DSH ToolRuntime registers the definition and enforces pre-execute denial', async () => {
  const f = fixture(); await ready
  const ctx = new Context()
  new SystemPrompt(ctx, {})
  const runtime = new ToolRuntime(ctx)
  const dispose = runtime.register(checkpointTool(f.fs, validator, { projectRoot: project }))
  try {
    const removeGate = ctx.on('tools/pre-execute', async () => ({ kind: 'deny', reason: 'test policy' }))
    const denied = await runtime.execute({ callId: 'c1' as never, name: 'xiaotao_checkpoint',
      arguments: { operation: 'save', ...f.input }, signal: new AbortController().signal })
    assert.equal(denied.isError, true)
    assert.equal(f.files.size, 2)
    removeGate()
    const callerless = await runtime.execute({ callId: 'c2' as never, name: 'xiaotao_checkpoint',
      arguments: { operation: 'inspect', kind: 'temporary', target_id: 'test' }, signal: new AbortController().signal })
    assert.equal(callerless.isError, false)
    if (!callerless.isError) assert.equal((callerless.value as { code: string }).code, 'caller_session_required')
    const agent = { id: 's-real', ctx, session: { header: { cwd: project } } } as unknown as ToolRunContext['agent']
    const saved = await runtime.execute({ callId: 'c3' as never, name: 'xiaotao_checkpoint', agent,
      arguments: { operation: 'save', ...f.input }, signal: new AbortController().signal })
    assert.equal(saved.isError, false)
    if (!saved.isError) assert.equal((saved.value as { status: string }).status, 'committed')
  } finally { dispose() }
})

test('Task progress target uses existing progress.md without creating current.md', async () => {
  const f = fixture(), w = await f.writer()
  const taskPath = '.xiaotao/tasks/task-one/progress.md'
  f.files.set(f.key(taskPath), { content: initial, version: 1 })
  f.files.set(f.key('.xiaotao/tasks/task-one/task.yaml'), { content: `id: task-one\nobjective: Work\nstatus: active\ncreated_at: ${stamp}\nupdated_at: ${stamp}\nupdated_by: user\nrevision: 0\n`, version: 1 })
  await w.save({ ...f.input, kind: 'task', target_id: 'task-one' })
  assert.match(f.files.get(f.key(taskPath))!.content, /revision: 1/)
  assert.equal(f.files.has(f.key('.xiaotao/tasks/task-one/current.md')), false)
})

test('adapter defaults to automatic project binding, supports opt-out and plain Skill fallback', async () => {
  const f = fixture()
  for (const [checkpoint, withTools, expected] of [[undefined, true, 1], [false, true, 0], [{}, false, 0], [{ projectRoot: project }, true, 1]] as const) {
    const registered: string[] = [], cleanups: Array<() => void> = []
    let skillCount = 0
    const skills = { register: () => { skillCount++; return () => {} } }
    const services: Record<string, unknown> = { skills, fs: f.fs }
    if (withTools) services.tools = { register: (definition: { name: string }) => {
      registered.push(definition.name); return () => registered.pop()
    } }
    const ctx = { skills, get: (name: string) => services[name], provide: () => () => {},
      inject: (deps: string[], callback: (ctx: unknown) => unknown) => {
        if (deps.every((dep) => services[dep])) pending.push(Promise.resolve(callback(ctx)))
      },
      effect: (factory: () => () => void) => { cleanups.push(factory()) }, logger: { info() {}, warn() {} } }
    const pending: Promise<unknown>[] = []
    await apply(ctx as never, { coreDir: fileURLToPath(new URL('../../../xiaotao/', import.meta.url)),
      checkpoint })
    for (const task of pending) await task
    assert.equal(skillCount, 1)
    assert.equal(registered.length, expected)
    cleanups.reverse().forEach((cleanup) => cleanup())
    assert.equal(registered.length, 0)
  }
})

test('adapter activates the lifecycle trigger only when auto config and agent service are present', async () => {
  const f = fixture()
  const listeners: Array<{ name: string; options?: { global?: boolean } }> = []
  const pending: Promise<unknown>[] = []
  const services: Record<string, unknown> = {
    skills: { register: () => () => {} },
    fs: f.fs,
    tools: { register: () => () => {} },
    agents: {},
  }
  const ctx = {
    skills: services.skills,
    get: (key: string) => services[key],
    provide: () => () => {},
    inject: (deps: string[], callback: (value: unknown) => unknown) => {
      if (deps.every((dep) => services[dep])) pending.push(Promise.resolve(callback(ctx)))
    },
    on: (name: string, _listener: unknown, options?: { global?: boolean }) => {
      listeners.push({ name, options }); return () => false
    },
    effect: (factory: () => () => void) => { factory() },
    logger: { info() {}, warn() {} },
  }
  await apply(ctx as never, { coreDir: fileURLToPath(new URL('../../../xiaotao/', import.meta.url)),
    checkpoint: { auto: { pressureThreshold: 0.8, cooldownTurns: 3, timeoutMs: 250 } } })
  for (let index = 0; index < pending.length; index++) await pending[index]
  assert.deepEqual(listeners, [
    { name: 'agent/turn-stopping', options: { global: true } },
    { name: 'agent/session-start', options: { global: true } },
  ])
})

test('real Cordis activates checkpoint when fs and tools arrive after the adapter', async () => {
  const f = fixture()
  const ctx = new Context()
  ctx.provide('skills', { register: () => () => {} } as never)
  const adapter = await ctx.plugin(apply, { coreDir: fileURLToPath(new URL('../../../xiaotao/', import.meta.url)) })
  assert.equal(ctx.get('xiaotao.stateStore'), undefined)
  assert.equal(ctx.get('xiaotao.transactionStore'), undefined)
  new SystemPrompt(ctx, {})
  const runtime = new ToolRuntime(ctx)
  assert.equal(runtime.get('xiaotao_checkpoint'), undefined)
  const releaseFs = ctx.provide('fs', f.fs as never)
  try {
    const deadline = Date.now() + 2000
    while (!runtime.get('xiaotao_checkpoint') && Date.now() < deadline) await new Promise((resolve) => setTimeout(resolve, 10))
    assert.ok(runtime.get('xiaotao_checkpoint'))
    assert.ok(ctx.get('xiaotao.transactionStore'))
    await adapter.dispose()
    assert.equal(runtime.get('xiaotao_checkpoint'), undefined)
    assert.equal(ctx.get('xiaotao.transactionStore'), undefined)
  } finally { releaseFs(); await ctx.fiber.dispose() }
})

test('one automatic tool isolates simultaneous projects and recovery archives, with restart retry', async () => {
  const f = fixture(); await ready
  const other = path.resolve('second-project')
  for (const rel of [statePath, metaPath]) f.files.set(path.join(other, rel), { ...f.files.get(f.key(rel))! })
  const tool = checkpointTool(f.fs, validator, { recoveryRoot: backup })
  const context = (cwd: string) => ({ agent: { id: cwd, session: { header: { cwd } } },
    signal: new AbortController().signal }) as ToolRunContext
  const results = await Promise.all([project, other].map((cwd) => tool.execute({ operation: 'save', ...f.input }, context(cwd))))
  for (const result of results) assert.equal((result as { status: string }).status, 'committed')
  for (const cwd of [project, other]) {
    assert.match(f.files.get(path.join(cwd, statePath))!.content, /revision: 1/)
    assert.ok([...f.files.keys()].some((p) => p.startsWith(path.join(backup, hash(cwd)) + path.sep)))
    const fresh = checkpointTool(f.fs, validator, { recoveryRoot: backup })
    const retried = await fresh.execute({ operation: 'retry', kind: 'temporary', target_id: 'test', request_id: 'save_1' }, context(cwd))
    assert.equal((retried as { status: string }).status, 'already_committed')
  }
  assert.ok(![...f.files.keys()].some((p) => p.startsWith(path.join(backup, hash(project), hash(project)) + path.sep)))
})

test('automatic binding rejects absent/relative session and model root overrides without writing', async () => {
  const f = fixture(); await ready
  const tool = checkpointTool(f.fs, validator)
  const args = { operation: 'inspect', kind: 'temporary', target_id: 'test' }
  const context = (cwd: string) => ({ agent: { id: 's', session: { header: { cwd } } }, signal: new AbortController().signal }) as ToolRunContext
  const before = [...f.files.entries()]
  for (const [cwd, code] of [['', 'caller_session_required'], ['relative', 'absolute_project_required']]) {
    assert.equal((await tool.execute(args, context(cwd)) as { code: string }).code, code)
  }
  assert.equal((await tool.execute({ ...args, projectRoot: backup }, context(project)) as { code: string }).code, 'invalid_arguments')
  assert.equal((await checkpointTool(f.fs, validator, { recoveryRoot: project }).execute(args, context(project)) as { code: string }).code, 'recovery_root_overlaps_project')
  assert.deepEqual([...f.files.entries()], before)
})

test('save carries session policy and falls back to project when default external archive is denied', async () => {
  const f = fixture(); await ready
  const original = f.fs.writeText
  const policy = { mode: 'workspace-write', workspaceRoot: project }
  const seen: unknown[] = []
  f.fs.writeText = async (...args: Parameters<typeof original>) => {
    seen.push((args as unknown[])[4])
    assert.equal((args as unknown[])[4], policy)
    if (!String(args[0].targetKey).startsWith(project + path.sep)) throw error('FS_SANDBOX_DENIED')
    return original(...args)
  }
  const exec = { agent: { id: 's', session: { header: { cwd: project } },
    ctx: { get: () => ({ resolve: () => policy }) } }, signal: new AbortController().signal } as unknown as ToolRunContext
  const tool = checkpointTool(f.fs, validator)
  const missing = await tool.execute({ operation: 'save', ...f.input, request_id: undefined }, exec)
  assert.notEqual((missing as { status: string }).status, 'committed')
  const result = await tool.execute({ operation: 'save', ...f.input }, exec)
  assert.equal((result as { status: string }).status, 'committed')
  assert.ok(seen.length > 0)
  assert.match(f.files.get(f.key(statePath))!.content, /revision: 1/)
  const readOnly = { ...exec, agent: { ...exec.agent, ctx: { get: () => ({ resolve: () => ({ mode: 'read-only', workspaceRoot: project }) }) } } } as unknown as ToolRunContext
  f.fs.writeText = async () => { throw error('FS_SANDBOX_DENIED') }
  const inspected = await tool.execute({ operation: 'inspect', kind: 'temporary', target_id: 'test' }, exec) as { revision: number; base_hash: string }
  const denied = await tool.execute({ operation: 'save', ...f.input, request_id: 'save_2', base_revision: inspected.revision, base_hash: inspected.base_hash }, readOnly)
  assert.equal((denied as { code: string }).code, 'filesystem_permission_denied')
})

test('automatic recovery uses canonical project identity across path aliases', async () => {
  const f = fixture(); await ready
  const aliasRoot = path.resolve('project-alias')
  f.setAlias((p) => p === aliasRoot || p.startsWith(aliasRoot + path.sep)
    ? project + p.slice(aliasRoot.length) : p)
  const tool = checkpointTool(f.fs, validator, { recoveryRoot: backup })
  const context = (cwd: string) => ({ agent: { id: 's', session: { header: { cwd } } }, signal: new AbortController().signal }) as ToolRunContext
  assert.equal((await tool.execute({ operation: 'save', ...f.input }, context(aliasRoot)) as { status: string }).status, 'committed')
  assert.ok([...f.files.keys()].some((p) => p.startsWith(path.join(backup, hash(project)) + path.sep)))
  assert.ok(![...f.files.keys()].some((p) => p.startsWith(path.join(backup, hash(aliasRoot)) + path.sep)))
  assert.equal((await tool.execute({ operation: 'retry', kind: 'temporary', target_id: 'test', request_id: 'save_1' }, context(project)) as { status: string }).status, 'already_committed')
})

test('a stale held lock survives retry until its owner is explicitly recovered', async () => {
  const f = fixture(), w = await f.writer()
  const lock = f.key('.xiaotao/locks/memory-temporary-active-test-current.md.lock')
  f.files.set(lock, { content: JSON.stringify({ owner: 'dead-or-unknown', state: 'held', acquiredAt: stamp,
    expiresAt: '2000-01-01T00:00:00Z' }), version: 1 })
  try { await w.save(f.input); assert.fail('must refuse contention') }
  catch (e) { assert.equal(w.failure(e).code, 'lock_contention') }
  assert.equal(f.files.get(f.key(statePath))!.content, initial)
  assert.equal(JSON.parse(f.files.get(lock)!.content).owner, 'dead-or-unknown')
})

test('unconfirmed prior receipt plus intervening user edit is reported ambiguous', async () => {
  const f = fixture(), w = await f.writer()
  f.setBefore((p) => { if (p.endsWith('.committed.json')) throw error('FS_IO_ERROR') })
  await assert.rejects(w.save(f.input))
  f.setBefore()
  const changed = f.files.get(f.key(statePath))!.content + '\nUser edit\n'
  f.files.set(f.key(statePath), { content: changed, version: 3 })
  await assert.rejects((await f.writer()).save({ ...f.input, request_id: 'save_2', base_revision: 1,
    base_hash: hash(changed) }), /previous_commit_ambiguous/)
  assert.equal(f.files.get(f.key(statePath))!.content, changed)
})

test('tool persists failure reason on secondary channel and status remains read-only', async () => {
  const f = fixture(); await ready
  const tool = checkpointTool(f.fs, validator, { projectRoot: project, recoveryRoot: backup })
  const exec = { agent: { id: 'session-2', session: { header: { cwd: project } } }, signal: new AbortController().signal } as ToolRunContext
  f.setBefore((p) => { if (p.startsWith(project + path.sep)) throw error('FS_IO_ERROR') })
  const result = await tool.execute({ operation: 'save', ...f.input }, exec) as { failure_recorded: boolean }
  assert.equal(result.failure_recorded, true)
  const eventPath = [...f.files.keys()].find((p) => p.includes('.failed-'))!
  assert.ok(eventPath.startsWith(backup + path.sep))
  assert.equal(JSON.parse(f.files.get(eventPath)!.content).request_id, 'save_1')
  const before = [...f.files.entries()]
  await tool.execute({ operation: 'status', kind: 'temporary', target_id: 'test', request_id: 'save_1' }, exec)
  assert.deepEqual([...f.files.entries()], before)
})

test('status stays queryable after the target is archived', async () => {
  const f = fixture(), w = await f.writer()
  await w.save(f.input)
  const meta = f.files.get(f.key(metaPath))!
  f.files.set(f.key(metaPath), { content: meta.content.replace('status: active', 'status: archive'), version: 2 })
  assert.equal((await (await f.writer()).status('temporary', 'test', 'save_1')).status, 'committed')
})

test('multibyte proposal over the byte limit reports proposal_too_large', async () => {
  const f = fixture(), w = await f.writer()
  // 40000 CJK chars = 120000 UTF-8 bytes: under the 131072-character schema max, but
  // combined with a large snapshot the proposal crosses the 128 KiB byte limit in
  // markdown(). This must surface as proposal_too_large, not state_too_large.
  const content = `---\nrevision: 0\nupdated_at: ${stamp}\nupdated_by: user\n---\n# 正文\n${'中'.repeat(40000)}\n`
  f.files.set(f.key(statePath), { content, version: 2 })
  f.input.base_hash = hash(content)
  f.input.snapshot = { ...f.input.snapshot, confirmed: Array(32).fill('证'.repeat(150)) }
  await assert.rejects(w.save(f.input), /proposal_too_large/)
  assert.equal(f.files.get(f.key(statePath))!.content, content)
})
