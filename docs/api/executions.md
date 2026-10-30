# Operation execution

[API reference](../api.md) · [Common conventions](common.md)

## Execution sessions

Audio execution applies the frozen file decisions. Rename execution applies frozen directory basenames and preserves mutable path identities. Metadata execution is described in [Metadata refresh task](tasks.md#metadata-refresh-task-schema-version-1); asynchronous cover publication has its own write lifecycle. It runs exactly one
operation revision, as frozen: the revision's component outcomes, roots and
effective targets are consumed as persisted — no reconcile, no live draft, no
client-supplied file list.

**Authorization.** The gates below all run before a session exists; they are
the plan's own facts, re-read at execution time:

| Rejection (409 unless noted) | Meaning |
| --- | --- |
| `PLAN_NOT_EXECUTABLE` + `NOT_CURRENT_REVISION` | the plan is not the operation's current revision |
| `PLAN_NOT_EXECUTABLE` + `DRAFT_CHANGED` | the live draft no longer matches the revision's frozen draft hash |
| `PLAN_NOT_EXECUTABLE` + `INPUT_CHANGED` | task-owned input freshness failed: conversion inventory changed, metadata's frozen directory is missing/renamed/non-directory, rename naming inputs changed, or revision members no longer resolve |
| `PLAN_NOT_EXECUTABLE` + `BLOCKED_COMPONENTS` | the frozen snapshot has a blocked component (a rename plan skips blocked components instead) |
| `PLAN_NOT_EXECUTABLE` + `ALREADY_EXECUTED` | this revision already ran (any terminal status) |
| `PLAN_NOT_EXECUTABLE` (no details) | a guard predicate refused the start under a concurrent change; re-read the operation |
| `EXECUTION_IN_PROGRESS` | another session of this operation is queued/running |
| `SCAN_IN_PROGRESS` | the library root is being scanned |
| `COVER_DELIVERY_PENDING` | actual rename roots still have pending cover deliveries |
| `BUSY` | rename enqueue conflicts with a registered scan, including another library's generation refresh |
| `PLAN_NOT_EXECUTABLE` + `GENERATION_IN_PROGRESS` | rename conflicts with any same-library generation |
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
Rename planning reads only distinct parent directories. Metadata planning skips
inventory refresh, freezes targets and checks directories directly. Metadata
admission and worker use the same directory-only rule for actual execution units:
one Lstat, real directory, frozen basename. No inode/size/mtime or live binding
comparison; same-name directory replacements are accepted. Empty worklists
access no directories. Missing/name/type changes are `INPUT_CHANGED`, permission
or I/O errors are `SCAN_FAILED` with diagnostics, and cancellation is `CANCELED`.
The batch executes frozen RJ targets without post-import revalidation. After its
execution and its own cover demands are terminal, a separate background job
rescans only target directories whose cover delivery reports
`INVENTORY_REFRESH_FAILED`, deduplicated into one selected-range scan. An empty
repair scope requests no file-operation admission or scan; batches without cover
demands register no follow-up. This reconciliation does not change execution
results or add HTTP/SSE fields; failures are logged independently without automatic
retry. Pending repairs are process-local and are not recovered after restart.
Actually published files can be discovered by a later Library Scan, subject to
attribute-cache delay. Reusing an existing cover does not change directory mtime;
an unrepaired inventory failure on that path needs a successful cover retry,
member refresh or Full Rescan. See [metadata task lifecycle](tasks.md).

Conversion fixes the tool configuration once per session. Actual encode work
checks tools and their version once, then verifies each actual output container
through a short PCM WAV FD encode/probe/full-decode roundtrip. Pure deletion and
empty execution require no encoder. Source path prechecks and live handles use
the same size rules and ±1 second frozen-mtime tolerance. Staged output identity and
size are checked for the entire ready component before its first
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

During orderly shutdown, unfinished execution sessions become `interrupted`
with `INTERRUPTED`, preserving committed component results. Explicit user
cancellation remains `canceled` / `CANCELED`; independent failures and completed
terminal outcomes retain their meaning. Shutdown waits for protected publication
and inventory finalization before closing SQLite, subject to the process-wide
[five-second exit deadline](common.md#shutdown).

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
at all is `pending` with empty result lists — nothing has run for it yet. Its
frozen `kept_decisions` may already contain files. The list is
ordered by `component_index` and may be paged through the detail route's
`components_from`/`components_limit`.

Each `components[]` entry: `component_index`, `component_id`, `root_path`,
`partition`, `status` (`pending` | `succeeded` | `failed` | `canceled`),
`stage` (the failing stage: `precheck`, `materialize`, `validate`, `commit`,
`remove`), `operations`, `completed_operations`, `committed[]`, `removed[]`,
`remaining[]` (operations not completed, in frozen order), `recovery[]`
(preserved files needing operator attention: soft-delete destinations,
replaced-old copies, leftover temporaries), `error_code`, `error_message`,
`inventory_synced`, `inventory_sync_error`, optional task-owned `task_result`,
`kept_decisions[]` (the frozen plan's file decisions whose `resolution` is
`keep`, in plan order).

`kept_decisions` is always an array, including `[]` for components with no kept
files and tasks without file decisions. Each decision has `path`, `resolution`
(`keep`), and optional `reason_code`, `message`, `target_path`, matching the
conversion plan's file-decision shape. These are plan conclusions, including
unmet targets, rather than observed execution results: pending, failed and
canceled components carry them too. The server joins the requested components
to their frozen plan; clients do not need a revision read. Every component
entry carries this field: detail pages (`components_from`/`components_limit`),
initial and reconnect `execution_snapshot` events, incremental `component`
events, and start (including idempotent replay) and cancel session responses.
Clients may replace an entire component entry when applying an event.

Conversion component failure codes (the payload shape is unchanged):

| `error_code` | Meaning / stage |
| --- | --- |
| `COMPONENT_INVALID_REQUEST` | malformed frozen request (`precheck`) |
| `COMPONENT_BLOCKED` | frozen component is blocked (`precheck`) |
| `COMPONENT_UNSUPPORTED_OPERATION` | unsupported frozen operation (`precheck`) |
| `COMPONENT_INVALID_DEPENDENCY` | invalid operation dependencies (`precheck`) |
| `COMPONENT_TARGET_CONFLICT` | target is occupied |
| `COMPONENT_FILE_CHANGED` | frozen input facts or identity changed |
| `COMPONENT_PATH_UNSAFE` | path containment or symlink check refused the path |
| `COMPONENT_TOOLS_UNAVAILABLE` | required encoding tools are unavailable (`precheck`) |
| `COMPONENT_ENCODE_FAILED` | source probing, preparation, or encoding the frozen target failed (`materialize`) |
| `COMPONENT_VALIDATE_FAILED` | stage flush/close, encoded output probe, decode/hash (Ogg page checks for Opus) or post-encode object check failed (`materialize`); the whole ready component's pre-publication identity pass failed (`validate`) |
| `COMPONENT_COMMIT_FAILED` | publication or replacement/recovery failed (`commit`) |
| `COMPONENT_DELETE_FAILED` | obsolete-file removal failed (`remove`) |
| `COMPONENT_CANCELED` | cancellation at the reported stage; cancellation takes precedence over a killed tool's diagnostics |

`stage` and `error_code` describe different facts: output validation runs inside
an encoding worker at `materialize`; `validate` names the component-wide pass
before its first publication. Later publication checks report `commit`, preserve
already published outputs, and never relabel that partial result as a validate
failure. Clients display `error_message`, with `error_code` as fallback.
Conversion failure messages retain the underlying diagnostic (tool stderr or
filesystem/object-check error) alongside the affected path.

Output validation starts after checking Sync/Close errors from the writable
stage handle and safely reopening the same object read-only. Linux and Windows use
the same lifecycle, with no SMB-specific stage lock or timestamp stabilization.
Flush/close failures stop the component before validation or publication.
Probe still checks codec, channels,
sample rate and source duration (0.1 s tolerance). Validation commands use
`-v error`: nonzero exit or non-whitespace stderr rejects the output, with
cancellation checked first. MP3 and M4A retain full decoding followed by one
SHA-256 read for lossy output credentials, without native MP3 Info/length/CRC
or M4A box/chunk-offset parsing and without packet counting. Opus verifies every
Ogg page CRC, sequence/continuation and a final EOS at EOF while hashing once,
replacing its per-file full decode.

Successful decoding and the duration comparison do not prove file completeness:
MP3 frame-boundary truncation can pass silently and retain the Info-header
duration; some M4A tail truncations can also pass. Reported encoding and writer
close errors stop publication. Object-fact checks detect identity/size changes
across writer close and read-only reopen, then identity/size changes during validation and before publication.
Staged mtime is an observed inventory/credential fact, updated from the existing
pre-publication stat; it is never an equality gate for an owned stage. Remote
timestamp updates alone do not fail conversion. Silent storage failure and
external mutations that preserve identity and size are accepted risks.
SHA-256 records observed bytes; it is not an independent expected output checksum.

The pool and component window use the library's `storage_class`: HDD is
`min(4, C)` and SSD is `C`, where `C = max(1, floor(logical CPU count × 2/3))`.
These are backend policy constants, not user worker-count settings.
The worker captures the width when it starts processing the queued session,
before preflight or input scanning; later storage edits do not resize that
session. A queued session therefore uses the library setting at processing
start. `config.json` does not override worker limits. Workers include validation.
A ready head
component commits and persists even if later task delivery is backpressured;
FIFO delivery and serial preparation/publication remain unchanged.

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
their row, committed outputs receive their extension-derived format and the scan
merge's `content_rev` semantics, clearing stale measured facts when size or mtime
changes). After a successful sync with new or changed files, the background
Media Facts Pass is woken to measure dimensions, duration and bitrate. A soft
removal lands under
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

## Rename component results and interrupted repair

Rename uses zero-encode components, commits directories serially in frozen order,
and persists each result before the next commit. Noop completes zero operations;
a committed rename completes one. `task_result` carries `source_path`,
`target_path`, `renamed`, optional `rolled_back`, and optional `recovered`.
Failures report the actual disk location through these facts and the existing
committed/removed fields:

| Component error code | Meaning |
| --- | --- |
| `COMPONENT_RENAME_FAILED` | target conflict or filesystem rename failure |
| `RENAME_STATE_SYNC_FAILED` | path-state transaction failed; this directory was moved back |
| `RENAME_ROLLBACK_FAILED` | path-state transaction and reverse move failed; disk may retain the new name while the database retains the old path |

Earlier committed directories remain on failure/cancellation. Target precheck is
only an explanatory check; platform no-replace primitives and process admission
provide the existing move guarantees.

Startup repair runs before workers and HTTP. For each interrupted rename it
checks only the first frozen unit with no component result. Source-present and
target-absent is unstarted and produces no diagnostic; later units are untouched.
Source-absent and a valid target directory triggers idempotent path relocation,
then the existing component-result write records success with `renamed:true` and
`recovered:true`, recalculates counts and clears position. This also covers a
crash after sync but before the result write. The session remains `interrupted`;
no remaining rename is executed. Repair failures leave the result absent for a
later startup retry. Already persisted failures, including
`RENAME_ROLLBACK_FAILED`, are not retried; resolve the underlying conflict and
rescan/regenerate manually, with the usual risk of losing old-path credentials.
No new journal, schema or HTTP/SSE envelope is introduced.
