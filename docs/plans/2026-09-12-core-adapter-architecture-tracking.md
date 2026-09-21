# Core / Adapter Architecture Tracking Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refresh Issue #14 with a versioned Core / Adapter boundary, an evidence-based host capability matrix, and separately tracked remaining work.

**Architecture:** Keep XiaoTao Core host-independent and describe host integrations as optional adapters whose capabilities are reported separately as available, activated, degraded, unsupported, or unverified. Do not add runtime behavior in this change; link every activation claim to current source, tests, or acceptance documentation and move implementation gaps into bounded follow-up issues.

**Tech Stack:** Markdown, GitHub Issues, existing TypeScript/Python adapter evidence.

---

### Task 1: Add the architecture boundary and capability matrix

**Files:**
- Create: `docs/architecture/core-adapter-boundary.md`

**Step 1: Document the dependency rule**

Define Core semantic decisions, Adapter deterministic execution, graceful fallback, and the distinction between capability availability and feature activation.

**Step 2: Record current host status**

Add a feature-by-host matrix for Bare Core, DeepSeek Harness, and Codex. Link claims to current adapter READMEs, architecture documents, source entry points, and manual acceptance requirements.

**Step 3: Record explicit non-goals and remaining packages**

Mark generic unrestricted storage tools, invented lifecycle hooks, multi-file transactions, native Worker isolation, and unverified live-host behavior accurately.

**Step 4: Verify terminology and links**

Run a repository link/path check and inspect the rendered Markdown structure manually.

### Task 2: Add the architecture document to repository navigation

**Files:**
- Modify: `README.md`

**Step 1: Add one concise architecture link**

Place the new document alongside the existing reliability and adapter documentation without duplicating its contents.

**Step 2: Verify the README reference**

Confirm the relative link resolves to the new file.

### Task 3: Split the remaining implementation and acceptance work

**External artifacts:**
- Create: GitHub Issue for DSH multi-file transaction / commit / restart recovery.
- Create: GitHub Issue for real-host Core + Adapter layered acceptance.
- Update: GitHub Issue #14.

**Step 1: Create the transaction issue**

Specify immutable transaction bundles, ordered locks, validation, CAS conflict handling, commit visibility, partial materialization recovery, and fault-injection tests.

**Step 2: Create the live acceptance issue**

Specify real DSH model/tool invocation, persistent filesystem, process restart/new Session recovery, induced primary/recovery failure, Bare Core operation without Adapter, and missing-capability degradation.

**Step 3: Refresh Issue #14**

Update its status without overstating activation. Link the architecture document and both follow-up issues, distinguish completed/partial/deferred work, and retain the architecture invariants.

**Step 4: Re-fetch all three issues**

Verify titles, links, bodies, open state, and cross-references from GitHub.

### Task 4: Final verification

**Files:**
- Verify: `docs/architecture/core-adapter-boundary.md`
- Verify: `README.md`
- Verify: `docs/plans/2026-09-12-core-adapter-architecture-tracking.md`

**Step 1: Inspect the local diff**

Confirm only the planned documentation files changed and preserve unrelated user work.

**Step 2: Run relevant documentation checks**

Run the repository's existing contract/document validation commands that do not mutate product state.

**Step 3: Report results**

Provide local file links, GitHub issue links, validation output, and any remaining blocker. Do not commit or open a PR unless separately requested.
