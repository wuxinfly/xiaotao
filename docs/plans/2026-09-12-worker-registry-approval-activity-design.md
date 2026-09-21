# Worker Registry Approval Activity Design

## Decision

Worker registry approval is represented by one immutable JSON record per approval under
`.xiaotao/workers/approvals/<approval-id>.approval.json`. The mutable
`.xiaotao/workers/registry.yaml` remains the authority for which reusable Workers are currently
available; an approval record is the authority for when a particular registry revision first made
one Worker available. Activity reads these records but never creates them and therefore does not
change the existing approval boundary.

Embedding `approved_at` in each registry Worker was rejected because later registry edits can
rewrite it. An append-only aggregate log was rejected because it would become a second event log
and would make partial corruption and concurrent publication harder to isolate. Individual records
fit the existing immutable Decision Record model and give each approval a stable source path.

## Record contract

Schema version 1 requires `approval_id`, `worker_id`, `registry_id`, `registry_revision`,
`worker_digest`, `approved_at`, and `approved_by`. IDs use the existing kebab-case convention.
`worker_digest` is the lowercase SHA-256 digest of the canonical JSON encoding of the approved
Worker specification (UTF-8, keys sorted, compact separators). `worker_name` is copied into the
record for durable presentation. Together they bind and describe the historical approval without
requiring the old registry revision to remain available.

The filename must equal `<approval_id>.approval.json`. Approval files are immutable after
publication. A writer validates the proposed record against the registry snapshot before an atomic
create; Activity later validates the self-contained record rather than the mutable current registry.
Replacement of an existing approval file is forbidden. This issue defines the record and validation
protocol but does not grant any caller permission to publish it.

Historical registries without records remain valid and produce no Worker approval events. A record
with invalid fields or filename mismatch is corrupt authority and makes Activity
rebuilding fail visibly. Later registry edits or Worker removal do not invalidate historical facts.

## Activity projection

Every valid approval record derives one `worker_approved` event. `occurred_at` comes only from
`approved_at`; neither registry `updated_at`, filesystem mtime, nor Git history is consulted. The
event ID is generated from event type, approval ID, and normalized approval time using the existing
deterministic helper. Its source reference is the approval record itself. Registry edits cannot
duplicate the event because the immutable approval ID remains stable; a rebuild produces the same
ID.

Activity source fingerprinting includes the self-contained approval files, but not the mutable
registry. Approval source changes invalidate and rebuild the derived index. The existing time-window
and event-type filters work unchanged after `worker_approved` is added to the schema and CLI choice
set.

## Verification

Schema fixtures cover a valid record, malformed fields, invalid timezone forms, and whitespace-only
display fields. Native-validator tests ensure JSON Schema and handwritten validation agree. Activity
tests cover successful projection, deterministic rebuild, missing historical records, invalid
filenames, and invalid records. Protocol documentation specifies publication and compatibility rules.
