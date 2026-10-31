/**
 * XiaoTao DeepSeek Harness adapter — the single Cordis plugin entry point.
 *
 * This is the only module that imports dsh packages, so a dsh API change
 * (dsh is a v0.1 preview with an explicit "compatibility-breaking changes"
 * warning) is contained to this one file plus the type-only imports in the
 * other `src/*.ts` modules.
 *
 * Flow: detect capabilities → fail fast if skills are missing → register the
 * XiaoTao Core Skill → mount the enhancement paths that the detected seams
 * support. When `ctx.fs` / `ctx.agents` are absent the adapter degrades to the
 * plain-skill fallback and the Core still works.
 *
 * ## What is actually wired today
 *
 * - **Product A (complete)**: the XiaoTao Core Skill is registered.
 * - **Product B**: when `ctx.fs` exists, the deterministic
 *   `XiaoTaoStateStore` and `XiaoTaoSchemaValidator` are constructed and
 *   registered as Cordis services (`xiaotao.stateStore` /
 *   `xiaotao.schemaValidator`). They are reachable via `ctx.get(...)` but are
 *   not exposed as unrestricted raw tools. A session-bound checkpoint
 *   tool uses them for single-target snapshot saves when `ctx.tools` exists.
 *   Other Core reads/writes still follow `storage.md` through host tools.
 * - **Lifecycle hooks**: when explicitly enabled, `ctx.agents` supplies the
 *   awaited turn-stopping pressure trigger. This is a fallback trigger, not a
 *   pre-compaction or Session End lifecycle claim.
 *
 * @module @xiaotao-ai/dsh-adapter
 */

import path from 'node:path'
import type { Context } from '@deepseek-ai/cordis'
import type { FileSystem } from '@deepseek-ai/dsh-fs'
import type { ToolRuntime } from '@deepseek-ai/dsh-tools'
import { checkpointTool } from './checkpoint-tool'
import { AutoCheckpointCoordinator, registerLifecycleHooks, injectSessionRuntimeContext } from './hooks'
import { assertSkills, detectCapabilities, planActivation } from './detect'
import { loadCoreSkill, registerCoreSkill, resolveCoreDir } from './skill'
import { XiaoTaoStateStore } from './storage'
import { XiaoTaoTransactionStore } from './transaction'
import { XiaoTaoSchemaValidator } from './validate'
import type { AdapterConfig } from './types'

export * from './guard'
export * from './transaction'

/** Cordis plugin name. */
export const name = 'xiaotao-adapter'

/** Only skills gate the Core; child injections wait for optional enhancements. */
export const inject = ['skills']

/** Cordis service name under which the deterministic state store is provided. */
export const STATE_STORE_SERVICE = 'xiaotao.stateStore'

/** Cordis service name under which the schema validator is provided. */
export const SCHEMA_VALIDATOR_SERVICE = 'xiaotao.schemaValidator'

/** Internal create/replace transaction mechanism; not a model-facing tool. */
export const TRANSACTION_STORE_SERVICE = 'xiaotao.transactionStore'

/**
 * Mount the adapter. Async because it reads `SKILL.md` from disk during setup.
 *
 * @param ctx - the Cordis context.
 * @param config - adapter config (see {@link AdapterConfig}).
 */
