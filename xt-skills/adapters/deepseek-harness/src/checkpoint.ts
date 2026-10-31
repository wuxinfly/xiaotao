import path from 'node:path'
import { createHash, randomUUID } from 'node:crypto'
import { parseDocument, stringify } from 'yaml'
import { acquireLock, XiaoTaoStateStore, type StateFileSystem } from './storage'
import { XiaoTaoSchemaValidator } from './validate'
import type { FsDirEntry, FsTarget } from '@deepseek-ai/dsh-fs'

const SCHEMA = 'https://xiaotao.local/schemas/checkpoint.schema.json'
const OBSERVATION_SCHEMA = 'https://xiaotao.local/schemas/checkpoint-observation.schema.json'
const LIMIT = 128 * 1024
const START = '<!-- xiaotao-checkpoint:start -->'
const END = '<!-- xiaotao-checkpoint:end -->'
export const hash = (text: string): string => createHash('sha256').update(text).digest('hex')
const json = (value: unknown): string => JSON.stringify(value)

export interface Snapshot {
  objective: string
  confirmed: string[]
  rejected: string[]
  in_progress: string[]
  next: string[]
  open_questions: string[]
  source_refs: string[]
}
export interface CheckpointInput {
  kind: 'temporary' | 'task'
  target_id: string
  request_id: string
  base_revision: number
  base_hash: string
  snapshot: Snapshot
}
interface RecordData extends CheckpointInput {
  schema_version: 1
  project: string
  session_id: string
  input_hash: string
  source_hash: string
  proposal: string
  proposal_hash: string
}
interface ObservationData {
  request_id: string
  record_hash: string
  proposal_hash: string
  revision: number
  completion?: 'save' | 'recovery'
  committed_at?: string
}
export interface CheckpointConfig { projectRoot: string; recoveryRoot?: string; optionalRecovery?: boolean }
export interface CheckpointFs extends StateFileSystem {
  listDir(target: FsTarget, signal?: AbortSignal): Promise<FsDirEntry[]>
}
export class CheckpointError extends Error {
  constructor(readonly code: string) { super(code) }
}
function requireThat(condition: unknown, code: string): asserts condition {
  if (!condition) throw new CheckpointError(code)
}
function yamlObject(text: string): Record<string, unknown> {
  const doc = parseDocument(text, { uniqueKeys: true })
  requireThat(doc.errors.length === 0, 'invalid_yaml')
  const value = doc.toJS({ maxAliasCount: 0 })
  requireThat(value && typeof value === 'object' && !Array.isArray(value), 'invalid_yaml')
  return value
}
function markdown(text: string, tooLargeCode = 'state_too_large') {
  requireThat(Buffer.byteLength(text) <= LIMIT, tooLargeCode)
  const match = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)([\s\S]*)$/.exec(text)
  requireThat(match, 'invalid_frontmatter')
  const meta = yamlObject(match[1])
  requireThat(Number.isSafeInteger(meta.revision) && Number(meta.revision) >= 0
    && Number(meta.revision) < Number.MAX_SAFE_INTEGER, 'invalid_revision')
  requireThat(typeof meta.updated_by === 'string' && meta.updated_by.length > 0
    && typeof meta.updated_at === 'string' && Number.isFinite(Date.parse(meta.updated_at)), 'invalid_frontmatter')
  return { meta, body: match[2] }
}
function target(kind: string, id: string) {
  requireThat((kind === 'temporary' || kind === 'task') && typeof id === 'string'
    && id.length <= 128 && id === id.normalize('NFC')
    && /^(?!\.)(?!.*[.]$)[\p{L}\p{N}._-]+$/u.test(id)
    && !/^(con|prn|aux|nul|com[0-9]|lpt[0-9])(?:\.|$)/i.test(id), 'invalid_target')
  const root = kind === 'temporary' ? `.xiaotao/memory/temporary/active/${id}` : `.xiaotao/tasks/${id}`
  return { root, state: `${root}/${kind === 'temporary' ? 'current.md' : 'progress.md'}`,
    meta: `${root}/${kind === 'temporary' ? 'meta.yaml' : 'task.yaml'}` }
}
function requestId(id: string) {
  requireThat(typeof id === 'string' && /^[a-z0-9][a-z0-9_-]{0,63}$/.test(id), 'invalid_request_id')
}
function timestampWithTimezone(value: unknown): value is string {
  return typeof value === 'string'
    && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[Zz]|[+-]\d{2}:\d{2})$/.test(value)
    && Number.isFinite(Date.parse(value))
}

