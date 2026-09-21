/**
 * Schema validation (capability plugin): validate XiaoTao's mutable-state
 * documents against the JSON Schemas shipped inside the Core Skill.
 *
 * XiaoTao Core already owns the schemas (`xiaotao/references/schemas/*.json`,
 * JSON Schema draft 2020-12). The adapter reuses those files verbatim and adds
 * only the runtime validation call — it does not re-declare or reinterpret the
 * contracts. This keeps the Core as the single source of truth.
 *
 * @module @xiaotao-ai/dsh-adapter/validate
 */

import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import Ajv2020 from 'ajv/dist/2020.js'

/** Outcome of a single validation run. */
export interface ValidationResult {
  ok: boolean
  errors: string[]
}

/**
 * Validator that loads and evaluates the Core Skill's JSON Schemas.
 */
export class XiaoTaoSchemaValidator {
  private readonly ajv = new Ajv2020({ allErrors: true, strict: false })

  has(schemaId: string): boolean {
    return this.ajv.getSchema(schemaId) !== undefined
  }

  /**
   * Load every `*.json` schema from the Core's `schemas/` directory and
   * register it under its `$id` so {@link validate} can address it.
   *
   * A missing directory is not fatal: the adapter's graceful-fallback
   * contract means validation simply reports "schema not loaded" later,
   * while the rest of the storage path keeps working.
   *
   * @param schemasDir - absolute path to `xiaotao/references/schemas/`.
   * @returns the number of schemas registered.
   */
  async loadAll(schemasDir: string): Promise<number> {
    let entries: string[]
    try {
      entries = await readdir(schemasDir)
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') return 0
      throw error
    }
    let loaded = 0
    for (const entry of entries) {
      if (!entry.endsWith('.json')) continue
      const raw = await readFile(path.join(schemasDir, entry), 'utf8')
      const schema = JSON.parse(raw) as { $id?: string }
      this.ajv.addSchema(schema)
      loaded += 1
    }
    return loaded
  }

  /**
   * Validate a document against the schema identified by `$id`.
   *
   * @param schemaId - the schema `$id` (e.g. `https://xiaotao.local/schemas/task.schema.json`).
   * @param data - the parsed document.
   */
  validate(schemaId: string, data: unknown): ValidationResult {
    const validator = this.ajv.getSchema(schemaId)
    if (validator === undefined) {
      return { ok: false, errors: [`schema not loaded: ${schemaId}`] }
    }
    const ok = validator(data)
    if (ok) return { ok: true, errors: [] }
    const errors = (validator.errors ?? []).map(
      (error) => `${error.instancePath || '/'} ${error.message ?? 'invalid'}`,
    )
    return { ok: false, errors }
  }
}
