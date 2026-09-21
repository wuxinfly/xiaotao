/**
 * Restart-recoverable multi-file transaction mechanism for the DSH adapter.
 *
 * DSH exposes atomic single-file create/CAS writes, but no atomic batch,
 * rename, or delete seam. Logical atomicity therefore comes from an immutable
 * terminal marker plus an overlay view; canonical files are materialized only
 * after the commit marker exists. This service intentionally supports only
 * create/replace members. Lifecycle moves remain unsupported until the host
 * exposes a trustworthy delete/rename primitive.
 */

import { createHash } from 'node:crypto'
import type { FsDirEntry, FsTarget } from '@deepseek-ai/dsh-fs'
import { parse, stringify } from 'yaml'
import {
  acquireLock,
  XiaoTaoStateStore,
  type AcquireLockOptions,
  type StateFileSystem,
} from './storage'

const TX_ROOT = '.xiaotao/transactions'
const MAX_MEMBERS = 32
const MAX_MEMBER_BYTES = 128 * 1024
const MAX_TRANSACTION_BYTES = 512 * 1024
const ID = /^[a-z0-9][a-z0-9_-]{0,63}$/

export interface TransactionFileSystem extends StateFileSystem {
  listDir(target: FsTarget, signal?: AbortSignal): Promise<FsDirEntry[]>
}

export interface TransactionMemberInput {
  kind: 'create' | 'replace'
  path: string
  staged: string
  base_hash?: string
  base_revision?: number
  intended_revision?: number
}

export interface TransactionInput {
  transaction_id: string
  operation: string
  actor: string
  members: TransactionMemberInput[]
}

export interface TransactionExecuteOptions {
  signal?: AbortSignal
  /** Required fail-closed seam for schema, source-ref, lifecycle, and operation-specific checks. */
  validateMember: (member: Readonly<TransactionIntentMember>, staged: string) => void | Promise<void>
  locks?: AcquireLockOptions
  /** Test/diagnostic hook. Production callers normally omit it. */
  afterStep?: (step: string) => void | Promise<void>
}

export interface TransactionRecoverOptions {
  actor: string
  signal?: AbortSignal
  locks?: AcquireLockOptions
  afterStep?: (step: string) => void | Promise<void>
}

interface SnapshotRef {
  state: 'absent' | 'present'
  path?: string
  sha256?: string
  revision?: number
  /** Evidence only. Recovery obtains a new opaque token from stat(). */
  observed_version?: string
}

export interface TransactionIntentMember {
  sequence: number
  kind: 'create' | 'replace'
  path: string
  before: SnapshotRef
  staged_path: string
  staged_sha256: string
  intended_revision?: number
}

interface TransactionIntent {
  schema_version: 1
  transaction_id: string
  operation: string
  actor: string
  created_at: string
  members: TransactionIntentMember[]
}

interface TerminalRecord {
  schema_version: 1
  transaction_id: string
  intent_sha256: string
  actor: string
  recorded_at: string
  result: 'committed' | 'failed'
  reason?: string
}

interface AppliedRecord {
  schema_version: 1
  transaction_id: string
  sequence: number
  path: string
  staged_sha256: string
  actor: string
  applied_at: string
}

interface LoadedTransaction {
  intent: TransactionIntent
  intentText: string
  before: Array<string | undefined>
  staged: string[]
  terminal?: TerminalRecord
}

export type TransactionStatus =
  | { status: 'missing'; transaction_id: string }
  | { status: 'prepared'; transaction_id: string }
  | { status: 'failed'; transaction_id: string; reason?: string }
  | { status: 'committed'; transaction_id: string; materialization: 'complete' | 'pending' | 'conflict'; applied: number; members: number }

export interface OverlayRead {
  source: 'canonical' | 'overlay' | 'absent'
  content?: string
  transaction_id?: string
}

export class TransactionError extends Error {
  constructor(readonly code: string, message = code) {
    super(message)
    this.name = 'TransactionError'
  }
}

function fail(condition: unknown, code: string): asserts condition {
  if (!condition) throw new TransactionError(code)
}

function checkSignal(signal?: AbortSignal): void {
  if (signal?.aborted) throw new TransactionError('transaction_cancelled')
}

