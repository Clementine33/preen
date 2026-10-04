# Rename and metadata refresh tasks

[API reference](../api.md) · [Common conventions](common.md)

## Rename task (schema version 1)

Use the operation routes with `{type}=rename`. The draft payload is:

```json
{
  "schema_version": 1,
  "common": { "template": "[{rj}] {title}", "strip_bracketed_title": false, "strip_tilde_title": false, "strip_book_title": false },
  "members": [{ "member_id": "…", "excluded": true }]
}
```

Only exclusions are stored per member; naming overrides are rejected. Members
without a record participate. Common template saves use the deployment-wide
settings rules. Exactly one `{rj}` is required; the other allowed placeholders
are `{title}`, `{circle}`, `{voice_actors}`, `{language}`, `{release_date}`.
Unknown placeholders, unmatched braces, duplicate/unknown members and overrides
are `INVALID_DRAFT` (400); unknown versions are `UNSUPPORTED_SCHEMA_VERSION`
(400). Metadata text stays literal. Optional `strip_bracketed_title`, `strip_tilde_title` and `strip_book_title`
(default false, omitted when false) filter only the `{title}` value. Brackets
match `【…】`; tildes match same-kind `~…~` or `～…～`; book-title brackets match `《…》`.
U+301C `〜` stays literal to avoid removing words. All complete
segments are removed, including delimiters, using the nearest terminator rather
than nesting: `【a【b】` is removed in full. Mixed/unclosed pairs remain. Brackets
run first, then tildes, then book-title brackets. Internal spaces stay unchanged (`Title 【特典】 Vol.2`
becomes `Title  Vol.2`). An empty used title is still `METADATA_MISSING`. Old
plans decode absent switches as off; schema version and old off-state hashes remain
compatible.

Each participating member produces one frozen component without modifying disk
or bindings. `task.payload.policy` holds the template and filters;
`task.payload.components[]` contains `member_id`, `source_name`, `work_code`,
`raw_name`, `target_name`, `action` (`rename`, `noop`, `blocked`) and optional
`reason_code`. The classifier has empty tags. Used metadata must be nonempty.
Illegal filename characters become `_`; outer whitespace and trailing dots/spaces
are removed. The result must still identify exactly the bound Work. Names over
255 UTF-8 bytes are blocked without truncation or automatic suffixes.

Block reasons: `METADATA_MISSING`, `WORK_UNBOUND`, `SOURCE_MISSING`,
`SOURCE_NOT_DIRECTORY`, `NAME_TOO_LONG`, `NAME_RESERVED`, `WORK_CODE_AMBIGUOUS`,
`TARGET_EXISTS`, `TARGET_COLLISION` (multiple proposals), `CASE_ONLY_RENAME`
(Windows roots). Planning uses the selected work paths directly and reads each
distinct parent once, without scanning classification directories or work
subtrees. Freshness compares the live naming proposal with the frozen proposal:
binding, template-used naming fields, source existence/name/type and target
occupancy. Directory mtime, media changes, sales, personal data, fetch timestamps
and unused fields do not stale plans. Existing plans use the same rules, ignoring
their old mtime-based fingerprints. Review always shows frozen names.

Execution uses the existing execution routes and per-component progress. Full
runs check the whole execution scope; selected runs check only selected roots;
empty runs access no directories. A blocked component anywhere still rejects
execution: exclude it and regenerate. Pending cover deliveries under actual
rename roots refuse admission with `COVER_DELIVERY_PENDING` (409); wait or cancel
those deliveries first. Noops and terminal failed deliveries do not block.

Same-library generation and queued/running rename exclude each other, atomically
with session creation. Member refresh refuses queued/running rename in its
library, atomically with scan registration. Rename enqueue also refuses any
registered scan under the process-wide scan count (`BUSY`), including member
refreshes performed by generations in other libraries. Other-library queued
generation alone does not block. Execution performs no subtree scan or postcheck.

Each successful rename relocates inventory paths, conversion credentials, copy
bindings and all same-library workset member paths in one transaction. IDs and
media facts remain unchanged; frozen plans, requests and cover targets remain
historical. Other affected plans become stale. Existing recovery folders keep
their old names. Results and interrupted repair are described in
[Operation execution](executions.md).

## Metadata refresh task (schema version 1)

Use the operation routes with `{type}=metadata_refresh`. Audio files are not
required. The draft payload defaults to:

```json
{
  "schema_version": 1,
  "common": { "mode": "fill_missing", "locale": "ja-jp", "download_cover": true },
  "members": []
}
```

Modes: `fill_missing`, `refresh_dynamic`, `refresh_all`; locales: `ja-jp`,
`zh-cn`, `zh-tw`. Only `member_id` / `excluded` are stored per member; unknown
fields and duplicate/unknown members are rejected. Dynamic refresh normalizes
`download_cover` to false. Common saves follow the deployment-wide rules.

