# Codex Subagent Packet Injection Design

## Context

Official Codex Hooks expose a stable `SubagentStart` event with `session_id`, `agent_id`,
`agent_type`, `permission_mode`, and model-visible additional developer context. Codex does not
expose a stable correlation field chosen by `spawn_agent`, and Hook context injection does not
enforce a tool allowlist. The first integration therefore remains the documented single-Worker
acceptance mode and must report degraded isolation.

`PreCompact` is not used for automatic summarization. Its documented input contains the trigger
and common Session fields but no structured XiaoTao target or facts; the transcript format is
explicitly unstable. Existing canonical checkpoint requests already provide durable recovery and
remain visible through SessionStart.

## Selected design

Before spawning one bounded Worker, the main Agent materializes the canonical persistent
`delegation.json`, then creates `.xiaotao/runtime/codex/pending-delegation.json`. The envelope contains
only `schema_version`, the current Codex `session_id`, and the project-relative Delegation Packet
path. Only one pending envelope is allowed.

On `SubagentStart`, the plugin locates the same valid XiaoTao project, verifies the envelope Session
binding and path, then atomically renames it into a claimed directory keyed by a hash of the
Codex-provided agent ID. This one-time claim prevents replay and ensures concurrent starts cannot
consume the same packet. Invalid claimed data remains as local evidence and produces no model
context.

The Hook validates the Delegation Packet shape, requires `host_adapter.id: codex` and
`host_adapter.status: degraded`, with `tool-isolation` and `worker-snapshot-cross-check` explicitly
unsupported. It checks that the persistent path matches `worker_id`, and verifies
every resolved required instruction by recomputing its path-delimited SHA-256 digest. Core sources
must come from the project-managed XiaoTao Core; project sources resolve from the project root.
Context, result, Handoff, Worker snapshot, and instruction paths must remain regular files or safe
project-relative destinations. The first increment does not parse YAML Worker snapshots for full
tool/context/permission subset validation and therefore cannot claim supported isolation.

Successful output injects bounded objective, completion condition, exact resolved instruction
content, context references, tools, permission ceilings, degraded requirements, and output paths.
The text explicitly states that these are maximum requested bounds rather than host-enforced
isolation. The Hook never reads a transcript or automatically starts another Worker.

## Failure behavior

Missing pending state is a silent no-op. Session mismatch leaves the envelope untouched for the
correct Session. A malformed or unsafe envelope is not consumed. After a successful atomic claim,
Packet or digest failure produces no model context and leaves the claimed envelope as local evidence.
Hook failure never blocks subagent startup.

## Verification

Tests cover successful injection, digest verification, one-time consumption, Session mismatch,
concurrent claim behavior, path traversal, unsupported/supported status rejection, invalid Packet
shape, no transcript reads, installed-plugin execution, and installer packaging.
