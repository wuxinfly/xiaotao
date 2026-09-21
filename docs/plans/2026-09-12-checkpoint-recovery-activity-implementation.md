# Checkpoint Recovery Activity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Project explicit successful checkpoint retries into Activity as deterministic `checkpoint_recovered` milestones with an authoritative completion time.

**Architecture:** The DSH Adapter publishes a versioned immutable checkpoint observation whose stable fields bind it to the request and whose `completion`/`committed_at` fields describe the successful entry point and completion boundary. The Python Activity catalog scans only canonical target checkpoint directories, validates recovery observations against adjacent requests, and derives a rebuildable event view.

**Tech Stack:** TypeScript, Node test runner, JSON Schema 2020-12, Python 3, PowerShell contract checks.

---

### Task 1: Version checkpoint observations

**Files:**
- Create: `xiaotao/references/schemas/checkpoint-observation.schema.json`
- Modify: `adapters/deepseek-harness/src/checkpoint.ts`
- Test: `adapters/deepseek-harness/src/checkpoint.test.ts`

**Steps:**
1. Add failing tests proving `save` emits `completion: save`, `retry` emits `completion: recovery`, timestamps remain immutable, legacy four-field observations remain readable, and malformed/new mismatched observations fail.
2. Run `npm test -- --test-name-pattern observation` in the Adapter and confirm the new assertions fail.
3. Add the dual-version schema and pass `save | recovery` explicitly into the shared commit path.
4. Replace string regeneration checks with schema validation plus strict request binding; preserve exact legacy validation.
5. Run Adapter typecheck and checkpoint tests; commit the passing protocol increment.

### Task 2: Derive checkpoint recovery Activity events

**Files:**
- Modify: `xiaotao/references/schemas/activity-event.schema.json`
- Modify: `xiaotao/scripts/validate.py`
- Modify: `xiaotao/scripts/activity_catalog.py`
- Create: `xiaotao/references/scenarios/validator-fixtures/activity-event-checkpoint-recovered-valid.json`
- Test: `test/activity-catalog.test.js`

**Steps:**
1. Add failing tests for active/archived Task and Temporary recovery events, save/legacy omission, deterministic IDs, digest invalidation, and corrupt authority failure.
2. Run `node --test test/activity-catalog.test.js` and confirm failures.
3. Add `checkpoint_recovered` to schema and native event validators.
4. Discover only canonical target checkpoint directories, validate recovery observation/request bindings, normalize `committed_at`, and derive events whose source is the committed observation.
5. Hash the checkpoint sources used by the new projection and reject unsafe, duplicate, missing, or mismatched recovery authority.
6. Run Activity and contract validator tests; commit the passing derivation increment.

### Task 3: Align lifecycle documentation and contracts

**Files:**
- Modify: `xiaotao/references/activity.md`
- Modify: `xiaotao/references/storage.md`
- Modify: `docs/architecture/checkpoint-contract.md`
- Modify: `docs/manual-acceptance.md`
- Modify: `scripts/verify-contracts.ps1`

**Steps:**
1. Document the explicit-retry-only milestone, observation formats, authoritative time, exclusions, compatibility, scan boundaries, and deletion safety.
2. Add required contract assertions for the new schema and lifecycle terms.
3. Run `npm run test:contracts` and fix all contract drift.
4. Commit the documentation/contract increment.

### Task 4: Full verification

**Files:**
- Verify all changed files.

**Steps:**
1. Run Adapter `npm run typecheck`, `npm test`, and `npm run test:package`.
2. Run repository `npm test`, `npm run test:contracts`, and `npm pack --dry-run`.
3. Run `git diff --check` and review the full diff for unintended authority expansion.
4. Report exact results and remaining manual acceptance, if any.

