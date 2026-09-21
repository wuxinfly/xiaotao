import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import type { FsDirEntry, FsInfo, FsTarget, FsVersion, FsWriteIntent, FsWriteOutcome } from '@deepseek-ai/dsh-fs'
import {
  XiaoTaoTransactionStore,
  TransactionError,
  type TransactionExecuteOptions,
  type TransactionFileSystem,
  type TransactionInput,
} from './transaction'

const target = (key: string): FsTarget => ({ targetKey: key, displayPath: key }) as unknown as FsTarget
const version = (n: number): FsVersion => String(n) as unknown as FsVersion
const error = (code: string) => Object.assign(new Error(code), { code })

function memoryFs() {
  const files = new Map<string, { content: string; version: number }>()
  const writeLog: Array<{ path: string; content: string }> = []
  let writes = 0
  let failAt = -1
  const fs: TransactionFileSystem = {
    async resolve(path: string) { return target(path.replaceAll('\\', '/').replace(/\/$/, '')) },
    async stat(t) {
      const key = String(t.targetKey)
      const file = files.get(key)
      if (file) return { type: 'file', version: version(file.version), size: file.content.length } as FsInfo
      const prefix = `${key}/`
      if ([...files.keys()].some((candidate) => candidate.startsWith(prefix))) {
        return { type: 'directory', version: version(1) } as FsInfo
      }
      return undefined
    },
    async readText(t) {
      const file = files.get(String(t.targetKey))
      if (!file) throw error('FS_NOT_FOUND')
      return file.content
    },
    async writeText(t, content, expected?: FsWriteIntent): Promise<FsWriteOutcome> {
      writes += 1
      if (writes === failAt) throw error('FS_IO_ERROR')
      const key = String(t.targetKey)
      writeLog.push({ path: key, content })
      const current = files.get(key)
      if (expected?.kind === 'createIfAbsent') {
        if (current) throw error('FS_NOT_OBSERVED')
      } else if (expected?.kind === 'replaceIfVersion') {
        if (!current || current.version !== Number(expected.version)) throw error('FS_STALE_VERSION')
      }
      const next = { content, version: (current?.version ?? 0) + 1 }
      files.set(key, next)
      return { operation: current ? 'update' : 'create', version: version(next.version), before: current?.content ?? null, after: content }
    },
    contains(parent, child) {
      const p = String(parent.targetKey), c = String(child.targetKey)
      return c === p || c.startsWith(`${p}/`)
    },
    async listDir(t): Promise<FsDirEntry[]> {
      const root = String(t.targetKey)
      const prefix = `${root}/`
      const children = new Map<string, 'file' | 'directory'>()
      for (const key of files.keys()) {
        if (!key.startsWith(prefix)) continue
        const rest = key.slice(prefix.length)
        const slash = rest.indexOf('/')
        children.set(slash < 0 ? rest : rest.slice(0, slash), slash < 0 ? 'file' : 'directory')
      }
      if (children.size === 0) throw error('FS_NOT_FOUND')
      return [...children].sort(([a], [b]) => a.localeCompare(b)).map(([name, type]) => ({
        name, type, target: target(`${root}/${name}`), version: version(1),
      })) as FsDirEntry[]
    },
  }
  return {
    fs, files, writeLog,
    put(path: string, content: string) { files.set(path, { content, version: (files.get(path)?.version ?? 0) + 1 }) },
    failWrite(number: number) { failAt = number },
    writeCount() { return writes },
  }
}

const h = (text: string) => createHash('sha256').update(text).digest('hex')

function input(): TransactionInput {
  return {
    transaction_id: 'tx-1', operation: 'test replace and create', actor: 'test-agent',
    members: [
      { kind: 'create', path: '.xiaotao/tasks/new/progress.md', staged: 'new' },
      { kind: 'replace', path: '.xiaotao/memory/current.md', staged: 'after', base_hash: '' },
    ],
  }
}

function execute(tx: XiaoTaoTransactionStore, request: TransactionInput,
  options: Partial<TransactionExecuteOptions> = {}) {
  return tx.execute(request, { validateMember: () => {}, ...options })
}

test('rejects unsafe, duplicate, reserved, and unsupported members before writing', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  const base = { transaction_id: 'tx', operation: 'test', actor: 'agent', members: [] } as unknown as TransactionInput
  for (const members of [
    [{ kind: 'create', path: '../escape', staged: 'x' }],
    [{ kind: 'create', path: '.xiaotao/transactions/evil', staged: 'x' }],
    [{ kind: 'delete', path: '.xiaotao/a', staged: '' }],
    [{ kind: 'create', path: '.xiaotao/a', staged: 'x' }, { kind: 'create', path: '.xiaotao/a', staged: 'y' }],
  ]) await assert.rejects(() => execute(tx, { ...base, members } as TransactionInput), TransactionError)
  assert.equal(f.files.size, 0)
})

