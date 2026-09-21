# Active Temporary Memory Routing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Define deterministic, host-independent selection and routing rules for active Temporary Memory.

**Architecture:** Add the routing decision contract to coordination, keep loading and switching safety in the memory reference, and formalize minimal routing metadata in storage. Add behavioral scenarios as a reviewable regression baseline without introducing a Runtime or host-specific API.

**Tech Stack:** Markdown-based Codex Skill references and scenario fixtures.

---

### Task 1: Define the routing decision contract

**Files:**
- Modify: `xiaotao/references/coordination.md`

1. Document the precedence of explicit reference, Session binding, unique semantic match,
   confirmation, and new-topic handling.
2. Specify behavior for zero, one, and multiple active candidates.
3. Define concrete positive and insufficient routing evidence.
4. Require an explanation-capable unique match before automatic selection.

### Task 2: Define context and storage boundaries

**Files:**
- Modify: `xiaotao/references/memory.md`
- Modify: `xiaotao/references/storage.md`

1. Limit candidate discovery to routing metadata and current state.
2. Define Session binding creation, replacement, and invalidation.
3. Specify required and optional `meta.yaml` routing fields.
4. Preserve unrelated Temporary state during topic switches.

### Task 3: Add routing behavior scenarios

**Files:**
- Create: `xiaotao/references/scenarios/temporary-routing.md`

1. Add fixtures for all acceptance paths and forbidden behaviors.
2. Review every scenario against the normative references.
3. Run repository-wide consistency and formatting checks.
