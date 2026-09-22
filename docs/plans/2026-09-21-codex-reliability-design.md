# Codex Adapter Reliability Design

## Context

The Codex Adapter currently launches a project- or user-provided `memory_catalog.py` during
`SessionStart`. A globally trusted Hook must not automatically execute code supplied by the opened
repository. The same path also combines a five-second child-process timeout with a five-second Hook
timeout and performs directory scans without explicit entry or byte budgets.

Worker Handoff discovery is bounded only after choosing the first three active Task IDs in lexical
order. A newer Handoff on a later Task can therefore be omitted. Handoff shape validation is also
embedded inside the session launcher, making the recovery flow harder to test independently.

## Selected design

Keep the Adapter read-only and implement the startup overview in Node.js. The Hook reads only small,
known metadata files under the selected project root and never imports or launches project code.
Directory scans use explicit entry, file-size, and aggregate-byte budgets. When a budget is exceeded
or metadata cannot be safely interpreted, the Hook emits a bounded degraded warning and retains any
safe recovery information already found.

The overview is intentionally smaller than the Core Memory Catalog. It extracts active Task,
Temporary, pending follow-up, Long-term count, and recoverable checkpoint metadata. It does not
create or refresh `index.json` or `manifest.md`; full catalog building remains an explicit Core
operation.

Move Handoff structural validation into a small Adapter contract module. Handoff discovery scans a
bounded set of active Tasks, ranks valid Handoffs globally by modification time, and only then keeps
the newest three. Result and Worker State references must resolve to regular project-contained files.

## Error handling and compatibility

The Hook remains fail-open for Codex: malformed state never blocks session startup. User-visible
context reports stable, non-sensitive degradation codes rather than raw exceptions or file contents.
The plugin command timeout remains five seconds, but all local work is synchronous only at the
filesystem API level and has much smaller bounded inputs; no nested five-second subprocess exists.

Existing project paths and recovery guidance remain compatible. The behavioral change is deliberate:
`SessionStart` no longer mutates derived Memory Catalog files.

## Verification

Adapter tests cover non-execution of a planted project script, no catalog writes, bounded scans,
global newest-Handoff selection, contract validation, malformed input, installer packaging, and the
existing recovery scenarios. The complete repository tests, contract suite, package dry run, and
diff checks remain required.