test('commits before materialization and completes create/replace members', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input()
  request.members[1]!.base_hash = h('before')
  const steps: string[] = []
  const result = await execute(tx, request, { afterStep: (step) => { steps.push(step) } })
  assert.deepEqual(result, { status: 'committed', transaction_id: 'tx-1', materialization: 'complete', applied: 2, members: 2 })
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'after')
  assert.equal(f.files.get('.xiaotao/tasks/new/progress.md')?.content, 'new')
  assert.ok(steps.indexOf('committed') < steps.indexOf('materialized:1'))
})

test('validator failure publishes failed terminal and preserves canonical bytes', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await assert.rejects(() => tx.execute(request, { validateMember: () => { throw new TransactionError('invalid_staged') } }))
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'before')
  assert.equal((await tx.status('tx-1')).status, 'failed')
  assert.equal(f.files.has('.xiaotao/transactions/tx-1/committed.yaml'), false)
})

test('execute fails closed when the required validation seam is missing', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await assert.rejects(() => tx.execute(request, undefined as unknown as TransactionExecuteOptions),
    (e: unknown) => (e as TransactionError).code === 'transaction_validator_required')
  assert.equal([...f.files.keys()].some((path) => path.startsWith('.xiaotao/transactions/')), false)
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'before')
})

test('committed interruption exposes overlay and recover materializes idempotently', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  const result = await execute(tx, request, { afterStep: (step) => {
    if (step === 'committed') throw new Error('simulated crash')
  } })
  assert.equal(result.status, 'committed')
  assert.equal(result.status === 'committed' && result.materialization, 'pending')
  assert.deepEqual(await tx.readOverlay('.xiaotao/memory/current.md'), {
    source: 'overlay', transaction_id: 'tx-1', content: 'after',
  })
  const recovered = await tx.recover('tx-1', { actor: 'recovery-agent' })
  assert.equal(recovered.status === 'committed' && recovered.materialization, 'complete')
  assert.equal((await tx.recover('tx-1', { actor: 'recovery-agent' })).status, 'committed')
})

test('recovery refuses canonical bytes matching neither before nor staged', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await execute(tx, request, { afterStep: (step) => { if (step === 'committed') throw new Error('stop') } })
  f.put('.xiaotao/memory/current.md', 'newer')
  const status = await tx.recover('tx-1', { actor: 'recovery-agent' })
  assert.equal(status.status === 'committed' && status.materialization, 'conflict')
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'newer')
})

test('missing applied record is repaired without rewriting staged canonical bytes', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await execute(tx, request)
  f.files.delete('.xiaotao/transactions/tx-1/applied/0001.yaml')
  const versionBefore = f.files.get('.xiaotao/tasks/new/progress.md')?.version
  assert.equal((await tx.status('tx-1')).status === 'committed', true)
  const recovered = await tx.recover('tx-1', { actor: 'recovery-agent' })
  assert.equal(recovered.status === 'committed' && recovered.materialization, 'complete')
  assert.equal(f.files.get('.xiaotao/tasks/new/progress.md')?.version, versionBefore)
})

test('tampered immutable snapshot and dual terminals are rejected', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await execute(tx, request)
  f.put('.xiaotao/transactions/tx-1/staged/0001.txt', 'tampered')
  await assert.rejects(() => tx.status('tx-1'), (e: unknown) => (e as TransactionError).code === 'invalid_staged_snapshot')

  const clean = memoryFs(), cleanTx = new XiaoTaoTransactionStore(clean.fs)
  clean.put('.xiaotao/memory/current.md', 'before')
  const cleanRequest = input(); cleanRequest.members[1]!.base_hash = h('before')
  await execute(cleanTx, cleanRequest)
  clean.put('.xiaotao/transactions/tx-1/failed.yaml', clean.files.get('.xiaotao/transactions/tx-1/committed.yaml')!.content
    .replace('result: committed', 'result: failed'))
  await assert.rejects(() => cleanTx.status('tx-1'),
    (e: unknown) => (e as TransactionError).code === 'terminal_conflict')
})

