# Memory Evolution Contract Design

## Context

XiaoTao already separates Temporary, Task, and Long-term Memory, invokes a Memory Worker at
durable boundaries, requires reachable source references, and gates Long-term promotion on review.
The missing piece is a machine-checkable decision between extraction and review. A candidate can
currently say only what it contains, not whether it should update, merge with, create, or skip a
Long-term entry.

Issue #9 asks for that decision layer without changing the three visible memory tiers, introducing
a Memory Runtime, or turning Temporary Memory into permanent logs.

## Approaches considered

### 1. Add a separate evolution response artifact

The Memory Worker would first emit candidates and a second pass would emit actions. This makes the
stages explicit, but doubles persistence and failure handling while leaving the two artifacts open
to drift.

### 2. Replace candidates with opaque executable operations

The response would contain write operations against Long-term files. This is compact, but gives the
Memory Worker too much authority and couples the protocol to a storage layout.

### 3. Enrich each candidate with a reviewed action proposal

Keep `long_term_candidates`, but require every candidate to describe its classification against
current Long-term Memory and propose `UPDATE`, `MERGE`, `CREATE`, or `SKIP`. The proposal remains
non-authoritative until reviewed. This is the selected design because it preserves the current
pipeline, stays host-independent, and makes unsafe or contradictory proposals detectable before a
write.

## Contract

Each Long-term candidate receives a stable `candidate_id`, a `memory_kind`, an `action`, a `match`
record, a `conflict_status`, a rationale, and minimal structured source metadata in addition to the
existing title, content, and reachable `source_refs`.

`match.classification` records why the proposal exists:

- `novel`: no existing entry covers the durable claim;
- `duplicate`: an existing entry already covers it;
- `overlap`: one or more existing entries should absorb or consolidate it;
- `conflict`: new evidence contradicts an existing entry;
- `low-value`: the material is temporary, one-off, or insufficiently reusable.

Actions and matches obey these invariants:

- `CREATE` is valid only for `novel` and has no target entries.
- `UPDATE` targets exactly one entry and is valid for `overlap` or `conflict`.
- `MERGE` targets at least two entries and is valid for `overlap` or `conflict`.
- `SKIP` is valid only for `duplicate` or `low-value`. Duplicate proposals name at least one
  matching entry; low-value proposals name none.
- `conflict_status` is `none` for non-conflicts. Conflicts are `pending-confirmation` or
  `confirmed`; neither status grants write authority.

The source object records `type`, `id`, `created_at`, and an optional `workspace_id`. Reachable
`source_refs` remain authoritative evidence; the source object is routing and audit metadata rather
than a substitute for evidence.

## Data flow and authority

At an existing Memory Worker boundary, the caller supplies current Long-term Memory in
`current_memory`. The worker extracts durable claims, compares them with entries by stable ID, and
emits one classified proposal per candidate. Exact duplicates and low-value material are emitted as
`SKIP` rather than silently disappearing so repeated weak claims can be audited.

The protocol validator checks shape, action/match cardinality, conflict consistency, reachable
references, and uniqueness of candidate IDs. It does not execute an action. Old Zhou or a
strong-model reviewer verifies the evidence, may change or reject the proposal, and publishes an
immutable decision. Approved state changes then use the existing revision, lock, and transaction
protocol. A known conflict can never silently overwrite current truth.

## Error handling

Invalid candidate proposals follow the existing one-repair-attempt rule. A second failure is kept
as `.invalid.json` under pending memory and cannot be treated as a Memory Worker response. Missing
targets, inconsistent conflict status, duplicate candidate IDs, or unreachable source references
all fail validation. Business-task completion remains independent: failed memory consolidation is
recorded as `memory_pending`, and the Task may still archive.

## Verification

Schema fixtures cover all four actions and every match classification. Invalid fixtures cover
action/target mismatch, conflict-status mismatch, and duplicate candidate IDs. The zero-dependency
Python validator must agree with the JSON Schema for shape rules and add the cross-item uniqueness
check. Repository contract checks also assert the documented update-first ordering, reviewer gate,
and provenance rule.
