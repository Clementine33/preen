# Rename and metadata refresh tasks

[API reference](../api.md) · [Common conventions](common.md)

## Task settings

The common settings of a Task kind are deployment-wide (ADR 0008): one set per
kind, independent of any record, and this endpoint is the only way to read or
write it. `{type}` is `conversion`, `rename` or `metadata_refresh`; any other
value is 404 `UNKNOWN_OPERATION_TYPE`.

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/api/v1/tasks/{type}/settings` | `{"operation_type","settings"}` with an `ETag` header. `settings` is the stored set, or the Task's seed value while none was saved; it works in a library with no record. |
| PUT | `/api/v1/tasks/{type}/settings` | Body `{"settings":{…}}`, `If-Match` = the `ETag` last read (`VERSION_REQUIRED` 400 without it, `VERSION_CONFLICT` 409 when the set changed meanwhile). 200 with the new set and `ETag`. |

`settings` is the task's common section, the same object its draft carries:
conversion `{other_tags, mode, delete_mode, no_effect_tags, matched, unmatched,
other}`; rename `{template, strip_bracketed_title, strip_tilde_title,
strip_book_title, voice_actor_separator}`; metadata refresh `{mode, locale,
download_cover}`. The `ETag` is the canonical-JSON hash of that object (a quoted
string), so equal settings have equal tags. An unreadable or invalid payload is
400 (`INVALID_SETTINGS`, or the task's own `INVALID_DRAFT`).

A save is all-or-nothing and runs in one transaction: it stores the set and
rewrites the common section of every editable record of the kind, each with its
draft hash and operation version, while member overrides, exclusions and
generated plans stay as they were (a rewritten record reads `needs_planning`).
Orphaned records never take part. `COMMON_SETTINGS_BUSY` (409) names a record
whose planning session or execution is queued/running; `COMMON_SETTINGS_UNREADABLE`
(409) names one whose stored draft cannot be read. Either refuses the whole
save. A record created afterwards starts from the stored set.

The draft PUT (`PUT /api/v1/worksets/{id}/operations/{type}/draft`) never
changes these settings: a common section that differs from the stored set is
refused with 409 `COMMON_SETTINGS_MISMATCH`. While no set was ever saved, the
reference is the record's own stored common section, so member-only edits keep
working.

## Rename task (schema version 1)

Use the operation routes with `{type}=rename`. The draft payload is:

```json
{
  "schema_version": 1,
  "common": { "template": "[{rj}] {title}", "strip_bracketed_title": false, "strip_tilde_title": false, "strip_book_title": false, "voice_actor_separator": "、" },
  "members": [{ "member_id": "…", "excluded": true }, { "member_id": "…", "name": "[RJ01234567] my name" }]
}
```

A member record holds an exclusion, a literal `name`, or both; a template per
member is rejected. A `name` replaces the template for that folder (it is
trimmed, sanitized and length/reserved checked like a template result, and the
title filters do not apply). It must contain exactly one work code, otherwise
the folder is blocked with `NAME_CODE_INVALID`. A code different from the bound
work's re-binds the folder's copy to that work when it is executed
(`rebind_to`). Members without a record participate. The template and filters are
the deployment-wide [Task settings](#task-settings). Exactly one `{rj}` is required; the other allowed placeholders
are `{title}`, `{circle}`, `{voice_actors}`, `{language}`, `{release_date}`.
`voice_actor_separator` (optional, omitted or empty means `、`) joins the actors
of `{voice_actors}` (sorted, deduplicated); it is at most 16 characters with no
control characters and none of `<>:"/\|?*`, else `INVALID_DRAFT`. No other
placeholder has more than one value, so none has a separator.
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
the member review's `task.payload.components[]` contains `member_id`, `source_name`, `work_code`,
`raw_name`, `target_name`, `action` (`rename`, `noop`, `blocked`) and optional
`reason_code`, `custom_name`, `observed_name` and `rebind_to`. The classifier has empty tags. In revision details, each
`members[].task` is that member's single frozen component, unchanged
(including every optional field). Excluded members and members without planning
input omit `task`; the workset-owned `member_id`, `excluded`, `input` and
`has_components` remain directly on each member conclusion. The rename plan table and its tab counts
read these member conclusions. Used metadata must be nonempty.
Illegal filename characters become `_`; outer whitespace and trailing dots/spaces
are removed. The result must still identify exactly the bound Work. Names over
255 UTF-8 bytes are blocked without truncation or automatic suffixes.

