/**
 * Deterministic state store (capability plugin): map XiaoTao's mutable-state
 * write protocol from `references/storage.md` onto dsh's `ctx.fs` primitives.
 *
 * The key realisation from reading dsh's `FileSystem` seam: `writeText` with a
 * `replaceIfVersion` intent is already a compare-and-swap, and `createIfAbsent`
 * is an exclusive create. These two primitives let the adapter implement
 * XiaoTao's "read → guard → atomically replace" protocol as optimistic
 * concurrency, with the opaque `FsVersion` playing the role of the freshness
 * token.
 *
 * The adapter owns only the *mechanism*. The XiaoTao Core (the skill) still
 * decides what the `revision` / `updated_at` / `updated_by` fields inside the
 * file mean — it reads a snapshot, edits the content, and calls
 * {@link writeGuarded}. The store never interprets those fields.
 *
 * ## Safety boundary
 *
 * `storage.md` File rules require every write to resolve beneath the project's
 * `.xiaotao/` directory. The store enforces this in two layers for every path
 * it touches: a lexical check rejects absolute paths and `..` traversal before
 * resolution, and a canonical containment check (`FileSystem.contains`) rejects
 * any path whose resolved target escapes the state root — even via symlinks or
 * backend aliases that the lexical check cannot see.
 *
 * @module @xiaotao-ai/dsh-adapter/storage
 */

import type {
  FsInfo,
  FsTarget,
  FsVersion,
  FsWriteIntent,
  FsWriteOutcome,
} from '@deepseek-ai/dsh-fs'

/**
 * The subset of the dsh `FileSystem` seam the store actually uses. Defined
 * structurally so the CAS/lock logic is unit-testable against a bare fake seam
 * without constructing a real `FileSystem` (which needs a Cordis context and
 * pulls in the whole dsh runtime). The real `FileSystem` satisfies this shape.
 */
export interface StateFileSystem {
  resolve(path: string, opts?: { cwd?: string; signal?: AbortSignal }): Promise<FsTarget>
  stat(target: FsTarget, signal?: AbortSignal): Promise<FsInfo | undefined>
  readText(target: FsTarget, signal?: AbortSignal): Promise<string>
  writeText(
    target: FsTarget,
    content: string,
    expected?: FsWriteIntent,
    signal?: AbortSignal,
  ): Promise<FsWriteOutcome>
  contains(parent: FsTarget, child: FsTarget): boolean
}

/** The state root every write must resolve beneath (project-relative). */
export const STATE_ROOT = '.xiaotao'

/** The lock directory inside the state root. */
const LOCK_ROOT = '.xiaotao/locks'

/**
 * A point-in-time read of a mutable state file: its resolved target, its
 * opaque freshness token (the CAS guard), and its decoded text content.
 */
export interface StateSnapshot {
  /** Resolved target for follow-up operations. */
  target: FsTarget
  /** Opaque version token used by {@link XiaoTaoStateStore.writeGuarded}. */
  version: FsVersion
  /** Decoded UTF-8 content at read time. */
  content: string
}

/**
 * Raised when a supplied state path violates the `.xiaotao/` containment rule
 * (absolute path, `..` traversal, or a target that escapes the state root).
 */
export class StatePathError extends Error {}

/** Reject absolute paths and `..` traversal before any backend call. */
function assertSafeStatePath(statePath: string): void {
  const normalized = statePath.replace(/\\/g, '/')
  if (
    normalized.startsWith('/') ||
    /^[A-Za-z]:/.test(normalized) ||
    normalized.split('/').some((segment) => segment === '..')
  ) {
    throw new StatePathError(
      `@xiaotao-ai/dsh-adapter: invalid state path "${statePath}" — absolute paths and ` +
        '`..` traversal are rejected per storage.md File rules.',
    )
  }
}

/** Narrow an unknown error to a dsh-fs error carrying the given code. */
function isFsError(error: unknown, code: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code?: unknown }).code === code
  )
}

/**
 * Deterministic state store over the dsh filesystem seam.
 *
 * Not a Cordis service class on purpose: the adapter keeps its dsh-facing
 * surface in `index.ts` and passes a bare `FileSystem` here so the CAS logic
 * stays unit-testable against a fake seam.
 */
