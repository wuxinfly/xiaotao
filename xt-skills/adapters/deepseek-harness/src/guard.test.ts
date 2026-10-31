import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  CANONICAL_CORE_GUARD_PROMPT,
  CORE_GUARD_BOUNDARIES,
  CORE_GUARD_MAX_CHARS,
  getCoreGuardNotice,
  validateCoreGuardText,
} from './guard'

test('canonical Core Guard satisfies character budget and boundary checks', () => {
  assert.ok(CANONICAL_CORE_GUARD_PROMPT.length <= CORE_GUARD_MAX_CHARS)
  assert.ok(CANONICAL_CORE_GUARD_PROMPT.length < 800) // approx ~300 tokens
  const result = validateCoreGuardText(CANONICAL_CORE_GUARD_PROMPT)
  assert.equal(result.valid, true)
  assert.equal(result.missingBoundaries.length, 0)
  assert.equal(result.charCount, CANONICAL_CORE_GUARD_PROMPT.length)
  assert.match(CANONICAL_CORE_GUARD_PROMPT, /单会话明确小改动默认由小涛直通/)
  assert.match(CANONICAL_CORE_GUARD_PROMPT, /工具隔离或缺失能力.*明确要求委派.*有界 Worker/)
})

test('Core Guard notice generator returns canonical prompt', () => {
  assert.equal(getCoreGuardNotice(), CANONICAL_CORE_GUARD_PROMPT)
})

test('Core Guard validator detects missing boundaries and budget violations', () => {
  // Missing all boundaries
  const emptyResult = validateCoreGuardText('一些无关文本')
  assert.equal(emptyResult.valid, false)
  assert.deepEqual(emptyResult.missingBoundaries, ['role', 'authorization', 'worker', 'cognition'])

  // Oversized text
  const oversized = CANONICAL_CORE_GUARD_PROMPT + 'x'.repeat(CORE_GUARD_MAX_CHARS)
  const overResult = validateCoreGuardText(oversized)
  assert.equal(overResult.valid, false)
  assert.ok(overResult.charCount > CORE_GUARD_MAX_CHARS)
})
