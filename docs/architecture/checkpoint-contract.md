# Recoverable checkpoint: snapshot writer and DSH evidence

Status: **opt-in snapshot tool implemented; live model/backend acceptance remains open**.
Scope: second increment for [#26](https://github.com/IHongTaoI/xiaotao/issues/26),
within [#14](https://github.com/IHongTaoI/xiaotao/issues/14).
Updated 2026-09-10. Core owns target selection and saved facts; Adapter owns validation,
write-ahead records, locks, CAS and recovery. M1 does not depend on automatic triggers and adds no
Memory layer. The opt-in M2 pressure trigger reuses this writer; see
[automatic-checkpoint.md](automatic-checkpoint.md).
Checkpoint and recovery only support the current Worker-based protocol. Legacy Role directories,
`role:*`, `role_state_path` and old Role tasks are not discovered, migrated or restored.

## Verified host seams

Run `node adapters/deepseek-harness/scripts/audit-checkpoint.mjs` from the repository root.
It reports installed/locked versions, declaration locations and hashes, not live availability.
The audited packages are DSH 0.1.1-rc.2. Their package.json repository provenance identifies
deepseek-ai/deepseek-harness; this is versioned npm declaration evidence, not a claim about
an uninspected current upstream checkout.

| Seam | Evidence under @deepseek-ai package lib/types/ | Actual use |
| --- | --- | --- |
| Native tool | dsh-tools/index.d.ts: ToolRuntime.register(ToolDefinition); ToolRunContext.agent/signal; tools/pre-execute pipeline | Implemented in checkpoint-tool.ts and registered by index.ts only with explicit config, fs/tools and checkpoint schema. Real ToolRuntime tests verify registration, denial and dispatch with a fixture Agent. |
| Source log | dsh-session/index.d.ts: events snapshot, seq as next/exclusive event number | Not captured by this snapshot tool. No transcript watermarks claimed. |
| Flush | dsh-session/index.d.ts: SessionStore.flush returns participating-listener status | Not proof of backend storage independence or retention; not used as a substitute for publishing our request. |
| Physical persistence read | dsh-session-persistence/index.d.ts: readFrom(id, fromSeq, signal) returns stored prefix/suffix without synthetic closers | Verified interface, not connected here. Needed for a future session-events mode. May parse the entire artifact on sequential backends. |
| Logical inspection | Same file: inspect can return a live immutable view with open turns | Not used to claim durable recovery. load can repair cold history and rejects unsafe repair of live turns. |
| Lifecycle | dsh-agent/runtime-types.d.ts: request context + assistant usage + awaited turn-stopping serial | Opt-in pressure trigger is mounted only with agents/fs/tools. turn-stopping is explicitly reported as a fallback, not pre-compaction or Session End. |
| Worker / prompt | dsh-agent/index.d.ts factory; dsh-system-prompt/index.d.ts providers | Not activated by checkpoint. Current Agent supplies facts; fresh Worker context is not assumed. |
| State store / validator | Adapter storage.ts / validate.ts | Used by checkpoint only; other Core writes do not automatically route through these services. |

## Activation and trust boundary

The operator configures an absolute projectRoot and optional absolute recoveryRoot outside
the project. The tool takes neither root as model input. Canonical caller Session cwd must
equal the configured project root. Filesystem operations use the host fs service with that
explicit cwd, containment checks and cooperative cancellation; no Node filesystem escape
hatch or implicit sandbox escalation is added.

The tool goes through native DSH tool policy. Invocation is for a current explicit save/handoff
request; old checkpoints are data, not renewed authorization. A shared recovery directory
partitions records by canonical project identity hash and target, but its permissions/retention
are the operator's responsibility. A separate path can still share a disk/backend failure domain.

## Implemented operations and destinations

- inspect(kind, target_id): read existing content, Core revision and byte hash.
- save(..., request_id, base_revision, base_hash, snapshot): persist a bounded immutable
  source/proposal request, then attempt guarded commit.
- status(kind, target_id, request_id): inspect that exact request and its commit evidence.
  Read-only and queryable after the target is archived; it does not gate on an active lifecycle.
- retry(kind, target_id, request_id): revalidate and reconcile that exact persisted proposal;
  it does not regenerate a summary or invent newer coverage.

Temporary targets use memory/temporary/active/<id>/current.md; Task targets use
tasks/<id>/progress.md, both under .xiaotao/. Metadata must identify that same active target.
Worker targets, promoted Tasks and transaction overlays are unsupported. Until overlay
resolution lands, any nonempty .xiaotao/transactions directory blocks the operation
conservatively, including completed bundles. Do not delete evidence to bypass the check.

The seven snapshot fields are objective, confirmed, rejected, in_progress, next,
open_questions and source_refs. Facts are current-Agent supplied; required arrays can be empty.
Snapshot JSON is limited to 16 KiB, tool arguments to 32 KiB, proposed state to 128 KiB
(measured in UTF-8 bytes; a proposal that passes the schema character limit but exceeds the
byte limit reports `proposal_too_large`), and request reads to 384 KiB. No silent truncation. source_refs must resolve to existing
project-contained paths. The snapshot does not prove that every Session fact was supplied.

## Actual record layout and commit authority

The first design proposed separate request/source/proposal preparation files. Implementation
embeds those bytes in ONE exclusive JSON request, eliminating orphan preparation ordering:

```text
<selected-target>/references/checkpoints/
  <request-id>.json
  <request-id>.committed.json
  <request-id>.failed-<attempt-id>.json  # best-effort diagnostic evidence

<configured-recovery-root>/<project-key-hash>/<kind>/<target-id>/
  <request-id>.json                 # optional identical write-ahead copy
```

The request follows [checkpoint.schema.json](../../xiaotao/references/schemas/checkpoint.schema.json).
It contains project/target/Session binding, input hash, base Core revision/hash, bounded source
facts/hash and exact proposal/hash. Recovery rereads and validates the schema, hashes, binding,
revision and receipt. Hashes detect mismatch/corruption; they are not signatures or extra authority.

Write the optional secondary copy and verify it, then exclusively publish/verify the project
request. If a configured secondary cannot be written, fail before modifying current state.
A failed project publication can be retried from the already verified secondary request.
A request ID cannot name a different payload.

The one atomic current-state replacement is the commit point. It adds checkpoint_receipt
(request_id, source_hash, revision), increments Core revision once and updates timestamps/actor.
User text is preserved; only the single managed Saved checkpoint JSON section is replaced.
No Long-term entries, task lifecycle metadata or multi-file business state change in this operation.

Hold metadata and state locks in lexical path order, recheck active lifecycle and base revision/hash,
and use host FsVersion CAS. FsVersion and Core revision are distinct. Re-read the committed
bytes before publishing the immutable committed observation. New observations follow
[checkpoint-observation.schema.json](../../xiaotao/references/schemas/checkpoint-observation.schema.json)
and contain request hash, proposal hash, revision, `completion` (`save` or `recovery`) and the
first-publication `committed_at`; legacy four-field observations remain readable. Only the explicit
`retry` entry point writes `completion: recovery`; a repeated `save` remains `completion: save`.
Release all locks even when another release fails. Lock waiting is bounded/nonblocking;
expired held locks are never automatically stolen.

## Retry, concurrency and failure behavior

- Matching committed observation: return already_committed without changing current state.
- Exact proposal bytes already in current state: reconstruct missing observation, no second write.
- Exact recorded base revision AND bytes still present: retry guarded publication.
- Different target bytes/version: visible conflict. Never infer success from age or overwrite newer work.

Before a later checkpoint replaces an earlier receipt, verify or repair its observation.
An unrelated writer that changed the bytes and erased unconfirmed evidence can make the
previous outcome ambiguous; stop rather than infer success. Strong process termination may
leave held locks. Owner liveness must be established by the existing storage recovery procedure;
the tool does not claim autonomous dead-owner recovery.

Pending means an immutable request without verified commit. status is request-scoped; it is
not a global queue or automatic catalog. A newer successful request does not clear older pending
requests. On conflicts, preserve the old record and explicitly reconcile facts into a new request.
No silent supersession/deletion is implemented.

All write paths stop for cancellation, invalid schema, missing refs, lifecycle changes or CAS
conflicts. Failure reports recovery=none/project/secondary and has_recoverable_record states
whether a validated source/proposal request is durable enough to status/retry. The same request
ID is returned when valid, for status/retry. Status reports the proposed revision, not an
unconditionally committed revision.

If both primary and secondary request publication fail, do not promise recoverability.
If the original source refs disappear before an uncommitted retry, stop. A fully failed storage
backend may prevent reading even the target; a secondary record preserves input but does not
make that backend operational. No background retry or undeclared Session-persistence writes occur.

## Core compatibility and remaining acceptance

For failed save/retry operations with a verified source request, the tool attempts to publish
an immutable failure observation containing request/hash, reason code, time and recovery source.
If project writes fail it tries the configured secondary directory. failure_recorded reports
whether that diagnostic was confirmed. It does not replace the request or prove the summary
was committed. Cancellation stops new diagnostic writes; status and inspect never write them.

The [Memory](../../xiaotao/references/memory.md) and [storage](../../xiaotao/references/storage.md)
references describe optional checkpoint recovery. Read the selected target's managed snapshot
alongside its user-authored context; refresh the Memory catalog after successful formal writes.
Catalog failure does not roll back the committed checkpoint. Bare Core remains usable.

Activity may derive one `checkpoint_recovered` milestone from a valid new observation whose
completion is `recovery`. Its event time comes only from `committed_at`; save observations, legacy
observations and failure diagnostics are not projected. Activity is a deletable view and never
participates in checkpoint status, retry or commit decisions.

Automated tests cover receipt/no-op retries, changed-payload IDs, new input, lost acknowledgements,
observation repair, CAS races, source/lifecycle changes, invalid records, path escape,
primary failure with secondary recovery, total storage failure, cancellation and native DSH policy.
The filesystem and Agent used in those tests are fixtures; actual ToolRuntime is used.

Still open for the next acceptance stage: a configured DSH model/provider and persistent
filesystem, actual model-issued save, process exit, fresh-session request selection/status/retry,
and induced storage failure. Record destination, exact revision and recovery source. Do not
close #26 based solely on mocked storage, declaration scans or the Codex recovery reminder.
M2 pressure triggering is implemented behind opt-in configuration; real DSH acceptance and native
pre-compaction/Session End support remain open. M3 independent Worker/per-step injection remains separate.
Use the Issue #26 section in [manual-acceptance.md](../manual-acceptance.md) to record this evidence.
