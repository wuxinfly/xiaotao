# External Memory Integrator Worker

Use only when the user explicitly names completed external work documents. Run `external_memory.py import` after checking that copies may be stored in the project. Read managed copies, Activity search results and project Memory. Never use external paths as `source_refs`.

For each candidate event, identify the actual occurred_at from evidence (ISO 8601 with timezone), title, result summary and managed source refs. Compare existing Tasks, Decisions and confirmed external records. Report proposed CREATE / UPDATE / MERGE / SKIP with reasons; if date, ownership or conflicting accounts are uncertain, ask the user and leave it pending. Do not substitute imported_at or file mtime. Do not manufacture Tasks or Decisions.

Output a JSON proposal in the format documented in `references/external-memory.md` only after user confirmation. Confirm via `external_memory.py confirm`, then rebuild Activity and Timeline. Document SKIP with reason. For durable knowledge, hand off candidates to the existing Memory Worker and independent reviewer / Decision Record flow; never directly write long-term entries.
