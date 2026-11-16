# Processing records and operations

[API reference](../api.md) · [JSON schemas and endpoint/event bindings](schema/contracts.json) · [Common conventions](common.md)

A **workset** is the library's shared current folder batch: a fixed, ordered
set of 1–10,000 selected Work Roots, including roots without audio. Each library has at most one workset, enforced by a storage-level unique index.
`conversion`, `rename` and `metadata_refresh` reuse the same members and each
retain independent drafts, versions, planning sessions, current plans and
execution results. Decision: [ADR 0004](../adr/0004-task-seam-and-opaque-payloads.md).

There is no revision-history route: each operation exposes exactly one current
plan, and publishing a new plan makes the one it replaced — with its payload,
roots, units and execution sessions — inaccessible and cancels unfinished cover
demands in the same transaction. A failed generation keeps the current plan,
execution and pending cover demands. A database from another schema generation
is refused at startup, never cleared; only release databases upgrade, through
migrations (ADR 0011).

## Creating the current record

`PUT /api/v1/libraries/{id}/operations/{type}/current` creates the library's
shared batch and seeds every registered Task's independent draft from its
common settings. `{type}` validates a registered Task without scoping the batch;
`GET` through any registered Task returns the same `workset_id` and member
identities.

`selection` ([recordCreateRequest](schema/recordCreateRequest.json)) addresses
the `dir_id`s listed by `GET /api/v1/libraries/{id}/dirs` so body size does not
grow with path length, and its two forms are mutually exclusive: either
`all: true` (with optional `except_dir_ids`; unknown ids are ignored) or
`dir_ids`. Combining `dir_ids` with `all`, or `except_dir_ids` without `all`, is
`INVALID_SELECTION` (400). Members are ordered by inventory path regardless of
request order. `recorded` reports the member count actually written (which may
differ from what the caller saw when inventory changed concurrently), and every
unusable `dir_id` is reported in `skipped[]` with its
[reason](schema/recordCreateResponse.json) (`ambiguous` means two directories
claim the id per [ADR 0003](../adr/0003-short-routes-and-directory-identity.md)).

