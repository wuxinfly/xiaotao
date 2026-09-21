# Checkpoint Contract Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver the first #26 increment: an evidence-backed DSH capability audit and a reviewable checkpoint protocol, without claiming checkpoint execution is enabled.

**Architecture:** Core owns target selection and saved-state semantics. Adapter owns source capture, durable recovery records, guarded publication and host integration. This increment documents the proposed protocol outside the active Skill; execution and schema activation follow in a separate implementation.

**Tech Stack:** Markdown, Node.js built-ins, installed DSH declarations and the adapter lockfile.

The user has authorized execution in this session; proceed locally without delegation. The optional external execution skill named in the template is not needed for this documentation/audit increment.

### Task 1: Inspect the actual host seams

- Read `adapters/deepseek-harness/src/{index,detect,hooks,storage,validate}.ts`.
- Inspect installed `@deepseek-ai/dsh-{agent,session,system-prompt}` declarations and package versions.
- Separate declaration evidence from runtime availability, persistence durability and XiaoTao activation.

### Task 2: Make the evidence repeatable

- Create `adapters/deepseek-harness/scripts/audit-checkpoint.mjs`.
- Emit installed/locked versions, declaration paths, line numbers and hashes for selected symbols.
- Report missing files/symbols explicitly, with nonzero exit for incomplete evidence; do not inspect user sessions or activate services.
- Run from the repository root and a different working directory; expect identical structured output.

### Task 3: Specify the minimal checkpoint contract

- Create `docs/architecture/checkpoint-contract.md` with the source matrix, target mapping, durable request layout, commit authority, retries, conflict handling and recovery acceptance scenarios.
- Distinguish source position from model coverage and FsVersion from Core revision.
- Define how an interrupted successful write is recognized without a multi-file commit.
- Link from `adapters/deepseek-harness/README.md` without changing active Core instructions.

### Task 4: Verify and hand off

- Run the audit, `node --check` and `git diff --check`.
- Check local Markdown links and exact source symbols.
- No behavior-mirroring tests for prose or string scans; actual checkpoint integration tests belong to the next implementation.
- Commit the bounded change and create a PR referencing #14 / #26 without closing them.
- Record that live DSH durability, model-facing tool registration and end-to-end recovery remain unverified.