export class XiaoTaoStateStore {
  /** Resolved `.xiaotao/` root target, used as the containment boundary. */
  private rootPromise?: Promise<FsTarget>

  constructor(private readonly fs: StateFileSystem, private readonly rootPath: string = STATE_ROOT) {}

  private root(): Promise<FsTarget> {
    return this.rootPromise ??= this.fs.resolve(this.rootPath)
  }

  /**
   * Resolve a state path into a stable target, enforcing the `.xiaotao/`
   * containment rule. This is the single choke point every state operation
   * routes through.
   *
   * @param statePath - project-relative path under `.xiaotao/`.
   * @throws {@link StatePathError} when the path escapes the state root.
   */
  private async resolveGuarded(statePath: string): Promise<FsTarget> {
    assertSafeStatePath(statePath)
    const [root, target] = await Promise.all([this.root(), this.fs.resolve(statePath)])
    if (!this.fs.contains(root, target)) {
      throw new StatePathError(
        `@xiaotao-ai/dsh-adapter: state path "${statePath}" resolves outside the state ` +
          `root (${STATE_ROOT}/). All state writes must live under .xiaotao/.`,
      )
    }
    return target
  }

  /**
   * Resolve a state path into a stable target (containment-checked).
   * @param statePath - project-relative path under `.xiaotao/`.
   */
  resolve(statePath: string): Promise<FsTarget> {
    return this.resolveGuarded(statePath)
  }

  /**
   * Read a mutable state file, capturing both its content and its freshness
   * token. Returns `undefined` when the file does not exist.
   *
   * @param statePath - project-relative path under `.xiaotao/`.
   */
  async readSnapshot(statePath: string): Promise<StateSnapshot | undefined> {
    const target = await this.resolveGuarded(statePath)
    const info = await this.fs.stat(target)
    if (info === undefined) return undefined
    const content = await this.fs.readText(target)
    return { target, version: info.version, content }
  }

  /**
   * Atomically replace a state file only if it still matches the snapshot the
   * caller read. A concurrent write makes the seam throw `FS_STALE_VERSION`;
   * the caller re-reads and retries (or surfaces the conflict), exactly as
   * `storage.md` prescribes for a revision mismatch.
   *
   * @param snapshot - the {@link readSnapshot} result the edit was based on.
   * @param content - the complete replacement text.
   */
  async writeGuarded(snapshot: StateSnapshot, content: string): Promise<FsWriteOutcome> {
    // Re-verify containment here, not just in readSnapshot(): StateSnapshot is
    // a public interface, so a caller could hand us a fabricated snapshot whose
    // target escapes `.xiaotao/`. The canonical contains() check makes that
    // impossible regardless of how the snapshot was obtained.
    const root = await this.root()
    if (!this.fs.contains(root, snapshot.target)) {
      throw new StatePathError(
        `@xiaotao-ai/dsh-adapter: snapshot target escapes the state root (${STATE_ROOT}/).`,
      )
    }
    return this.fs.writeText(snapshot.target, content, {
      kind: 'replaceIfVersion',
      version: snapshot.version,
    })
  }

  /**
   * Atomically create a file, failing if it already exists. This is the
   * exclusive create-if-absent primitive XiaoTao's lock protocol needs.
   *
   * @param statePath - project-relative path under `.xiaotao/`.
   * @param content - lock owner / lease metadata.
   */
  async createIfAbsent(statePath: string, content: string): Promise<FsWriteOutcome> {
    const target = await this.resolveGuarded(statePath)
    return this.fs.writeText(target, content, { kind: 'createIfAbsent' })
  }

  /**
   * Overwrite a state file. `version` turns the write into a guarded replace
   * (`replaceIfVersion`); omitting it is an unconditional overwrite. Used by
   * the lock protocol to tombstone a released lock.
   *
   * @param statePath - project-relative path under `.xiaotao/`.
   * @param content - the complete replacement text.
   * @param version - when present, only replace if the file still has this version.
   */
  async release(
    statePath: string,
    content: string,
    version?: FsVersion,
  ): Promise<FsWriteOutcome> {
    const target = await this.resolveGuarded(statePath)
    if (version !== undefined) {
      return this.fs.writeText(target, content, { kind: 'replaceIfVersion', version })
    }
    return this.fs.writeText(target, content)
  }