/** Bind every relative resolution to the configured project, never process.cwd(). */
export function scopedFs(fs: StateFileSystem, cwd: string, signal?: AbortSignal): StateFileSystem {
  return {
    resolve: (p) => fs.resolve(p, { cwd, signal }),
    stat: (t) => fs.stat(t, signal), readText: async (t) => {
      const info = await fs.stat(t, signal)
      requireThat(!info || (typeof info.size === 'number' && info.size <= LIMIT * 3), 'file_too_large')
      return fs.readText(t, signal)
    },
    writeText: (t, content, expected) => fs.writeText(t, content, expected, signal),
    contains: (a, b) => fs.contains(a, b),
  }
}

/** One invocation; no background work or process-local source of recovery truth. */
export class CheckpointWriter {
  private readonly store: XiaoTaoStateStore
  private readonly locks: XiaoTaoStateStore
  private project = ''
  private backup?: string
  private backupKey?: string
  private recovery: 'none' | 'project' | 'secondary' = 'none'
  private failureStage: 'validation' | 'secondary_write' | 'project_request' | 'canonical_write' | 'observation_write' = 'validation'
  private lastRecord?: RecordData

  constructor(private readonly fs: CheckpointFs, private readonly validator: XiaoTaoSchemaValidator,
    private readonly config: CheckpointConfig, private readonly sessionId: string,
    private readonly signal: AbortSignal) {
    this.store = new XiaoTaoStateStore(scopedFs(fs, config.projectRoot, signal))
    // Cleanup cannot inherit an aborted signal. Contention is nonblocking, never force-reclaimed.
    this.locks = new XiaoTaoStateStore(scopedFs(fs, config.projectRoot))
  }

  async initialize(callerCwd: string): Promise<void> {
    requireThat(path.isAbsolute(this.config.projectRoot) && path.isAbsolute(callerCwd), 'absolute_project_required')
    const root = await this.fs.resolve(this.config.projectRoot)
    const caller = await this.fs.resolve(callerCwd)
    requireThat(root.targetKey === caller.targetKey, 'caller_project_mismatch')
    this.project = String(root.targetKey)
    if (this.config.recoveryRoot) {
      requireThat(path.isAbsolute(this.config.recoveryRoot), 'absolute_recovery_root_required')
      const recovery = await this.fs.resolve(this.config.recoveryRoot)
      requireThat(!this.fs.contains(root, recovery) && !this.fs.contains(recovery, root), 'recovery_root_overlaps_project')
      this.backup = this.config.recoveryRoot
      this.backupKey = String(recovery.targetKey)
    }
  }

  private async active(kind: 'temporary' | 'task', id: string) {
    const t = target(kind, id)
    await this.boundTarget(kind, id)
    // Until overlay resolution is implemented, do not operate in a project with
    // transaction bundles (even completed ones). Empty scaffolding is allowed.
    const tx = await this.store.resolve('.xiaotao/transactions')
    if (await this.fs.stat(tx, this.signal)) {
      requireThat((await this.fs.listDir(tx, this.signal)).length === 0, 'transaction_overlay_unsupported')
    }
    const state = await this.store.readSnapshot(t.meta)
    requireThat(state, 'target_not_found')
    const meta = yamlObject(state.content)
    const schema = kind === 'task' ? 'task' : 'temporary-meta'
    requireThat(this.validator.validate(`https://xiaotao.local/schemas/${schema}.schema.json`, meta).ok,
      'invalid_target_metadata')
    requireThat(meta.id === id && meta.status === 'active', 'target_not_active')
    requireThat(['created_at', 'updated_at'].every((key) => typeof meta[key] === 'string'
      && Number.isFinite(Date.parse(meta[key] as string))), 'invalid_target_dates')
    // Promoted Tasks require transaction-overlay resolution, outside this single-file tool.
    requireThat(!meta.promotion_transaction, 'transaction_target_unsupported')
    return t
  }