function isFsError(error: unknown, code: string): boolean {
  return typeof error === 'object' && error !== null && (error as { code?: unknown }).code === code
}

function sha256(content: string): string {
  return createHash('sha256').update(content, 'utf8').digest('hex')
}

function bytes(content: string): number {
  return Buffer.byteLength(content, 'utf8')
}

function safeText(value: unknown, max: number): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= max && !/[\u0000-\u001f\u007f]/.test(value)
}

function normalizeStatePath(value: unknown): string {
  fail(typeof value === 'string', 'invalid_member_path')
  const normalized = value.replaceAll('\\', '/').replace(/^\.\//, '')
  fail(normalized.startsWith('.xiaotao/'), 'invalid_member_path')
  fail(!normalized.startsWith(`${TX_ROOT}/`) && !normalized.startsWith('.xiaotao/locks/'), 'reserved_member_path')
  fail(!normalized.split('/').some((part) => part === '' || part === '.' || part === '..'), 'invalid_member_path')
  fail(!/^[A-Za-z]:/.test(normalized) && !normalized.startsWith('/'), 'invalid_member_path')
  return normalized
}

function revisionOf(content: string): number | undefined {
  try {
    let value: unknown
    if (content.startsWith('---\n')) {
      const end = content.indexOf('\n---', 4)
      if (end < 0) return undefined
      value = parse(content.slice(4, end))
    } else {
      value = JSON.parse(content)
    }
    if (typeof value !== 'object' || value === null) return undefined
    const revision = (value as { revision?: unknown }).revision
    return Number.isInteger(revision) && Number(revision) >= 0 ? Number(revision) : undefined
  } catch {
    return undefined
  }
}

function txPath(id: string, suffix: string): string {
  return `${TX_ROOT}/${id}/${suffix}`
}

function snapshotName(sequence: number): string {
  return String(sequence).padStart(4, '0') + '.txt'
}

function yaml(value: unknown): string {
  return stringify(value, { lineWidth: 0, sortMapEntries: false })
}

function comparePath(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

function parseYaml(content: string, code: string): unknown {
  try { return parse(content) }
  catch { throw new TransactionError(code) }
}

function object(value: unknown, code: string): Record<string, unknown> {
  fail(typeof value === 'object' && value !== null && !Array.isArray(value), code)
  return value as Record<string, unknown>
}

function validateId(id: unknown): asserts id is string {
  fail(typeof id === 'string' && ID.test(id), 'invalid_transaction_id')
}

function validateInput(input: TransactionInput): TransactionInput {
  fail(typeof input === 'object' && input !== null, 'invalid_transaction')
  validateId(input.transaction_id)
  fail(safeText(input.operation, 128), 'invalid_operation')
  fail(safeText(input.actor, 128), 'invalid_actor')
  fail(Array.isArray(input.members) && input.members.length > 0 && input.members.length <= MAX_MEMBERS, 'invalid_members')
  const seen = new Set<string>()
  let total = 0
  const members = input.members.map((raw) => {
    const member = object(raw, 'invalid_member') as unknown as TransactionMemberInput
    fail(member.kind === 'create' || member.kind === 'replace', 'unsupported_member_kind')
    const path = normalizeStatePath(member.path)
    fail(!seen.has(path), 'duplicate_member_path')
    seen.add(path)
    fail(typeof member.staged === 'string' && bytes(member.staged) <= MAX_MEMBER_BYTES, 'member_too_large')
    total += bytes(member.staged)
    if (member.kind === 'create') {
      fail(member.base_hash === undefined && member.base_revision === undefined, 'invalid_create_precondition')
    } else {
      fail(typeof member.base_hash === 'string' && /^[a-f0-9]{64}$/.test(member.base_hash), 'invalid_replace_precondition')
      if (member.base_revision !== undefined) fail(Number.isInteger(member.base_revision) && member.base_revision >= 0, 'invalid_base_revision')
    }
    if (member.intended_revision !== undefined) {
      fail(Number.isInteger(member.intended_revision) && member.intended_revision >= 0, 'invalid_intended_revision')
    }
    return { ...member, path }
  }).sort((a, b) => comparePath(a.path, b.path))
  fail(total <= MAX_TRANSACTION_BYTES, 'transaction_too_large')
  return { ...input, members }
}

/** Internal transaction mechanism. It is not a model-facing tool. */
export class XiaoTaoTransactionStore {
  private readonly store: XiaoTaoStateStore

  constructor(private readonly fs: TransactionFileSystem) {
    this.store = new XiaoTaoStateStore(fs)
  }

  private async immutable(path: string, content: string, signal?: AbortSignal): Promise<void> {
    checkSignal(signal)
    try {
      await this.store.createIfAbsent(path, content)
    } catch (error) {
      if (!isFsError(error, 'FS_NOT_OBSERVED')) throw error
    }
    checkSignal(signal)
    const actual = await this.store.readSnapshot(path)
    fail(actual?.content === content, 'immutable_conflict')
  }

  private async acquire(paths: string[], owner: string, options?: AcquireLockOptions): Promise<Array<() => Promise<void>>> {
    const releases: Array<() => Promise<void>> = []
    try {
      for (const path of [...new Set(paths)].sort(comparePath)) {
        checkSignal(options?.signal)
        releases.push(await acquireLock(this.store, path, owner, options))
      }
      return releases
    } catch (error) {
      await Promise.allSettled(releases.reverse().map((release) => release()))
      throw error
    }
  }

  private async releaseAll(releases: Array<() => Promise<void>>): Promise<void> {
    const results = await Promise.allSettled(releases.reverse().map((release) => release()))
    fail(!results.some((result) => result.status === 'rejected'), 'lock_release_failed')
  }

  private async prepare(input: TransactionInput, options: TransactionExecuteOptions): Promise<LoadedTransaction> {
    fail(!(await this.store.readSnapshot(txPath(input.transaction_id, 'intent.yaml'))), 'transaction_exists')
    const before: Array<string | undefined> = []
    const staged: string[] = []
    const intentMembers: TransactionIntentMember[] = []

    for (let index = 0; index < input.members.length; index += 1) {
      checkSignal(options.signal)
      const member = input.members[index]!
      const sequence = index + 1
      const current = await this.store.readSnapshot(member.path)
      if (member.kind === 'create') {
        fail(current === undefined, 'precondition_conflict')
      } else {
        fail(current !== undefined && sha256(current.content) === member.base_hash, 'precondition_conflict')
        fail(bytes(current.content) <= MAX_MEMBER_BYTES, 'before_member_too_large')
        if (member.base_revision !== undefined) fail(revisionOf(current.content) === member.base_revision, 'precondition_conflict')
      }
      if (member.intended_revision !== undefined) fail(revisionOf(member.staged) === member.intended_revision, 'invalid_staged_revision')
      before.push(current?.content)
      staged.push(member.staged)
      const name = snapshotName(sequence)
      const beforePath = current ? txPath(input.transaction_id, `before/${name}`) : undefined
      const stagedPath = txPath(input.transaction_id, `staged/${name}`)
      intentMembers.push({
        sequence,
        kind: member.kind,
        path: member.path,
        before: current ? {
          state: 'present', path: beforePath, sha256: sha256(current.content),
          ...(member.base_revision === undefined ? {} : { revision: member.base_revision }),
          observed_version: String(current.version),
        } : { state: 'absent' },
        staged_path: stagedPath,
        staged_sha256: sha256(member.staged),
        ...(member.intended_revision === undefined ? {} : { intended_revision: member.intended_revision }),
      })
    }

    const intent: TransactionIntent = {
      schema_version: 1,
      transaction_id: input.transaction_id,
      operation: input.operation,
      actor: input.actor,
      created_at: new Date().toISOString(),
      members: intentMembers,
    }
    const intentText = yaml(intent)
    fail(bytes(intentText) + before.reduce((n, value) => n + bytes(value ?? ''), 0)
      + staged.reduce((n, value) => n + bytes(value), 0) <= MAX_TRANSACTION_BYTES * 2, 'transaction_record_too_large')
    for (const member of intentMembers) {
      checkSignal(options.signal)
      if (member.before.state === 'present') {
        await this.immutable(member.before.path!, before[member.sequence - 1]!, options.signal)
      }
      await this.immutable(member.staged_path, staged[member.sequence - 1]!, options.signal)
      await options.afterStep?.(`snapshot:${member.sequence}`)
      checkSignal(options.signal)
    }
    await this.immutable(txPath(input.transaction_id, 'intent.yaml'), intentText, options.signal)
    await options.afterStep?.('intent')
    checkSignal(options.signal)
    return { intent, intentText, before, staged }
  }

  private assertSameRequest(loaded: LoadedTransaction, input: TransactionInput): void {
    fail(loaded.intent.operation === input.operation && loaded.intent.actor === input.actor
      && loaded.intent.members.length === input.members.length, 'transaction_request_conflict')
    for (let index = 0; index < input.members.length; index += 1) {
      const expected = input.members[index]!
      const actual = loaded.intent.members[index]!
      fail(actual.kind === expected.kind && actual.path === expected.path
        && loaded.staged[index] === expected.staged
        && actual.intended_revision === expected.intended_revision,
      'transaction_request_conflict')
      if (expected.kind === 'create') {
        fail(actual.before.state === 'absent', 'transaction_request_conflict')
      } else {
        fail(actual.before.state === 'present' && actual.before.sha256 === expected.base_hash
          && actual.before.revision === expected.base_revision,
        'transaction_request_conflict')
      }
    }
  }

  private async revalidateCanonicalPreconditions(loaded: LoadedTransaction): Promise<void> {
    for (const member of loaded.intent.members) {
      const current = await this.store.readSnapshot(member.path)
      if (member.kind === 'create') {
        fail(current === undefined, 'precondition_conflict')
        continue
      }
      fail(current !== undefined && sha256(current.content) === member.before.sha256,
        'precondition_conflict')
      if (member.before.revision !== undefined) {
        fail(revisionOf(current.content) === member.before.revision, 'precondition_conflict')
      }
    }
  }

  private async terminal(id: string, result: 'committed' | 'failed', intentText: string, actor: string,
    reason?: string, signal?: AbortSignal): Promise<TerminalRecord> {
    checkSignal(signal)
    const opposite = result === 'committed' ? 'failed.yaml' : 'committed.yaml'
    fail(!(await this.store.readSnapshot(txPath(id, opposite))), 'terminal_conflict')
    const record: TerminalRecord = {
      schema_version: 1,
      transaction_id: id,
      intent_sha256: sha256(intentText),
      actor,
      recorded_at: new Date().toISOString(),
      result,
      ...(reason === undefined ? {} : { reason }),
    }
    await this.immutable(txPath(id, `${result}.yaml`), yaml(record), signal)
    checkSignal(signal)
    fail(!(await this.store.readSnapshot(txPath(id, opposite))), 'terminal_conflict')
    return record
  }

  private validateIntent(raw: unknown, id: string): TransactionIntent {
    const value = object(raw, 'invalid_intent')
    fail(value.schema_version === 1 && value.transaction_id === id, 'invalid_intent')
    fail(safeText(value.operation, 128) && safeText(value.actor, 128), 'invalid_intent')
    fail(typeof value.created_at === 'string' && Number.isFinite(Date.parse(value.created_at)), 'invalid_intent')
    fail(Array.isArray(value.members) && value.members.length > 0 && value.members.length <= MAX_MEMBERS, 'invalid_intent')
    const paths = new Set<string>()
    const members = value.members.map((item, index) => {
      const member = object(item, 'invalid_intent_member')
      const sequence = index + 1
      fail(member.sequence === sequence && (member.kind === 'create' || member.kind === 'replace'), 'invalid_intent_member')
      const path = normalizeStatePath(member.path)
      fail(!paths.has(path), 'duplicate_member_path')
      paths.add(path)
      const before = object(member.before, 'invalid_intent_member')
      fail(before.state === 'absent' || before.state === 'present', 'invalid_intent_member')
      if (member.kind === 'create') fail(before.state === 'absent', 'invalid_intent_member')
      if (member.kind === 'replace') {
        fail(before.state === 'present' && typeof before.path === 'string'
          && typeof before.sha256 === 'string' && /^[a-f0-9]{64}$/.test(before.sha256), 'invalid_intent_member')
        fail(before.path === txPath(id, `before/${snapshotName(sequence)}`), 'invalid_snapshot_path')
        fail(typeof before.observed_version === 'string' && before.observed_version.length > 0,
          'invalid_intent_member')
        if (before.revision !== undefined) {
          fail(Number.isInteger(before.revision) && Number(before.revision) >= 0, 'invalid_intent_member')
        }
      }
      fail(member.staged_path === txPath(id, `staged/${snapshotName(sequence)}`), 'invalid_snapshot_path')
      fail(typeof member.staged_sha256 === 'string' && /^[a-f0-9]{64}$/.test(member.staged_sha256), 'invalid_intent_member')
      if (member.intended_revision !== undefined) {
        fail(Number.isInteger(member.intended_revision) && Number(member.intended_revision) >= 0,
          'invalid_intent_member')
      }
      return { ...member, path, before } as unknown as TransactionIntentMember
    })
    const sorted = [...members].sort((a, b) => comparePath(a.path, b.path))
    fail(members.every((member, index) => member.path === sorted[index]!.path), 'unstable_member_order')
    return { ...value, members } as unknown as TransactionIntent
  }

  private validateTerminal(raw: unknown, loaded: Pick<LoadedTransaction, 'intent' | 'intentText'>, expected: 'committed' | 'failed'): TerminalRecord {
    const value = object(raw, 'invalid_terminal')
    fail(value.schema_version === 1 && value.transaction_id === loaded.intent.transaction_id
      && value.intent_sha256 === sha256(loaded.intentText) && value.result === expected,
    'invalid_terminal')
    fail(safeText(value.actor, 128) && typeof value.recorded_at === 'string'
      && Number.isFinite(Date.parse(value.recorded_at)), 'invalid_terminal')
    return value as unknown as TerminalRecord
  }

  private async load(id: string): Promise<LoadedTransaction | undefined> {
    validateId(id)
    const intentSnapshot = await this.store.readSnapshot(txPath(id, 'intent.yaml'))
    if (!intentSnapshot) return undefined
    fail(bytes(intentSnapshot.content) <= MAX_TRANSACTION_BYTES, 'intent_too_large')
    const intent = this.validateIntent(parseYaml(intentSnapshot.content, 'invalid_intent'), id)
    const before: Array<string | undefined> = []
    const staged: string[] = []
    for (const member of intent.members) {
      let beforeText: string | undefined
      if (member.before.state === 'present') {
        const snapshot = await this.store.readSnapshot(member.before.path!)
        fail(snapshot && bytes(snapshot.content) <= MAX_MEMBER_BYTES
          && sha256(snapshot.content) === member.before.sha256, 'invalid_before_snapshot')
        beforeText = snapshot.content
      }
      const snapshot = await this.store.readSnapshot(member.staged_path)
      fail(snapshot && bytes(snapshot.content) <= MAX_MEMBER_BYTES
        && sha256(snapshot.content) === member.staged_sha256, 'invalid_staged_snapshot')
      before.push(beforeText)
      staged.push(snapshot.content)
    }
    const committed = await this.store.readSnapshot(txPath(id, 'committed.yaml'))
    const failed = await this.store.readSnapshot(txPath(id, 'failed.yaml'))
    fail(!(committed && failed), 'terminal_conflict')
    const loaded: LoadedTransaction = { intent, intentText: intentSnapshot.content, before, staged }
    if (committed) {
      fail(bytes(committed.content) <= 16 * 1024, 'invalid_terminal')
      loaded.terminal = this.validateTerminal(parseYaml(committed.content, 'invalid_terminal'), loaded, 'committed')
    }
    if (failed) {
      fail(bytes(failed.content) <= 16 * 1024, 'invalid_terminal')
      loaded.terminal = this.validateTerminal(parseYaml(failed.content, 'invalid_terminal'), loaded, 'failed')
    }
    return loaded
  }

  private appliedPath(loaded: LoadedTransaction, member: TransactionIntentMember): string {
    return txPath(loaded.intent.transaction_id,
      `applied/${snapshotName(member.sequence).replace(/\.txt$/, '.yaml')}`)
  }

  private async hasApplied(loaded: LoadedTransaction, member: TransactionIntentMember): Promise<boolean> {
    const existing = await this.store.readSnapshot(this.appliedPath(loaded, member))
    if (!existing) return false
    const value = object(parseYaml(existing.content, 'invalid_applied_record'), 'invalid_applied_record')
    fail(value.schema_version === 1 && value.transaction_id === loaded.intent.transaction_id
      && value.sequence === member.sequence && value.path === member.path
      && value.staged_sha256 === member.staged_sha256, 'invalid_applied_record')
    fail(safeText(value.actor, 128) && typeof value.applied_at === 'string'
      && Number.isFinite(Date.parse(value.applied_at)), 'invalid_applied_record')
    return true
  }

  private async applied(loaded: LoadedTransaction, member: TransactionIntentMember, actor: string,
    signal?: AbortSignal): Promise<void> {
    checkSignal(signal)
    if (await this.hasApplied(loaded, member)) return
    const record: AppliedRecord = {
      schema_version: 1,
      transaction_id: loaded.intent.transaction_id,
      sequence: member.sequence,
      path: member.path,
      staged_sha256: member.staged_sha256,
      actor,
      applied_at: new Date().toISOString(),
    }
    await this.immutable(this.appliedPath(loaded, member), yaml(record), signal)
  }

  private async materialize(loaded: LoadedTransaction, actor: string,
    afterStep?: (step: string) => void | Promise<void>, signal?: AbortSignal): Promise<void> {
    fail(loaded.terminal?.result === 'committed', 'transaction_not_committed')
    for (let index = 0; index < loaded.intent.members.length; index += 1) {
      checkSignal(signal)
      const member = loaded.intent.members[index]!
      const staged = loaded.staged[index]!
      if (await this.hasApplied(loaded, member)) continue
      let current = await this.store.readSnapshot(member.path)
      if (!current || sha256(current.content) !== member.staged_sha256) {
        if (member.kind === 'create') {
          fail(current === undefined, 'materialization_conflict')
          try { await this.store.createIfAbsent(member.path, staged) }
          catch (error) {
            if (!isFsError(error, 'FS_NOT_OBSERVED')) throw error
          }
        } else {
          fail(current !== undefined && sha256(current.content) === member.before.sha256, 'materialization_conflict')
          try { await this.store.writeGuarded(current, staged) }
          catch (error) {
            if (!isFsError(error, 'FS_STALE_VERSION')) throw error
          }
        }
        current = await this.store.readSnapshot(member.path)
        fail(current && sha256(current.content) === member.staged_sha256, 'materialization_conflict')
      }
      await afterStep?.(`materialized:${member.sequence}`)
      checkSignal(signal)
      await this.applied(loaded, member, actor, signal)
      await afterStep?.(`applied:${member.sequence}`)
      checkSignal(signal)
    }
  }

  async execute(raw: TransactionInput, options: TransactionExecuteOptions): Promise<TransactionStatus> {
    fail(typeof options?.validateMember === 'function', 'transaction_validator_required')
    const input = validateInput(raw)
    const owner = `transaction/${input.transaction_id}/${input.actor}`
    const lockPaths = [txPath(input.transaction_id, 'terminal'), ...input.members.map((member) => member.path)]
    const releases = await this.acquire(lockPaths, owner, options.locks)
    let prepared: LoadedTransaction | undefined
    let committed = false
    try {
      checkSignal(options.signal)
      const existing = await this.load(input.transaction_id)
      if (existing) {
        this.assertSameRequest(existing, input)
        prepared = existing
        if (existing.terminal?.result === 'failed') return this.status(input.transaction_id)
        if (existing.terminal?.result === 'committed') {
          committed = true
          try { await this.materialize(existing, input.actor, options.afterStep, options.signal) }
          catch { return this.status(input.transaction_id) }
          return this.status(input.transaction_id)
        }
      }
      for (const member of input.members) {
        fail((await this.readOverlay(member.path)).source !== 'overlay', 'transaction_overlay_conflict')
      }
      prepared ??= await this.prepare(input, options)
      for (let index = 0; index < prepared.intent.members.length; index += 1) {
        checkSignal(options.signal)
        await options.validateMember(prepared.intent.members[index]!, prepared.staged[index]!)
      }
      checkSignal(options.signal)
      await this.revalidateCanonicalPreconditions(prepared)
      checkSignal(options.signal)
      prepared.terminal = await this.terminal(input.transaction_id, 'committed', prepared.intentText,
        input.actor, undefined, options.signal)
      committed = true
      try {
        await options.afterStep?.('committed')
        checkSignal(options.signal)
        await this.materialize(prepared, input.actor, options.afterStep, options.signal)
      } catch {
        return this.status(input.transaction_id)
      }
      return this.status(input.transaction_id)
    } catch (error) {
      if (prepared && !committed && !options.signal?.aborted) {
        const reason = error instanceof TransactionError ? error.code : 'precommit_failure'
        try { prepared.terminal = await this.terminal(input.transaction_id, 'failed', prepared.intentText, input.actor, reason) }
        catch { /* Preserve the original failure; status will surface terminal corruption if present. */ }
      }
      throw error
    } finally {
      await this.releaseAll(releases)
    }
  }

  async status(id: string): Promise<TransactionStatus> {
    const loaded = await this.load(id)
    if (!loaded) return { status: 'missing', transaction_id: id }
    if (!loaded.terminal) return { status: 'prepared', transaction_id: id }
    if (loaded.terminal.result === 'failed') {
      return { status: 'failed', transaction_id: id, ...(loaded.terminal.reason ? { reason: loaded.terminal.reason } : {}) }
    }
    let applied = 0
    let pending = false
    let conflict = false
    for (let index = 0; index < loaded.intent.members.length; index += 1) {
      const member = loaded.intent.members[index]!
      if (await this.hasApplied(loaded, member)) {
        applied += 1
        continue
      }
      const current = await this.store.readSnapshot(member.path)
      const currentHash = current && sha256(current.content)
      if (currentHash === member.staged_sha256) {
        pending = true
      } else if ((member.before.state === 'absent' && !current)
        || (member.before.state === 'present' && currentHash === member.before.sha256)) pending = true
      else conflict = true
    }
    return {
      status: 'committed',
      transaction_id: id,
      materialization: conflict ? 'conflict' : pending ? 'pending' : 'complete',
      applied,
      members: loaded.intent.members.length,
    }
  }

  async recover(id: string, options: TransactionRecoverOptions): Promise<TransactionStatus> {
    fail(safeText(options.actor, 128), 'invalid_actor')
    const loaded = await this.load(id)
    fail(loaded, 'transaction_not_found')
    fail(loaded.terminal?.result === 'committed', 'transaction_not_committed')
    const paths = [txPath(id, 'terminal'), ...loaded.intent.members.map((member) => member.path)]
    const releases = await this.acquire(paths, `transaction-recovery/${id}/${options.actor}`, options.locks)
    try {
      const current = await this.load(id)
      fail(current?.terminal?.result === 'committed', 'transaction_not_committed')
      checkSignal(options.signal)
      try { await this.materialize(current, options.actor, options.afterStep, options.signal) }
      catch { return this.status(id) }
      return this.status(id)
    } finally {
      await this.releaseAll(releases)
    }
  }

  async readOverlay(statePath: string): Promise<OverlayRead> {
    const path = normalizeStatePath(statePath)
    const root = await this.store.resolve(TX_ROOT)
    let entries: FsDirEntry[]
    try { entries = await this.fs.listDir(root) }
    catch (error) {
      if (isFsError(error, 'FS_NOT_FOUND')) return this.canonical(path)
      throw error
    }
    const matches: Array<{ transaction_id: string; content: string }> = []
    for (const entry of entries) {
      fail(entry.type === 'directory' && ID.test(entry.name), 'invalid_transaction_entry')
      const loaded = await this.load(entry.name)
      if (!loaded?.terminal || loaded.terminal.result !== 'committed') continue
      const status = await this.status(entry.name)
      if (status.status === 'committed' && status.materialization === 'complete') continue
      const index = loaded.intent.members.findIndex((member) => member.path === path)
      if (index >= 0) matches.push({ transaction_id: entry.name, content: loaded.staged[index]! })
    }
    fail(matches.length <= 1, 'multiple_transaction_overlays')
    return matches.length === 1
      ? { source: 'overlay', transaction_id: matches[0]!.transaction_id, content: matches[0]!.content }
      : this.canonical(path)
  }

  private async canonical(path: string): Promise<OverlayRead> {
    const snapshot = await this.store.readSnapshot(path)
    return snapshot ? { source: 'canonical', content: snapshot.content } : { source: 'absent' }
  }
}
