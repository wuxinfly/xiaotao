# Codex Worker Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Teach the XiaoTao Codex recovery reminder to map bounded Workers to native subagents while preserving separate Codex task creation for explicit user requests.

**Architecture:** Keep the portable XiaoTao Core unchanged and add one host-specific prompt contract to the existing `SessionStart` Hook. Document the Adapter-only guarantee and separate tool-safe identifiers from optional Chinese user-facing labels.

**Tech Stack:** Node.js ESM, `node:test`, Codex SessionStart Hook JSON, Markdown documentation.

---

### Task 1: Add the failing routing prompt contract

**Files:**
- Test: `test/codex-adapter.test.js`

1. Create a valid XiaoTao project fixture and call `recoveryContext`.
2. Assert that the injected context maps a bounded Worker to an available Codex-native subagent.
3. Assert that a separate user-owned task or conversation requires an explicit user request.
4. Assert that tool-facing identifiers follow the visible schema and Chinese labels remain user-facing.
5. Run `node --test test/codex-adapter.test.js` and confirm the new assertions fail before implementation.

### Task 2: Implement the minimal Adapter reminder

**Files:**
- Modify: `adapters/codex/xiaotao-codex/scripts/session-start.mjs:68`

1. Add one bounded reminder entry next to the existing delegation contract.
2. Refer to the current `spawn_agent` name only as an example of a native subagent capability.
3. Preserve authorization, permission, waiting, and fallback semantics.
4. Run `node --test test/codex-adapter.test.js` and confirm the prompt contract passes.

### Task 3: Document scope and manual acceptance

**Files:**
- Modify: `adapters/codex/README.md`

1. Document the Worker/subagent and separate-task distinction.
2. Explain machine-safe identifiers versus Chinese user-facing labels.
3. State that the guarantee requires an enabled and trusted Adapter Hook.
4. Add manual acceptance rows for delegation, explicit task creation, and unavailable subagent capability.

### Task 4: Verify the complete change

**Files:**
- Verify: `adapters/codex/xiaotao-codex/scripts/session-start.mjs`
- Verify: `test/codex-adapter.test.js`
- Verify: `adapters/codex/README.md`

1. Run `node --test test/codex-adapter.test.js`.
2. Run `npm test`.
3. Run `npm run test:contracts`.
4. Run `npm pack --dry-run --offline --ignore-scripts`.
5. Run `git diff --check` and inspect the final diff.
