# Processing records and operations

[API reference](../api.md) · [Common conventions](common.md)

A **workset** is the library's shared current folder batch: a fixed, ordered
set of 1–500 selected Work Roots, including roots without audio. Each library
has at most one workset, enforced by a storage-level unique index.
`conversion`, `rename` and `metadata_refresh` reuse the same members and each
retain independent drafts, versions, planning sessions, current plans and
execution results. Decision: [ADR 0004](../adr/0004-task-seam-and-opaque-payloads.md).

**Status: implemented and machine-checked.** The Go handler, repository and
e2e tests are the reference for the shapes below. There is no revision-history
route: each operation keeps exactly one plan, the current one, and publishing a new
plan retires the one it replaced — with its payload, roots, units, execution
sessions and pending cover demands — in the same transaction. A failed generation
keeps the current plan, execution and pending cover demands. The pre-operation
workset routes (`/worksets/{id}/draft`, `/worksets/{id}/revisions`,
`/worksets/{id}/planning-sessions/*`) and the old record-creation route
(`POST /worksets` with `folder_ids`) were removed in the same delivery. A
database from another schema generation is refused at startup, never cleared;
only release databases upgrade, through migrations (ADR 0011).

## Creating the current record

`PUT /api/v1/libraries/{id}/operations/{type}/current` creates the library's
shared batch and seeds every registered Task's independent draft from its
common settings. The existing `{type}` entry point validates a registered
Task; it does not select a separate batch. GET through any registered Task
returns the same `workset_id` and member identities. It validates the requested scope against the **scanned
inventory** — the same listing the caller selected from — and reports every
directory that did not become a member instead of dropping it silently:

| `skipped[].reason` | Meaning |
| --- | --- |
| `missing` | the inventory does not know it as a directory of this root |
| `not_work_root` | a scanned directory is not marked as a work root |
| `recovery_dir` | `Delete/` and its descendants at any depth are not members |
| `invalid_path` | not a plain library-relative path |
| `duplicate` | selected more than once |

A selection whose every entry is unusable creates nothing and answers
`NO_MEMBERS` (400), with the skipped paths in `details`. Batch membership does
not depend on the entry-point Task or on audio count. More than 500
selected paths is `INVALID_FOLDER_COUNT` (400) — never a silent truncation.
`expected_current_id` is the record the caller saw as current (absent when it
saw none): a mismatch is `RECORD_REPLACED` (409) instead of an overwrite, and a
record with a queued or running session is `RECORD_BUSY` (409) instead of being
stranded. A replayed `Idempotency-Key` with the same request answers with the
record it already created; the same key with a different request is
`IDEMPOTENCY_KEY_REUSED` (409).

## Versions

Two independent counters, never interchangeable:

| Version | Advances on | Guarded by |
| --- | --- | --- |
| `workset.version` (metadata) | rename | `If-Match` on `PATCH /worksets/{id}` |
| `operation.version` | draft save, revision publication | `If-Match` on draft save, generation start |

A rename therefore never dirties an operation, and
another operation's change never advances this operation's version. There is no
separate draft counter: `GET .../draft` returns the operation version to echo
back as `If-Match`.

## Routes