export async function apply(ctx: Context, config: AdapterConfig = {}): Promise<void> {
  const checkpoint = config.checkpoint === false ? false : config.checkpoint ?? {}
  const autoCheckpoint = checkpoint && checkpoint.auto !== false ? checkpoint.auto : undefined
  const capabilities = detectCapabilities(ctx)
  assertSkills(capabilities)
  const activation = planActivation(capabilities)

  // Product A — register the portable XiaoTao Core Skill. This always runs.
  const coreDir = await resolveCoreDir(config, process.cwd())
  const registration = await loadCoreSkill(coreDir)
  const disposer = registerCoreSkill(ctx, registration)
  ctx.effect(() => disposer, 'xiaotao-adapter: core skill')

  // Do not snapshot optional services at startup: DSH can publish fs/tools later.
  // Child injections preserve the plain Skill and follow service disposal/reload.
  ctx.inject(['fs'], async (ctx) => {
    const fs = ctx.get('fs') as FileSystem
    const store = new XiaoTaoStateStore(fs)
    const validator = new XiaoTaoSchemaValidator()
    const transactions = new XiaoTaoTransactionStore(fs)
    const schemaCount = await validator.loadAll(path.join(coreDir, 'references', 'schemas'))
    if (schemaCount === 0) {
      ctx.logger.warn(
        'xiaotao-adapter: no JSON Schemas loaded from references/schemas; validation stays disabled',
      )
    }

    // Provide both as Cordis services so the rest of the runtime can reach them.
    // The optional checkpoint tool below exposes a bounded validated workflow;
    // arbitrary Core storage operations are not automatically routed through it.
    const disposeValidator = ctx.provide(SCHEMA_VALIDATOR_SERVICE, validator)
    const disposeStore = ctx.provide(STATE_STORE_SERVICE, store)
    const disposeTransactions = ctx.provide(TRANSACTION_STORE_SERVICE, transactions)
    ctx.effect(() => () => {
      disposeTransactions()
      disposeStore()
      disposeValidator()
    }, 'xiaotao-adapter: storage services')

    if (checkpoint && validator.has('https://xiaotao.local/schemas/checkpoint.schema.json')) {
      if ((checkpoint.projectRoot !== undefined && !path.isAbsolute(checkpoint.projectRoot))
        || (checkpoint.recoveryRoot !== undefined && !path.isAbsolute(checkpoint.recoveryRoot))) {
        throw new Error('xiaotao-adapter: checkpoint roots must be absolute operator configuration')
      }
      ctx.inject(['tools'], (ctx) => {
        const tools = ctx.get('tools') as ToolRuntime
        const disposeCheckpoint = tools.register(checkpointTool(fs, validator, checkpoint))
        ctx.effect(() => disposeCheckpoint, 'xiaotao-adapter: checkpoint tool')
        ctx.logger.info('xiaotao-adapter: checkpoint tool registered (snapshot mode; live durability not verified)')
      })
    } else if (checkpoint) {
      ctx.logger.warn('xiaotao-adapter: checkpoint not activated; tools or schemas unavailable')
    }
  })
  if (checkpoint && !activation.storage) {
    ctx.logger.info('xiaotao-adapter: checkpoint waiting for filesystem service')
  }

  // Lifecycle hooks: agent registry queues non-waking session-start Runtime Context
  // and provides the optional turn-stopping checkpoint fallback.
  ctx.inject(['agents'], (ctx) => {
    const coordinator = autoCheckpoint !== undefined ? new AutoCheckpointCoordinator(autoCheckpoint) : undefined
    const pressureUnavailable = new WeakSet<object>()
    registerLifecycleHooks(ctx, {
      onSessionStart: async (payload) => {
        const fs = ctx.get('fs') as FileSystem | undefined
        const injected = await injectSessionRuntimeContext(payload, fs as never)
        if (injected) {
          ctx.logger.info('xiaotao-adapter: queued non-waking bounded runtime context for session')
        }
      },
      onTurnStopping: coordinator ? (payload) => {
        const decision = coordinator.evaluateAndTrigger(payload)
        if (decision === 'triggered') {
          ctx.logger.info('xiaotao-adapter: automatic checkpoint step requested by context pressure')
        } else if (decision === 'pressure-unknown' && !pressureUnavailable.has(payload.agent)) {
          pressureUnavailable.add(payload.agent)
          ctx.logger.warn(
            'xiaotao-adapter: automatic checkpoint pressure trigger unavailable for this session; ' +
            'DSH projection and provider prompt-usage fallback were both unavailable',
          )
        }
      } : undefined,
    }, { timeoutMs: autoCheckpoint?.timeoutMs })
    ctx.logger.info(
      'xiaotao-adapter: lifecycle hooks registered (non-waking session-start runtime context; ' +
      (coordinator ? `turn-stopping fallback threshold=${coordinator.threshold})` : 'turn-stopping disabled)'),
    )
  })

  if (!capabilities.agents) {
    ctx.logger.info('xiaotao-adapter: lifecycle hooks waiting for agent service')
  }

  ctx.logger.info(
    `xiaotao-adapter: registered skill "${registration.name}" ` +
      `(storage=${activation.storage}, autoCheckpointRequested=${autoCheckpoint !== undefined}, ` +
      `degraded=${activation.degraded})`,
  )
}

export default apply