A folder that was renamed by hand is followed from the parent listing planning
already reads: when the recorded name is gone and the same parent holds exactly
one other directory with the same single work code that no other member
records, the component carries it as `observed_name` and is planned from it
(`noop` when it already has the target name). Execution renames from
`observed_name`, then in the same transaction moves the records from the
recorded path to the final name; a folder renamed by hand and then back to its
recorded name moves nothing but still applies `rebind_to`. If that sync fails
the disk is renamed back to `observed_name` and the records stay at the
recorded path. A `noop` follow only moves the records to `observed_name`. None
or several candidates leave the folder `SOURCE_MISSING`.

Block reasons: `METADATA_MISSING`, `WORK_UNBOUND`, `SOURCE_MISSING`,
`SOURCE_NOT_DIRECTORY`, `NAME_TOO_LONG`, `NAME_RESERVED`, `NAME_CODE_INVALID`, `WORK_CODE_AMBIGUOUS`,
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
empty runs access no directories. A blocked component never rejects
execution: it is skipped (the component result has `renamed:false` and
`skipped:<reason_code>`) and the other components run. To leave a folder out on
purpose, remove it from the batch. Pending cover deliveries under actual
rename roots refuse admission with `COVER_DELIVERY_PENDING` (409); wait or cancel
those deliveries first. Noops and terminal failed deliveries do not block.

Same-library generation and queued/running rename exclude each other, atomically
with session creation. Member refresh refuses queued/running rename in its
library, atomically with scan registration. Rename enqueue also refuses any
registered scan under the process-wide scan count (`BUSY`), including member
refreshes performed by generations in other libraries. Other-library queued
generation alone does not block. Execution performs no subtree scan or postcheck.

Each successful rename relocates inventory paths, conversion credentials, copy
bindings and all same-library workset member paths in one transaction, and
re-binds the copy when the component has `rebind_to`. IDs and
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
  "common": { "mode": "fill_missing", "locale": "ja-jp", "download_cover": false },
  "members": []
}
```

Modes: `fill_missing`, `refresh_dynamic`, `refresh_all`; locales: `ja-jp`,
`zh-cn`, `zh-tw`. Only `member_id` / `excluded` are stored per member; unknown
fields and duplicate/unknown members are rejected. Dynamic refresh normalizes
`download_cover` to false. The common section is written through the deployment-wide [Task settings](#task-settings).

Planning freezes selected directories, copy/work targets and settings without
upstream requests or inventory scans. It checks each selected directory directly,
without scanning its contents or the classification directories. Each participating
member produces a frozen component with `member_id`, `target` (`copy_id`, `rj`,
`library_root`, `rel_path`, `identity`), `action` (`refresh`, `cover` or `skip`) and an
optional `reason_code`. In revision details, each
`members[].task` is that member's single frozen component with all
fields preserved, including the entire `target` and any optional `reason_code`.
Excluded members and members without planning input omit `task`. Plan counts
come from these conclusions: `refresh` runs; `cover` and `skip` count as skipped
metadata fetches. An unbound directory skips with `NO_WORK_BINDING`; in
`fill_missing` mode a work whose four data groups are all stored (in any tag
language) plans as `skip` with `ALREADY_COMPLETE`, or as `cover` when
`download_cover` is on: no fetch, only cover delivery;
a missing directory blocks with `SOURCE_MISSING`. Summary adds `work_count`
(distinct RJ); `operation_count` counts bound participating copies that are fetched. New plans
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
be terminal (`succeeded`, `skipped`, `failed`, or `canceled`). It then collects
only target directories whose delivery reports `INVENTORY_REFRESH_FAILED`, for
both newly published and reused existing covers, and deduplicates them into one
scan. The scan uses bounded parallel folder walks (the built-in default of 32),
one staging writer and one selected-range merge. Inventory and bindings outside
that repair scope are preserved. An empty scope requests no file-operation
admission and creates no scan session. Batches without cover demands register no
follow-up. Download failures, publication failures and cancellations alone do not
trigger scans. A repair waits for file-operation admission, not unrelated cover
downloads. Scan failures are logged and do not change metadata or cover results;
there is no automatic scan retry. Pending repairs are process-local and do not
resume after backend restart. Actually published files can be discovered by a
later Library Scan because publication changes directory mtime, subject to
attribute-cache delay. Reusing an existing cover does not change directory mtime;
an unrepaired inventory failure on that path needs a successful cover retry,
member refresh or Full Rescan. Explicit cover retries do not schedule another
completion scan; they retain normal exact inventory updates.

Execution deduplicates upstream metadata by RJ and retains every selected copy
as a cover target. Per-work source failures continue; database failures or
cancellation stop the batch and preserve committed results. Each component and
SSE `component` event may contain optional `task_result`: `{rj, outcome, issues,
main_url?, backup_url?}` for refresh, or the frozen skip payload. Issue fields
are `group?`, `source?`, `code`, `message`; outcome contains `applied`, `ignored`,
`warnings`. A partial failure yields session
`failed` / `METADATA_PARTIAL_FAILURE` after other works finish.

Metadata results and cover demands are persisted before metadata execution
ends. Covers run asynchronously and do not alter that session's terminal status.
Stopping metadata execution leaves previously queued demands intact; use the
separate cover cancel route to stop them. Metadata execution follows the usual
startup interruption rule; pending covers resume automatically.
Successful replacement of its plan or deletion of its processing record removes
that execution's pending demands. Failed planning preserves them. Shared
downloads continue while another demand for the same work needs them;
published cover files remain.

## Copy covers and asynchronous delivery

All routes below require the normal Bearer authentication.

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/api/v1/works/{rj}/copies/{copyId}/cover` | Read the copy's existing local main image. Validates copy ownership; never starts a download. Actual image MIME, digest ETag, `private, no-cache` and `nosniff`; conditional GET and ranges use standard HTTP semantics. 404 `COPY_NOT_FOUND` / `COVER_UNAVAILABLE`, 409 `COVER_CONFLICT` / `INPUT_CHANGED`. |
| GET | `/api/v1/worksets/{id}/operations/metadata_refresh/executions/{executionId}/covers` | `{items: [...], counts: {...}}`; execution ownership is checked, including operation kind. |
| POST | same path + `/retry` | Requeue every failed or canceled demand of the execution. Returns the updated report. Does not restart metadata execution. |
| POST | same path + `/cancel` | Stop this execution's pending/running demands. A transfer still needed by another execution's demand for the same work continues. Returns the updated report. |

