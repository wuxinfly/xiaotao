# Codex Adapter Reliability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make Codex SessionStart recovery non-executable, predictably bounded, and correctly ordered while preserving useful project recovery context.

**Architecture:** Replace the Python Catalog subprocess with a read-only bounded Node.js metadata scanner. Extract Handoff contract validation into a small packaged module and rank valid Handoffs globally after bounded Task discovery.

**Tech Stack:** Node.js ESM, `node:test`, Codex Hook JSON, Markdown.

---

### Task 1: Lock down the no-execution and no-write contract

**Files:**
- Modify: `test/codex-adapter.test.js`
- Modify: `adapters/codex/xiaotao-codex/scripts/session-start.mjs`

1. Add a test containing a planted project `memory_catalog.py` that would create a sentinel file.
2. Assert SessionStart returns a bounded overview without creating the sentinel, index, or manifest.
3. Run `node --test test/codex-adapter.test.js` and confirm the old implementation fails.
4. Remove subprocess discovery and invocation from the Hook.
5. Implement minimal read-only Task, Temporary, follow-up, Long-term, and checkpoint discovery.
6. Run the focused test and confirm it passes.

### Task 2: Add deterministic scan budgets

**Files:**
- Modify: `test/codex-adapter.test.js`
- Modify: `adapters/codex/xiaotao-codex/scripts/session-start.mjs`

1. Add tests for oversized metadata and excessive directory entries.
2. Add shared limits for directory entries, metadata bytes, total bytes, and active Task scans.
3. Return a stable `scan_budget_exceeded` degraded warning without including private file content.
4. Run the Adapter tests.

### Task 3: Extract the Handoff contract and fix recency ordering

**Files:**
- Create: `adapters/codex/xiaotao-codex/scripts/handoff-contract.mjs`
- Modify: `adapters/codex/xiaotao-codex/scripts/session-start.mjs`
- Modify: `adapters/codex/install-local.mjs`
- Modify: `test/codex-adapter.test.js`

1. Add contract tests for valid and invalid Handoff fixtures.
2. Add a regression test with four active Tasks whose newest Handoff belongs to the lexically last Task.
3. Extract strict shape validation into `handoff-contract.mjs`.
4. Scan bounded active Tasks, collect valid Handoffs, globally sort by mtime, and keep three.
5. Package the new module in the local installer and assert it is installed.
6. Run Adapter tests.

### Task 4: Update documentation and perform full verification

**Files:**
- Modify: `adapters/codex/README.md`
- Modify: `docs/architecture/core-adapter-boundary.md`

1. Document that SessionStart never executes project code or rebuilds the Catalog.
2. Document scan degradation and global Handoff recency semantics.
3. Run `node --test test/codex-adapter.test.js`.
4. Run `npm test`.
5. Run `npm run test:contracts`.
6. Run `npm pack --dry-run --offline --ignore-scripts`.
7. Run `git diff --check` and inspect `git status --short`.
