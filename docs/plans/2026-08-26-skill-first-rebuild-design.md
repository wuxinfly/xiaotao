# XiaoTao Skill-first Rebuild Design

## Decision

XiaoTao is delivered as one installable Codex Skill. The installed Skill is the product entrypoint;
project state lives in `.xiaotao/` inside the user's project. Users invoke XiaoTao through natural
language and never need npm, a Node.js CLI, or manually prepared model-response JSON.

## Package

The repository contains an installable `xiaotao/` directory. Its `SKILL.md` routes Codex to focused
references for coordination, memory, storage, handoffs, Playbooks, and individual roles. JSON
schemas document the internal Memory Worker and Handoff contracts. No executable helper is included
in v1 because Codex already has filesystem and sub-agent tools; deterministic scripts can be added
later only for a demonstrated repeated operation.

## Runtime interpretation

The initialization plan's “Runtime” is a responsibility boundary, not a separately installed
application. Codex performs mechanical storage operations with its host tools while following the
Skill's storage protocol: contained project paths, explicit confirmation before formal Task
creation, source-linked memory, safe writes, and review before long-term promotion. Old Zhou keeps
all business sequencing decisions; Playbooks remain optional guidance.

## Installation and use

Users copy `xiaotao/` into their Codex skills directory or upload the directory/ZIP as a Skill.
They then ask Codex to use XiaoTao, speak to Old Zhou, or invoke a named role. On first use in a
project, XiaoTao creates only the `.xiaotao/` directories required by the current request.

## Validation

The Codex skill validator must accept the package. All links from `SKILL.md` must resolve, YAML and
JSON files must parse, the package must contain no JavaScript or npm metadata, and a clean-room
inspection must show how a user installs and invokes it.