All routes require auth and use the [standard error envelope](common.md#error-envelope).

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/api/v1/libraries/{id}/operations/{type}/current` | The library's shared current batch (the same for every registered type), or `{"workset": null}` with 200 when it has none. A library that does not exist is 404. |
| PUT | `/api/v1/libraries/{id}/operations/{type}/current` | Create or replace. Body `{"folder_paths":[...],"expected_current_id":"…","title":"…"}` (title optional; the library's name is the default), `Idempotency-Key` optional (`INVALID_IDEMPOTENCY_KEY` when malformed, and an absent key means the call is never a replay). 201 `{"workset":…,"created":true,"recorded":N,"skipped":[{"path","reason"}]}`; 200 for an idempotent replay. The new record and the removal of the one it replaces — members and every operation, draft, plan and session — are one transaction. |
| GET | `/api/v1/worksets` | Keyset list (`limit`, `cursor` → `next_cursor`, `library_id`, `status=active` excludes orphaned). |
| GET | `/api/v1/worksets/{id}` | Metadata view: `workset_id`, `title`, `version`, `library`, `members[]`, `operations[]`. Member coverage and planning state belong to an operation, never to the member or the workset. |
| PATCH | `/api/v1/worksets/{id}` | Rename only. `If-Match` = workset metadata version (`VERSION_REQUIRED` without it, `VERSION_CONFLICT` when stale, `ORPHANED_WORKSET` when the library is gone). |
| GET | `/api/v1/worksets/{id}/operations/{type}` | Operation view: `version`, `planning_state`, `current_revision`, `active_generation`, `latest_generation`, `active_execution`, `latest_execution`. Unknown workset, unsupported type and unestablished operation are all 404 (`UNKNOWN_OPERATION_TYPE` for a type this iteration does not implement). |
| GET | `/api/v1/worksets/{id}/operations/{type}/draft` | The operation version plus the sparse draft wrapped in its task envelope: `task: {kind, schema_version, payload}`. The payload is task-owned (conversion: the sparse document). |
| PUT | `/api/v1/worksets/{id}/operations/{type}/draft` | Full replacement of the sparse document, `If-Match` = operation version. Structural validation only: an incomplete draft saves. The common section is not written here: one that differs from the deployment-wide [Task settings](tasks.md#task-settings) is refused with `COMMON_SETTINGS_MISMATCH` (409), neither applied nor ignored (ADR 0008). `GENERATION_IN_PROGRESS` while this operation's session is queued/running; `EXECUTION_IN_PROGRESS` while its execution session is; `ORPHANED_WORKSET` when read-only. |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions` | Start generation. No request body; `If-Match` (operation version) and `Idempotency-Key` are both required. 202 `{"created":true,"generation":…}`; 202 `{"created":false,"generation":…}` for an effective key replay; 200 `{"created":false,"revision":…}` when no effective key matches and the current revision has never executed and nothing semantic changed. A new key after execution creates a fresh plan even with unchanged settings and folders. At most one queued/running generation exists per workset operation, including concurrent starts: the same effective key and request replay its session; another key gets `GENERATION_IN_PROGRESS` (409). Different worksets and operations may start independently. Conflicts: `IDEMPOTENCY_KEY_REUSED` for a different request under an effective key, `GENERATION_IN_PROGRESS`, `EXECUTION_IN_PROGRESS`, `SCAN_IN_PROGRESS`, `NO_ACTIVE_MEMBERS`, `VERSION_CONFLICT`; `INVALID_POLICY` is 400. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}` | Session detail. Statuses `queued`, `running`, `completed`, `failed`, `canceled`, `interrupted`. A session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/events` | SSE progress (`session_snapshot`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/cancel` | Cooperative cancel; idempotent on terminal sessions. |
| GET | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}` | The immutable snapshot of the record's **current** plan (a replaced plan no longer resolves — 404): `root_path`, `snapshot_token`, `status`, `summary`, the plan payload in its task envelope (`task: {kind, schema_version, payload}` — conversion: policy, classifier, summary, components), `counts`, frozen `members[]` (effective settings + per-unit `sources`), `roots[]`, `component_roots[]`, and `execution` (the session that ran this revision, if any). |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}/executions` | Execute the operation's current revision (see [Execution sessions](executions.md#execution-sessions)), optionally scoped to some of its folders. Body optional: `{"folder_paths":["albumA",…]}` — record-relative member paths; absent or empty runs the whole revision. The worklist and the session options (the obsolete-audio handling the draft declares) are always the frozen revision's. `If-Match` (operation version) and `Idempotency-Key` are both required. 202 `{"created":true,"execution":…}`; 200 with the same session for a key replay. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}` | Session detail, including each component's result. Optional `components_from` (default 0, inclusive) and `components_limit` (default: every remaining component) page the component list, which is ordered by `component_index`; a session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/events` | SSE execution stream (`execution_snapshot`, `component`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/cancel` | Cooperative cancel; idempotent on terminal sessions (200 with the session either way). |

Task payloads: [Audio conversion](conversion.md), [rename and metadata refresh](tasks.md). Frozen conversion revisions are described in [Frozen revisions](conversion.md#frozen-revisions).

Shutdown interrupts active planning sessions with status `interrupted`, without
setting a user cancellation flag or the `CANCELED` error code. Interrupted
planning releases its idempotency key so a later request can generate again.
A session claimed after worker admission closes is left for startup interruption.
Already committed revisions and independently failed sessions retain their outcome.

An effective planning key belongs to a `queued`, `running`, or `completed`
session. Key replay is checked before current-plan reuse, `If-Match`, and
active-session admission, including after publication advances the operation
version. `failed`, `canceled`, and `interrupted` rows release their keys. A
completed session and its key remain available for at least 30 days after
completion under automatic retention; background retention may remove them
later, and a manual [database cleanup](maintenance.md) removes finished planning
sessions and their keys immediately. Replacing its plan
does not release the key or restore the old plan: the replayed completed
session may have an empty `revision_id` because the old plan was deleted. A
client observing that result should refresh the current operation and report
that the original plan was replaced, without requesting the deleted revision.

Member `rel_path` and creation `folder_paths` may contain nested library-relative paths, for example `分类/RJ000001 album`. Classification directories and internal directories of a work are not members. Audio count never filters the shared batch; conversion plans the observed audio within each participating member.
