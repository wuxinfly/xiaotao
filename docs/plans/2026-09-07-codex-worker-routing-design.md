# Codex Worker Routing Design

## Context

XiaoTao Core models a Worker as a bounded unit executed through the host's native subagent
mechanism. The Codex recovery plugin currently reminds the model about delegation boundaries but
does not distinguish a spawned subagent from a separate user-owned Codex task or conversation.
Observed runs can therefore try `create_thread` before discovering the native subagent capability.

Codex documentation describes subagents as delegated agent threads that handle specific work and
return a summary to the main thread. Current Codex releases may delegate after a direct user request
or applicable project or Skill instructions. The mapping is host-specific, so it belongs in the
Codex Adapter rather than the portable Core.

## Selected design

Extend the existing bounded `SessionStart.additionalContext` reminder with one short Codex mapping
rule. A bounded XiaoTao Worker uses an available Codex-native subagent capability. A separate
user-owned Codex task or conversation is created only when the user explicitly requests one. The
text names `spawn_agent` only as a current example and must not claim that a particular API is
always present.

Dynamic Worker naming has two layers. Tool-facing identifiers must satisfy the visible tool schema;
for a schema limited to lowercase letters, numbers, and underscores, use a short `snake_case` value.
User-facing delegation, progress, and summary text should use a concise Chinese role label when the
host supports it. The Adapter does not promise a Chinese UI thread name when the host exposes no
separate display-name field.

## Scope and verification

This behavior is guaranteed only when the XiaoTao Codex Adapter is installed, enabled, trusted, and
its `SessionStart` Hook runs for a valid XiaoTao project. It does not modify Core or other adapters.
Unit tests assert the semantic prompt contract without pretending to execute model tool selection.
The Codex guide records three manual acceptance paths: Worker delegation, explicit separate-task
creation, and graceful behavior when subagent capability is unavailable.

Official reference: https://learn.chatgpt.com/zh-Hans/docs/agent-configuration/subagents