  private async sources(snapshot: Snapshot) {
    const root = await this.fs.resolve(this.config.projectRoot, { signal: this.signal })
    for (const ref of snapshot.source_refs) {
      requireThat(ref.length > 0 && !path.isAbsolute(ref) && !/^[a-zA-Z]+:/.test(ref)
        && !ref.replaceAll('\\', '/').split('/').includes('..'), 'invalid_source_ref')
      const resolved = await this.fs.resolve(ref, { cwd: this.config.projectRoot, signal: this.signal })
      requireThat(this.fs.contains(root, resolved) && await this.fs.stat(resolved, this.signal), 'source_unavailable')
    }
  }

  async inspect(kind: 'temporary' | 'task', id: string) {
    const t = await this.active(kind, id)
    const state = await this.store.readSnapshot(t.state)
    requireThat(state, 'state_not_found')
    const parsed = markdown(state.content)
    return { status: 'inspected', revision: Number(parsed.meta.revision), base_hash: hash(state.content),
      content: state.content, target: t.state }
  }

  private recordPath(kind: string, id: string, rid: string) {
    requestId(rid)
    return `${target(kind, id).root}/references/checkpoints/${rid}.json`
  }
  private async boundTarget(kind: string, id: string, rid?: string) {
    const t = target(kind, id)
    const root = await this.store.resolve(t.root)
    const paths = [t.meta, t.state]
    if (rid) paths.push(this.recordPath(kind, id, rid), `${t.root}/references/checkpoints/${rid}.committed.json`)
    for (const p of paths) requireThat(this.fs.contains(root, await this.store.resolve(p)), 'target_path_escape')
  }
  private async secondary(kind: string, id: string, rid: string, content?: string, suffix = '') {
    if (!this.backup) return undefined
    requestId(rid); target(kind, id)
    const root = await this.fs.resolve(this.backup, { signal: this.signal })
    requireThat(String(root.targetKey) === this.backupKey, 'recovery_root_changed')
    const file = await this.fs.resolve(path.join(this.backup, hash(this.project), kind, id, `${rid}${suffix}.json`), { signal: this.signal })
    requireThat(this.fs.contains(root, file), 'recovery_path_escape')
    if (content !== undefined) {
      try { await this.fs.writeText(file, content, { kind: 'createIfAbsent' }, this.signal) }
      catch (error) {
        if ((error as { code?: string }).code !== 'FS_NOT_OBSERVED') throw error
      }
    }
    const info = await this.fs.stat(file, this.signal)
    if (!info) return undefined
    requireThat(typeof info.size === 'number' && info.size <= LIMIT * 3, 'record_too_large')
    const actual = await this.fs.readText(file, this.signal)
    if (content !== undefined) requireThat(actual === content, 'recovery_record_conflict')
    return actual
  }

  private validateRecord(value: unknown): RecordData {
    requireThat(this.validator.validate(SCHEMA, value).ok, 'invalid_checkpoint_record')
    const r = value as RecordData
    requireThat(r.project === this.project && r.source_hash === hash(json(r.snapshot))
      && r.proposal_hash === hash(r.proposal), 'checkpoint_hash_mismatch')
    const proposal = markdown(r.proposal, 'proposal_too_large')
    requireThat(Number(proposal.meta.revision) === r.base_revision + 1, 'checkpoint_revision_mismatch')
    requireThat(json(proposal.meta.checkpoint_receipt) === json({ request_id: r.request_id,
      source_hash: r.source_hash, revision: r.base_revision + 1 }), 'invalid_receipt')
    const { kind, target_id, request_id, base_revision, base_hash, snapshot } = r
    requireThat(r.input_hash === hash(json({ kind, target_id, request_id, base_revision, base_hash, snapshot })), 'checkpoint_input_mismatch')
    return r
  }