A selection whose every entry is unusable creates nothing and answers
`NO_MEMBERS` (400), with the skipped ids in `details`. Batch membership does
not depend on the entry-point Task or on audio count. More than 10,000
resolved members, or more than 10,000 listed `dir_ids`, is
`INVALID_FOLDER_COUNT` (400) — never a silent truncation.
The current limit is advertised as `workset_max_members` by
[`GET /api/v1/health`](common.md#health-endpoint). The overview shows selected
count / limit and disables task entry above that limit, with an explanation.
While the limit is unavailable, entry remains enabled and the server's count
validation remains authoritative.
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

| Method | Path | Schema | Behavior |
| --- | --- | --- | --- |
| GET | `/api/v1/libraries/{id}/operations/{type}/current` | [currentRecordResponse](schema/currentRecordResponse.json) | The library's shared current batch (`workset: null` when none). A library that does not exist is 404. |
| PUT | `/api/v1/libraries/{id}/operations/{type}/current` | Req: [recordCreateRequest](schema/recordCreateRequest.json) · Res: [recordCreateResponse](schema/recordCreateResponse.json) | Create or replace atomically (`title` defaults to library name). `Idempotency-Key` optional (`INVALID_IDEMPOTENCY_KEY` when malformed; absent means never a replay): 201 when created, 200 on replay. Atomic replacement retires the old record, members, operations, drafts, plans and sessions, cancels unfinished cover demands, and removes stored payloads in background batches. |
| GET | `/api/v1/worksets` | [worksetListResponse](schema/worksetListResponse.json) | Keyset list (`limit`, `cursor` → `next_cursor`, `library_id`, `status=active` excludes orphaned). |
| GET | `/api/v1/worksets/{id}` | [worksetResponse](schema/worksetResponse.json) | Metadata and members only; no operations. Member coverage and planning state belong to an operation, never to the member or workset. |
| PATCH | `/api/v1/worksets/{id}` | Req: [worksetPatchRequest](schema/worksetPatchRequest.json) · Res: [worksetResponse](schema/worksetResponse.json) | Rename only. `If-Match` = workset metadata version (`VERSION_REQUIRED` without it, `VERSION_CONFLICT` when stale, `ORPHANED_WORKSET` when the library is gone). |
| GET | `/api/v1/worksets/{id}/operations/{type}` | [operationResponse](schema/operationResponse.json) | Operation view including `max_participants` (omitted when the task declares none — only `conversion` does, 3000). Unknown workset, unsupported type and unestablished operation are all 404 (`UNKNOWN_OPERATION_TYPE` for an unknown `{type}`). |
| GET | `/api/v1/worksets/{id}/operations/{type}/draft` | [draftResponse](schema/draftResponse.json) | Operation version (`If-Match` token) plus the task-owned sparse draft envelope. |
| PUT | `/api/v1/worksets/{id}/operations/{type}/draft` | Req: [draftDocumentRequest](schema/draftDocumentRequest.json) · Res: [operationResponse](schema/operationResponse.json) | Full replacement of the sparse document, `If-Match` = operation version. Structural validation only: an incomplete draft saves. The common section is not written here: one that differs from the deployment-wide [Task settings](tasks.md#task-settings) is refused with `COMMON_SETTINGS_MISMATCH` (409), neither applied nor ignored (ADR 0008). `GENERATION_IN_PROGRESS` while this operation's session is queued/running; `EXECUTION_IN_PROGRESS` while its execution session is; `ORPHANED_WORKSET` when read-only. |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions` | Req: [emptyRequest](schema/emptyRequest.json) · Res: 202 [generationStartResponse](schema/generationStartResponse.json) / 200 [generationReplayRevisionResponse](schema/generationReplayRevisionResponse.json) | Start generation. `If-Match` (operation version) and `Idempotency-Key` are both required. 202 (`created: true`, or `false` on effective key replay); 200 when no effective key matches and the unexecuted current revision had no semantic changes. A new key after execution creates a fresh plan even with unchanged settings and folders. At most one queued/running generation exists per workset operation: matching effective key + request replays its session; another key gets `GENERATION_IN_PROGRESS` (409). Different worksets and operations start independently. Conflicts: `IDEMPOTENCY_KEY_REUSED`, `GENERATION_IN_PROGRESS`, `EXECUTION_IN_PROGRESS`, `SCAN_IN_PROGRESS`, `NO_ACTIVE_MEMBERS`, `VERSION_CONFLICT`; `INVALID_POLICY` is 400. `TOO_MANY_PARTICIPANTS` is 400 (`details`: `limit=N`, `participants=M`; excluding members lets generation proceed). |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}` | [generationViewResponse](schema/generationViewResponse.json) | Session detail. A session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/events` | `sse.generation` in [contracts.json](schema/contracts.json) | SSE progress (`session_snapshot`, `progress`, terminal event). `session_snapshot` matches the session detail schema. |
| POST | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/cancel` | [generationViewResponse](schema/generationViewResponse.json) | Cooperative cancel; idempotent on terminal sessions. |
| GET | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}` | [revisionResponse](schema/revisionResponse.json) | Lightweight detail of the **current** plan (replaced plans return 404 `REVISION_NOT_FOUND`; no `roots`, `component_roots`, member settings, sources, or `components`). `category_counts` omits uncategorized members (`{}` when none). Optional `?category=X&limit=N` (both required together, `limit` a positive integer; else 400 `INVALID_ARGUMENT`) narrows `members[]` to the first `N` entries in member order and adds `category_total`. Frozen policy and step summary are always present; unreviewable plans (`task_schema_version` unsupported, step count != 1, or missing policy/summary) fail with 400 `UNSUPPORTED_SCHEMA_VERSION` (when refused by rename/metadata) or 500 `INTERNAL`, never returning `null`. Cache by `plan_id` with finite freshness: input validation and execution change in place. |
| GET | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}/members/{memberId}` | [memberReviewResponse](schema/memberReviewResponse.json) | One member's Plan Review with frozen `effective`, `sources`, `conclusion`, `roots[]` and `components[]`. Server resolves component ownership (no `component_roots`). Excluded and no-input members return 200 with empty components. 404 `MEMBER_NOT_FOUND` (outside record) or `REVISION_NOT_FOUND` (replaced plan; refresh operation and re-read). Same unreviewable-plan failures and finite-freshness cache rules apply. |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}/executions` | Req: [executionStartRequest](schema/executionStartRequest.json) · Res: [executionStartResponse](schema/executionStartResponse.json) | Execute the operation's current revision ([Execution sessions](executions.md#execution-sessions)). Optional body scopes to `member_ids` or `all_except_member_ids` (absent/empty runs all); worklist and options come from the frozen revision. `If-Match` (operation version) and `Idempotency-Key` are both required. 202 when created, 200 on key replay. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}` | [executionViewResponse](schema/executionViewResponse.json) | Session detail; optional `view=members` or `member_id` (see [executions](executions.md)). A session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/events` | `sse.execution` in [contracts.json](schema/contracts.json) | SSE execution stream (`execution_snapshot`, `component`/`member`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/cancel` | [executionViewResponse](schema/executionViewResponse.json) | Cooperative cancel; idempotent on terminal sessions (200 either way). |

Each revision member's `conclusion` ([revisionMemberResponse](schema/revisionMemberResponse.json))
evaluates `input` (`none`: no frozen planning input; `missing`: frozen root
missing; `unavailable`: record has no library; `stale`: live inventory differs
from frozen input; `valid`: matches) in that strict precedence order (`none`,
`missing`, `unavailable`, `stale`, `valid`). Exclusion and missing input take
precedence over task status in the UI. Input validity is evaluated on every
read; clients use a finite cache lifetime (the web client uses 60 seconds), even
when `plan_id` stays the same. Excluded members and members without planning
input omit the task-owned payload.

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

Member `rel_path` may contain nested library-relative paths, for example `分类/RJ000001 album`. Classification directories and internal directories of a work are not members. Audio count never filters the shared batch; conversion plans the observed audio within each participating member.
