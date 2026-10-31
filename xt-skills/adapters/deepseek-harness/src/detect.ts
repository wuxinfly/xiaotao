/**
 * Capability detection and graceful-fallback decision for the adapter.
 *
 * This is a point-in-time status snapshot. Optional fs/tools enhancements use
 * child injections in index.ts so late services activate without blocking Core.
 *
 * @module @xiaotao-ai/dsh-adapter/detect
 */

import type { Context } from '@deepseek-ai/cordis'
import type { Activation, Capabilities } from './types'

/**
 * Probe which dsh capability seams are mounted in this context.
 *
 * @param ctx - the Cordis context the plugin was applied with.
 * @returns a {@link Capabilities} record; only `skills` is required.
 */
export function detectCapabilities(ctx: Context): Capabilities {
  return {
    skills: ctx.get('skills') !== undefined,
    fs: ctx.get('fs') !== undefined,
    agents: ctx.get('agents') !== undefined,
    persistence: ctx.get('sessionPersistence') !== undefined,
  }
}

/**
 * Guard the one hard requirement. A missing skill registry cannot be worked
 * around by the fallback path, so fail fast with an actionable message.
 *
 * @param capabilities - the result of {@link detectCapabilities}.
 */
export function assertSkills(capabilities: Capabilities): void {
  if (!capabilities.skills) {
    throw new Error(
      '@xiaotao-ai/dsh-adapter requires the dsh skill registry (`ctx.skills`). ' +
        'Load a profile that includes the skill plugin (dsh-base ships it).',
    )
  }
}

/**
 * Resolve the enhancement paths from detected capabilities. The result is
 * pure data; `index.ts` mounts exactly the plugins this decision selects.
 *
 * This is a dependency-only startup snapshot. `hooks` stays `false` here
 * because the opt-in lifecycle handler is mounted later, after config and all
 * injected capabilities are available; `index.ts` reports real activation.
 *
 * @param capabilities - the result of {@link detectCapabilities}.
 * @returns the {@link Activation} plan.
 */
export function planActivation(capabilities: Capabilities): Activation {
  const skill = capabilities.skills
  const storage = capabilities.fs
  const hooks = false
  return {
    skill,
    storage,
    hooks,
    degraded: !storage && !hooks,
  }
}
