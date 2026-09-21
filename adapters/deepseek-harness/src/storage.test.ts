import { test } from 'node:test'
import assert from 'node:assert/strict'
import type {
  FsInfo,
  FsTarget,
  FsVersion,
  FsWriteIntent,
  FsWriteOutcome,
} from '@deepseek-ai/dsh-fs'
import {
  acquireLock,
  XiaoTaoStateStore,
  StatePathError,
  type StateFileSystem,
} from './storage'

/** Cast a plain string to dsh's opaque branded types (type-only at runtime). */
const target = (key: string): FsTarget => ({ targetKey: key, displayPath: key }) as unknown as FsTarget
const version = (n: number): FsVersion => String(n) as unknown as FsVersion

interface FileEntry {
  content: string
  version: number
}

function fsError(code: string): Error {
  return Object.assign(new Error(code), { code })
}

/**
 * Minimal in-memory `StateFileSystem` modelling the dsh-fs write-intent
 * semantics: `createIfAbsent` → FS_NOT_OBSERVED on existing, `replaceIfVersion`
 * → FS_STALE_VERSION on absence or mismatch. `resolve` can be overridden to
 * simulate targets that escape the state root.
 */
function makeFs(overrides: Partial<Pick<StateFileSystem, 'resolve'>> = {}) {
  const files = new Map<string, FileEntry>()
  let failNextReplace = false
  const fs: StateFileSystem = {
    async resolve(path: string): Promise<FsTarget> {
      return target(path)
    },
    async stat(t: FsTarget): Promise<FsInfo | undefined> {
      const entry = files.get(t.targetKey as string)
      if (entry === undefined) return undefined
      return { version: version(entry.version), type: 'file', size: entry.content.length } as FsInfo
    },
    async readText(t: FsTarget): Promise<string> {
      const entry = files.get(t.targetKey as string)
      if (entry === undefined) throw fsError('FS_NOT_FOUND')
      return entry.content
    },
    async writeText(
      t: FsTarget,
      content: string,
      expected?: FsWriteIntent,
    ): Promise<FsWriteOutcome> {
      const key = t.targetKey as string
      const existing = files.get(key)
      if (expected?.kind === 'createIfAbsent') {
        if (existing !== undefined) throw fsError('FS_NOT_OBSERVED')
        const created: FileEntry = { content, version: 1 }
        files.set(key, created)
        return { operation: 'create', version: version(1), before: null, after: content } as FsWriteOutcome
      }
      if (expected?.kind === 'replaceIfVersion') {
        if (failNextReplace) {
          failNextReplace = false
          throw fsError('FS_IO_ERROR')
        }
        if (existing === undefined || existing.version !== Number(expected.version)) {
          throw fsError('FS_STALE_VERSION')
        }
        const updated: FileEntry = { content, version: existing.version + 1 }
        files.set(key, updated)
        return {
          operation: 'update',
          version: version(updated.version),
          before: existing.content,
          after: content,
        } as FsWriteOutcome
      }
      const next: FileEntry = {
        content,
        version: existing === undefined ? 1 : existing.version + 1,
      }
      files.set(key, next)
      return {
        operation: existing === undefined ? 'create' : 'update',
        version: version(next.version),
        before: existing?.content ?? null,
        after: content,
      } as FsWriteOutcome
    },
    contains(parent: FsTarget, child: FsTarget): boolean {
      const p = parent.targetKey as string
      const c = child.targetKey as string
      return c === p || c.startsWith(`${p}/`)
    },
    ...overrides,
  }
  return {
    fs,
    files,
    /** Arm a one-shot `replaceIfVersion` failure (next guarded write throws). */
    failNextReplace: () => {
      failNextReplace = true
    },
  }
}

test('lockPathFor derives a stable lock path under .xiaotao/locks', () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  assert.equal(store.lockPathFor('memory/long-term/current.md'), '.xiaotao/locks/memory-long-term-current.md.lock')
  assert.equal(store.lockPathFor('.xiaotao/memory/long-term/current.md'), '.xiaotao/locks/memory-long-term-current.md.lock')
})

test('lockPathFor rejects traversal and absolute paths', () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  assert.throws(() => store.lockPathFor('../escape'), StatePathError)
  assert.throws(() => store.lockPathFor('a/../../b'), StatePathError)
  assert.throws(() => store.lockPathFor('/abs/path'), StatePathError)
  assert.throws(() => store.lockPathFor('C:\\abs\\path'), StatePathError)
})

test('readSnapshot / createIfAbsent reject paths outside .xiaotao', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  await assert.rejects(() => store.readSnapshot('../outside.md'), StatePathError)
  await assert.rejects(() => store.createIfAbsent('/abs/outside.md', 'x'), StatePathError)
})

