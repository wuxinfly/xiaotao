/**
 * Shared types for the XiaoTao DeepSeek Harness adapter.
 *
 * This module deliberately imports nothing from `@deepseek-ai/*`: the config
 * and capability shapes are host-independent so the adapter's own logic stays
 * testable without a live dsh runtime.
 *
 * @module @xiaotao-ai/dsh-adapter/types
 */

/** Adapter configuration supplied by the Cordis profile / bundle. */
export interface AdapterConfig {
  /**
   * Absolute or cwd-relative path to the XiaoTao Core directory (the folder
   * that contains `SKILL.md`, `references/`, and `schemas/`). When omitted,
   * the adapter probes the default roots below.
   */
  coreDir?: string
  /** Automatic session binding by default; false disables the tool. */
  checkpoint?: false | {
    projectRoot?: string
    /** Automatic archive base, or exact archive root when projectRoot is set. */
    recoveryRoot?: string
    /**
     * Opt-in context-pressure trigger. This is a DSH turn-boundary fallback,
     * not a claim that DSH exposes a pre-compaction hook.
     */
    auto?: false | AutoCheckpointConfig
  }
}

/** Experimental automatic checkpoint trigger settings. */
export interface AutoCheckpointConfig {
  /** Projected next-request prompt ratio (or provider prompt-usage fallback). */
  pressureThreshold?: number
  /** Minimum number of turns before another automatic reminder. */
  cooldownTurns?: number
  /** Maximum time the serial lifecycle hook waits for its handler. */
  timeoutMs?: number
}

/** Default probe order for the XiaoTao Core directory, relative to cwd. */
export const DEFAULT_CORE_DIRS = ['.dsh/skills/xiaotao', 'xiaotao'] as const

/**
 * The dsh capability seams the adapter can build on. `skills` is mandatory
 * (nothing works without a skill registry); every other seam is optional and
 * gates an enhancement path only.
 */
export interface Capabilities {
  /** `ctx.skills` — skill registry (required). */
  skills: boolean
  /** `ctx.fs` — filesystem seam; enables the deterministic state store. */
  fs: boolean
  /** `ctx.agents` — agent registry; enables session-lifecycle hooks. */
  agents: boolean
  /** `ctx.sessionPersistence` — durable session store (future use). */
  persistence: boolean
}

/** What the adapter actually activated, derived from {@link Capabilities}. */
export interface Activation {
  /** Core skill was registered (always true when `skills` is present). */
  skill: boolean
  /** Deterministic state store was mounted (requires `fs`). */
  storage: boolean
  /** Base startup snapshot; dynamic lifecycle activation is reported separately. */
  hooks: boolean
  /** Whether the adapter degraded to the plain-skill fallback. */
  degraded: boolean
}

/** Minimal parsed frontmatter from a XiaoTao `SKILL.md`. */
export interface SkillFrontmatter {
  name: string
  description: string
}