Planning freezes selected directories, copy/work targets and settings without
upstream requests or inventory scans. It checks each selected directory directly,
without scanning its contents or the classification directories. Each participating
member produces a frozen component with `member_id`, `target` (`copy_id`, `rj`,
`library_root`, `rel_path`, `identity`), `action` (`refresh` or `skip`) and an
optional `reason_code`. An unbound directory skips with `NO_WORK_BINDING`;
a missing directory blocks with `SOURCE_MISSING`. Summary adds `work_count`
(distinct RJ); `operation_count` counts bound participating copies. New plans
write `identity:""`; the same lightweight rule applies to existing plans and
their legacy identity fields are ignored. Freshness checks only the directory's
existence, real-directory type and frozen basename, using one Lstat. Binding,
physical identity, mtime, metadata and media/cover changes do not stale the plan.
Same-name directory replacements are allowed; rebinding does not change the
frozen RJ or cover target. Execution folder scope applies before deduplication
and cover enqueueing. Unbound skips are not re-resolved during execution.

StartExecution and the worker check the same actual execution scope; full runs
check all units, selected runs check only selected units, and empty worklists
access no directories. Global blocking, version, admission and one-session
rules still apply. Missing/renamed/non-directory inputs yield `INPUT_CHANGED`
(an admission `PLAN_NOT_EXECUTABLE` detail, or worker session failure). Permission
and I/O failures retain diagnostics as `SCAN_FAILED`; cancellation takes
precedence as `CANCELED`. No retries or scan fallback are performed. The batch
does not revalidate directories/inventory after importing metadata.

After the batch has stopped enqueueing covers and its execution is terminal,
an independent background refresh waits for this execution's cover demands to
be terminal (`succeeded`, `skipped`, `failed`, or `canceled`). It then reconciles
only the actual executed folder scope, including frozen skips, using bounded
parallel folder walks (`scan.workers`, at most 32), one staging writer and one
selected-range merge. Unselected inventory and bindings are preserved. No covers
or disabled cover downloading does not prevent this refresh; empty executions
do not schedule it. It waits for file-operation admission, not unrelated shared
downloads. Scan failures are logged and do not change metadata or cover results;
there is no automatic scan retry. Pending completion refreshes are process-local
and do not resume after backend restart. Explicit cover retries do not schedule
another completion scan; they retain normal incremental inventory updates.

Execution deduplicates upstream metadata by RJ and retains every selected copy
as a cover target. Per-work source failures continue; database failures or
cancellation stop the batch and preserve committed results. Each component and
SSE `component` event may contain optional `task_result`: `{rj, outcome, issues,
main_url?, backup_url?}` for refresh, or the frozen skip payload. Issue fields
are `group?`, `source?`, `code`, `message`; outcome contains `applied`, `ignored`,
`warnings`, `created_placeholders`. Detail's optional `task_result[]` collects
results from the returned component page. A partial failure yields session
`failed` / `METADATA_PARTIAL_FAILURE` after other works finish.

Metadata results and cover demands are persisted before metadata execution
ends. Covers run asynchronously and do not alter that session's terminal status.
Stopping metadata execution leaves previously queued demands intact; use the
separate cover cancel route to stop them. Metadata execution follows the usual
startup interruption rule; pending covers resume automatically.

## Copy covers and asynchronous delivery