  /**
   * Derive the stable lock path for a state path. Mirrors `storage.md`: a
   * normalized project-relative state key maps to a dedicated lock entry under
   * `.xiaotao/locks/`.
   *
   * @param statePath - project-relative path under `.xiaotao/`.
   * @throws {@link StatePathError} when the path escapes the project root.
   */
  lockPathFor(statePath: string): string {
    assertSafeStatePath(statePath)
    // NOTE: flat key mapping — `/` collapses to `-`, so `a/b` and a file
    // literally named `a-b` share a key. State paths under `.xiaotao/` are
    // directory-structured (tasks/<id>/…, memory/…), so this does not collide
    // in practice; a length-prefixed encoding can replace it if it ever does.
    const key = statePath
      .replace(/\\/g, '/')
      .replace(/^(\.\/)+/, '')
      .replace(/^\.xiaotao\//, '')
      .replace(/\/+/g, '-')
    return `${LOCK_ROOT}/${key}.lock`
  }
}

/** Lease metadata recorded inside a lock file. */
export interface LockLease {
  owner: string
  acquiredAt: string
  expiresAt: string
  /** `held` while owned; `released` once the owner tombstoned it on exit. */
  state?: 'held' | 'released'
  /** Set when the owner released the lock (tombstone). */
  releasedAt?: string
}

/** Tunables for {@link acquireLock}. */
export interface AcquireLockOptions {
  /** Cooperatively stop lock acquisition while preserving any acquired-lock cleanup. */
  signal?: AbortSignal
  /** Lease lifetime in milliseconds (default 5 minutes). */
  leaseMs?: number
  /** Bounded contention window in milliseconds (default 30 seconds). */
  timeoutMs?: number
  /** Delay between contention retries in milliseconds (default 250). */
  retryDelayMs?: number
  /**
   * Authorize reclaiming an expired-but-still-`held` lock. Invoked with the
   * recorded lease; return `true` only after confirming the recorded owner is
   * actually inactive. Per `storage.md`, clock age alone is insufficient proof
   * of inactivity — when this option is omitted, an expired `held` lock is
   * **never** auto-reclaimed and the caller surfaces the conflict instead.
   */
  canReclaim?: (lease: LockLease) => boolean | Promise<boolean>
}

const DEFAULT_LEASE_MS = 5 * 60 * 1000
const DEFAULT_TIMEOUT_MS = 30 * 1000
const DEFAULT_RETRY_DELAY_MS = 250

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) return Promise.reject(signal.reason)
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', abort)
      resolve()
    }, ms)
    const abort = () => {
      clearTimeout(timer)
      reject(signal?.reason)
    }
    signal?.addEventListener('abort', abort, { once: true })
  })
}

/** Parse a lock file's JSON lease; `undefined` for unparseable content. */
function parseLease(content: string): LockLease | undefined {
  try {
    const parsed = JSON.parse(content) as Partial<LockLease>
    if (
      typeof parsed.owner === 'string' &&
      typeof parsed.acquiredAt === 'string' &&
      typeof parsed.expiresAt === 'string'
    ) {
      return parsed as LockLease
    }
  } catch {
    // Unparseable lock content is treated as "held by an unknown owner" below
    // (never auto-reclaimed), so corruption cannot silently drop a lock.
  }
  return undefined
}

/**
 * Build the release function for a lock the caller just won. Release is
 * idempotent (guarded by a local flag) and a guarded replace: it only
 * overwrites the version we acquired, so if a reclaimer already replaced the
 * lock after our lease lapsed, the release is a harmless no-op.
 */
