# Memory Git Semantic Merge and Provenance Contract Design

## Context

XiaoTao previously separated Temporary, Task, and Long-term Memory tiers and introduced reviewed
Memory Evolution proposals (`UPDATE`, `MERGE`, `CREATE`, `SKIP`) for single-branch consolidation.
However, in multi-developer and multi-branch team workflows, shared team memory (such as Long-term
Memory, Playbooks, and confirmed conventions) committed to Git will encounter merge conflicts when
concurrent branches diverge.

Standard Git text-based merge algorithms (like `recursive` or `ort`) only resolve mechanical text
insertions, frequently producing broken Markdown, lost knowledge, or unresolved conflict markers.
Naïvely picking `ours` or `theirs` deletes one developer's verified findings. Conversely, allowing an
AI model to silently arbitrate contradictory factual claims introduces hallucinations and risks
overriding verified regressions.

Issue #10 defines a **Memory Git Semantic Merge Protocol** and **Provenance Tracking Contract**
that enables safe multi-branch collaboration without central servers, external databases, or
uncontrolled automated writes.

## Git Boundary: Local Runtime State vs. Team Shared Memory

To prevent high-frequency noise from polluting Git branches and generating meaningless merge
conflicts, XiaoTao strictly delineates between local runtime state and team shared memory:

### Local Runtime State (Excluded from Git)
- `.xiaotao/memory/temporary/` (exploratory discussions, scratchpads, active Worker state)
- `.xiaotao/memory/pending/` (raw uncompressed/repaired memory inputs)
- `.xiaotao/locks/` and `.xiaotao/transactions/` (local concurrency controls)
- `.xiaotao/tasks/` (local active execution context, per-role runs, transient selections)

### Team Shared Memory (Tracked in Git)
- `.xiaotao/memory/long-term/` (`current.md`, `decisions/`, `candidates/`, `conflicts/`)
- `.xiaotao/playbooks/` (reusable team processes and check sequences)
- `.xiaotao/workers/registry.yaml` (reviewed, project-shared reusable Worker specs)
- `.xiaotao/config.yaml` (shared project settings)

## 3-Way Semantic Merge Protocol

A semantic merge operation requires 3-way input:
- `BASE`: The common ancestor version of the shared memory entries before divergence.
- `OURS`: The current branch version.
- `THEIRS`: The incoming branch version being merged.
- `file_path`: The project-relative path of the shared memory file (e.g. `.xiaotao/memory/long-term/current.md`).

Relying on `BASE` is essential to disambiguate whether a difference represents a new addition, a
refinement/update, a deletion, or a genuine contradiction between branches.

### Core Merge Rules

1. **Additive Non-conflicting Claims**:
   When both `OURS` and `THEIRS` introduce novel, non-overlapping claims against `BASE`, both
   entries are retained.

2. **Equivalent / Duplicate Claims**:
   When both branches independently discover or articulate the same underlying experience, the
   merge operation consolidates them into a single, comprehensive entry with unified wording while
   preserving all unique reachable `source_refs` from both sides.

3. **Contradictory / Mutually Exclusive Claims**:
   When branches arrive at conflicting conclusions (e.g. Branch A concludes module initialization
   can be asynchronous, while Branch B proves synchronous initialization is required to prevent
   auth failure), the AI **must not silently choose a winner**.
   The conflict must be flagged as `unresolved_conflicts` with `status: pending-confirmation` and
   `requires_human_review: true`.

4. **Tombstone & Anti-resurrection Invariant**:
   If an entry was marked `superseded` or `rejected` in a branch's verified decision history, merging
   an older branch that still contains the active entry **must not resurrect it as active**. The
   rejection or supersession tombstone takes precedence unless accompanied by explicit new
   contradicting evidence.

## Provenance Contract for Unresolved Conflicts

When an automated merge cannot safely resolve a divergence, both claims must be preserved alongside
complete provenance and evidence chains:

```yaml
conflict_id: cnf-20260828t120000z-a1b2
topic: Module A Initialization Strategy
status: pending-confirmation
reason: Conflicting runtime findings regarding async initialization safety
ours:
  author: developer-a
  branch: feature/perf-opt
  commit: 7a8b9c0
  task_id: task-20260828-perf
  memory_path: .xiaotao/memory/long-term/current.md
  claim: Module A can be lazily initialized after startup
  source_refs:
    - .xiaotao/references/scenarios/validator-fixtures/files/source.md
  created_at: 2026-08-28T10:00:00Z
theirs:
  author: developer-b
  branch: fix/login-race
  commit: 3d4e5f6
  task_id: task-20260828-login
  memory_path: .xiaotao/memory/long-term/current.md
  claim: Module A must initialize synchronously to avoid login auth race
  source_refs:
    - .xiaotao/references/scenarios/validator-fixtures/files/result.md
  created_at: 2026-08-28T11:30:00Z
```

### Conflict Lifecycle

1. `conflict detected`: Emerges during 3-way semantic comparison.
2. `pending-confirmation`: Emitted in the merge response and stored under
   `.xiaotao/memory/long-term/conflicts/` awaiting reviewer or evidence arbitration.
3. `resolved`: An explicit decision by Old Zhou, strong model, or human reviewer arbitrates the
   dispute.
4. `active` / `superseded` / `rejected`: The winning claim is integrated into `current.md`, while the
   superseded/rejected claim is archived in `decisions/` with `superseded_by` links preserving full
   historical context.

## Memory Merger Worker

A dedicated built-in worker `memory-merger` is defined with capabilities:
`["conflict-resolution", "memory-merge", "provenance-tracking"]`.

Responsibilities:
- Ingest `BASE`, `OURS`, and `THEIRS` memory entries.
- Perform safe deduplication, addition, and tombstone checks.
- Emit structured merged entries and unresolved conflicts with provenance.
- Set `requires_human_review: true` whenever any contradiction remains unresolved.
- Never make unauthorized architecture decisions or alter Task scopes.

## Protocol Schemas & Validation

Two new schemas govern this protocol:
1. `memory-merge-request.schema.json`
2. `memory-merge-response.schema.json`

The zero-dependency Python validator `xiaotao/scripts/validate.py` will support:
- `python xiaotao/scripts/validate.py memory-merge-request <file> --project-root <root>`
- `python xiaotao/scripts/validate.py memory-merge-response <file> --project-root <root>`

Validating shape, RFC 3339 timestamps, reachability of source references across all branches, unique
conflict IDs, and strict parity between `unresolved_conflicts` and `requires_human_review`.
