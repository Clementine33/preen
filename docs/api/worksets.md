# Processing records and operations

[API reference](../api.md) · [Common conventions](common.md)

A **record** is the current processing record of one `(library, operation)`
pair: the fixed, ordered set of 1–500 member folders a user selected, plus the
operation that works on them. `conversion` and `rename` are registered operation types. `rename` supports
draft, generation and review; its execution is refused. At most one
record exists per pair — that is a storage-level unique index, not an
application check. Decisions: [ADR 0001](../adr/0001-library-workbench-and-processing-record.md).

**Status: implemented and machine-checked.** The Go handler, repository and
e2e tests are the reference for the shapes below. There is no revision-history
route: a record keeps exactly one plan, the current one, and publishing a new
plan retires the one it replaced — with its payload, roots, units and
execution sessions — in the same transaction. The pre-operation workset routes
(`/worksets/{id}/draft`, `/worksets/{id}/revisions`,
`/worksets/{id}/planning-sessions/*`) and the old record-creation route
(`POST /worksets` with `folder_ids`) were removed in the same delivery. An
incompatible database is refused at startup, never migrated or cleared
(ADR 0008 §3).

## Creating the current record

`PUT /api/v1/libraries/{id}/operations/{type}/current` is the only way a record
comes into existence. It validates the requested scope against the **scanned
inventory** — the same listing the caller selected from — and reports every
directory that did not become a member instead of dropping it silently:

| `skipped[].reason` | Meaning |
| --- | --- |
| `no_audio` | the directory holds no audio anywhere beneath it; conversion skips it |
| `missing` | the inventory does not know it as a directory of this root |
| `not_direct_child` | only direct children of the library root are members |
| `recovery_dir` | the library-level `Delete/` is not a member |
| `invalid_path` | not a plain library-relative path |
| `duplicate` | selected more than once |

A selection whose every entry is unusable creates nothing and answers
`NO_AUDIO_MEMBERS` (400) for conversion, or `NO_MEMBERS` (400) for rename,
with the skipped paths in `details`. Rename accepts scanned direct-child
directories even when their audio count is zero. More than 500
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
| GET | `/api/v1/libraries/{id}/operations/{type}/current` | The library's current record for one operation, or `{"workset": null}` with 200 when it has none. A library that does not exist is 404. |
| PUT | `/api/v1/libraries/{id}/operations/{type}/current` | Create or replace. Body `{"folder_paths":[...],"expected_current_id":"…","title":"…"}` (title optional; the library's name is the default), `Idempotency-Key` optional (`INVALID_IDEMPOTENCY_KEY` when malformed, and an absent key means the call is never a replay). 201 `{"workset":…,"created":true,"recorded":N,"skipped":[{"path","reason"}]}`; 200 for an idempotent replay. The new record and the removal of the one it replaces — members, operation, draft, plan and sessions — are one transaction. |
| GET | `/api/v1/worksets` | Keyset list (`limit`, `cursor` → `next_cursor`, `library_id`, `status=active` excludes orphaned). |
| GET | `/api/v1/worksets/{id}` | Metadata view: `workset_id`, `title`, `version`, `library`, `members[]`, `operations[]`. Member coverage and planning state belong to an operation, never to the member or the workset. |
| PATCH | `/api/v1/worksets/{id}` | Rename only. `If-Match` = workset metadata version (`VERSION_REQUIRED` without it, `VERSION_CONFLICT` when stale, `ORPHANED_WORKSET` when the library is gone). |
| GET | `/api/v1/worksets/{id}/operations/{type}` | Operation view: `version`, `planning_state`, `current_revision`, `active_generation`, `latest_generation`, `active_execution`, `latest_execution`. Unknown workset, unsupported type and unestablished operation are all 404 (`UNKNOWN_OPERATION_TYPE` for a type this iteration does not implement). |
| GET | `/api/v1/worksets/{id}/operations/{type}/draft` | The operation version plus the sparse draft wrapped in its task envelope: `task: {kind, schema_version, payload}`. The payload is task-owned (conversion: the sparse document). `common_settings_current` is true only when this draft's common settings match a stored deployment-wide value; it is false before the first global save. |
| PUT | `/api/v1/worksets/{id}/operations/{type}/draft` | Full replacement of the sparse document, `If-Match` = operation version. Structural validation only: an incomplete draft saves. A save whose common settings differ from the ones this operation holds is deployment-wide: the stored common settings and every other editable record's common fields move in the same transaction, each with its draft hash and operation version, while member overrides, exclusions and generated plans stay exactly as they were (ADR 0010 §2). `GENERATION_IN_PROGRESS` while this operation's session is queued/running; `EXECUTION_IN_PROGRESS` while its execution session is; `COMMON_SETTINGS_BUSY` (409) when another record of the same kind holds one — the whole save is refused then, never a part of it; `ORPHANED_WORKSET` when read-only. |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions` | Start generation. No request body; `If-Match` (operation version) and `Idempotency-Key` are both required. 202 `{"created":true,"generation":…}`; 200 with `revision` when the current revision has never executed and nothing semantic changed, or 202 with `generation` for a key replay. A new key after execution creates a fresh plan even with unchanged settings and folders. Conflicts: `GENERATION_IN_PROGRESS`, `EXECUTION_IN_PROGRESS`, `SCAN_IN_PROGRESS`, `NO_ACTIVE_MEMBERS`, `VERSION_CONFLICT`; `INVALID_POLICY` is 400. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}` | Session detail. Statuses `queued`, `running`, `completed`, `failed`, `canceled`, `interrupted`. A session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/events` | SSE progress (`session_snapshot`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/cancel` | Cooperative cancel; idempotent on terminal sessions. |
| GET | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}` | The immutable snapshot of the record's **current** plan (a replaced plan no longer resolves — 404): `root_path`, `snapshot_token`, `status`, `summary`, the plan payload in its task envelope (`task: {kind, schema_version, payload}` — conversion: policy, classifier, summary, components), `counts`, frozen `members[]` (effective settings + per-unit `sources`), `roots[]`, `component_roots[]`, and `execution` (the session that ran this revision, if any). |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}/executions` | Execute the operation's current revision (see [Execution sessions](executions.md#execution-sessions)), optionally scoped to some of its folders. Body optional: `{"folder_paths":["albumA",…]}` — record-relative member paths; absent or empty runs the whole revision. The worklist and the session options (the obsolete-audio handling the draft declares) are always the frozen revision's. `If-Match` (operation version) and `Idempotency-Key` are both required. 202 `{"created":true,"execution":…}`; 200 with the same session for a key replay. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}` | Session detail, including each component's result. Optional `components_from` (default 0, inclusive) and `components_limit` (default: every remaining component) page the component list, which is ordered by `component_index`; a session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/events` | SSE execution stream (`execution_snapshot`, `component`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/cancel` | Cooperative cancel; idempotent on terminal sessions (200 with the session either way). |

Task payloads: [Audio conversion](conversion.md), [rename and metadata refresh](tasks.md). Frozen conversion revisions are described in [Frozen revisions](conversion.md#frozen-revisions).
