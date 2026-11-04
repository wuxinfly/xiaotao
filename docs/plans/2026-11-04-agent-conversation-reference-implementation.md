# Agent 对话引用 Implementation Plan

> **For agent:** Use the project rules in the repository. Implement task by task, run focused tests after each task, and do not overwrite unrelated working-tree changes.

**Goal:** 在 Agent 输入框支持 `@` 引用直接对话，并让引用内容按冻结快照、字符上限和 token 预算进入模型上下文。

**Architecture:** 后端新增引用候选搜索、独立快照表和发送引用参数；消息只保存原始正文和引用 ID，读取历史时补全快照状态。前端用 Tiptap inline node 表示引用标签，发送时提取结构化引用；查询工具只读取当前消息直接引用的冻结范围。

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), React 19, TypeScript, Tiptap 3, Vite, Playwright browser tests.

---

## Task 1: 引用数据与快照

**Files:**
- Modify: `xt-client/internal/agent/types.go`
- Modify: `xt-client/internal/agent/store.go`
- Create: `xt-client/internal/agent/reference.go`
- Create: `xt-client/internal/agent/reference_test.go`

Steps:
1. Add serializable reference summaries, request items, and search candidates.
2. Create `conversation_references` and indexes with `CREATE TABLE IF NOT EXISTS`.
3. Read source text messages inside a consistent transaction and freeze the last position.
4. Filter only user/assistant text, enforce 5 sources, 64 KiB per source, 128 KiB total, and build a compact deterministic summary.
5. Persist snapshots and attach them to user messages; run `go test ./internal/agent -run Reference`.

## Task 2: 发送接口与模型上下文

**Files:**
- Modify: `xt-client/internal/agent/service.go`
- Modify: `xt-client/internal/app/agent_handlers.go`
- Modify: `xt-client/internal/app/agent_proxy_auth_test.go`
- Create: `xt-client/internal/agent/reference_model_test.go`

Steps:
1. Keep the legacy `Send` and `SendWithModel` signatures working.
2. Add a reference-aware send path and accept optional `references` in `/messages`.
3. Add reference summaries as a separate user-visible context block in model messages.
4. Keep `references` omitted compatible with existing callers and tests.
5. Add `read_conversation_reference` only when the current session has direct references; cap results and keep tool-call IDs paired.
6. Run focused Go tests and `go test ./...`.

## Task 3: Reference Search API

**Files:**
- Modify: `xt-client/internal/agent/reference.go`
- Modify: `xt-client/internal/app/agent_handlers.go`
- Create: `xt-client/internal/app/agent_reference_search_test.go`
- Modify: `xt-client/frontend/src/lib/api.ts`

Steps:
1. Add same-origin `GET /api/agent/references/sessions` with query, scope, project path, and exclusion.
2. Return only summary metadata, capped and sorted by update time.
3. Add the typed API client method.
4. Test current-project filtering, all-project search, case-insensitive title search, exclusion, and malformed scope.

## Task 4: Tiptap `@` Composer

**Files:**
- Create: `xt-client/frontend/src/features/agent/ConversationReference.ts`
- Modify: `xt-client/frontend/src/features/agent/AgentPage.tsx`
- Modify: `xt-client/frontend/src/features/agent/agent.css`
- Create: `xt-client/frontend/tests/agent-references.browser.test.mjs`

Steps:
1. Add an inline conversation-reference node and Markdown fallback.
2. Detect `@query` only in eligible text, ignoring code blocks and IME composition.
3. Search current project by default with an all-project option.
4. Support arrows, Enter, Escape, click selection, duplicate prevention, chip removal, and body-mounted positioning.
5. Send structured reference IDs while preserving the legacy content body.
6. Run frontend build and the focused browser test.

## Task 5: Reference Display and Source Status

**Files:**
- Modify: `xt-client/frontend/src/lib/api.ts`
- Modify: `xt-client/frontend/src/features/agent/AgentPage.tsx`
- Modify: `xt-client/frontend/src/features/agent/agent.css`

Steps:
1. Render reference chips in user messages without duplicating the inline `@title`.
2. Show source unavailable and new-content states from the history response.
3. Allow opening a still-available source and show a readable error when unavailable.
4. Keep old messages on their frozen snapshots after source changes.
5. Verify narrow layout, keyboard activation, and reduced motion.

## Task 6: Documentation and Verification

**Files:**
- Modify: `xt-client/README.md`
- Modify: `docs/architecture/xt-client-agent.md`
- Modify: `docs/plans/2026-11-04-agent-conversation-reference-design.md`

Steps:
1. Document the implemented API, snapshot limits, and user-visible behavior.
2. In `xt-client/frontend`, run `npm run build`.
3. In `xt-client`, run `go test ./...` and `go vet ./...`.
4. Run focused browser tests and `node tools/agent-rules.mjs check` from the repository root.
5. Do not manually edit generated web assets; run the project build flow if frontend assets need regeneration.
6. Record any unverified WebView2 behavior and reusable-rule candidates in the final summary.
