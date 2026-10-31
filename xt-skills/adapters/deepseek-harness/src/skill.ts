/**
 * Skill adapter (thin layer): register the portable XiaoTao Core Skill as a
 * dsh skill.
 *
 * XiaoTao Core is a directory containing `SKILL.md` (with `name` /
 * `description` frontmatter) plus `references/` and `schemas/`. dsh's skill
 * model is the same shape, so the adapter only has to read that one file and
 * hand the parsed body to `ctx.skills.register()`. The `resourceBase` marks the
 * Core directory so the model can resolve the relative `references/…` paths
 * that `SKILL.md` points at — progressive disclosure is preserved, not
 * flattened.
 *
 * The frontmatter parser below mirrors `skill-filesystem`'s behaviour for the
 * two fields XiaoTao actually uses, so the adapter does not depend on the dsh
 * provider plugin.
 *
 * @module @xiaotao-ai/dsh-adapter/skill
 */

import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse as parseYaml } from 'yaml'
import type { Context } from '@deepseek-ai/cordis'
import type { SkillRegistration } from '@deepseek-ai/dsh-skill'
import { DEFAULT_CORE_DIRS, type AdapterConfig, type SkillFrontmatter } from './types'

const SKILL_FILE = 'SKILL.md'
const PROVIDER = 'xiaotao-adapter'
const SOURCE = 'custom' as const
const PACKAGED_CORE_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'core')

/**
 * Resolve the XiaoTao Core directory from config, probing default roots when
 * `coreDir` is omitted. The first candidate that contains a `SKILL.md` wins.
 *
 * @param config - adapter config.
 * @param cwd - the working directory relative paths resolve against.
 * @returns the resolved Core directory.
 * @throws when no candidate contains a `SKILL.md`.
 */
export async function resolveCoreDir(config: AdapterConfig, cwd: string): Promise<string> {
  const candidates: string[] = []
  if (config.coreDir !== undefined) {
    candidates.push(path.resolve(cwd, config.coreDir))
  } else {
    for (const dir of DEFAULT_CORE_DIRS) {
      candidates.push(path.resolve(cwd, dir))
    }
    candidates.push(PACKAGED_CORE_DIR)
  }
  for (const candidate of candidates) {
    try {
      await readFile(path.join(candidate, SKILL_FILE), 'utf8')
      return candidate
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error
    }
  }
  throw new Error(
    `@xiaotao-ai/dsh-adapter: no XiaoTao Core found (looked for ${SKILL_FILE} in ${candidates.join(', ')}). ` +
      'Set `coreDir` to the directory that contains SKILL.md.',
  )
}

/**
 * Split a `SKILL.md` into its YAML frontmatter and Markdown body. Returns
 * `undefined` when the file has no valid `--- … ---` frontmatter block.
 *
 * @param raw - the raw file text.
 * @returns parsed frontmatter data and body, or `undefined`.
 */
export function parseFrontmatter(raw: string): { data: Record<string, unknown>; body: string } | undefined {
  const firstLineEnd = raw.indexOf('\n')
  if (firstLineEnd < 0) return undefined
  const firstLine = raw.slice(0, firstLineEnd).replace(/\r$/, '')
  if (firstLine !== '---') return undefined

  // Walk line by line to find the closing `---` (same as dsh skill-filesystem).
  let lineStart = firstLineEnd + 1
  while (lineStart <= raw.length) {
    const nextNewline = raw.indexOf('\n', lineStart)
    const lineEnd = nextNewline < 0 ? raw.length : nextNewline
    const line = raw.slice(lineStart, lineEnd).replace(/\r$/, '')
    if (line === '---') {
      const yamlText = raw.slice(firstLineEnd + 1, lineStart)
      const parsed = parseYaml(yamlText) as unknown
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return undefined
      const bodyStart = nextNewline < 0 ? raw.length : nextNewline + 1
      return { data: parsed as Record<string, unknown>, body: raw.slice(bodyStart) }
    }
    if (nextNewline < 0) return undefined
    lineStart = nextNewline + 1
  }
  return undefined
}

/**
 * Extract the `name` / `description` frontmatter fields the dsh skill registry
 * requires.
 *
 * @param data - parsed frontmatter data.
 * @returns the validated {@link SkillFrontmatter}.
 * @throws when either field is missing or empty.
 */
export function readSkillFrontmatter(data: Record<string, unknown>): SkillFrontmatter {
  const name = data.name
  const description = data.description
  if (typeof name !== 'string' || name.length === 0) {
    throw new Error('XiaoTao SKILL.md frontmatter is missing a non-empty `name`.')
  }
  if (typeof description !== 'string' || description.length === 0) {
    throw new Error('XiaoTao SKILL.md frontmatter is missing a non-empty `description`.')
  }
  return { name, description }
}

/**
 * Load the XiaoTao Core Skill from disk into a dsh {@link SkillRegistration}.
 *
 * @param coreDir - resolved Core directory (see {@link resolveCoreDir}).
 * @returns a registration ready for `ctx.skills.register()`.
 */
export async function loadCoreSkill(coreDir: string): Promise<SkillRegistration> {
  const raw = await readFile(path.join(coreDir, SKILL_FILE), 'utf8')
  const parsed = parseFrontmatter(raw)
  if (parsed === undefined) {
    throw new Error(`XiaoTao ${SKILL_FILE} at ${coreDir} has no valid YAML frontmatter.`)
  }
  const { name, description } = readSkillFrontmatter(parsed.data)
  return {
    name,
    description,
    content: parsed.body.trim(),
    source: SOURCE,
    provider: PROVIDER,
    resourceBase: { kind: 'directory', path: coreDir },
  }
}

/**
 * Register the loaded Core Skill on the context's skill registry. Returns the
 * exact effect disposer so a composite effect can nest teardown order.
 *
 * @param ctx - the Cordis context.
 * @param registration - the result of {@link loadCoreSkill}.
 * @returns the registry disposer.
 */
export function registerCoreSkill(ctx: Context, registration: SkillRegistration): () => void {
  return ctx.skills.register(registration)
}
