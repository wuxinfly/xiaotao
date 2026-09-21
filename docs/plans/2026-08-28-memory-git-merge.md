# Memory Git Semantic Merge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement 3-way semantic memory merge, provenance contract for conflicts, Memory Merger worker, and schemas for Issue #10.

**Architecture:** Introduce JSON Schema Draft 2020-12 contracts for 3-way memory merge requests and responses, register `memory-merger` in the built-in registry, extend the Python protocol validator, provide valid and invalid fixtures, and document the Git tracking boundary, merge rules, and provenance contract across XiaoTao references.

**Tech Stack:** Markdown contracts, JSON Schema Draft 2020-12, Python 3 standard library, PowerShell contract runner, Ajv CLI.

---

### Task 1: Add JSON Schemas for Memory Merge

**Files:**
- Create: `xiaotao/references/schemas/memory-merge-request.schema.json`
- Create: `xiaotao/references/schemas/memory-merge-response.schema.json`

**Step 1:** Create `memory-merge-request.schema.json` with `base_entries`, `ours_entries`, `theirs_entries`, `file_path`, and optional `merge_hints`.
**Step 2:** Create `memory-merge-response.schema.json` with `status: completed`, `merged_entries`, `resolved`, `unresolved_conflicts` with provenance, and `requires_human_review`.

---

### Task 2: Register Built-in Memory Merger Worker

**Files:**
- Modify: `xiaotao/references/workers/builtin-registry.json`

**Step 1:** Add the `memory-merger` worker entry with capabilities `["conflict-resolution", "memory-merge", "provenance-tracking"]`.

---

### Task 3: Implement Validator Support & Fixtures

**Files:**
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-request-valid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-request-schema-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-request-missing-reference-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-response-valid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-response-schema-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-response-conflict-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-response-date-time-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-response-duplicate-id-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-merge-response-missing-reference-invalid.json`
- Modify: `xiaotao/scripts/validate.py`

**Step 1:** Add fixtures covering valid requests/responses and all failure modes (missing refs, bad schema, non-RFC3339 datetime, conflict status mismatch, duplicate conflict ID).
**Step 2:** Extend `validate.py` with `memory-merge-request` and `memory-merge-response` validators.

---

### Task 4: Update Reference Docs & Contracts

**Files:**
- Modify: `xiaotao/references/memory.md`
- Modify: `xiaotao/references/storage.md`
- Modify: `xiaotao/references/workers.md`
- Modify: `xiaotao/references/scenarios/reliability.md`
- Modify: `xiaotao/SKILL.md`
- Modify: `README.md`
- Modify: `scripts/verify-contracts.ps1`

**Step 1:** Document Git boundary (Local vs Team Shared), 3-way merge rules, conflict provenance contract, and anti-resurrection rules in `memory.md` and `storage.md`.
**Step 2:** Add multi-user Git memory merge reliability scenarios.
**Step 3:** Register schemas and fixtures in `verify-contracts.ps1` and assert contract texts.

---

### Task 5: Verify, Commit, and Create PR

**Files:**
- Review: all changed files

**Step 1:** Run `./scripts/verify-contracts.ps1`.
**Step 2:** Commit with descriptive commit message.
**Step 3:** Push branch `codex/issue-10-memory-git-merge` and create a PR targeting `master` referencing `Closes #10`.
