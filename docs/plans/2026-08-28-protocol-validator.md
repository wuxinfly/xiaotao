# Protocol Validator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a zero-dependency CLI that validates persisted Handoff and Memory Worker JSON artifacts without triggering workflow behavior.

**Architecture:** Implement exact validators for the repository's three current JSON schemas using Python's standard library, plus common project-relative file-reference checks. Exercise the CLI through repository fixtures and the existing PowerShell contract suite, then document the persistence boundary.

**Tech Stack:** Python 3 standard library, JSON fixtures, PowerShell contract tests, Markdown.

---

### Task 1: Add protocol fixtures and failing contract cases

**Files:**
- Create: `xiaotao/references/scenarios/validator-fixtures/*`
- Modify: `scripts/verify-contracts.ps1`

**Step 1:** Add valid Handoff, Memory request, and Memory response artifacts with real referenced files.

**Step 2:** Add invalid JSON, schema-invalid, missing-reference, and traversal fixtures.

**Step 3:** Add a PowerShell helper that invokes `python xiaotao/scripts/validate.py` and asserts exit codes.

**Step 4:** Run `pwsh -File scripts/verify-contracts.ps1` and confirm it fails because the CLI does not exist.

### Task 2: Implement the validator CLI

**Files:**
- Create: `xiaotao/scripts/validate.py`

**Step 1:** Add reusable diagnostics, strict JSON type checks, object property checks, and array checks.

**Step 2:** Implement the Handoff invariants, including exactly one state path and conditional questions.

**Step 3:** Implement Memory Worker request and response invariants.

**Step 4:** Implement portable project-relative path validation and existing-file checks.

**Step 5:** Implement text and `--json` output with exit codes `0`, `1`, and `2`.

**Step 6:** Run every targeted validator fixture and confirm expected results.

### Task 3: Integrate the persistence contract

**Files:**
- Modify: `xiaotao/references/handoffs.md`
- Modify: `xiaotao/references/memory.md`
- Modify: `scripts/verify-contracts.ps1`

**Step 1:** Require validator success immediately before persisting formal artifacts.

**Step 2:** Document one repair attempt and invalid raw-result preservation as orchestrator behavior.

**Step 3:** State explicitly that validation cannot create or transition XiaoTao workflow state.

**Step 4:** Add stable documentation contract assertions.

### Task 4: Verify the complete change

**Files:**
- Verify all modified files.

**Step 1:** Run Python syntax compilation for `xiaotao/scripts/validate.py`.

**Step 2:** Run targeted CLI fixtures in text and JSON modes.

**Step 3:** Run `pwsh -File scripts/verify-contracts.ps1` and expect `All XiaoTao contract checks passed.`

**Step 4:** Inspect `git diff --check`, `git status --short`, and the final diff.

### Task 5: Harden untrusted-input handling after review

**Files:**
- Modify: `xiaotao/scripts/validate.py`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-response-nan-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-response-infinity-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/memory-response-negative-infinity-invalid.json`
- Create: `xiaotao/references/scenarios/validator-fixtures/handoff-control-character-invalid.json`
- Modify: `scripts/verify-contracts.ps1`

**Step 1:** Add failing cases for every non-standard JSON constant and a control character in a
file reference; require stable machine-readable diagnostics.

**Step 2:** Run the valid and schema-invalid fixtures for each protocol through both AJV and the
Python CLI with matching expected outcomes.

**Step 3:** Reject non-standard constants during parsing, reject control characters before host
path APIs, and convert `ValueError` from path operations into validation diagnostics.

**Step 4:** Run targeted cases, the full contract suite, syntax parsing, and `git diff --check`.
