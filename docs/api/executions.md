# Operation execution

[API reference](../api.md) · [Common conventions](common.md)

## Execution sessions

Audio execution applies the frozen file decisions. Metadata execution is described in [Metadata refresh task](tasks.md#metadata-refresh-task-schema-version-1); asynchronous cover publication has its own write lifecycle. It runs exactly one
operation revision, as frozen: the revision's component outcomes, roots and
effective targets are consumed as persisted — no reconcile, no live draft, no
client-supplied file list.

**Authorization.** The gates below all run before a session exists; they are
the plan's own facts, re-read at execution time:

| Rejection (409 unless noted) | Meaning |
| --- | --- |
| `PLAN_NOT_EXECUTABLE` + `NOT_CURRENT_REVISION` | the plan is not the operation's current revision |
| `PLAN_NOT_EXECUTABLE` + `DRAFT_CHANGED` | the live draft no longer matches the revision's frozen draft hash |
| `PLAN_NOT_EXECUTABLE` + `INPUT_CHANGED` | a root is missing or its live inventory fingerprint no longer matches the frozen one, or the revision's members no longer resolve |
| `PLAN_NOT_EXECUTABLE` + `BLOCKED_COMPONENTS` | the frozen snapshot has a blocked component |
| `PLAN_NOT_EXECUTABLE` + `ALREADY_EXECUTED` | this revision already ran (any terminal status) |
| `PLAN_NOT_EXECUTABLE` (no details) | a guard predicate refused the start under a concurrent change; re-read the operation |
| `EXECUTION_IN_PROGRESS` | another session of this operation is queued/running |
| `SCAN_IN_PROGRESS` | the library root is being scanned |
| `VERSION_CONFLICT` | stale `If-Match`; re-read the operation |
| `IDEMPOTENCY_KEY_REUSED` | the key was used for another revision |
| `ORPHANED_WORKSET` | the library is gone; orphaned worksets are read-only |
| `FOLDER_NOT_IN_RECORD` (400) | `folder_paths` names something that is not a member of this record |

**Scope.** A session covers the whole revision unless `folder_paths` names
some of its members. A path the record does not hold is refused with
`FOLDER_NOT_IN_RECORD` (400) rather than skipped — running fewer folders than
were asked for would be a scope change nobody approved; a selected folder the
plan found nothing to do in simply contributes no components, which is the
plan's conclusion about that folder, not a refusal. The session's own
`total_components`, the component list and SSE progress cover the scope alone, and
`selected_folders` (the record-relative paths) says what that scope was, so a
client that did not start the run can tell it was partial.

**One session per revision.** The storage enforces it (a unique index on the
revision): a retry with the same `Idempotency-Key` returns its own session
(200), a new key on an executed revision is `ALREADY_EXECUTED`, and concurrent
starts have exactly one winner. A scoped run spends the revision like any
other: the folders it left out are the next plan's work — regenerate a plan and
execute that — never a second run of this one. The idempotency key covers the
scope, so the same key with a different `folder_paths` is
`IDEMPOTENCY_KEY_REUSED`. Failures, partial completions and interrupted
sessions do not auto-retry: refresh the inputs (rescan), then generate a new
revision — a revision never runs twice.

**Fresh facts before writing.** Conversion admission compares stored inventory
only for the roots owned by actual execution units. Revision detail continues
to evaluate the whole plan; clients use each root's existing `stale` and missing
status for selected execution, while draft, version, global blocked components
and one-session rules still apply. An empty worklist checks no input roots.

Before preparing any conversion unit, the worker walks those same roots with
bounded folder concurrency and sequential descendants. Audio count and
fingerprint use integer Unix-second mtime, matching inventory storage. A match
performs no scan session, staging write or inventory merge and does not refresh
non-audio inventory. Drift or a missing root triggers a best-effort serial
member refresh and fails with `INPUT_CHANGED`, including any refresh diagnostic.
Initial walk permission/I/O errors are `SCAN_FAILED`; cancellation is `CANCELED`.
Rename and metadata retain their original refresh flow.

Conversion fixes the tool configuration once per session. Actual encode work
checks tools and their version once, then verifies each actual output container
through a short PCM WAV FD encode/probe/full-decode roundtrip. Pure deletion and
empty execution require no encoder. Source path prechecks and live handles use
the same size rules and ±1 second frozen-mtime tolerance. Staged output identity,
size and full mtime are checked for the entire ready component before its first
publication (`validate`), then again immediately before each publication
(`commit`). The first check waits only for that component's encodes. A second
check failure preserves prior published outputs; obsolete audio is removed only
after all component outputs publish. Encoding workers capture hashes and facts;
commit does not reopen published outputs to hash or stat them.

After input validation and before the first encode, actual required moves are
probed once per mount. Linux uses mount IDs, including bind mounts, rather than
device numbers. Strict publication and recovery refuse overwrite; unsupported
no-replace rename may use regular-file hard links within the same mount. An
uncertain outcome protects possible surviving names and discloses inventory
uncertainty. Link success followed by source unlink failure reports the known
published output and preserves both names. Recovery targets claimed by another
process are preserved. Probe scratch uses `.tmp.<unique>` names and its own
cleanup list; leftovers are disclosed and naming does not promise automatic
scan exclusion or cross-process cleanup.

| Session failure | Meaning |
| --- | --- |
| `FILESYSTEM_OPERATION_UNSUPPORTED` | conversion did not start: required strict moves or mount boundaries are unsupported |
| `FILESYSTEM_PRECHECK_FAILED` | conversion did not start: permission/I/O or scratch cleanup prevented the filesystem probe |

These codes do not claim that scratch creation/cleanup left the disk untouched.
Soft recovery is probed only for actual removals or replacements, in the deepest
existing planned recovery ancestor. Hard mode creates no recovery directory.
Generation refreshes participating scopes through one bounded walk pool, one
staging writer and one explicit selected-range merge; failed or canceled scans
do not merge, and an empty selection cannot mean a whole-library cleanup.

**Audio lifecycle.** Statuses `queued` → `running` → `succeeded` | `failed` |
`canceled` | `interrupted`; a terminal status never regresses. Sessions run
serially (one worker) in frozen root/component order; the first component
failure stops admission, and every later component stays `pending` in the
report. Each component's result is committed as its own row the moment it
finishes — one short transaction that also advances the session's counters and
its position — so a crash keeps the facts of everything that already happened on
disk, and nothing ever rewrites what the components before it did. On startup
any leftover queued/running session is marked `interrupted`: partial results
stay visible, nothing is resumed, re-encoded or re-deleted, and the in-flight
component's disk state must be treated as unverified (a temporary
`<target>.tmp.<token>` output may remain).