All routes below require the normal Bearer authentication.

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/api/v1/works/{rj}/copies/{copyId}/cover` | Read the copy's existing local main image. Validates copy ownership; never starts a download. Actual image MIME, digest ETag, `private, no-cache` and `nosniff`; conditional GET and ranges use standard HTTP semantics. 404 `COPY_NOT_FOUND` / `COVER_UNAVAILABLE`, 409 `COVER_CONFLICT` / `INPUT_CHANGED`. |
| GET | `/api/v1/worksets/{id}/operations/metadata_refresh/executions/{executionId}/covers` | `{items: [...], counts: {...}, downloads: {...}}`; execution ownership is checked, including operation kind. |
| POST | same path + `/retry` | Retry failed or stopped demands only when their candidate version is current. Returns the updated report. Does not restart metadata execution. |
| POST | same path + `/cancel` | Stop this execution's pending/running demands. Shared transfers needed by other executions continue. Returns the updated report. |

Each item contains `id`, `execution_id`, `target` (the frozen copy target),
`refresh`, `status`, optional `error_code`, `error_message`, `published`, `digest`.
Statuses/count keys: `pending`, `running`, `succeeded`, `skipped`, `failed`,
`canceled`. `downloads` counts distinct shared jobs: `total`, `queued`, `running`,
`done`; a done transfer may still have failed copy results. Inventory failure
reports `INVENTORY_REFRESH_FAILED` with `published` retained. Source supersession
reports canceled / `SOURCE_UPDATED`; stale-version demands cannot be revived.

Only main artwork is downloaded: DLsite `image_main.url`, then ASMR.one
`mainCoverUrl` on absence/download failure, independently of Core group priority.
Only HTTPS `img.dlsite.jp` is initially allowed, including every redirect and DNS
connection; unknown/nonpublic addresses yield `UNSUPPORTED_CDN`. No metadata
cookies or application token are sent. Per-request timeout 10 seconds, size cap
16 MiB, at most three attempts for network/5xx/429 with cancelable backoff.
Actual image signatures and decoding determine `webp`, `jpg`, `png`, `gif`;
HTML/damaged content is `INVALID_IMAGE`, excess bytes are `IMAGE_TOO_LARGE`.
Bytes are not transcoded. Refresh conditionals are sent only for a verified
matching local file; an unsolicited 304 is refused.

Two workers share temporary staged bytes, retaining them while targets remain.
Restart requeues interrupted transfers, reuses complete stages and cleans
unreferenced staging files; stopped demands remain stopped. Network transfers do
not hold directory admission. Publication waits for file-operation admission,
then checks the frozen directory exists with its frozen name and retains safe
path containment and symlink restrictions. It uses the frozen copy target without
comparing its current binding or physical identity and never creates a missing
directory. A moved/removed/non-directory path fails only its target; a same-name
directory replacement is accepted. The cover GET route still verifies current
copy ownership.

`fill_missing` skips exactly one valid supported `cover.*`; damaged files can be
replaced; multiple valid covers are `COVER_CONFLICT`. `refresh_all` publishes
`cover.{actual ext}` before removing old top-level supported `cover` names, never
other pictures. A durable publication journal completes extension cleanup on
retry after interruption. Normal publication checks write/Sync/Close, reuses the
downloaded bytes' digest and cleans old extensions without rereading the new
image. Recovery still confirms the journal digest before deleting old covers.
Partial cleanup reports the known published file and actual deletions. Observed
file facts and exact removed paths synchronize only those cover inventory rows,
preserving audio facts, bindings and unrelated rows without a member scan.
`INVENTORY_REFRESH_FAILED` retains `published` and does not roll back the file or
change metadata execution's terminal status. The journal remains until inventory
synchronization succeeds; retry can therefore remove stale inventory rows for
old files already deleted in a previous attempt.
No permanent central cover cache or scaled variants are created.

## Callable metadata refresh service

`metadata.Service.Refresh(ctx, Request)` accepts `WorkCodes`, `Mode`, `Locale`.
The entire request is validated before network activity; IDs are canonicalized,
deduplicated and sorted. Unknown works get per-work issues and are never fetched.
Defaults: `fill_missing`, `ja-jp`; alternatives: `refresh_dynamic`, `refresh_all`,
`zh-cn`, `zh-tw`. Results keep canonical ID order, with import outcomes and
source/group issues. One failed work does not abort other works.

DLsite static supplies core/editions; dynamic supplies sales/relations. Static
relations may fill unavailable dynamic reports. ASMR.one supplies only missing
core/relations/editions; sales always come from DLsite. Independently validated
complete groups go through the existing works import, preserving source priority,
timestamps, copies and personal data. Missing reports stay absent; explicit empty
edition/credit arrays remain empty. `Locale` / draft `locale` selects source tag
names only (UI: 标签语言); titles follow the Work's reported Chinese edition
language (`CHI_HANS` → `zh-cn`, `CHI_HANT` → `zh-tw`), and originals use `ja-jp`.
Chinese pages/translations take their circle from the Japanese original's
`ja-jp` snapshot. Accepted Core snapshots refresh source tag/circle display names
without changing entity IDs. Missing localized titles or original circles leave
Core unsubmitted and report issues; other complete groups still apply.

Refresh follows only reported parent/original references upward, importing
ancestor core/relations/editions into relation placeholders, without ancestor
sales or cover demands. Translation reports and edition lists are limited to
`CHI_HANS` and `CHI_HANT`; the original remains a separate root. Sibling IDs in
Chinese parent reports may create placeholders but are not fetched. Responses
are shared by Work/source/locale within one refresh. Upstream failures are
attached to the selected Work's issues with the upstream ID; progress/results
still describe only selected Works. No guessed parents or downward family crawl.

Static workers: 8; dynamic batches: 500, recursively split on HTTP 414. Shared
client limits: DLsite 8 in flight / 10 starts per second; ASMR.one 2 in flight /
3 starts per second; default timeout 10 seconds; response limit 16 MiB; at most
3 attempts for network/5xx errors. HTTP 403/429 or challenge headers stop further
source requests for at least 60 seconds, honoring longer `Retry-After`.
Cancellation stops new requests/imports. Neither scanning nor startup triggers
refresh automatically.

Shared [operation routes and versions](worksets.md) and [execution sessions](executions.md#execution-sessions) apply to these tasks. Metadata imports use the [work catalog contract](works.md#source-metadata-import).

Rename planning accepts nested scanned work roots and changes only their basename. Occupancy and batch conflicts are checked within each actual parent; equal targets in distinct parents are allowed. Plan freshness checks each participating parent. Rename execution remains unavailable.

Conversion planning handles at most four work roots concurrently per session and preserves request order. Scan, planning and encoding limits are independent. Metadata planning also uses the saved library root and accepts copies beneath different classification directories.
