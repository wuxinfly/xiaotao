# Playbook Candidate Contract Design

## Context

XiaoTao already separates Temporary, Task, and Long-term Memory and lets the Memory Worker propose
reviewed Long-term changes. That flow answers "what do we now know?" but does not preserve a
separate, reviewable answer to "how should a similar task be handled next time?"

Issue #15 adds that second path without adding a Worker role, workflow runtime, automatic learning
loop, or fixed role sequence.

## Approaches considered

### 1. Store reusable procedures as Long-term `experience` entries

This needs no new contract, but it mixes project knowledge with ordered operating guidance and
makes Playbooks impossible to maintain as their own reviewed collection.

### 2. Promote every successful Task directly into a Playbook

This creates visible guidance quickly, but a single success does not prove general applicability.
It would also let the Memory Worker modify project-authored guidance without user approval.

### 3. Emit reviewed Playbook Candidate proposals

The existing consolidation boundary emits a second candidate collection beside
`long_term_candidates`. Each candidate is compared with an index of current Playbooks and proposes
`UPDATE`, `MERGE`, `CREATE`, or `SKIP`. The proposal remains inert until a user explicitly approves
promotion. This is the selected approach because it reuses the current host-independent review
pipeline and keeps Playbooks optional guidance.

## Boundary

Long-term Memory stores durable declarative knowledge: verified facts, project-specific experience,
decisions, constraints, and principles. A Playbook Candidate stores procedural guidance only when
it has an explicit trigger, ordered reusable steps, and evidence from executed work.

Routine commands, temporary experiments, unverified suggestions, project-only paths, and a single
Task's chronology are low-value procedure material and must produce `SKIP`, not a new Playbook.
Temporary Memory may contribute source material, but a non-`SKIP` candidate requires evidence of
real execution. Discussion alone cannot establish a reusable method.

## Contract

Every Memory Worker request includes `current_playbooks`, including an empty array when none exist.
Each indexed Playbook exposes a stable ID, title, trigger, ordered steps, status, and reachable
source references. This is a compatibility change for request producers, but it is required to
enforce maintenance before proliferation.

Every `playbook_candidates` item includes a stable candidate ID, title, trigger, ordered steps,
checks, an action, a match classification and Playbook IDs, rationale, source metadata,
reachable `source_refs`, `evidence_refs`, and `status: candidate`.

The action rules mirror Memory Evolution:

- `CREATE` is valid only for `novel` with no target Playbooks.
- `UPDATE` targets one `overlap` or `conflict`.
- `MERGE` targets at least two `overlap` or `conflict` Playbooks.
- `SKIP` covers a `duplicate` with targets or `low-value` with none.

Candidates and current Playbook IDs must be unique. All source and evidence references must be
reachable project-relative files.

Each response audits the exact validated request artifact that produced it, but does not select the
trusted validation context. The caller supplies that immutable request independently; the protocol
guard confirms the response's audit path names the same file and rejects any candidate target absent
from its `current_playbooks`. Response shape validation alone cannot establish this cross-artifact
invariant. Evidence is conditional: non-`SKIP` actions require it, while a traceable `SKIP` may have
no execution evidence.

## Authority and lifecycle

The Memory Worker may extract and compare candidates, but cannot approve or write a Playbook.
Canonical candidate artifacts live under `.xiaotao/playbooks/candidates/`; immutable approval,
rejection, and supersession records live under `.xiaotao/playbooks/decisions/`. The first version
supports explicit user approval only. Repeated success can add evidence to a candidate but cannot
promote it automatically.

Approval applies the reviewed action through the existing mutable-state write protocol. Updating
or merging preserves prior evidence and decision history. Rejected candidates remain auditable so
the same weak procedure is not proposed repeatedly.

Formal Playbooks remain optional guidance. They may recommend checks, capabilities, or typical
sequencing, but cannot grant permissions, bypass explicit authorization, or require a fixed Role
order.

Managed Markdown and YAML Playbooks carry stable identity and revision metadata in front matter or
top-level fields. Existing unstructured files require one explicitly approved migration; their IDs
are persisted once and never regenerated. UPDATE changes one revision, while MERGE transactionally
updates a chosen survivor and marks the other target files superseded.

The current Playbook index may reference only canonical files under `.xiaotao/playbooks/`, excluding
candidate and decision records. Request validation reads each canonical file and confirms its stored
identity, path, revision, and status match the index so an arbitrary existing project file cannot be
treated as active guidance.

## Verification

JSON Schema and the zero-dependency Python validator enforce equivalent request and response
shapes. Fixtures cover a reusable `CREATE`, an existing-Playbook `UPDATE`, low-value `SKIP`, invalid
action cardinality, invalid automatic approval, duplicate IDs, and unreachable evidence. Repository
contract checks assert the Memory/Playbook boundary, user approval gate, update-first ordering, and
guidance-only rule.