  private async load(kind: 'temporary' | 'task', id: string, rid: string) {
    await this.boundTarget(kind, id, rid)
    let text: string | undefined
    let primaryError: unknown
    try { text = (await this.store.readSnapshot(this.recordPath(kind, id, rid)))?.content }
    catch (error) { primaryError = error }
    if (text !== undefined) this.recovery = 'project'
    else {
      text = await this.secondary(kind, id, rid)
      if (text !== undefined) this.recovery = 'secondary'
    }
    if (text === undefined) { if (primaryError) throw primaryError; return undefined }
    requireThat(Buffer.byteLength(text) <= LIMIT * 3, 'record_too_large')
    const r = this.validateRecord(JSON.parse(text))
    requireThat(r.kind === kind && r.target_id === id && r.request_id === rid, 'record_binding_mismatch')
    return r
  }

  private async immutable(file: string, content: string) {
    try { await this.store.createIfAbsent(file, content) }
    catch (error) { if ((error as { code?: string }).code !== 'FS_NOT_OBSERVED') throw error }
    requireThat((await this.store.readSnapshot(file))?.content === content, 'immutable_conflict')
  }

  async save(input: CheckpointInput) {
    const { kind, target_id, request_id, base_revision, base_hash, snapshot } = input
    requestId(request_id); target(kind, target_id)
    // Canonical field order keeps hashes stable across tool JSON property ordering.
    const normalizedSnapshot = Object.fromEntries(['objective', 'confirmed', 'rejected', 'in_progress', 'next',
      'open_questions', 'source_refs'].map((key) => [key, snapshot?.[key as keyof Snapshot]])) as unknown as Snapshot
    requireThat(json(snapshot) !== undefined && Object.keys(snapshot).length === 7, 'invalid_snapshot')
    const normalized = { kind, target_id, request_id, base_revision, base_hash, snapshot: normalizedSnapshot }
    const inputHash = hash(json(normalized))
    const previous = await this.load(kind, target_id, request_id)
    if (previous) {
      requireThat(previous.input_hash === inputHash, 'request_id_reused')
      this.lastRecord = previous
      return this.commit(previous, 'save')
    }
    const t = await this.active(kind, target_id)
    const state = await this.store.readSnapshot(t.state)
    requireThat(state, 'state_not_found')
    const parsed = markdown(state.content)
    requireThat(parsed.meta.revision === base_revision && hash(state.content) === base_hash, 'conflict')
    const sourceHash = hash(json(normalizedSnapshot))
    const receipt = { request_id, source_hash: sourceHash, revision: base_revision + 1 }
    // Keep all user-authored text. Only replace our own single fenced JSON section.
    const start = parsed.body.indexOf(START), end = parsed.body.indexOf(END)
    requireThat((start === -1 && end === -1) || (start >= 0 && end > start
      && parsed.body.indexOf(START, start + START.length) === -1
      && parsed.body.indexOf(END, end + END.length) === -1), 'invalid_checkpoint_section')
    const section = `${START}\n## Saved checkpoint\n\n\`\`\`json\n${JSON.stringify(normalizedSnapshot, null, 2).replaceAll('<', '\\u003c').replaceAll('`', '\\u0060')}\n\`\`\`\n${END}`
    const body = start === -1 ? `${parsed.body}\n${section}\n`
      : parsed.body.slice(0, start) + section + parsed.body.slice(end + END.length)
    const proposal = `---\n${stringify({ ...parsed.meta, revision: base_revision + 1,
      updated_at: new Date().toISOString(), updated_by: `checkpoint/${this.sessionId}`,
      checkpoint_receipt: receipt })}---\n${body}`
    const record = this.validateRecord({ ...normalized, schema_version: 1, project: this.project,
      session_id: this.sessionId, input_hash: inputHash, source_hash: sourceHash, proposal, proposal_hash: hash(proposal) })
    this.lastRecord = record
    requireThat(Buffer.byteLength(json(record)) <= LIMIT * 3 && Buffer.byteLength(json(normalizedSnapshot)) <= 16384,
      'snapshot_too_large')
    await this.sources(record.snapshot)
    // Secondary write-ahead copy contains source, target binding AND exact proposal.
    if (this.backup) {
      try {
        this.failureStage = 'secondary_write'
        await this.secondary(kind, target_id, request_id, json(record))
        this.recovery = 'secondary'
      } catch (error) {
        // Default external archive may be outside the session's writable roots.
        // Never widen permission; the project request remains the recovery source.
        if (!this.config.optionalRecovery || (error as { code?: string }).code !== 'FS_SANDBOX_DENIED') throw error
      }
    }
    this.failureStage = 'project_request'
    await this.immutable(this.recordPath(kind, target_id, request_id), json(record))
    if (this.recovery === 'none') this.recovery = 'project'
    return this.commit(record, 'save')
  }

