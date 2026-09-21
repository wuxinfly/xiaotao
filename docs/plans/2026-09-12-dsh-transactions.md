# DSH Transactions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a restart-recoverable DSH transaction service for multi-file create/replace operations with commit-marker logical visibility.

**Architecture:** Persist immutable before/staged snapshots and intent under `.xiaotao/transactions/<id>`, publish exactly one terminal marker as the logical commit point, then materialize members with per-file CAS and immutable applied observations. Register the engine as an internal Cordis service; do not expose a generic model-facing tool or claim lifecycle move support without a DSH delete/rename seam.

**Tech Stack:** TypeScript, `@deepseek-ai/dsh-fs`, Cordis services, Node test runner, YAML.

---

### Task 1: Define transaction records and input validation

**Files:**
- Create: `adapters/deepseek-harness/src/transaction.ts`
- Create: `adapters/deepseek-harness/src/transaction.test.ts`

**Step 1:** Add failing tests for safe transaction IDs, create/replace members, duplicate paths, traversal, absolute paths, unsupported move/delete, and byte limits.

**Step 2:** Run `npm.cmd test -- --test-name-pattern transaction` in `adapters/deepseek-harness` and confirm failures.

**Step 3:** Implement record types, canonical serialization/hash helpers, path normalization, and stable error codes.

**Step 4:** Re-run focused tests and confirm they pass.

### Task 2: Persist immutable preparation records

**Files:**
- Modify: `adapters/deepseek-harness/src/transaction.ts`
- Modify: `adapters/deepseek-harness/src/transaction.test.ts`

**Step 1:** Add failing tests for before/staged snapshots, intent-last publication, exact reread verification, and validator failure before commit.

**Step 2:** Implement stable lock ordering, before observation checks, immutable snapshot publication, intent publication, and staged validation callback.

**Step 3:** Verify preparation failures publish no commit and preserve canonical bytes.

### Task 3: Commit and materialize members

**Files:**
- Modify: `adapters/deepseek-harness/src/transaction.ts`
- Modify: `adapters/deepseek-harness/src/transaction.test.ts`

**Step 1:** Add failing tests for mutually exclusive terminal markers, commit-before-materialization, create/replace CAS, applied records, and reverse lock release.

**Step 2:** Implement terminal publication and per-member materialization with reread hash confirmation.

**Step 3:** Verify normal transactions expose one committed logical result and fully materialize all members.

### Task 4: Add status, overlay reads, and restart recovery

**Files:**
- Modify: `adapters/deepseek-harness/src/transaction.ts`
- Modify: `adapters/deepseek-harness/src/transaction.test.ts`

**Step 1:** Add failing tests that inject interruption after commit and between member/applied writes.

**Step 2:** Implement intent/snapshot/terminal validation, status classification, overlay resolution, and idempotent recovery using newly observed FsVersion tokens.

**Step 3:** Add conflict tests where canonical bytes match neither before nor staged, plus terminal corruption and multiple-overlay rejection.

**Step 4:** Verify recovery never rolls back committed state or overwrites newer bytes.

### Task 5: Register the internal Cordis service

**Files:**
- Modify: `adapters/deepseek-harness/src/index.ts`
- Modify: `adapters/deepseek-harness/src/types.ts`
- Modify: `adapters/deepseek-harness/src/transaction.test.ts`

**Step 1:** Export a stable transaction service name and register/dispose it beside stateStore and schemaValidator when `ctx.fs` is available.

**Step 2:** Verify the service is internal-only and no generic model-facing tool is registered.

### Task 6: Document truthful capability status

**Files:**
- Modify: `adapters/deepseek-harness/README.md`
- Modify: `docs/architecture/core-adapter-boundary.md`

**Step 1:** Document create/replace-only support, overlay/recovery requirements, and the missing delete/rename limitation.

**Step 2:** Mark the mechanism/service available, model-facing business paths inactive, and live backend unverified.

### Task 7: Full verification

**Files:**
- Verify all files above.

**Step 1:** Run DSH typecheck, unit tests, and package tests.

**Step 2:** Run repository unit tests and contract checks.

**Step 3:** Run `git diff --check`, inspect the final diff, and report any live-host work left for #69.
