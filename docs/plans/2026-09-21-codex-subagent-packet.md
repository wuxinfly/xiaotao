# Codex Subagent Packet Injection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Inject one validated, session-bound XiaoTao Delegation Packet into a real Codex subagent without claiming tool or permission isolation.

**Architecture:** A persistent main Agent stages one canonical Packet plus a tiny Session-bound envelope. `SubagentStart` atomically claims the envelope, validates Packet boundaries and instruction digests, and returns bounded developer context.

**Tech Stack:** Node.js ESM, Codex command Hooks, SHA-256, `node:test`, JSON.

---

### Task 1: Define failing lifecycle tests

**Files:**
- Modify: `test/codex-adapter.test.js`

1. Create a valid Task Worker Packet fixture with one Core instruction.
2. Assert `SubagentStart` injects objective, instruction content, bounds, and output paths.
3. Assert a second invocation cannot reuse the pending envelope.
4. Assert Session mismatch, traversal, digest mismatch, and non-degraded host status produce no context.
5. Run the focused test and confirm the missing implementation fails.

### Task 2: Implement Packet validation and one-time claim

**Files:**
- Create: `adapters/codex/xiaotao-codex/scripts/delegation-contract.mjs`
- Create: `adapters/codex/xiaotao-codex/scripts/subagent-start.mjs`

1. Implement strict envelope and Packet shape validation.
2. Resolve the XiaoTao project without crossing repository boundaries.
3. Atomically rename the pending envelope to a hashed claim path.
4. Recompute resolved instruction digests and reject mismatch or missing required refs.
5. Emit bounded `SubagentStart.additionalContext` only after all checks pass.
6. Run focused tests.

### Task 3: Wire and package the Hook

**Files:**
- Modify: `adapters/codex/xiaotao-codex/hooks/hooks.json`
- Modify: `adapters/codex/install-local.mjs`
- Modify: `test/codex-adapter.test.js`

1. Register synchronous `SubagentStart` with a five-second timeout and bounded context limit.
2. Add the two modules to managed plugin files and version hashing.
3. Assert installed plugin execution from a path containing spaces and metacharacters.
4. Run Adapter tests.

### Task 4: Document activation and verify

**Files:**
- Modify: `adapters/codex/README.md`
- Modify: `docs/architecture/core-adapter-boundary.md`

1. Document staging, single-Worker scope, one-time claim, degraded isolation, and failure behavior.
2. Keep automatic PreCompact checkpoint marked inactive with the official limitation.
3. Run `node --test test/codex-adapter.test.js`.
4. Run `npm test` and `npm run test:contracts`.
5. Run `npm pack --dry-run --offline --ignore-scripts` and `git diff --check`.