**Detail payload** (also the SSE snapshot): `execution_id`, `workset_id`,
`operation_type`, `plan_id`, `status`, `options` (the task-owned frozen
session options — conversion: `{"delete_mode":"soft"|"hard"}`),
`total_components`,
`completed_components` (components that are no longer pending — derived from the
recorded component results, never incremented on its own),
`total_operations`, `completed_operations`, `current_root`,
`current_component_id`, `current_phase` (`component` while a component runs),
`components[]`, `selected_folders` (the scope this session ran; absent when it
ran the whole revision), `error_code`, `error_message`, `started_at`,
`finished_at`, `created_at`.

`components[]` carries the run's scope: the frozen worklist in component order,
with each component's own result folded in. A component with no recorded result
at all is `pending` with empty lists — nothing has run for it yet. The list is
ordered by `component_index` and may be paged through the detail route's
`components_from`/`components_limit`.

Each `components[]` entry: `component_index`, `component_id`, `root_path`,
`partition`, `status` (`pending` | `succeeded` | `failed` | `canceled`),
`stage` (the failing stage: `precheck`, `materialize`, `validate`, `commit`,
`remove`), `operations`, `completed_operations`, `committed[]`, `removed[]`,
`remaining[]` (operations not completed, in frozen order), `recovery[]`
(preserved files needing operator attention: soft-delete destinations,
replaced-old copies, leftover temporaries), `error_code`, `error_message`,
`inventory_synced`, `inventory_sync_error`, optional task-owned `task_result`.

**Events.** Every connection first receives `execution_snapshot` (the full
detail above), then `component` (one component's own entry, exactly as it
appears in `components[]`, sent as that component's result lands), `progress`
(counts + current component; never a fabricated percentage) and exactly one
terminal event: `succeeded`, `failed`, `canceled` or `interrupted`. A component
result and the final persisted progress are emitted before the terminal event,
so a client that watches a run to its end has seen every component it ran and
the final counts, including partial counts after failure or cancellation.
A connection to an already-terminal session gets those counts in its initial
snapshot. There is no event-log
replay: a client that missed events — or attached late, or holds only the first
page of a large run — re-reads the detail route, and the terminal event's
calibration read closes any gap left by a result written outside component
order. Disconnecting never cancels the session — only `POST …/cancel` or the
process lifecycle does.

**Cancellation.** Canceling a queued session ends it immediately; canceling a
running session sets a cooperative flag, and the worker stops at the
component's next safe stage boundary (a started commit finishes, so a recovery
copy is never destroyed halfway). Cancellation is idempotent.

**Metadata.** An encode carries the source's tags into every target
(`-map_metadata 0`): `.mp3` and `.m4a` on the container, `.flac` and `.opus` in
their own tag blocks. Embedded pictures are not part of the model — the
workflow's first source is WAV, which cannot hold one — so no target gets
container-specific picture handling.

**Inventory sync.** After each component the observed disk changes are applied
to the entries inventory for the affected paths only (removed sources lose
their row, committed outputs are refreshed with the scan merge's `content_rev`
semantics). A soft removal lands under
`<work parent>/Delete/<work name>/<member-relative path>`, so a nested member
`分类/RJ000001` recovers beside itself at `分类/Delete/RJ000001/…`.
Recovery paths remain in the execution report; recovery files are excluded from
the inventory and scans at every depth. A sync failure is disclosed per
component (`inventory_synced:false`, `inventory_sync_error`) instead of being
reported as "unchanged". The library folder counts are computed from the
inventory, so a component's sync moves them before the next scan does. The
executed revision keeps its frozen fingerprints, so it reports
`validation_state:"stale"` once the inventory moved — a follow-up run needs a
new generation.

**Mutual exclusion.** While a session is queued/running for an operation: its
drafts cannot be edited and it cannot be re-generated (`EXECUTION_IN_PROGRESS`
on both routes), the owning library cannot be deleted, and its library root
cannot be scanned.

Session routes and version guards are listed in [Processing records and operations](worksets.md#routes). Conversion drafts and frozen revisions are described in [Audio conversion](conversion.md).
