# Codex Worker Snapshot Cross-check Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Validate persistent Delegation Packets against immutable Worker YAML snapshots before Codex injects them into native subagents.

**Architecture:** Add a dependency-free, fail-closed YAML projection parser and Worker/Packet cross-check module. Resolve and constrain the snapshot beside the Packet scope, then retain `tool-isolation` as the sole Codex degradation.

**Tech Stack:** Node.js ESM, built-in filesystem/path APIs, Node test runner.

---

### Task 1: Define failing contract tests

**Files:**
- Modify: `test/codex-adapter.test.js`

1. Update the valid Packet fixture to create a schema-v2 Worker snapshot and declare its path.
2. Add acceptance tests for snapshot-backed injection.
3. Add rejection cases for tools, permissions, context, outputs, instructions, identity, path placement, and malformed YAML.
4. Run `node --test test/codex-adapter.test.js` and confirm failures.

### Task 2: Implement bounded snapshot validation

**Files:**
- Create: `adapters/codex/xiaotao-codex/scripts/worker-snapshot-contract.mjs`
- Modify: `adapters/codex/xiaotao-codex/scripts/delegation-contract.mjs`
- Modify: `adapters/codex/xiaotao-codex/scripts/subagent-start.mjs`

1. Implement the fail-closed YAML projection parser.
2. Validate the projected Worker fields and subset relationships.
3. Require the canonical sibling snapshot path for persistent Packet layouts.
4. Change the Codex degradation contract to require only `tool-isolation`.
5. Run the focused adapter tests and confirm they pass.

### Task 3: Package and document the contract

**Files:**
- Modify: `adapters/codex/install-local.mjs`
- Modify: `README.md`
- Modify: `adapters/codex/README.md`
- Modify: `docs/architecture/core-adapter-boundary.md`

1. Add the snapshot contract module to managed installer files.
2. Document the enforced snapshot boundary and remaining Hook limitation.
3. Run installer and packaging tests.

### Task 4: Full verification

1. Run `npm test`.
2. Run `npm run test:contracts`.
3. Run `npm pack --dry-run --offline --ignore-scripts`.
4. Run `git diff --check`.
5. Review the final diff without committing.
