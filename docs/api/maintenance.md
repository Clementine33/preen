# Database record cleanup

These deployment-wide routes use the [common authentication, same-origin and error rules](common.md).
They delete database records only. Media, covers, thumbnails, recovery files,
current plans, execution results and inventory are retained. Manual cleanup never
runs VACUUM and does not reset the automatic maintenance clock.

## Routes

| Method | Path under `/api/v1` | Result |
| --- | --- | --- |
| POST | `/maintenance/cleanup/preview` | 200 preview of the selected categories |
| POST | `/maintenance/cleanup` | 202 accepted run summary |
| GET | `/maintenance/cleanup` | 200 current/latest summary, or `{"run":null}` |
| POST | `/maintenance/cleanup/{runId}/cancel` | 200 current summary after requesting stop |

Preview and start require a nonempty category selection, for example `{"categories":["works","source_tags"]}`.
The array must be nonempty and contain only supported keys. Duplicate keys are
normalized to the order shown. Unknown fields (including `request_id`) and
invalid categories return `400 INVALID_ARGUMENT`. Cancel accepts an empty body
or `{}`. Busy admission returns `409 BUSY`; unknown or replaced run IDs return
`404 CLEANUP_RUN_NOT_FOUND`. Unexpected failures use a safe `500 INTERNAL` envelope.

The currently supported categories are:

| Category | Eligibility |
| --- | --- |
| `scan_sessions` | completed/failed/canceled/interrupted, with a terminal time before the run starts, regardless of age; missing finished_at falls back to started_at; running scans are never eligible |
| `plan_generations` | finished_at present and before the run starts, regardless of age; unfinished generations are never eligible |
| `cover_downloads` | done, with no delivery reference of any status |
| `generation_records` | no inventory entry at the record's path: the output was moved or deleted outside the app, or its library is gone. A file found again later is judged by its measured rate |
| `works` | no stored copy binding, personal rating, nonempty note, user tag or collection membership; listening positions and Finished marks do not protect it |
| `relation_reports` | reports outside the fixed-point dependency closure of all surviving works |
| `source_tags` | shared source tags with no work reference |
| `source_voice_actors` | shared source voice actors with no work reference |
| `source_circles` | shared source circles with no work reference |

Manual cleanup ignores retention windows: every eligible finished scan and
planning record is deleted immediately, including the idempotency key of a
finished planning session. Automatic maintenance keeps its own windows (7 days
for scans, 30 days for planning records) and is unchanged by manual runs.

## Preview

```json
{
  "checked_at":"2026-10-23T04:00:00Z",
  "categories":["scan_sessions"],
  "candidates":{"scan_sessions":12},
  "rules":{"scan_sessions":{"protection":"Only terminal scans; missing finished_at falls back to started_at."}}
}
```

Only selected categories appear; each rule carries a `protection` description. Preview
counts source references and report roots after projected eligible work deletion only when `works` is selected;
otherwise every current work reference remains effective. Works with offline
copies remain protected. Empty user-data rows, collection time and source
metadata alone do not protect a work; notes are not trimmed. Listening positions and Finished marks do not protect a
work and are deleted with it.

Report roots are every surviving work, including works without copies when
`works` was not selected. Retention follows effective parent chains and keeps
each reached node's own report and **all** incoming declaring reports, including
rejected or ambiguous claims. Newly retained report subjects are expanded until
the set is stable. `OriginalRJ` alone does not retain a report, and raw child
lists do not turn siblings into roots. Unused mutually referencing reports can
be reclaimed together.

Report groups delete dependents before dependencies; strongly connected groups
commit together. Each write transaction recomputes current roots and logical
dependencies, protects reports outside its finite batch and their dependencies,
and validates the remaining evidence before committing. Existing works keep
their resolved relations and conflict diagnostics. Concurrent additions or
changed dependencies can cause old candidates to be skipped, including an old
group that can no longer be deleted safely; another explicit run may reclaim
them later. Reports created after discovery are never added to this run.