Each item contains `id`, `execution_id`, `target` (the frozen copy target),
`refresh`, `status`, optional `error_code`, `error_message`, `published`, `digest`.
Statuses/count keys: `pending`, `running`, `succeeded`, `skipped`, `failed`,
`canceled`. `running` marks a demand a worker has claimed; a restart returns it
to `pending`. Inventory failure reports `INVENTORY_REFRESH_FAILED` with
`published` retained.

Only main artwork is downloaded: the work's current stored cover URL, read when
the download starts, then ASMR.one `mainCoverUrl` on absence/rejection/download
failure.
Only HTTPS `img.dlsite.jp` is initially allowed, including every redirect and DNS
connection; unknown/nonpublic addresses yield `UNSUPPORTED_CDN`. No metadata
cookies or application token are sent. Per-request timeout 10 seconds, size cap
16 MiB, at most three attempts for network/5xx/429 with cancelable backoff.
Actual image signatures and decoding determine `webp`, `jpg`, `png`, `gif`;
HTML/damaged content is `INVALID_IMAGE`, excess bytes are `IMAGE_TOO_LARGE`.
Bytes are not transcoded. No conditional requests are sent.

Two workers each claim one pending demand together with every other pending
demand for the same work, download once into memory and publish to each copy.
Restart returns claimed demands to pending and downloads again; no bytes are
kept between attempts, and stopped demands remain stopped. Refresh-all always
downloads and leaves a file whose digest already matches untouched (`skipped`). Network transfers do
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

Refresh follows only effective parent references upward, saving independent
relation reports for those canonical numbers. It does not follow OriginalRJ
alone, sibling IDs or language-edition lists. Ancestors receive no complete
Core, Statistics or Editions imports and no cover demands; the original page
may be read temporarily to obtain the selected Work's circle. Translation
reports and edition lists are limited to `CHI_HANS` and `CHI_HANT`; the original
remains a separate root. External reports never create Works. Stored effective
parents drive traversal even when an incoming report is stale, lower priority,
unavailable or already present in missing-only mode. Each accepted report
commits atomically after its network request; failed retrieval retains existing
evidence, leaves missing evidence unknown and reports issues. Responses are
shared by Work/source/locale within one refresh; ancestor reports are deduplicated
and cycles terminate. Upstream failures are attached to the selected Work's
issues with the upstream ID; progress/results still describe only selected Works.

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
