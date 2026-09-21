# Multi-host CLI MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship a small npm CLI that installs and updates the portable XiaoTao Skill for Codex,
Claude Code, and OpenCode, then verifies each managed installation.

**Architecture:** Keep `xiaotao/` as the canonical semantic Core and add a zero-dependency Node.js
adapter CLI. A declarative host registry maps tool IDs to project-local Skill destinations; managed
markers and `.xiaotao/installation.json` make updates safe and repeatable.

**Tech Stack:** Node.js 22 ESM, built-in `node:test`, npm packaging, PowerShell contract checks.

---

### Task 1: Define the npm package and host registry

**Files:**
- Create: `package.json`
- Create: `bin/xiaotao.js`
- Create: `cli/hosts.js`
- Test: `test/hosts.test.js`

1. Write tests for the three canonical tool IDs, paths, aliases, and detection paths.
2. Run `node --test test/hosts.test.js` and confirm it fails because the registry is absent.
3. Implement the package metadata, executable shim, and declarative registry.
4. Re-run the targeted test and confirm it passes.

### Task 2: Implement safe managed installation

**Files:**
- Create: `cli/install.js`
- Test: `test/install.test.js`

1. Write tests that install every selected host into a temporary project.
2. Cover ownership markers, repeatable refresh, preservation of unknown files, and refusal to
   overwrite a non-empty unmanaged destination.
3. Run the test and confirm the expected failures.
4. Implement source discovery, managed copying, marker writes, and installation metadata.
5. Re-run the targeted test and confirm it passes.

### Task 3: Implement init, update, doctor, and CLI parsing

**Files:**
- Create: `cli/index.js`
- Test: `test/cli.test.js`

1. Write subprocess tests for `--help`, `--version`, non-interactive `init --tools`, `update`, and
   successful/failed `doctor` runs.
2. Run the test and confirm it fails because command dispatch is absent.
3. Implement argument parsing, interactive selection, diagnostics, JSON output, and exit codes.
4. Re-run the targeted test and confirm it passes.

### Task 4: Align documentation and contracts

**Files:**
- Modify: `README.md`
- Modify: `xiaotao/SKILL.md`
- Modify: `.github/workflows/contracts.yml`
- Modify: `scripts/verify-contracts.ps1`

1. Replace Codex-only and no-CLI release claims with the portable Core plus mechanical CLI model.
2. Document installation, supported tools, generated paths, updating, and CLI boundaries.
3. Add contract assertions that prevent semantic workflow logic from moving into the host registry.
4. Make CI run `npm test`, the existing contract suite, and `npm pack --dry-run`.

### Task 5: Verify the publishable artifact

**Files:**
- Test: generated npm tarball inspection only; do not commit the tarball.

1. Run `npm test`.
2. Run `pwsh -File scripts/verify-contracts.ps1`.
3. Run `npm pack --dry-run` and confirm `bin/`, `cli/`, and the complete `xiaotao/` tree are present.
4. Run a clean global-style smoke test with `npm exec --package . -- xiaotao --version` or an
   equivalent packed-package invocation.
5. Inspect `git diff --check`, status, and the complete diff before committing only confirmed paths.