  async retry(kind: 'temporary' | 'task', id: string, rid: string) {
    const record = await this.load(kind, id, rid)
    requireThat(record, 'request_not_found')
    this.lastRecord = record
    return this.commit(record, 'recovery')
  }

  private observationPath(r: RecordData) {
    return `${target(r.kind, r.target_id).root}/references/checkpoints/${r.request_id}.committed.json`
  }
  private legacyObservation(r: RecordData) {
    return json({ request_id: r.request_id, record_hash: hash(json(r)), proposal_hash: r.proposal_hash,
      revision: r.base_revision + 1 })
  }
  private observation(r: RecordData, completion: 'save' | 'recovery') {
    return json({ request_id: r.request_id, record_hash: hash(json(r)), proposal_hash: r.proposal_hash,
      revision: r.base_revision + 1, completion, committed_at: new Date().toISOString() })
  }
  private validateObservation(content: string, r: RecordData): ObservationData {
    let value: unknown
    try { value = JSON.parse(content) }
    catch { throw new CheckpointError('invalid_observation') }
    requireThat(this.validator.validate(OBSERVATION_SCHEMA, value).ok, 'invalid_observation')
    const observation = value as ObservationData
    requireThat(observation.request_id === r.request_id
      && observation.record_hash === hash(json(r))
      && observation.proposal_hash === r.proposal_hash
      && observation.revision === r.base_revision + 1, 'invalid_observation')
    if (observation.completion === undefined) {
      requireThat(content === this.legacyObservation(r), 'invalid_observation')
    } else {
      requireThat(timestampWithTimezone(observation.committed_at), 'invalid_observation')
    }
    return observation
  }
  async status(kind: 'temporary' | 'task', id: string, rid: string) {
    const r = await this.load(kind, id, rid)
    requireThat(r, 'request_not_found')
    // status is read-only: unlike save/retry it must stay queryable after the target is
    // archived, so it does not gate on the target still being `active`.
    const state = await this.store.readSnapshot(target(kind, id).state)
    const observation = await this.store.readSnapshot(this.observationPath(r))
    if (observation) this.validateObservation(observation.content, r)
    return { status: (observation || (state && hash(state.content) === r.proposal_hash)) ? 'committed' : 'pending',
      request_id: rid, proposed_revision: r.base_revision + 1, recovery: this.recovery }
  }

