# Codex Worker Snapshot Cross-check Design

## Decision

The Codex `SubagentStart` hook will validate the immutable Worker snapshot before injecting a persistent Delegation Packet. The plugin will use a small, read-only, fail-closed YAML projection parser instead of adding a runtime dependency or introducing a second JSON snapshot.

The parser accepts only the YAML forms needed by Worker schema version 2: nested mappings, block sequences of scalar strings, and empty flow sequences (`[]`). It rejects duplicate keys, tabs, anchors, aliases, tags, merge keys, multiple documents, block scalars, non-string sequence items, and unknown structure inside the projected fields. The hook remains bounded by the existing 16 KiB snapshot limit and never executes project code.

## Validation boundary

A persistent Packet must declare `worker_snapshot_path`. The path must be the sibling immutable snapshot for the Packet worker under the same Task or Temporary scope. The snapshot must identify the same active Worker and use schema version 2.

The Packet is accepted only when:

- Packet tools are a subset of snapshot tools.
- Packet autonomous and conditional permissions are subsets of their corresponding snapshot limits.
- Every Packet context reference is equal to, or a descendant of, one of the snapshot read paths.
- Packet result and Handoff destinations are equal to, or descendants of, snapshot write paths.
- Packet required instruction refs exactly match snapshot required refs.
- Packet optional instruction refs are a subset of snapshot optional refs.

Path comparisons operate on normalized project-relative `/` paths and preserve segment boundaries. Existing realpath containment checks remain in force for files and output destinations.

## Adapter truthfulness and failure behavior

After this change, Codex has implemented the Worker snapshot cross-check, so the only required degradation is `tool-isolation`: injected tool and permission bounds still cannot be enforced by the Hook API. Packets that continue to claim `worker-snapshot-cross-check` as unsupported are rejected as stale contract data.

Any missing, malformed, mismatched, or out-of-bounds snapshot causes the already claimed Packet to be skipped without blocking native subagent startup. Tests cover positive Task and Temporary paths plus each subset boundary, malformed YAML, snapshot-path mismatch, and installer portability.