function makeRelease(
  store: XiaoTaoStateStore,
  lockPath: string,
  version: FsVersion,
  held: LockLease,
): () => Promise<void> {
  let done = false
  return async () => {
    if (done) return
    const released: LockLease = {
      ...held,
      state: 'released',
      releasedAt: new Date().toISOString(),
    }
    try {
      await store.release(lockPath, JSON.stringify(released), version)
      done = true
    } catch (error) {
      if (isFsError(error, 'FS_STALE_VERSION')) {
        // We no longer own the lock (a reclaimer replaced it) — nothing to
        // release, so treat the release as complete and don't retry.
        done = true
      } else {
        // Transient failure: leave `done` unset so a later release() call can
        // retry instead of silently giving up on a still-held lock.
        throw error
      }
    }
  }
}

/**
 * Acquire an exclusive lock for a state path, returning a release function.
 *
 * dsh's `FileSystem` seam currently exposes no delete/remove primitive, so a
 * lock file cannot be cleanly removed on release. Release instead writes a
 * `state: 'released'` tombstone; the next acquisition sees that tombstone and
 * reclaims immediately, so a normal release is re-acquirable right away.
 *
 * Reclaiming an *expired but still held* lock is gated on
 * {@link AcquireLockOptions.canReclaim}: `storage.md` requires the recorded
 * owner to be known inactive before reclaim, and clock age alone is not proof.
 * Without `canReclaim` the adapter never force-reclaims, and the caller is
 * responsible for recording the reclaim in a transaction or Evidence record
 * once it does authorize one.
 *
 * @param store - the state store.
 * @param statePath - project-relative path under `.xiaotao/`.
 * @param owner - the acquiring actor's identifier.
 * @param options - lease/contention/reclaim tunables.
 * @returns an async release function (safe to call once).
 * @throws on lock contention past the bounded timeout.
 */
export async function acquireLock(
  store: XiaoTaoStateStore,
  statePath: string,
  owner: string,
  options: AcquireLockOptions = {},
): Promise<() => Promise<void>> {
  const leaseMs = options.leaseMs ?? DEFAULT_LEASE_MS
  const deadline = Date.now() + (options.timeoutMs ?? DEFAULT_TIMEOUT_MS)
  const retryDelayMs = options.retryDelayMs ?? DEFAULT_RETRY_DELAY_MS
  const lockPath = store.lockPathFor(statePath)

  for (;;) {
    options.signal?.throwIfAborted()
    const now = Date.now()
    const held: LockLease = {
      owner,
      acquiredAt: new Date(now).toISOString(),
      expiresAt: new Date(now + leaseMs).toISOString(),
      state: 'held',
    }
    const heldPayload = JSON.stringify(held)

    // 1. Exclusive create. Winning here means we own a fresh lock.
    try {
      const outcome = await store.createIfAbsent(lockPath, heldPayload)
      return makeRelease(store, lockPath, outcome.version, held)
    } catch (error) {
      if (!isFsError(error, 'FS_NOT_OBSERVED')) throw error
    }

    // 2. The lock already exists — decide whether it is reclaimable.
    const snapshot = await store.readSnapshot(lockPath)
    const lease = snapshot === undefined ? undefined : parseLease(snapshot.content)
    const released = lease?.state === 'released'
    const expired = lease !== undefined && Date.parse(lease.expiresAt) <= Date.now()

    // A released tombstone is explicit proof the owner finished → always safe.
    let reclaimable = released
    if (!reclaimable && expired && options.canReclaim !== undefined) {
      // storage.md: clock age alone is insufficient — confirm owner inactive.
      options.signal?.throwIfAborted()
      reclaimable = await options.canReclaim(lease!)
    }

    if (reclaimable && snapshot !== undefined) {
      try {
        const outcome = await store.writeGuarded(snapshot, heldPayload)
        return makeRelease(store, lockPath, outcome.version, held)
      } catch (error) {
        // A concurrent reclaimer won the CAS; fall through to bounded retry.
        if (!isFsError(error, 'FS_STALE_VERSION')) throw error
      }
    }

    if (Date.now() + retryDelayMs > deadline) {
      throw new Error(
        `@xiaotao-ai/dsh-adapter: lock contention on "${statePath}" — still held after ` +
          `${options.timeoutMs ?? DEFAULT_TIMEOUT_MS}ms. Never force the write; surface the ` +
          'conflict per storage.md.',
      )
    }
    await sleep(retryDelayMs, options.signal)
  }
}
