# Capability-based Workers Design

## Context

Issue #7 asks XiaoTao to preserve a small set of stable organizational roles while selecting or
creating execution workers from the capabilities required by the current work. The mechanism must
remain Skill-first and host-independent, preserve direct role calls, and respect the reliability
and authorization boundaries established by issue #6.

The first release implements the complete routing loop without automatic learning: describe
requirements, resolve a reusable worker, compose a small compatible set when necessary, or create a
bounded worker aligned with the existing Task, Temporary, or one-off Session. Promotion of repeated
temporary workers into reusable workers remains a reviewed future action.

## Decisions

### Roles and workers are separate layers

A role describes organizational responsibility such as architecture, implementation, testing, or
delivery. A worker is an execution specification for a bounded capability set. Built-in roles remain
directly invocable and are also represented as reusable built-in workers so capability routing can
select them. A generated worker does not create a new installed Skill or permanent role.

### Canonical capability identifiers

Capability identifiers use lowercase kebab-case. The registry is the authority for known aliases;
the resolver must not persist model confidence, embeddings, or invented synonym mappings. Required
and optional capabilities are recorded separately so matching remains explainable.

### Resolver behavior

The resolver filters out disabled, unavailable, out-of-scope, or over-permissioned workers before
matching. It then chooses in this order:

1. One worker covering every required and optional capability (`exact`).
2. One worker covering every required capability (`compatible`).
3. The smallest compatible set whose union covers every required capability (`composed`).
4. One generated Task-, Temporary-, or Session-scoped worker when no safe match exists
   (`generated`).

Within a class, fewer workers, fewer unrelated capabilities, and lexical worker ID order provide a
deterministic tie-break. The resolver records its rationale; it is guidance to Old Zhou rather than
a mandatory workflow state.

### Registry and snapshots

XiaoTao ships an immutable built-in registry. Projects may add a small mutable registry under
`.xiaotao/workers/registry.yaml`; it uses the revision, lock, and atomic replacement protocol from
issue #6. Every persisted selected worker is copied as an immutable Task or Temporary snapshot
before execution. Resumption uses the snapshot, not the current registry, so later registry edits
cannot change in-flight or historical behavior.

Persisted workers live under the matching Task or Temporary state. Their specification states
responsibility, capabilities, inputs, outputs, tools, context paths, requested permissions, and
lifecycle. Task workers expire with the Task, Temporary workers expire when their Temporary leaves
the active lifecycle, and one-off Session workers are not persisted. None enters the reusable
registry automatically.

### Authorization

Worker permissions are requested action categories, not grants. Selection intersects the worker's
requests with the Task scope, available host tools, and current user authorization. High-risk
actions still require action-, target-, and scope-specific authorization immediately before use.
A registry entry, generated specification, resolver result, role recommendation, or prior approval
cannot expand authority.

### Handoffs and failure handling

Role Handoffs remain valid. Dynamic workers use `worker_state_path` and may recommend either a named
role or a set of capabilities for the next delegation. Invalid registry entries are ignored and
reported rather than repaired silently. Ambiguous canonicalization, unavailable tools, permission
excess, or incompatible composition produces a visible no-match result; generation is permitted
only when a bounded, valid specification can be produced.

## Verification

JSON Schemas validate capability requirements, worker specifications, registries, resolver
selections, and both role and worker Handoffs. Fixtures cover exact reuse, composition, generated
Task, Temporary, and Session scope, invalid lifetime, permission bypass, duplicate registry IDs,
and incompatible Handoff state paths. Behavioral scenarios prove direct role compatibility,
deterministic matching, snapshot stability, and non-promotion of temporary workers.

## Non-goals

- Automatic learning, scoring, ranking, reinforcement learning, or Worker marketplaces.
- Installing generated Skills or expanding the stable role catalog.
- A background service, mandatory Runtime, embeddings, or host-specific routing API.
- Automatic promotion of temporary workers into the reusable registry.
