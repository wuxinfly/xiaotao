# Memory Evolution Contract Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make Memory Worker output express and validate reviewed UPDATE, MERGE, CREATE, and SKIP proposals for Issue #9.

**Architecture:** Enrich the existing `long_term_candidates` response objects instead of adding a runtime or storage tier. JSON Schema and the zero-dependency Python validator enforce the same per-candidate rules, while documentation preserves reviewer authority and existing mutable-state transactions.

**Tech Stack:** Markdown contracts, JSON Schema Draft 2020-12, Python 3 standard library, PowerShell contract runner, Ajv CLI.

---

### Task 1: Add failing evolution fixtures

**Files:**
- Modify: `xiaotao/references/scenarios/validator-fixtures/memory-response-valid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-response-action-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-response-conflict-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-response-duplicate-id-invalid.json`
- Modify: `scripts/verify-contracts.ps1`

**Step 1:** Expand the valid response with CREATE, UPDATE, MERGE, and SKIP examples.

**Step 2:** Add invalid action-cardinality, conflict-status, and duplicate-ID cases.

**Step 3:** Register the new cases in `verify-contracts.ps1`.

**Step 4:** Run `./scripts/verify-contracts.ps1` and verify the new valid fixture fails against the old schema.

### Task 2: Implement the response contract

**Files:**
- Modify: `xiaotao/references/schemas/memory-worker-response.schema.json`
- Modify: `xiaotao/scripts/validate.py`

**Step 1:** Add reusable JSON Schema definitions for source, match, and Long-term candidate proposals.

**Step 2:** Encode action/match target cardinality and conflict-status conditions.

**Step 3:** Mirror the same rules in the Python validator.

**Step 4:** Reject duplicate `candidate_id` values across a response.

**Step 5:** Run focused Ajv and Python validation cases, then run the full contract suite.

### Task 3: Document promotion and review behavior

**Files:**
- Modify: `xiaotao/references/memory.md`
- Modify: `xiaotao/references/storage.md`
- Modify: `xiaotao/references/coordination.md`
- Modify: `README.md`

**Step 1:** Document extraction, comparison, proposal ordering, and forbidden direct copies.

**Step 2:** Define stable Long-term entry IDs, candidate persistence, provenance, conflicts, and reviewer authority.

**Step 3:** Update completion flow to review proposed actions rather than generic candidates.

**Step 4:** Add contract-string assertions and run the full contract suite.

### Task 4: Verify and publish

**Files:**
- Review: all changed files

**Step 1:** Run `./scripts/verify-contracts.ps1` from a clean PowerShell process.

**Step 2:** Inspect `git diff --check`, the full diff, and staged paths.

**Step 3:** Commit only Issue #9 files with an issue-focused message.

**Step 4:** Push `codex/issue-9-memory-evolution` and create a draft PR targeting `master` with `Closes #9`.