test('an unfinished committed overlay blocks a newer transaction on the same member', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const first = input(); first.members[1]!.base_hash = h('before')
  await execute(tx, first, { afterStep: (step) => { if (step === 'committed') throw new Error('stop') } })
  const second: TransactionInput = {
    transaction_id: 'tx-2', operation: 'overlap', actor: 'other-agent',
    members: [{ kind: 'replace', path: '.xiaotao/memory/current.md', staged: 'later', base_hash: h('before') }],
  }
  await assert.rejects(() => execute(tx, second),
    (e: unknown) => (e as TransactionError).code === 'transaction_overlay_conflict')
  assert.equal(f.files.has('.xiaotao/transactions/tx-2/intent.yaml'), false)
})

test('precommit cancellation stops new transaction writes and leaves canonical bytes untouched', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs), controller = new AbortController()
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await assert.rejects(() => execute(tx, request, {
    signal: controller.signal,
    afterStep: (step) => { if (step === 'intent') controller.abort() },
  }), (e: unknown) => (e as TransactionError).code === 'transaction_cancelled')
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'before')
  assert.equal(f.files.has('.xiaotao/transactions/tx-1/committed.yaml'), false)
  assert.equal(f.files.has('.xiaotao/transactions/tx-1/failed.yaml'), false)
  assert.equal((await tx.status('tx-1')).status, 'prepared')
  const resumed = await execute(tx, request)
  assert.equal(resumed.status === 'committed' && resumed.materialization, 'complete')
})

test('postcommit cancellation reports pending and remains recoverable', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs), controller = new AbortController()
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  const result = await execute(tx, request, {
    signal: controller.signal,
    afterStep: (step) => { if (step === 'committed') controller.abort() },
  })
  assert.equal(result.status === 'committed' && result.materialization, 'pending')
  const recovered = await tx.recover('tx-1', { actor: 'recovery-agent' })
  assert.equal(recovered.status === 'committed' && recovered.materialization, 'complete')
})

test('prepared retry rejects a different request with the same transaction id', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs), controller = new AbortController()
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await assert.rejects(() => execute(tx, request, {
    signal: controller.signal,
    afterStep: (step) => { if (step === 'intent') controller.abort() },
  }))
  request.members[0]!.staged = 'different'
  await assert.rejects(() => execute(tx, request),
    (e: unknown) => (e as TransactionError).code === 'transaction_request_conflict')
})

test('prepared retry revalidates canonical preconditions before publishing commit', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs), controller = new AbortController()
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await assert.rejects(() => execute(tx, request, {
    signal: controller.signal,
    afterStep: (step) => { if (step === 'intent') controller.abort() },
  }))
  f.put('.xiaotao/memory/current.md', 'newer')
  await assert.rejects(() => execute(tx, request),
    (e: unknown) => (e as TransactionError).code === 'precondition_conflict')
  assert.equal(f.files.has('.xiaotao/transactions/tx-1/committed.yaml'), false)
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'newer')
  const status = await tx.status('tx-1')
  assert.equal(status.status, 'failed')
})

test('every persisted precommit boundary can be retried with the same request', async () => {
  for (const boundary of ['snapshot:1', 'snapshot:2', 'intent']) {
    const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
    f.put('.xiaotao/memory/current.md', 'before')
    const request = input(); request.members[1]!.base_hash = h('before')
    await assert.rejects(() => execute(tx, request, {
      afterStep: (step) => { if (step === boundary) throw new Error(`stop at ${boundary}`) },
    }))
    assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'before')
    const result = await execute(tx, request)
    assert.equal(result.status === 'committed' && result.materialization, 'complete')
  }
})

test('every postcommit boundary can be recovered deterministically', async () => {
  for (const boundary of ['committed', 'materialized:1', 'applied:1', 'materialized:2', 'applied:2']) {
    const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
    f.put('.xiaotao/memory/current.md', 'before')
    const request = input(); request.members[1]!.base_hash = h('before')
    const interrupted = await execute(tx, request, {
      afterStep: (step) => { if (step === boundary) throw new Error(`stop at ${boundary}`) },
    })
    assert.equal(interrupted.status, 'committed')
    const recovered = await tx.recover('tx-1', { actor: 'recovery-agent' })
    assert.equal(recovered.status === 'committed' && recovered.materialization, 'complete')
    assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'after')
    assert.equal(f.files.get('.xiaotao/tasks/new/progress.md')?.content, 'new')
  }
})

test('locks are acquired in stable member order and released as tombstones', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await execute(tx, request)
  const held = f.writeLog.filter(({ path, content }) => path.startsWith('.xiaotao/locks/')
    && JSON.parse(content).state === 'held').map(({ path }) => path)
  assert.deepEqual(held, [
    '.xiaotao/locks/memory-current.md.lock',
    '.xiaotao/locks/tasks-new-progress.md.lock',
    '.xiaotao/locks/transactions-tx-1-terminal.lock',
  ])
  for (const path of held) assert.equal(JSON.parse(f.files.get(path)!.content).state, 'released')
})

