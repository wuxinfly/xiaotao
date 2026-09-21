# Memory Awareness Layer Design

## Context

XiaoTao already preserves Temporary, Task, and reviewed Long-term Memory, but a new Session has no
small overview of what exists. Reading every Memory file or all historical References wastes
context and makes unrelated experience influence the current task.

Issue #27 requires awareness and progressive retrieval without introducing a background Runtime,
vector database, new approval path, or second source of truth.

## Selected design

Formal Memory remains authoritative. A deterministic `memory_catalog.py` helper derives two local
cache files:

- `.xiaotao/memory/manifest.md`: a short human- and Agent-readable map loaded at Session start;
- `.xiaotao/memory/index.json`: validated records used for bounded retrieval.

The builder covers structured Long-term entries, active Temporary routing context, active Task
objectives, and current role or Worker state. It intentionally excludes historical Reference
trees. A SHA-256 digest over the contributing source paths and bytes detects stale catalogs.

Long-term entries remain in `long-term/current.md`, but each current claim uses a fenced
`xiaotao-memory-entry` JSON block. This gives every entry a deterministic boundary and stable ID.
The helper's `show` command extracts only one selected block, so the Agent does not need to place
the whole Long-term file in model context.

## Retrieval

`search` uses current request text, optional Skill or Worker context, the current binding, layer and
Memory-kind filters. It returns at most five active candidates with a plain relevance reason.
Keyword, tag, alias, summary, current-state, and binding matches are supported in Phase 1. No match
is a valid result. Inactive, superseded, rejected, archived, or disputed entries are not recalled by
default.

After the Agent judges one candidate relevant, `show <memory-id>` returns only that record's detail.
The catalog never approves a proposal, resolves a conflict, or changes formal Memory.

## Lifecycle and failure handling

The catalog is checked at the start of a substantial Session and rebuilt after formal Memory or
current-state lifecycle changes. Writes use atomic replacement. Catalog files are excluded from
Git because they combine shared Long-term Memory with local Temporary and Task state.

If the catalog is missing, invalid, or stale, `search` rebuilds it by default. A build failure is
visible and falls back to existing bounded source reads; it cannot roll back or block an already
committed formal Memory change.

## Deferred work

Semantic embeddings, a vector database, incremental indexing, learned ranking, and large-scale
retrieval evaluation remain Phase 2 work. They are not prerequisites for the deterministic MVP.