test('containment check rejects a target that resolves outside the root', async () => {
  // `resolve` maps a lexically-safe path to a target that escapes `.xiaotao/`,
  // simulating a symlink/backend alias the lexical check cannot see.
  const { fs } = makeFs({
    resolve: async (path: string) => {
      if (path === '.xiaotao/leak') return target('/outside/leak')
      return target(path)
    },
  })
  const store = new XiaoTaoStateStore(fs)
  await assert.rejects(() => store.readSnapshot('.xiaotao/leak'), StatePathError)
})

test('writeGuarded replaces only when the version still matches', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  await store.createIfAbsent('.xiaotao/memory/current.md', 'v0')
  const snap = await store.readSnapshot('.xiaotao/memory/current.md')
  assert.ok(snap)
  await store.writeGuarded(snap, 'v1')
  // The stale snapshot no longer matches the current version.
  await assert.rejects(
    () => store.writeGuarded(snap, 'v2'),
    (error: unknown) => (error as { code?: string }).code === 'FS_STALE_VERSION',
  )
})

test('acquireLock: release tombstone lets a second writer re-acquire immediately', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a')
  await release()
  // Re-acquire right away — must not block on lease expiry or contention.
  const release2 = await acquireLock(store, 'memory/long-term/current.md', 'agent-b', { timeoutMs: 1000 })
  await release2()
  const lockKey = store.lockPathFor('memory/long-term/current.md')
  const content = await store.readSnapshot(lockKey)
  assert.ok(content)
  const lease = JSON.parse(content.content)
  assert.equal(lease.state, 'released')
  assert.equal(lease.owner, 'agent-b')
})

test('acquireLock: expired held lock is NOT reclaimed without canReclaim', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  // First owner holds the lock; its lease expires quickly.
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a', { leaseMs: 5 })
  try {
    await new Promise((r) => setTimeout(r, 20))
    // Second writer has no canReclaim → clock age alone is insufficient.
    await assert.rejects(
      () => acquireLock(store, 'memory/long-term/current.md', 'agent-b', { timeoutMs: 60, retryDelayMs: 10 }),
      /lock contention/,
    )
  } finally {
    await release()
  }
})

test('acquireLock: expired held lock is reclaimed when canReclaim confirms', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a', { leaseMs: 5 })
  await new Promise((r) => setTimeout(r, 20))
  const seen: string[] = []
  const release2 = await acquireLock(store, 'memory/long-term/current.md', 'agent-b', {
    timeoutMs: 1000,
    retryDelayMs: 10,
    canReclaim: (lease) => {
      seen.push(lease.owner)
      return true
    },
  })
  assert.deepEqual(seen, ['agent-a'])
  await release2()
  await release()
})

test('acquireLock: contention throws after the bounded timeout', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a', { leaseMs: 60_000 })
  try {
    await assert.rejects(
      () =>
        acquireLock(store, 'memory/long-term/current.md', 'agent-b', {
          timeoutMs: 50,
          retryDelayMs: 10,
        }),
      /lock contention/,
    )
  } finally {
    await release()
  }
})

test('acquireLock: contention wait is cooperatively cancellable', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a')
  const controller = new AbortController()
  const waiting = acquireLock(store, 'memory/long-term/current.md', 'agent-b', {
    timeoutMs: 10_000,
    retryDelayMs: 1_000,
    signal: controller.signal,
  })
  controller.abort()
  try { await assert.rejects(() => waiting, { name: 'AbortError' }) }
  finally { await release() }
})

test('acquireLock: release is idempotent and safe to call twice', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a')
  await release()
  await release() // second call must not throw or corrupt state
})

test('writeGuarded rejects a fabricated snapshot whose target escapes .xiaotao', async () => {
  const { fs } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  // StateSnapshot is a public interface — a caller can hand us a snapshot
  // whose target points outside the state root. writeGuarded must still refuse.
  const fakeSnapshot = {
    target: target('/outside/evil.md'),
    version: version(1),
    content: 'x',
  }
  await assert.rejects(() => store.writeGuarded(fakeSnapshot, 'y'), StatePathError)
})

test('acquireLock: release retries after a transient write failure', async () => {
  const { fs, failNextReplace } = makeFs()
  const store = new XiaoTaoStateStore(fs)
  const release = await acquireLock(store, 'memory/long-term/current.md', 'agent-a')

  // First release attempt hits a transient I/O error and must surface it…
  failNextReplace()
  await assert.rejects(
    () => release(),
    (error: unknown) => (error as { code?: string }).code === 'FS_IO_ERROR',
  )

  // …but a second attempt (failure cleared) succeeds — the lock is released.
  await release()
  const lockKey = store.lockPathFor('memory/long-term/current.md')
  const snapshot = await store.readSnapshot(lockKey)
  assert.ok(snapshot)
  assert.equal(JSON.parse(snapshot.content).state, 'released')
})