test('missing staged members and forged persisted paths fail closed', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const request = input(); request.members[1]!.base_hash = h('before')
  await execute(tx, request)
  f.files.delete('.xiaotao/transactions/tx-1/staged/0001.txt')
  await assert.rejects(() => tx.status('tx-1'),
    (e: unknown) => (e as TransactionError).code === 'invalid_staged_snapshot')

  const forged = memoryFs(), forgedTx = new XiaoTaoTransactionStore(forged.fs)
  forged.put('.xiaotao/memory/current.md', 'before')
  const forgedRequest = input(); forgedRequest.members[1]!.base_hash = h('before')
  await execute(forgedTx, forgedRequest)
  const intentPath = '.xiaotao/transactions/tx-1/intent.yaml'
  forged.put(intentPath, forged.files.get(intentPath)!.content.replace('.xiaotao/memory/current.md', '../escape'))
  await assert.rejects(() => forgedTx.status('tx-1'),
    (e: unknown) => (e as TransactionError).code === 'invalid_member_path')

  const unexpected = memoryFs(), unexpectedTx = new XiaoTaoTransactionStore(unexpected.fs)
  unexpected.put('.xiaotao/transactions/not-a-bundle.txt', 'unexpected')
  await assert.rejects(() => unexpectedTx.readOverlay('.xiaotao/memory/current.md'),
    (e: unknown) => (e as TransactionError).code === 'invalid_transaction_entry')
})

test('member and transaction byte limits fail before transaction records are written', async () => {
  const oversized = memoryFs(), oversizedTx = new XiaoTaoTransactionStore(oversized.fs)
  const request: TransactionInput = {
    transaction_id: 'tx-size', operation: 'size limit', actor: 'test-agent',
    members: [{ kind: 'create', path: '.xiaotao/large.md', staged: 'x'.repeat(128 * 1024 + 1) }],
  }
  await assert.rejects(() => execute(oversizedTx, request),
    (e: unknown) => (e as TransactionError).code === 'member_too_large')
  assert.equal([...oversized.files.keys()].some((path) => path.startsWith('.xiaotao/transactions/')), false)

  const total = memoryFs(), totalTx = new XiaoTaoTransactionStore(total.fs)
  await assert.rejects(() => execute(totalTx, {
    transaction_id: 'tx-total-size', operation: 'total size limit', actor: 'test-agent',
    members: Array.from({ length: 5 }, (_, index) => ({
      kind: 'create' as const, path: `.xiaotao/large-${index}.md`, staged: 'x'.repeat(120 * 1024),
    })),
  }), (e: unknown) => (e as TransactionError).code === 'transaction_too_large')
  assert.equal([...total.files.keys()].some((path) => path.startsWith('.xiaotao/transactions/')), false)

  const before = memoryFs(), beforeTx = new XiaoTaoTransactionStore(before.fs)
  const hugeBefore = 'x'.repeat(128 * 1024 + 1)
  before.put('.xiaotao/large.md', hugeBefore)
  await assert.rejects(() => execute(beforeTx, {
    transaction_id: 'tx-before-size', operation: 'before size limit', actor: 'test-agent',
    members: [{ kind: 'replace', path: '.xiaotao/large.md', staged: 'small', base_hash: h(hugeBefore) }],
  }), (e: unknown) => (e as TransactionError).code === 'before_member_too_large')
  assert.equal([...before.files.keys()].some((path) => path.startsWith('.xiaotao/transactions/')), false)
})

test('a completed historical transaction never overlays or overwrites a later legal transaction', async () => {
  const f = memoryFs(), tx = new XiaoTaoTransactionStore(f.fs)
  f.put('.xiaotao/memory/current.md', 'before')
  const first = input(); first.members[1]!.base_hash = h('before')
  await execute(tx, first)
  const second: TransactionInput = {
    transaction_id: 'tx-2', operation: 'later update', actor: 'test-agent',
    members: [{
      kind: 'replace', path: '.xiaotao/memory/current.md', staged: 'later', base_hash: h('after'),
    }],
  }
  const secondResult = await execute(tx, second)
  assert.equal(secondResult.status === 'committed' && secondResult.materialization, 'complete')
  const historical = await tx.status('tx-1')
  assert.equal(historical.status === 'committed' && historical.materialization, 'complete')
  await tx.recover('tx-1', { actor: 'recovery-agent' })
  assert.equal(f.files.get('.xiaotao/memory/current.md')?.content, 'later')
  assert.deepEqual(await tx.readOverlay('.xiaotao/memory/current.md'), { source: 'canonical', content: 'later' })
})
