# Worker Registry Approval Activity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add immutable Worker registry approval records and derive reliable `worker_approved` Activity events from them.

**Architecture:** Keep the Worker registry as mutable current state and add one immutable, self-contained approval record per approved Worker revision. Validate it against the registry at publication time; later Activity rebuilds validate and project the record without depending on mutable current state.

**Tech Stack:** JSON Schema 2020-12, Python standard library validator/catalog, Node.js test runner.

---

### Task 1: Define the approval record contract

**Files:**
- Create: `xiaotao/references/schemas/worker-approval.schema.json`
- Create: `xiaotao/references/scenarios/schema-fixtures/worker-approval-valid.json`
- Create: `xiaotao/references/scenarios/schema-fixtures/worker-approval-invalid.json`
- Modify: `xiaotao/scripts/validate.py`
- Modify: `scripts/verify-contracts.ps1`

1. Add failing schema and native-validator fixture tests.
2. Run the targeted schema tests and confirm the new kind is unsupported.
3. Add the strict version-1 schema and equivalent native validation.
4. Register `worker-approval` in the validator CLI.
5. Re-run the targeted tests and confirm they pass.

### Task 2: Derive Worker approval Activity events

**Files:**
- Modify: `xiaotao/references/schemas/activity-event.schema.json`
- Modify: `xiaotao/scripts/activity_catalog.py`
- Modify: `test/activity-catalog.test.js`
- Create: `xiaotao/references/scenarios/validator-fixtures/activity-event-worker-approved-valid.json`

1. Add failing tests for valid projection, stable rebuild, and historical registries without approvals.
2. Add failing tests for malformed records and filename/ID duplication.
3. Add approval source discovery and include records plus registry in the source fingerprint.
4. Validate each immutable record while keeping historical projection independent of later registry edits.
5. Derive and validate `worker_approved` events and add the event type to schema/CLI filters.
6. Run Activity and validator tests until all cases pass.

### Task 3: Document the publication and compatibility protocol

**Files:**
- Modify: `xiaotao/references/workers.md`
- Modify: `xiaotao/references/activity.md`
- Modify: `xiaotao/references/storage.md`

1. Document the immutable approval path and canonical Worker digest.
2. State that approval publication preserves existing authorization and uses atomic create.
3. Document failure behavior and the no-guessing rule for legacy registries.
4. Verify documentation contract tests.

### Task 4: Verify the complete change

**Files:**
- Modify only if verification exposes an issue.

1. Run `npm test`.
2. Run `npm run test:contracts`.
3. Run `npm run verify` and inspect the package contents.
4. Review the final diff for generated files, accidental scope expansion, and compatibility gaps.
