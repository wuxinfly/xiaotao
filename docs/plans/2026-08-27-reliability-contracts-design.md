# XiaoTao Reliability Contracts Design

## Requirements

Issue #6 asks XiaoTao to become safer across Sessions, Tasks, role runs, and concurrent writes
without adding a standalone Runtime or turning dynamic delegation into a fixed workflow. The design
must prevent silent stale-state replacement, carry blocking questions in lightweight Handoffs,
separate exploration from execution, require fresh authority for high-risk actions, keep durable
memory subordinate to current evidence, and formalize only metadata needed for routing, recovery,
lifecycle, or conflict control.

## Architecture

Reliability remains a set of host-independent Skill contracts. JSON Schema validates lightweight
Handoffs and YAML metadata after parsing; Markdown references define decisions that cannot be
expressed as field validation; behavioral scenarios provide reviewable regression fixtures.

Mutable state uses one write protocol:

1. Read the state and retain its `revision`.
2. Acquire an exclusive lock for that logical state through the host's atomic create-if-absent
   filesystem operation.
3. Re-read under the lock and compare revisions.
4. On a mismatch, do not write. Reload, reconcile only non-conflicting changes with provenance, or
   surface a conflict for one owner to resolve.
5. On a match, write complete replacement content with `revision + 1`, `updated_at`, and
   `updated_by`, then release the lock.

The lock closes the race left by revision checking alone. A host that cannot guarantee exclusive
lock acquisition must serialize writes through one owner; it may not claim optimistic concurrency
safety. Locks are coordination artifacts, not business workflow. Abandoned locks may be reclaimed
only after their owner is known inactive and the recorded lease has expired; reclamation is itself
recorded.

Multi-file changes add a staged transaction overlay. The transaction stores immutable before
snapshots, complete staged replacements, hashes and revisions in its intent, and one applied event
per materialized file. Publishing `committed.yaml` is the only logical visibility switch: before
that marker, readers use the before state and ignore staged targets; after it, readers use the
staged state even while canonical files are still being materialized. An uncommitted transaction
may be discarded; a committed transaction must be finished. If a canonical file matches neither
its before nor staged hash, recovery reports a conflict instead of guessing.

## Key decisions and trade-offs

### ADR: exclusive lock plus optimistic revision

- **Decision:** combine per-state exclusive locks with revision comparison and atomic replacement.
- **Alternatives:** revision-only checking is vulnerable to two writers passing the same check;
  immutable versions plus a mutable current pointer still need coordination and add storage cost.
- **Consequences:** the contract is implementable with native filesystem primitives and detects
  stale writers, but hosts must expose atomic create-if-absent or serialize writes.

### Task promotion

Investigation, evidence collection, and reversible experiments stay Temporary. An explicit command
to implement or execute promotes the selected Temporary into a Task. The target remains
non-runnable while its transaction is preparing. The transaction atomically changes the logical
view from active Temporary to active Task when its commit marker is published; physical Task
materialization and Temporary archival may finish afterward from staged content. Ambiguous intent
remains Temporary and requires one concise confirmation.

### Authorization

Internal read-only work, non-destructive verification, XiaoTao state maintenance, and role
delegation are autonomous within approved scope. Deploying, publishing, releasing, externally
visible Git writes without prior authority, destructive or irreversible operations, access-control
changes, secret operations, and material scope expansion require explicit authorization close to
the action. A role recommendation never supplies that authority.

### Evidence and durable memory

Evidence precedence is current code/runtime evidence, current Task verified findings, Long-term
Memory, then historical References. Contradictions produce a sourced supersession decision: the old
entry remains auditable but is marked non-current and linked to its replacement or rejection.

## Failure handling

- Lock contention causes bounded retry or a visible conflict, never a forced overwrite.
- Revision mismatch preserves both sources and delegates reconciliation to one writer.
- Invalid Handoffs are rejected before persistence; a blocking Handoff without exact questions is
  invalid.
- Promotion failure before commit leaves the Temporary active and does not expose the staged Task.
  Failure after commit preserves the staged Task as authoritative and resumes materialization; it
  never makes both source Temporary and target Task logically active.
- Missing authority pauses only the risky action; safe analysis and preparation may continue.
- Contradictory Long-term Memory is not deleted or trusted while its review is pending.

## Verification

Schema checks cover valid and invalid Handoffs plus Temporary and Task examples. Behavioral
scenarios cover exploration, promotion, direct role invocation, Session Handoff, authorization,
stale revisions, and Long-term supersession. Repository checks also verify that normative terms and
schema fields remain aligned. The same checks run locally and in a lightweight pull-request and
master-branch GitHub Actions workflow.