Source entities are identified by their stored identity and references, never
name or language. An accepted refresh of a source ID reuses its identity across
Japanese/Chinese names. User-created tag vocabulary and classifier settings
are always retained. Work-owned source entities (`work_id` present) belong to
the work cascade and are excluded from independent source candidates.

Preview is read-only: it neither locks candidates nor
issues a deletion token. Start repeats eligibility checks. A run that finds
zero candidates can succeed. Preview counts are not a fixed progress denominator.

## Run summary

Start, GET and cancel return the same envelope:

```json
{
  "run":{
    "run_id":"server-generated-uuid",
    "categories":["scan_sessions"],
    "status":"running",
    "phase":"scan_sessions",
    "started_at":"2026-10-23T04:00:00Z",
    "finished_at":null,
    "counts":{"scan_sessions":{"checked":500,"deleted":498,"skipped":2}},
    "reason":""
  }
}
```

`phase` identifies the selected category being processed. Counts describe only
committed batches: checked candidates, actual top-level records deleted and
candidates skipped after rechecking. Cascaded rows are not separately counted:
each deleted work counts once,
including its associations and work-owned source entities even when source
categories were not selected. Shared entities count only when independently
deleted in their selected source stage. Work deletion preserves independent
relation reports and other works' derived relations. Each deleted relation report
counts once; its owned child declarations cascade without additional counts.
Deletion and the corresponding counters commit in one SQLite transaction.
The safe `reason` explains unsuccessful termination without exposing SQL,
credentials or connection information. `finished_at` becomes a UTC timestamp
on termination.

States are `running`, `stopping`, `stopped`, `succeeded`, `failed` and
`interrupted`. Stop changes running to stopping and cooperatively cancels
candidate discovery and batch execution. The current transaction commits
or rolls back before termination; earlier committed counts remain. A concurrent
batch or successful finish cannot overwrite stopping with running/succeeded.
Repeated stop of the same latest run, including a terminal run, is safe and
never restarts it.

There is one persistent current/latest slot, replaced by the next accepted run.
There is no historical list, request ID, replay key or automatic retry. If a
start response is lost or BUSY is returned, GET the global summary; the client
must not automatically submit another start or assume the run belongs to its
page. A new run requires a fresh explicit user confirmation.

## Lifecycle and admission

The worker belongs to the process context. Navigating away, closing the page or
canceling the starting HTTP request does not cancel an accepted run. Clients
poll GET; there is no cleanup SSE stream. A network failure means unknown state,
not successful completion. Process shutdown cancels and joins the worker before
SQLite closes. Terminal-summary writes retry after contention while the worker
and manual run marker remain active; shutdown cancellation does not abandon
persistence. The existing process shutdown guard handles a finalizer that cannot
make progress. Startup interrupts leftover running/stopping summaries before
requests are accepted, preserving committed counts without resuming work.

Stages run in the category order above: history, works, relation reports, then source entities.
Each run fixes its session cutoffs at started_at. Each selected stage captures
a finite, cancelable candidate set. Ordinary categories recheck batches of up to
500 records in the deletion transaction. Report batches target 500 reports,
without splitting an indivisible dependency group; a large group can exceed
500 and hold the write lock longer. Stopping may wait for that transaction.
A failure rolls back only its current batch.
Candidates created after that stage's discovery belong to a future run. Every
work's personal-data and copy protection and every source reference is rechecked
inside the same transaction as deletion and progress. A protection committed
after preview skips that candidate. When deletion commits first, later personal
edits return `404 WORK_NOT_FOUND`. Only committed catalog changes emit the
existing catalog notification, covering work lists/details, relation existence
and source filters.

The gate atomically reserves the manual run marker and first maintenance slot.
The marker survives each batch yield, preventing another manual run or automatic
maintenance from acquiring a slot. Ordinary scans/tasks may enter between
batches; if they hold admission when cleanup reenters, cleanup ends interrupted
and retains its committed results. Busy starts do not queue.

Automatic maintenance still handles only the four history categories (those
before `works`) and space maintenance. It attempts a pass before requests open, first waits 15 minutes in
its background loop, then waits 24 hours after a completed pass or 15 minutes
after a busy, incomplete or budget-limited pass. A manual run adds no timer and
changes none of these intervals.
