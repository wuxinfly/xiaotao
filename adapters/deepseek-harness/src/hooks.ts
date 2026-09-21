/**
 * Session-lifecycle hooks (capability plugin, skeleton): observe dsh agent
 * events and hand the adapter a deterministic, host-specific trigger for
 * XiaoTao's Handoff / session-boundary checks.
 *
 * The boundary is deliberate: the adapter only decides *when* a hook fires
 * (a turn stopping, a session starting). It never decides *what* XiaoTao does
 * with that signal — that stays in the Core Skill. This module is a mount
 * point, not business logic.
 *
 * dsh dispatches `agent/*` events scope-filtered (`this: Scoped<Agent>`), so a
 * root-context listener registers with `{ global: true }` to observe every
 * agent rather than one scoped agent. `agent/turn-stopping` is serial and has
 * no `next()`.
 *
 * @module @xiaotao-ai/dsh-adapter/hooks
 */

import { readFile, readdir, stat } from 'node:fs/promises'
import { existsSync, readdirSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import type { Context } from '@deepseek-ai/cordis'
import type { Agent } from '@deepseek-ai/dsh-agent'
import { createUserMessage } from '@deepseek-ai/dsh-llm'
import type { SessionEvent } from '@deepseek-ai/dsh-session'
import type { StateFileSystem } from './storage'
import type { AutoCheckpointConfig } from './types'

const currentDir = path.dirname(fileURLToPath(import.meta.url))

const MEMORY_FIRST_RUNTIME_RULE = `## XiaoTao 工作规则

你面向用户时的唯一身份是“小涛”。不要自称执行者、Worker、Agent 或 XiaoTao 工作流；使用简洁的大白话，先说结果，只在用户需要时补充过程。

当用户询问项目现有逻辑、历史原因、设计决策、旧问题或以前做过的工作时，必须先加载 XiaoTao Skill，使用用户问题中的关键词执行一次有界 Memory Catalog \`search\`。命中后最多 \`show\` 3 条相关记忆，再检查当前代码；未命中再直接检查代码。不得因为可以搜索代码而跳过记忆搜索。记忆只提供线索，最终以当前代码和可验证证据为准。`

interface RecoverableCheckpointInfo {
  status?: 'committed' | 'pending'
  scope: string
  binding: string
  revision?: number
  proposed_revision?: number
  request_id: string
}

async function findRecoverableCheckpointDsh(
  projectRoot: string,
): Promise<RecoverableCheckpointInfo | null> {
  const candidates: Array<RecoverableCheckpointInfo & { mtime: number }> = []

  const scanTargetCheckpoints = async (
    scope: string,
    binding: string,
    chkDir: string,
    receipt: { request_id: string; revision: number } | null,
    receiptMtime: number,
  ) => {
    const committedMap = new Map<string, { revision: number; mtime: number }>()
    if (receipt) {
      committedMap.set(receipt.request_id, { revision: receipt.revision, mtime: receiptMtime })
    }
    try {
      const files = await readdir(chkDir)
      for (const f of files) {
        if (f.endsWith('.committed.json')) {
          const reqId = f.slice(0, -'.committed.json'.length)
          try {
            const data = JSON.parse(await readFile(path.join(chkDir, f), 'utf8'))
            const st = await stat(path.join(chkDir, f))
            const rev = data && typeof data === 'object' && data.revision !== undefined ? Number(data.revision) : 1
            committedMap.set(reqId, { revision: rev, mtime: st.mtimeMs })
          } catch {}
        }
      }
      for (const f of files) {
        if (!f.endsWith('.json') || f.includes('.committed.') || f.includes('.failed-')) continue
        const reqId = f.slice(0, -'.json'.length)
        try {
          const data = JSON.parse(await readFile(path.join(chkDir, f), 'utf8'))
          if (data && typeof data === 'object' && data.request_id) {
            const st = await stat(path.join(chkDir, f))
            const targetScope = data.kind || scope
            const targetBinding = data.target_id || binding
            if (committedMap.has(reqId)) {
              const comm = committedMap.get(reqId)!
              candidates.push({
                status: 'committed',
                scope: targetScope,
                binding: targetBinding,
                revision: comm.revision,
                request_id: reqId,
                mtime: Math.max(st.mtimeMs, comm.mtime),
              })
            } else {
              const propRev = Number(data.base_revision || 0) + 1
              candidates.push({
                status: 'pending',
                scope: targetScope,
                binding: targetBinding,
                proposed_revision: propRev,
                revision: propRev,
                request_id: reqId,
                mtime: st.mtimeMs,
              })
            }
          }
        } catch {}
      }
    } catch {}

    for (const [reqId, comm] of committedMap.entries()) {
      if (!candidates.some(c => c.request_id === reqId)) {
        candidates.push({
          status: 'committed',
          scope,
          binding,
          revision: comm.revision,
          request_id: reqId,
          mtime: comm.mtime,
        })
      }
    }
  }

  // 1. Task checkpoints: .xiaotao/tasks/*/references/checkpoints/*.json and progress.md
  try {
    const tasksDir = path.join(projectRoot, '.xiaotao/tasks')
    const taskEntries = await readdir(tasksDir)
    for (const t of taskEntries) {
      if (t === 'archive' || t.startsWith('.')) continue
      const tDir = path.join(tasksDir, t)
      let receipt: { request_id: string; revision: number } | null = null
      let receiptMtime = 0
      try {
        const text = await readFile(path.join(tDir, 'progress.md'), 'utf8')
        const reqMatch = /request_id:\s*['"]?([a-z0-9][a-z0-9_-]*)['"]?/i.exec(text)
        const revMatch = /revision:\s*(\d+)/.exec(text)
        if (reqMatch && revMatch) {
          const st = await stat(path.join(tDir, 'progress.md'))
          receipt = { request_id: reqMatch[1], revision: Number(revMatch[1]) }
          receiptMtime = st.mtimeMs
        }
      } catch {}
      await scanTargetCheckpoints('task', t, path.join(tDir, 'references/checkpoints'), receipt, receiptMtime)
    }
  } catch {}

  // 2. Temporary checkpoints: .xiaotao/memory/temporary/active/*/references/checkpoints/*.json and current.md
  try {
    const tempDir = path.join(projectRoot, '.xiaotao/memory/temporary/active')
    const tempEntries = await readdir(tempDir)
    for (const t of tempEntries) {
      if (t.startsWith('.')) continue
      const tDir = path.join(tempDir, t)
      let receipt: { request_id: string; revision: number } | null = null
      let receiptMtime = 0
      try {
        const text = await readFile(path.join(tDir, 'current.md'), 'utf8')
        const reqMatch = /request_id:\s*['"]?([a-z0-9][a-z0-9_-]*)['"]?/i.exec(text)
        const revMatch = /revision:\s*(\d+)/.exec(text)
        if (reqMatch && revMatch) {
          const st = await stat(path.join(tDir, 'current.md'))
          receipt = { request_id: reqMatch[1], revision: Number(revMatch[1]) }
          receiptMtime = st.mtimeMs
        }
      } catch {}
      await scanTargetCheckpoints('temporary', t, path.join(tDir, 'references/checkpoints'), receipt, receiptMtime)
    }
  } catch {}

  // 3. Project checkpoints: .xiaotao/checkpoints/*.json
  await scanTargetCheckpoints('session', 'session', path.join(projectRoot, '.xiaotao/checkpoints'), null, 0)

  if (candidates.length === 0) return null

  // Deduplicate and prioritize committed over pending
  const seenReq = new Set<string>()
  const deduped: Array<RecoverableCheckpointInfo & { mtime: number }> = []
  candidates.sort((a, b) => {
    const statusA = a.status === 'committed' ? 0 : 1
    const statusB = b.status === 'committed' ? 0 : 1
    if (statusA !== statusB) return statusA - statusB
    return b.mtime - a.mtime || (b.revision || 0) - (a.revision || 0) || a.binding.localeCompare(b.binding) || a.request_id.localeCompare(b.request_id)
  })
  for (const c of candidates) {
    if (!seenReq.has(c.request_id)) {
      seenReq.add(c.request_id)
      deduped.push(c)
    }
  }
  return deduped.length > 0 ? deduped[0] : null
}

function formatBoundedRuntimeContextDsh(options: {
  tasks?: any[]
  temporaries?: any[]
  followups?: any[]
  longTermCount?: number
  checkpoint?: RecoverableCheckpointInfo | null
  degradedWarning?: string | null
  limit?: number
}): string {
  const { tasks = [], temporaries = [], followups = [], longTermCount = 0, checkpoint = null, degradedWarning = null, limit = 3 } = options
  const lines: string[] = [
    '# Memory Overview (Runtime Context)',
    '',
    '当前检测到项目存在活动工作：',
    '',
  ]

  if (degradedWarning) {
    lines.push(`> 警告：${degradedWarning}`, '')
  }

  if (checkpoint) {
    const status = checkpoint.status || 'committed'
    lines.push(
      '## Recoverable Checkpoint',
      'Recoverable checkpoint: yes',
      `status: ${status}`,
      `scope: ${checkpoint.scope}`,
      `binding: ${checkpoint.binding}`,
      status === 'pending'
        ? `proposed_revision: ${checkpoint.proposed_revision || checkpoint.revision}`
        : `revision: ${checkpoint.revision}`,
      '',
    )
  }

  lines.push(`## Active Tasks (${tasks.length})`)
  if (tasks.length > 0) {
    for (const t of tasks.slice(0, limit)) {
      lines.push(`- \`${String(t.memory_id || '').slice(0, 80)}\`: ${String(t.title || t.memory_id || '').slice(0, 120)}`)
    }
    if (tasks.length > limit) {
      lines.push(`  *(另外 ${tasks.length - limit} 项活动任务已省略，详情请使用 recent / show)*`)
    }
  } else {
    lines.push('- *(无活动任务)*')
  }
  lines.push('')

  lines.push(`## Active Temporary Memory (${temporaries.length})`)
  if (temporaries.length > 0) {
    for (const t of temporaries.slice(0, limit)) {
      lines.push(`- \`${String(t.memory_id || '').slice(0, 80)}\`: ${String(t.title || t.memory_id || '').slice(0, 120)}`)
    }
    if (temporaries.length > limit) {
      lines.push(`  *(另外 ${temporaries.length - limit} 项活动探索已省略，详情请使用 recent / show)*`)
    }
  } else {
    lines.push('- *(无活动探索)*')
  }
  lines.push('')

  if (followups.length > 0) {
    lines.push(`## Pending Follow-ups (${followups.length})`)
    for (const f of followups.slice(0, limit)) {
      lines.push(`- \`${String(f.followup_id || '').slice(0, 80)}\`: ${String(f.title || f.followup_id || '').slice(0, 120)}`)
    }
    if (followups.length > limit) {
      lines.push(`  *(另外 ${followups.length - limit} 项待跟进已省略，详情请使用 show)*`)
    }
    lines.push('')
  }

  lines.push(`## Long-term Memory (${longTermCount} 项已索引)`)
  lines.push('')
  lines.push('> 提示：启动时仅加载本有界总览；具体记忆正文严禁全量预加载，请按需使用 recent / search / show。')

  return lines.join('\n')
}

/**
 * Detect whether projectRoot has any authoritative source files for memory/tasks/checkpoints.
 * Derived files like .xiaotao/memory/index.json or manifest.md are NOT authoritative sources.
 */
export function hasAuthoritativeSourcesDsh(projectRoot: string): boolean {
  try {
    const tasksDir = path.join(projectRoot, '.xiaotao/tasks')
    if (existsSync(tasksDir)) {
      const taskEntries = readdirSync(tasksDir)
      for (const t of taskEntries) {
        if (t === 'archive' || t.startsWith('.')) continue
        const tDir = path.join(tasksDir, t)
        if (existsSync(path.join(tDir, 'task.yaml')) || existsSync(path.join(tDir, 'progress.md'))) return true
        const chkDir = path.join(tDir, 'references/checkpoints')
        if (existsSync(chkDir)) {
          const chkFiles = readdirSync(chkDir)
          if (chkFiles.some(f => f.endsWith('.json') && !f.includes('.failed-'))) return true
        }
      }
    }
  } catch {}

  try {
    const tempDir = path.join(projectRoot, '.xiaotao/memory/temporary/active')
    if (existsSync(tempDir)) {
      const tempEntries = readdirSync(tempDir)
      for (const t of tempEntries) {
        if (t.startsWith('.')) continue
        const tDir = path.join(tempDir, t)
        if (existsSync(path.join(tDir, 'meta.yaml')) || existsSync(path.join(tDir, 'current.md'))) return true
        const chkDir = path.join(tDir, 'references/checkpoints')
        if (existsSync(chkDir)) {
          const chkFiles = readdirSync(chkDir)
          if (chkFiles.some(f => f.endsWith('.json') && !f.includes('.failed-'))) return true
        }
      }
    }
  } catch {}

  try {
    const ltDir = path.join(projectRoot, '.xiaotao/memory/long-term/entries')
    if (existsSync(ltDir)) {
      const ltEntries = readdirSync(ltDir)
      if (ltEntries.some(e => e.endsWith('.md'))) return true
    }
  } catch {}

  try {
    if (existsSync(path.join(projectRoot, '.xiaotao/memory/long-term/current.md'))) return true
    if (existsSync(path.join(projectRoot, '.xiaotao/memory/legacy/memory.md'))) return true
  } catch {}

  try {
    const fuDir = path.join(projectRoot, '.xiaotao/memory/followups/pending')
    if (existsSync(fuDir)) {
      const fuEntries = readdirSync(fuDir)
      if (fuEntries.some(e => e.endsWith('.yaml') || e.endsWith('.yml'))) return true
    }
  } catch {}

  try {
    const chkDir = path.join(projectRoot, '.xiaotao/checkpoints')
    if (existsSync(chkDir)) {
      const chkEntries = readdirSync(chkDir)
      if (chkEntries.some(e => e.endsWith('.json') && !e.includes('.failed-'))) return true
    }
  } catch {}

  return false
}

/**
 * Load bounded Runtime Context summary from `.xiaotao/memory/index.json`.
 * If no active Task, Temporary, follow-up, or recoverable checkpoint exist, returns null.
 */
export async function loadBoundedRuntimeContext(
  projectRoot: string,
  fs?: StateFileSystem,
): Promise<string | null> {
  try {
    if (!hasAuthoritativeSourcesDsh(projectRoot)) {
      return null
    }

    // Session startup is latency-sensitive. Read the existing validated Catalog
    // snapshot without deriving or refreshing it; normal search/show operations
    // refresh on demand once the user asks for real work.
    if (path.isAbsolute(projectRoot)) {
      const candidateScripts = [
        path.resolve(currentDir, 'core/scripts/memory_catalog.py'),
        path.resolve(currentDir, '../lib/core/scripts/memory_catalog.py'),
        path.resolve(currentDir, '../../../xiaotao/scripts/memory_catalog.py'),
        path.join(projectRoot, 'xiaotao/scripts/memory_catalog.py'),
        path.join(projectRoot, '.agents/skills/xiaotao/scripts/memory_catalog.py'),
        path.join(projectRoot, '.xiaotao/scripts/memory_catalog.py'),
      ]
      let scriptPath: string | null = null
      for (const p of candidateScripts) {
        if (existsSync(p)) {
          scriptPath = p
          break
        }
      }

      if (scriptPath && process.env.XIAOTAO_FORCE_PYTHON_FAIL !== '1') {
        try {
          const pyCandidates = process.platform === 'win32' ? ['python', 'py', 'python3'] : ['python3', 'python']
          for (const python of pyCandidates) {
            try {
              const res = spawnSync(python, [scriptPath, '--project-root', projectRoot, 'overview', '--limit', '3', '--cached'], {
                encoding: 'utf8',
                timeout: 5000,
                windowsHide: true,
                env: { ...process.env, PYTHONIOENCODING: 'utf-8' },
              })
              if (res.status === 0 && typeof res.stdout === 'string') {
                const out = res.stdout.trim()
                if (out.includes('当前检测到项目存在活动工作：')) {
                  return out
                }
                if (out.includes('当前项目暂无活动任务或临时探索')) {
                  return null
                }
              }
            } catch {
              // Try next candidate
            }
          }
        } catch {
          // Fall through to degraded fallback
        }
      }
    }

    // 2. Degradation fallback: no valid cached snapshot is available. Do not
    // rebuild during SessionStart; preserve authoritative checkpoint recovery
    // and let the first memory-relevant user request refresh the Catalog.
    const checkpoint = await findRecoverableCheckpointDsh(projectRoot)
    return formatBoundedRuntimeContextDsh({
      checkpoint,
      degradedWarning: '检测到项目存在 XiaoTao 权威工作源，但启动时没有可用的 Catalog 快照。无需在新会话中自动重建；处理具体问题时再按需刷新。',
    })
  } catch {
    return null
  }
}

/**
 * Queue bounded Runtime Context for the next real agent request if active work exists.
 * `agent.inject()` is intentionally non-waking: session startup must not create
 * an autonomous model turn merely because a project has XiaoTao state.
 * Returns true if context was injected, false otherwise.
 */
export async function injectSessionRuntimeContext(
  payload: SessionStartPayload,
  fs?: StateFileSystem,
): Promise<boolean> {
  const projectRoot = payload.agent.session.header.cwd
  // DSH records the workspace on the immutable Session header. Never fall
  // back to process.cwd(): one DSH host can serve sessions from many projects.
  if (typeof projectRoot !== 'string' || !path.isAbsolute(projectRoot)) return false
  const runtimeContext = await loadBoundedRuntimeContext(projectRoot, fs)
  // Even a XiaoTao project without active work still needs the memory-first
  // routing rule. This check is metadata-only and never scans memory sources.
  if (!runtimeContext && !existsSync(path.join(projectRoot, '.xiaotao'))) return false
  payload.agent.inject(createUserMessage({
    content: [{
      type: 'text',
      // Keep this routing rule in the always-injected plugin context. Putting
      // it only in Core SKILL.md is insufficient when the host starts solving
      // a code question before explicitly loading the XiaoTao Skill.
      text: runtimeContext
        ? `${runtimeContext}\n\n${MEMORY_FIRST_RUNTIME_RULE}`
        : MEMORY_FIRST_RUNTIME_RULE,
    }],
    source: {
      kind: 'plugin',
      plugin: 'xiaotao-runtime-context',
      form: 'notice',
      summary: '当前项目存在活动工作，已注入运行时上下文。',
    },
  }))
  return true
}



const AUTO_SOURCE = 'xiaotao-auto-checkpoint'
const DEFAULT_PRESSURE_THRESHOLD = 0.72
const DEFAULT_COOLDOWN_TURNS = 2
const DEFAULT_TIMEOUT_MS = 1_000

/** Payload of dsh's `agent/turn-stopping` event (see dsh-agent runtime-types). */
export interface TurnStopPayload {
  agent: Agent
  turn: number
  signal: AbortSignal
}

/** Payload of dsh's `agent/session-start` event (see dsh-agent runtime-types). */
export interface SessionStartPayload {
  agent: Agent
}

/** Callbacks the adapter can invoke at deterministic lifecycle boundaries. */
export interface LifecycleHandlers {
  /** Invoked when a turn is about to close. */
  onTurnStopping?: (payload: TurnStopPayload) => void | Promise<void>
  /** Invoked when a session starts. */
  onSessionStart?: (payload: SessionStartPayload) => void | Promise<void>
}

export interface LifecycleHookOptions {
  timeoutMs?: number
}

export type AutoCheckpointDecision =
  | 'triggered'
  | 'below-threshold'
  | 'pressure-unknown'
  | 'no-new-progress'
  | 'cooldown'
  | 'cancelled'

interface CheckpointObservation {
  seq: number
  turn: number
}

function boundedInteger(value: number | undefined, fallback: number, min: number, max: number,
  name: string): number {
  const selected = value ?? fallback
  if (!Number.isSafeInteger(selected) || selected < min || selected > max) {
    throw new Error(`xiaotao-adapter: ${name} must be an integer from ${min} to ${max}`)
  }
  return selected
}

function pressureThreshold(value: number | undefined): number {
  const selected = value ?? DEFAULT_PRESSURE_THRESHOLD
  if (!Number.isFinite(selected) || selected < 0.5 || selected > 0.95) {
    throw new Error('xiaotao-adapter: checkpoint.auto.pressureThreshold must be from 0.5 to 0.95')
  }
  return selected
}

function nestedText(value: unknown): string {
  if (!Array.isArray(value)) return ''
  return value.flatMap((block) => {
    if (!block || typeof block !== 'object') return []
    const item = block as { type?: unknown; text?: unknown; content?: unknown }
    if (item.type === 'text' && typeof item.text === 'string') return [item.text]
    if (item.type === 'tool-result') return [nestedText(item.content)]
    return []
  }).join('')
}

function checkpointCalls(events: readonly SessionEvent[]): Map<string, { seq: number; turn: number }> {
  const calls = new Map<string, { seq: number; turn: number }>()
  for (const event of events) {
    if (event?.type !== 'tool/call' || event.data?.name !== 'xiaotao_checkpoint') continue
    try {
      const args = JSON.parse(event.data.arguments) as { operation?: unknown }
      if (args.operation === 'save' || args.operation === 'retry') {
        calls.set(String(event.data.callId), { seq: event.seq, turn: event.data.turn })
      }
    } catch {
      // Invalid model arguments are not a checkpoint observation.
    }
  }
  return calls
}

function lastCommittedCheckpoint(events: readonly SessionEvent[]): CheckpointObservation | undefined {
  const calls = checkpointCalls(events)
  let found: CheckpointObservation | undefined
  for (const event of events) {
    if (event?.type !== 'tool/result') continue
    const block = event.data?.message?.content?.[0]
    const call = calls.get(String(block?.toolCallId))
    if (!call || block?.isError) continue
    try {
      const result = JSON.parse(nestedText(block.content)) as { status?: unknown }
      if (result.status === 'committed' || result.status === 'already_committed') {
        found = { seq: event.seq, turn: call.turn }
      }
    } catch {
      // A non-JSON or differently rendered result cannot prove commit success.
    }
  }
  return found
}

function autoPrompt(event: SessionEvent): { seq: number; turn: number } | undefined {
  if (event?.type !== 'user/message') return undefined
  const plugin = event.data?.source?.kind === 'plugin' ? event.data.source.plugin : undefined
  const match = typeof plugin === 'string'
    ? new RegExp(`^${AUTO_SOURCE}/turn-(\\d+)$`).exec(plugin) : null
  return match ? { seq: event.seq, turn: Number(match[1]) } : undefined
}

function lastAutoPrompt(events: readonly SessionEvent[]): CheckpointObservation | undefined {
  let found: CheckpointObservation | undefined
  for (const event of events) found = autoPrompt(event) ?? found
  return found
}

function lastProgressSeq(events: readonly SessionEvent[], checkpointCallIds: ReadonlySet<string>): number {
  let seq = -1
  for (const event of events) {
    if (event?.type === 'user/message' && !autoPrompt(event)) seq = event.seq
    if (event?.type === 'tool/result') {
      const callId = String(event.data?.message?.content?.[0]?.toolCallId)
      if (!checkpointCallIds.has(callId)) seq = event.seq
    }
  }
  return seq
}

interface ContextPressureProjection {
  projectedTokens?: unknown
  contextWindow?: unknown
}

interface SessionProjectionReader {
  snapshot(session: Agent['session']): {
    values?: { contextPressure?: ContextPressureProjection }
  }
}

function projectedContextPressure(agent: Agent): number | undefined {
  const reader = agent.ctx?.get('sessionProjections') as SessionProjectionReader | undefined
  if (!reader || typeof reader.snapshot !== 'function') return undefined
  try {
    const projection = reader.snapshot(agent.session).values?.contextPressure
    const tokens = projection?.projectedTokens
    const window = projection?.contextWindow
    if (!Number.isFinite(tokens) || !Number.isFinite(window)
      || (tokens as number) < 0 || (window as number) <= 0) return undefined
    return (tokens as number) / (window as number)
  } catch {
    // A missing/unavailable projection is an optional capability, so fall back.
    return undefined
  }
}

function estimatedNextRequestPressure(agent: Agent): number | undefined {
  const projected = projectedContextPressure(agent)
  if (projected !== undefined) return projected
  const window = agent.session.requestContext()?.contextWindow
  if (!Number.isFinite(window) || !window || window <= 0) return undefined
  const events = agent.session.events
  for (let index = events.length - 1; index >= 0; index--) {
    const event = events[index]
    if (event.type !== 'assistant/message' || !event.data.usage) continue
    const usage = event.data.usage
    // Match DSH token-meter's prompt-side pressureTokens fallback. Output is
    // excluded because it is not part of the next request's prompt.
    const used = usage.inputTokens + (usage.cacheReadTokens ?? 0) + (usage.cacheWriteTokens ?? 0)
    return used / window
  }
  return undefined
}

/**
 * Turn-boundary checkpoint trigger for DSH hosts that expose request capacity.
 * It does not claim to be pre-compaction: the configurable threshold is an
 * early-warning experiment that asks the current model to reuse the M1 tool.
 */
export class AutoCheckpointCoordinator {
  readonly threshold: number
  readonly cooldownTurns: number

  constructor(config: AutoCheckpointConfig = {}) {
    this.threshold = pressureThreshold(config.pressureThreshold)
    this.cooldownTurns = boundedInteger(config.cooldownTurns, DEFAULT_COOLDOWN_TURNS, 1, 100,
      'checkpoint.auto.cooldownTurns')
  }

  evaluateAndTrigger(payload: TurnStopPayload): AutoCheckpointDecision {
    if (payload.signal.aborted) return 'cancelled'
    const events = payload.agent.session.events
    const pressure = estimatedNextRequestPressure(payload.agent)
    if (pressure === undefined) return 'pressure-unknown'
    if (pressure < this.threshold) return 'below-threshold'

    const calls = checkpointCalls(events)
    const committed = lastCommittedCheckpoint(events)
    const prompted = lastAutoPrompt(events)
    const boundary = Math.max(committed?.seq ?? -1, prompted?.seq ?? -1)
    if (lastProgressSeq(events, new Set(calls.keys())) <= boundary) return 'no-new-progress'

    const lastTurn = Math.max(committed?.turn ?? -Infinity, prompted?.turn ?? -Infinity)
    if (payload.turn - lastTurn < this.cooldownTurns) return 'cooldown'
    payload.signal.throwIfAborted()

    const percent = Math.round(pressure * 100)
    payload.agent.steer(createUserMessage({
      content: [{
        type: 'text',
        text: [
          `XiaoTao 自动 checkpoint 触发：预计下一次模型请求约占上下文窗口 ${percent}%，达到配置阈值。`,
          '这是 DSH 的真实 turn-stopping 压力提醒，不是 pre-compaction 事件。',
          '请先判断当前工作是否产生了值得恢复的新进展。若有，只选择当前相关且已存在的活动 Temporary 或 Task，调用 xiaotao_checkpoint：先 inspect，再用同一 revision/hash 执行 save；失败时保留同一 request_id 并按 status/retry 处理。',
          '不要创建新 Task，不要恢复无关旧任务，不要扩大权限，也不要向用户展开保存过程。若没有可安全选择的活动目标，则跳过并继续正常结束。',
        ].join('\n'),
      }],
      source: {
        kind: 'plugin',
        plugin: `${AUTO_SOURCE}/turn-${payload.turn}`,
        form: 'notice',
        summary: '上下文压力达到阈值，先检查是否需要保存 XiaoTao checkpoint。',
      },
    }))
    return 'triggered'
  }
}

/**
 * Register lifecycle listeners on the context. Cordis removes listeners when
 * the context is disposed, so no manual disposer is returned.
 *
 * `agent/turn-stopping` is a `@mode serial` event: the dispatcher awaits each
 * listener in order before the turn boundary commits. The listener therefore
 * **returns** the handler promise (wrapped so errors are logged, not thrown)
 * instead of fire-and-forgetting it — otherwise a turn could close while a
 * Handoff / memory save is still in flight.
 *
 * @param ctx - the Cordis context.
 * @param handlers - the callbacks to wire up.
 */
export function registerLifecycleHooks(ctx: Context, handlers: LifecycleHandlers,
  options: LifecycleHookOptions = {}): void {
  const onTurnStopping = handlers.onTurnStopping
  if (onTurnStopping !== undefined) {
    const timeoutMs = boundedInteger(options.timeoutMs, DEFAULT_TIMEOUT_MS, 50, 5_000,
      'checkpoint.auto.timeoutMs')
    ctx.on(
      'agent/turn-stopping',
      function (payload) {
        const timeout = new AbortController()
        const signal = AbortSignal.any([payload.signal, timeout.signal])
        let timer: ReturnType<typeof setTimeout> | undefined
        const deadline = new Promise<void>((resolve) => {
          timer = setTimeout(() => {
            timeout.abort(new Error('lifecycle hook timeout'))
            ctx.logger.warn(`xiaotao-adapter: onTurnStopping listener timed out after ${timeoutMs}ms`)
            resolve()
          }, timeoutMs)
        })
        const handled = Promise.resolve()
          .then(() => onTurnStopping({ ...payload, signal }))
          .catch((error: unknown) => {
            ctx.logger.warn(`xiaotao-adapter: onTurnStopping listener failed: ${String(error)}`)
          })
        return Promise.race([handled, deadline]).finally(() => {
          if (timer !== undefined) clearTimeout(timer)
        })
      },
      { global: true },
    )
  }

  const onSessionStart = handlers.onSessionStart
  if (onSessionStart !== undefined) {
    ctx.on(
      'agent/session-start',
      function (payload) {
        try {
          const handled = onSessionStart(payload)
          if (handled instanceof Promise) {
            handled.catch((error: unknown) => {
              ctx.logger.warn(`xiaotao-adapter: onSessionStart listener failed: ${String(error)}`)
            })
          }
        } catch (error: unknown) {
          ctx.logger.warn(`xiaotao-adapter: onSessionStart listener failed: ${String(error)}`)
        }
      },
      { global: true },
    )
  }
}