  private async commit(r: RecordData, completion: 'save' | 'recovery') {
    this.signal.throwIfAborted()
    const t = target(r.kind, r.target_id)
    const releases: Array<() => Promise<void>> = []
    try {
      for (const p of [t.meta, t.state].sort()) releases.push(await acquireLock(this.locks, p,
        `checkpoint/${this.sessionId}`, { timeoutMs: 0 }))
      this.signal.throwIfAborted()
      await this.active(r.kind, r.target_id)
      // Restore the project request first when retrying from secondary storage.
      this.failureStage = 'project_request'
      await this.immutable(this.recordPath(r.kind, r.target_id, r.request_id), json(r))
      const state = await this.store.readSnapshot(t.state)
      requireThat(state, 'state_not_found')
      const observation = await this.store.readSnapshot(this.observationPath(r))
      if (observation) {
        this.validateObservation(observation.content, r)
        return { status: 'already_committed', request_id: r.request_id, revision: r.base_revision + 1,
          catalog_refresh_required: true }
      }
      if (hash(state.content) !== r.proposal_hash) {
        const parsed = markdown(state.content)
        requireThat(parsed.meta.revision === r.base_revision && hash(state.content) === r.base_hash, 'conflict')
        // Repair the previous receipt before allowing another checkpoint to replace it.
        const prior = parsed.meta.checkpoint_receipt as { request_id?: string } | undefined
        if (prior) {
          requireThat(typeof prior.request_id === 'string', 'invalid_receipt')
          const previous = await this.load(r.kind, r.target_id, prior.request_id)
          requireThat(previous, 'previous_request_missing')
          const known = await this.store.readSnapshot(this.observationPath(previous))
          if (known) this.validateObservation(known.content, previous)
          else {
            requireThat(hash(state.content) === previous.proposal_hash, 'previous_commit_ambiguous')
            await this.immutable(this.observationPath(previous), this.observation(previous, 'save'))
          }
        }
        this.signal.throwIfAborted()
        await this.sources(r.snapshot)
        this.failureStage = 'canonical_write'
        await this.store.writeGuarded(state, r.proposal)
      }
      requireThat(hash((await this.store.readSnapshot(t.state))?.content ?? '') === r.proposal_hash, 'commit_unconfirmed')
      this.failureStage = 'observation_write'
      await this.immutable(this.observationPath(r), this.observation(r, completion))
      return { status: 'committed', request_id: r.request_id, revision: r.base_revision + 1,
        catalog_refresh_required: true }
    } finally {
      // All held locks get a release attempt even if one cleanup fails.
      const outcomes = await Promise.allSettled(releases.reverse().map((release) => release()))
      if (outcomes.some((outcome) => outcome.status === 'rejected')) throw new CheckpointError('lock_release_failed')
    }
  }

  failure(error: unknown) {
    const code = (error as { code?: string })?.code
    return { status: 'failed', code: error instanceof CheckpointError ? error.code
      : this.signal.aborted ? 'cancelled' : code === 'FS_STALE_VERSION' ? 'conflict'
        : code === 'FS_SANDBOX_DENIED' ? 'filesystem_permission_denied'
        : error instanceof Error && error.message.includes('lock contention') ? 'lock_contention'
          : 'storage_or_validation_error', recovery: this.recovery, failure_stage: this.failureStage,
    has_recoverable_record: this.recovery !== 'none' }
  }

  async reportFailure(error: unknown) {
    const result = this.failure(error)
    const r = this.lastRecord
    let failureRecorded = false
    // Failure evidence is best-effort, never a substitute for the immutable source
    // request. Cancellation stops new writes; recovery can still inspect pending.
    if (r && this.recovery !== 'none' && !this.signal.aborted) {
      const suffix = `.failed-${randomUUID()}`
      const content = json({ request_id: r.request_id, record_hash: hash(json(r)),
        code: result.code, recorded_at: new Date().toISOString(), recovery: this.recovery })
      try {
        const p = this.recordPath(r.kind, r.target_id, r.request_id).replace(/\.json$/, `${suffix}.json`)
        const root = await this.store.resolve(target(r.kind, r.target_id).root)
        requireThat(this.fs.contains(root, await this.store.resolve(p)), 'target_path_escape')
        await this.immutable(p, content)
        failureRecorded = true
      } catch { /* Keep the original outcome; try the configured secondary channel. */ }
      if (!failureRecorded && this.backup) {
        try { await this.secondary(r.kind, r.target_id, r.request_id, content, suffix); failureRecorded = true }
        catch { /* All outcome information remains visible in the tool result. */ }
      }
    }
    return { ...result, failure_recorded: failureRecorded }
  }
}
