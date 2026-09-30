# Onsei Backend HTTP / SSE API


The backend runs a single net/http listener (the Vue web client) over
loopback. It binds `127.0.0.1` only and owns the one SQLite writer, the scan
service and the workset service.

HTTP endpoints live under `/api/v1`. Machine-checked contract coverage lives
in the Go tests (`backend/go/tests/e2e/*_test.go` and the
`backend/go/internal/adapters/httpapi` handler tests); this document is the human
reference.

## Startup handshake

On startup the backend prints exactly one line to stdout:

```
ONSEI_BACKEND_READY token=%s version=%s http_port=%d
```

- `http_port` — HTTP port
- `token` — the configured `ONSEI_TOKEN` (empty when auth is disabled)
- `version` — build version stamp (`dev` by default)

The Vue dev script and the Playwright launchers scan stdout for the
`ONSEI_BACKEND_READY` line and read the HTTP port from it.

## Auth

Protected routes require:

```
Authorization: Bearer <token>
```

The token comes from the `ONSEI_TOKEN` env var. An empty token disables auth
entirely (local-developer mode). Missing or invalid tokens return
`401` with `{"code":"UNAUTHORIZED","message":"..."}`.

## CORS

`ONSEI_CORS_ORIGINS` is a comma-separated allowlist of browser origins. The
default is `http://localhost:5173,http://127.0.0.1:5173` (Vite dev servers).
Origins outside the allowlist receive no CORS headers and are blocked by the
browser. Preflight (`OPTIONS`) is answered by the CORS middleware before the
auth middleware runs, because browsers do not send the `Authorization` header
on preflight.

## Error envelope

Non-2xx JSON responses always use the same shape:

```json
{ "code": "LIBRARY_NOT_FOUND", "message": "library not found" }
```

Common codes: `INVALID_ARGUMENT` (400), `UNAUTHORIZED` (401),
`LIBRARY_NOT_FOUND` / `DIRECTORY_NOT_FOUND` / `MEMBER_MISSING` (404),
`LIBRARY_EXISTS` / `BUSY` (409), `INTERNAL` (500). The session routes add their
own guards: `IDEMPOTENCY_KEY_REQUIRED`, `VERSION_REQUIRED`,
`INVALID_IDEMPOTENCY_KEY`, `INVALID_PAGE` (400) and `PAYLOAD_TOO_LARGE` (413).
Codes such as `SCOPE_REQUIRED` are a planning session's `error_code`, never an
HTTP envelope.

The work catalog adds: `INVALID_PAGE` / `INVALID_CURSOR` /
`CURSOR_QUERY_MISMATCH` / `MATCH_MODE_REQUIRED` / `INVALID_TAG_NAME` /
`INVALID_TIMESTAMP` (400), `WORK_NOT_FOUND` / `USER_TAG_NOT_FOUND` (404),
`WORK_IN_USE` / `USER_TAG_EXISTS` / `WORK_IS_PLACEHOLDER` (409). A refusal may
carry `details[]` naming the references that hold a work in place.

## Endpoints

| Method | Path | Auth | Status | Response |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/health` | no | 200 | `{"ok":true,"version":"dev"}` |
| GET | `/api/v1/libraries` | yes | 200 | `{"libraries":[{...}]}` |
| POST | `/api/v1/libraries` | yes | 201 | library object |
| GET | `/api/v1/libraries/:id` | yes | 200 | library object |
| PATCH | `/api/v1/libraries/:id` | yes | 200 | library object |
| DELETE | `/api/v1/libraries/:id` | yes | 204 | — |
| POST | `/api/v1/libraries/:id/scans` | yes | 200 | SSE stream (see below) |
| GET | `/api/v1/libraries/:id/dirs` | yes | 200 | `{"dirs":[{...}]}` |
| GET | `/api/v1/libraries/:id/tree?dir=…` | yes | 200 | `{"tree":{...},"dir_id":…,"member_path":…}` (see below) |
| POST | `/api/v1/libraries/:id/tree/refresh?dir=…` | yes | 200 | `{"tree":{...},"dir_id":…,"member_path":…,"refreshed":true}` |
| POST | `/api/v1/libraries/:id/file-operations` | yes | 200 | per-item result (see below) |
| GET | `/api/v1/libraries/:id/operations/:type/current` | yes | 200 | `{"workset":{...}\|null}` |
| PUT | `/api/v1/libraries/:id/operations/:type/current` | yes | 201/200 | record + `skipped[]` (see below) |

### Library object

```json
{
  "id": "uuid",
  "name": "Music",
  "root_path": "/home/me/music",
  "created_at": "2026-08-22T00:00:00Z",
  "updated_at": "2026-08-22T00:00:00Z",
  "last_scan_at": null,
  "last_scan_status": "",
  "last_scan_error": ""
}
```

`POST /api/v1/libraries` body: `{"name":"...","root_path":"/abs/path"}`.
`PATCH` applies only the fields present (`name`, `root_path`). Changing
`root_path` clears the scanned inventory of the old root and the prior scan
state; the library must be scanned again before anything is planned from it.
A root change is rejected with `LIBRARY_HAS_WORKSETS` while the library still
has a processing record: delete the library — its record goes with it — and
re-create it at the new root.
Both a root change and a deletion take the direct-file-management slot, so
neither interleaves with file management (ADR 0002 §2).

`DELETE /api/v1/libraries/:id` removes the library **together with its
processing record**: the record's members, operation, draft, current plan and
execution results go in the same transaction, so no orphaned record survives
its library. Media files and the library-level `Delete/` recovery directory on
disk are never touched. Deletion is blocked by active operation generation
(`GENERATION_IN_PROGRESS`) and by an active execution session
(`EXECUTION_IN_PROGRESS`).

### Scan: POST + SSE

`POST /api/v1/libraries/:id/scans` takes a JSON object body; an empty body is
read as "no override", like `{}`. An optional `root_path` is retained for
client compatibility but must resolve to the selected library's configured
root; cross-library scan overrides are rejected with
`ROOT_PATH_OUTSIDE_LIBRARY`. The response is
`Content-Type: text/event-stream` and is cancelled automatically if the client
disconnects.

| Event | Data fields |
| --- | --- |
| `started` | `stage`, `message` |
| `progress` | `stage`, `files_scanned`, `dirs_scanned` |
| `completed` | `stage`, `scan_id`, `root_path`, `files_scanned` |
| `error` | `stage`, `code`, `message` |
| `cancelled` | `stage`, `message` |

A successful scan always emits `started` then one or more `progress` then
`completed` (`scan_id` is set). Failures end with `error`;
client-initiated cancellation with `cancelled`. The scan writes the library's
inventory — the only listing source: there is no separate derived folder table
to rebuild, and a rescan cannot renumber anything the workbench navigates by.
A `completed` scan state is recorded on the library.

A scan is refused with `EXECUTION_IN_PROGRESS` (409) while any record of the
library has a queued/running execution: an execution validates the inventory
the scan would rewrite. It is also refused with `BUSY` (409) while direct file
management holds the admission slot, and it takes the scanning side of that
slot itself, so a file operation refuses a running scan in turn (ADR 0002 §2).

### Directories and member trees

`GET /api/v1/libraries/:id/dirs` lists **every** direct child directory of the
library root, whether or not it holds audio, with that directory's subtree
counts. The library-level recovery directory is not a member and is left out:

```json
{
  "dirs": [
    { "name": "albumA", "path": "/home/me/music/albumA", "rel_path": "albumA",
      "dir_id": "4b7fa32eebf5b001bcf43b1976c1b163",
      "audio_file_count": 4, "file_count": 7 }
  ]
}
```

`rel_path` is the data identity: it is stable across rescans, where a
scan-scoped folder id is not, and it is what a record's scope and file
management address. `dir_id` is the navigation identity a page address carries
instead of the name (ADR 0003): the backend derives it from a fixed version
marker, the library, the canonical identity of its root and the stored relative
path (SHA-256, first 128 bits, lowercase hex). The same root and path keep the
same value across rescans and restarts; renaming the directory, or changing the
library root, makes the old value unknown. It is an identity, **not** a
credential: the same auth and path checks apply to it as to any other route.

`GET /api/v1/libraries/:id/tree?dir=<dir_id>` returns the stored tree of one
member directory, and answers with the identity it resolved and the path that
identity stands for, so a caller that only holds the identity learns both. The
tree carries each node's `rel_path` (relative to the member root), which is what
file management addresses:

```json
{
  "dir_id": "4b7fa32eebf5b001bcf43b1976c1b163",
  "member_path": "albumA",
  "tree": {
    "name": "albumA", "path": "/home/me/music/albumA", "rel_path": "", "type": "dir",
    "children": [
      { "name": "track1.flac", "path": "/home/me/music/albumA/track1.flac",
        "rel_path": "track1.flac",
        "type": "file", "size": 12345, "bitrate": 920000, "format": "flac" }
    ]
  }
}
```

The identity is resolved against the library's scanned inventory — the direct
child directories its listing shows, recovery directory excluded — and only the
single match is then checked on disk. The refusals are distinct:

| Condition | Answer |
|---|---|
| No `dir` parameter | 400 `DIR_ID_REQUIRED` |
| Not 32 lowercase hex characters | 400 `DIR_ID_INVALID` |
| No directory of this library has that identity | 404 `DIRECTORY_NOT_FOUND` |
| More than one directory claims it | 409 `DIRECTORY_AMBIGUOUS` (never the first match) |
| A direct child in the inventory that is gone from disk | 404 `MEMBER_MISSING` |
| Names the recovery directory, or is a symlink | 400 `MEMBER_PATH_INVALID` / `MEMBER_IS_SYMLINK` |

The retired `?folder=<rel_path>` parameter is not read: a request that carries
it instead of `dir` is answered as a missing identity. A directory that no scan
has recorded yet has no identity to address.

`POST /api/v1/libraries/:id/tree/refresh?dir=<dir_id>` re-scans that one
member directory and answers `{"tree": …, "dir_id": …, "member_path": …,
"refreshed": true}`; the refreshed directory keeps its own row in the inventory,
so it stays listed and stays addressable. It takes the
scanning side of the admission slot (see above) and answers `BUSY` (409) while
a file operation holds it. A failed refresh is `502` with the scan's code: the
caller keeps the tree it already shows and marks it unrefreshed.

### Direct file management

`POST /api/v1/libraries/:id/file-operations` is the second, explicitly
non-plan write path (ADR 0002 §1). It holds the direct-file-management slot
for the whole request — including the inventory refresh that follows the
writes — and is refused with `BUSY` (409) while any scan is running or any
planning session or execution is queued or running.

Body:

```json
{
  "member_path": "albumA",
  "operation": "rename",
  "items": [ { "source": "track1.flac", "name": "track01.flac" } ]
}
```

| Operation | Items |
| --- | --- |
| `rename` | one item; `name` is a plain name, never a path (`INVALID_NAME`) |
| `move` | one item; `target_dir` is an existing directory of the same member (`INVALID_TARGET_DIR`, `MOVE_INTO_SELF`) |
| `soft_delete` | one or more items, applied in order |

Every path is validated independently of the request: absolute paths,
traversal, a path that leaves the member, a path through a symlink and a
symlink itself are refused (`PATH_INVALID`, `OUTSIDE_MEMBER`, `SYMLINK`), and
the member root is never an item. A destination that exists is refused
(`TARGET_EXISTS`) — a rename or move never overwrites, and the no-replace
rename of the target platform is what enforces it. A batch stops at its first
failure: the rest are reported as `not_attempted`, and nothing already done is
undone. Selecting a directory and something inside it is one operation: the
child is reported `skipped` with `COVERED_BY_PARENT`.

Response (200):

```json
{
  "operation": "soft_delete",
  "member_path": "albumA",
  "items": [
    { "source": "track1.flac", "status": "ok", "recovered_path": "Delete/albumA/track1.flac" },
    { "source": "cover.jpg", "status": "failed", "code": "TARGET_EXISTS", "message": "…" }
  ],
  "succeeded": 1, "failed": 1, "untouched": 0,
  "refresh": { "ok": false, "code": "REFRESH_FAILED", "message": "files were modified, but refreshing the inventory failed: …" }
}
```

`status` is `ok`, `failed`, `skipped` or `not_attempted`; a rename or move
reports where the item landed as `target` (member-relative). A soft delete keeps
the item's path relative to the library root under `<library root>/Delete/` and
never overwrites media recycled earlier (a collision becomes `<stem>.N<ext>`);
`recovered_path` is that location, for the user to restore from with their own
file manager. A refresh failure is reported beside the item results and never
replaces them: the files were modified, and both facts are stated.

## Processing records and operations

A **record** is the current processing record of one `(library, operation)`
pair: the fixed, ordered set of 1–500 member folders a user selected, plus the
operation that works on them. `conversion` and `rename` are registered operation types. `rename` supports
draft, generation and review; its execution is refused. At most one
record exists per pair — that is a storage-level unique index, not an
application check. Decisions: [ADR 0001](adr/0001-library-workbench-and-processing-record.md).

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

### Creating the current record

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

### Versions

Two independent counters, never interchangeable:

| Version | Advances on | Guarded by |
| --- | --- | --- |
| `workset.version` (metadata) | rename | `If-Match` on `PATCH /worksets/{id}` |
| `operation.version` | draft save, revision publication | `If-Match` on draft save, generation start |

A rename therefore never dirties an operation, and
another operation's change never advances this operation's version. There is no
separate draft counter: `GET .../draft` returns the operation version to echo
back as `If-Match`.

### Routes

All routes require auth and use the standard error envelope.

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
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions` | Start generation. No request body; `If-Match` (operation version) and `Idempotency-Key` are both required. 202 `{"created":true,"generation":…}`; 200 with `revision` when nothing semantic changed, or 202 with `generation` for a key replay. Conflicts: `GENERATION_IN_PROGRESS`, `EXECUTION_IN_PROGRESS`, `SCAN_IN_PROGRESS`, `NO_ACTIVE_MEMBERS`, `VERSION_CONFLICT`; `INVALID_POLICY` is 400. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}` | Session detail. Statuses `queued`, `running`, `completed`, `failed`, `canceled`, `interrupted`. A session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/events` | SSE progress (`session_snapshot`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/cancel` | Cooperative cancel; idempotent on terminal sessions. |
| GET | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}` | The immutable snapshot of the record's **current** plan (a replaced plan no longer resolves — 404): `root_path`, `snapshot_token`, `status`, `summary`, the plan payload in its task envelope (`task: {kind, schema_version, payload}` — conversion: policy, classifier, summary, components), `counts`, frozen `members[]` (effective settings + per-unit `sources`), `roots[]`, `component_roots[]`, and `execution` (the session that ran this revision, if any). |
| POST | `/api/v1/worksets/{id}/operations/{type}/revisions/{planId}/executions` | Execute the operation's current revision (see below), optionally scoped to some of its folders. Body optional: `{"folder_paths":["albumA",…]}` — record-relative member paths; absent or empty runs the whole revision. The worklist and the session options (the obsolete-audio handling the draft declares) are always the frozen revision's. `If-Match` (operation version) and `Idempotency-Key` are both required. 202 `{"created":true,"execution":…}`; 200 with the same session for a key replay. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}` | Session detail, including each component's result. Optional `components_from` (default 0, inclusive) and `components_limit` (default: every remaining component) page the component list, which is ordered by `component_index`; a session of another workset or operation is 404. |
| GET | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/events` | SSE execution stream (`execution_snapshot`, `component`, `progress`, terminal event). |
| POST | `/api/v1/worksets/{id}/operations/{type}/executions/{executionId}/cancel` | Cooperative cancel; idempotent on terminal sessions (200 with the session either way). |

### Rename task (schema version 1)

Use the operation routes with `{type}=rename`. The draft payload is:

```json
{
  "schema_version": 1,
  "common": { "template": "[{rj}] {title}" },
  "members": [{ "member_id": "…", "excluded": true }]
}
```

Only exclusions are stored per member; naming overrides are rejected. Members
without a record participate. Common template saves use the deployment-wide
settings rules. Exactly one `{rj}` is required; the other allowed placeholders
are `{title}`, `{circle}`, `{voice_actors}`, `{language}`, `{release_date}`.
Unknown placeholders, unmatched braces, duplicate/unknown members and overrides
are `INVALID_DRAFT` (400); unknown versions are `UNSUPPORTED_SCHEMA_VERSION`
(400). Metadata text stays literal.

Each participating member produces one frozen component without modifying disk
or bindings. `task.payload.policy` holds the template;
`task.payload.components[]` contains `member_id`, `source_name`, `work_code`,
`raw_name`, `target_name`, `action` (`rename`, `noop`, `blocked`) and optional
`reason_code`. The classifier has empty tags. Used metadata must be nonempty.
Illegal filename characters become `_`; outer whitespace and trailing dots/spaces
are removed. The result must still identify exactly the bound Work. Names over
255 UTF-8 bytes are blocked without truncation or automatic suffixes.

Block reasons: `METADATA_MISSING`, `WORK_UNBOUND`, `SOURCE_MISSING`,
`SOURCE_NOT_DIRECTORY`, `NAME_TOO_LONG`, `NAME_RESERVED`, `WORK_CODE_AMBIGUOUS`,
`TARGET_EXISTS`, `TARGET_COLLISION` (multiple proposals), `CASE_ONLY_RENAME`
(Windows roots). Fingerprints cover binding, used metadata, source directory
state and relevant target occupancy. Sales, personal data, fetch timestamps and
unused fields do not stale plans. Review always shows frozen names. Execution
returns `PLAN_NOT_EXECUTABLE` (409), with `RENAME_EXECUTION_NOT_IMPLEMENTED` in
`details`, before an execution row is written.

### Metadata refresh task (schema version 1)

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

Planning freezes selected directories, copy/work bindings, physical directory
identities and settings without upstream requests or writes. Each participating
member produces a frozen component with `member_id`, `target` (`copy_id`, `rj`,
`library_root`, `rel_path`, `identity`), `action` (`refresh` or `skip`) and an
optional `reason_code`. An unbound directory skips with `NO_WORK_BINDING`;
a missing directory blocks with `SOURCE_MISSING`. Summary adds `work_count`
(distinct RJ); `operation_count` counts bound participating copies. Binding or
directory identity changes stale the plan; changing metadata or cover bytes does
not. Execution folder scope applies before deduplication and cover enqueueing.

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

### Copy covers and asynchronous delivery

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
then verifies binding, directory identity and safe paths again, and never creates
a replacement directory. A directory moved/removed/replaced fails only its target.

`fill_missing` skips exactly one valid supported `cover.*`; damaged files can be
replaced; multiple valid covers are `COVER_CONFLICT`. `refresh_all` publishes
`cover.{actual ext}` before removing old top-level supported `cover` names, never
other pictures. A durable publication journal completes extension cleanup on
retry after interruption; failed publication keeps the old cover. Successful
publication refreshes inventory and records its independent failure if necessary.
No permanent central cover cache or scaled variants are created.

### Callable metadata refresh service

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
edition/credit arrays remain empty. No parent guessing or recursive family fetch.

Static workers: 8; dynamic batches: 500, recursively split on HTTP 414. Shared
client limits: DLsite 8 in flight / 10 starts per second; ASMR.one 2 in flight /
3 starts per second; default timeout 10 seconds; response limit 16 MiB; at most
3 attempts for network/5xx errors. HTTP 403/429 or challenge headers stop further
source requests for at least 60 seconds, honoring longer `Retry-After`.
Cancellation stops new requests/imports. Neither scanning nor startup triggers
refresh automatically.

### Execution sessions

Audio execution applies the frozen file decisions. Metadata execution is described above; asynchronous cover publication has its own write lifecycle. It runs exactly one
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

**Fresh facts before writing.** A session refreshes the scanned inventory of
its roots through the scanner before its first write and re-judges the
revision's recorded inputs against what is on disk now. A drifted folder stops
the session as `failed` with `INPUT_CHANGED` and every file untouched — the
drift is caught at the start, never component by component halfway through. A
refresh that cannot complete (`SCAN_FAILED`) also leaves the disk alone.
Generation works the same way: its session scans the participating member
folders before planning, so a plan is never made from a stale inventory.

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
result is emitted before a terminal event that follows it, so a client that
watches a run to its end has seen every component it ran. There is no event-log
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
their row, committed outputs and `Delete/…` recovery files are refreshed with
the scan merge's `content_rev` semantics). A soft removal lands under the
library root (`<library root>/Delete/<member path>/…`), beside the member
folder rather than inside it, so recovered media never re-enters a member's own
inventory. A sync failure is disclosed per
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

### Operation draft document

The draft is one sparse document. The four common setting groups are the base;
a member record overrides only the units it explicitly replaces. The
operation-wide `delete_mode` (obsolete-audio handling: `soft` — the default —
or `hard`) is a common-level field, never a member override, and freezes into
every revision the draft produces; the execution uses the frozen value.

```json
{
  "schema_version": 1,
  "mode": "available_sources",
  "delete_mode": "soft",
  "classifier_tags": ["SEなし"],
  "matched":   {"lossless": {"codec": "wav"}, "encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
  "unmatched": {"lossless": {"codec": "wav"}, "encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
  "members": [
    {"member_id": "m-…", "overrides": {"matched": {"lossless": {"codec": "flac"}}}},
    {"member_id": "m-…", "excluded": true}
  ]
}
```

A profile names a codec per lane: `lossless` is `wav` or `flac` and carries no
quality; `encoded` is `mp3`, `aac` (an `.m4a`) or `opus` (an `.opus`) and
requires a positive `quality.bitrate`. The settings offer three named pairs —
Opus 160, AAC 256, MP3 320 — but a preset writes that same codec + bitrate pair,
so nothing about it is stored or validated differently from a hand-made one.

- **Absence means inheritance.** A unit missing from `overrides` follows the
  common value, whatever that value is now. Null is never the way to express
  "clear": `"classifier_tags": []` is an explicit empty set, and removing the
  key restores inheritance. The two behave differently after later common
  changes.
- **Sparse storage.** The server normalizes on save: a member record with
  neither an exclusion nor any override is dropped, records are stored in
  member order, and a missing `classifier_tags` on the common level is stored
  as `[]`. `GET` returns that canonical document, so a save/read round trip
  never materializes unmodified units.
- **Structural vs business validation.** Save rejects unknown JSON fields,
  wrong types, unknown/duplicate `member_id`, unknown `mode` or `delete_mode`
  values and unknown codec or quality literals. It accepts structurally valid
  but incomplete settings (empty tags, undeclared outputs, missing quality).
  Generation requires every participating member's effective settings to pass
  the full reconcile policy validation (`INVALID_POLICY`).
- **Exclusion** changes participation only: overrides and inheritance survive
  it, and restoring participation restores the previous relationships.
  Excluding every member saves but cannot generate (`NO_ACTIVE_MEMBERS`).
- **Common settings are deployment-wide** (ADR 0010). The five common values
  are stored once per operation kind, not per record: saving them from any
  record's draft pushes them to every other editable record's draft in the same
  transaction, and a record created afterwards seeds them. The PUT draft body
  may include `"apply_common_settings":true` to publish the current record's
  common values even when they did not change; this lets an older record
  establish the global value explicitly. Without a stored global value, a
  replacement keeps the replaced record's own common values, while a new
  record in another library starts from the built-in seed. A save that changes
  only member records stays inside its own record; a generated plan is never
  rewritten. A record whose common fields moved reports `needs_planning` and must be re-generated before
  it can run. Orphaned (read-only) records never take part, and a record whose
  session is queued/running refuses the whole save with `COMMON_SETTINGS_BUSY`
  — as does one whose stored draft cannot be read
  (`COMMON_SETTINGS_UNREADABLE`, which names the record to recreate) — instead
  of leaving some records updated.

### Frozen revisions

`GET .../revisions/{planId}` resolves the revision's frozen draft snapshot into
per-member effective settings and per-unit sources (`common` | `member`), so a
historical revision always reports the configuration it was planned with —
equal values with different provenance stay distinguishable, and later common
changes never rewrite history. `counts` reports independent facts
(`members`, `changed`, `unmet_targets`, `blocked`, `unchanged`) that may
overlap; no exclusive status label is derived from them.

Conversion modes (policy `mode`, applies to both partitions): `strict` (the
historical rules; missing mode = strict) and `available_sources` (relaxed per
Variant Group). A partition's declared profile is its final audio set in either
mode: missing declared outputs are materialized from the stem's qualified
lossless source, and files the profile does not declare — including the source
a declared output was encoded from — are removed once those replacements
commit. A profile that declares no output means the partition holds no managed
audio: every observed file in it is removed. `available_sources` differs in
`strict`'s failure behavior: a stem whose declared shape cannot be reached is
left exactly as it is and recorded as `UNMET_TARGET` (no generation, no
cleanup), where strict blocks the whole Component with zero operations.
Ambiguity and path conflicts block in both.
An encoded output is accepted by two entries, in this order:

1. **Its own measured rate**, compared per codec — a file that already measures
   its target is not touched, whoever wrote it:

   | Codec | Accepted when |
   | --- | --- |
   | MP3 | probed bitrate at or above the target (1 kbps tolerance for probe under-reporting) |
   | AAC | inside `[0.95, 1.10]x` of a target up to 192k, inside `[0.60, 1.10]x` above it |
   | Opus | probed average at or above the target |

   MP3 is written CBR and lands exactly on target — the number leaves nothing to
   ask, so MP3 is judged by it alone. AAC is exact while the encoder can honour
   the target and saturates near 222 kbps for stereo 44.1 kHz above it (measured:
   1.00x at 192k, 0.86x for a 256k target, 0.65x for 320k), so above that the
   floor admits what can be produced instead of rebuilding its own output
   forever; the ceiling replaces a file well above the declared shape like one
   below it. A bitrate no probe can establish never accepts.
2. **A generation record.** An execution writes one per committed encoded output
   — codec, encoder, target bitrate, rate-control mode (CBR/VBR), the encoder
   version, the output's size and mtime, and the SHA-256 of its bytes — and only
   after the stream check and the full decode passed, so the record's presence is
   the verification. It accepts the file only when it names exactly the current
   codec, bitrate and mode *and* its size and mtime still match what the
   inventory observes (the same content proxy the scan and the inventory
   fingerprint use; the hash is what keeps a deeper re-check possible later).

The second entry is what settles the codecs a measurement cannot judge: Opus is
written VBR, where the same 160k setting averages 0.01x on silence, 0.68x on
dense noise and 1.13x on sparse material, and AAC saturates above 192k as above.
A file neither entry accepts is **unconfirmed**, never silently adequate — and no
measured rate is evidence of quality either, since a low-bitrate file re-encoded
to a high target would not regain what it never had. Where a qualified lossless
source exists the target is regenerated once and recorded; where none exists the
file is kept as it is and the stem counts as `UNMET_TARGETS` (relaxed mode marks
that kept file `CONFORMANCE_UNCONFIRMED`; strict mode blocks the component with
the same code and zero operations). Lossless targets carry no record: a lossless
file is accepted on its codec.

`StepSummary.unmet_targets` counts kept-but-unsatisfied stems; `summary_reason`
may be `UNMET_TARGETS`. New operation drafts seed `mode: "available_sources"`.

A record's scope is fixed once created: it is replaced, never re-scanned in
place. `folder_paths` come from `GET /api/v1/libraries/:id/dirs` of the owning
library, as library-relative paths.

## Reusable policy slots and classifier tags

These authenticated routes supply reusable values; edits never rewrite saved
operation drafts or revisions.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/policy-slots` | 200 `{"slots":[{"slot":1,"name":"…","policy":null},…]}`; exactly three slots, initially unconfigured. |
| PUT | `/api/v1/policy-slots/{slot}` | Body `{"name":"…","policy":{…}}`; 200 slot object. Slot must be 1–3, trimmed name 1–120 characters, policy complete and valid. Errors: `INVALID_SLOT`, `INVALID_SLOT_NAME`, `INVALID_POLICY` (400). |
| GET | `/api/v1/classifier-tags` | 200 `{"default_tags":[…],"custom_tags":[{"id":1,"tag":"…","created_at":"…"}]}`. Defaults come from `prune.literal_tags` and are read-only here. |
| POST | `/api/v1/classifier-tags` | Body `{"tag":"…"}`; 201 custom tag object. Empty trimmed tags return `INVALID_TAG` (400). |
| DELETE | `/api/v1/classifier-tags/{id}` | 204; invalid IDs return `INVALID_ID` (400), missing tags `NOT_FOUND` (404). |

Applying a slot is a client edit that copies policy values into the operation
draft and uses the normal version-guarded save. There is no server-side live
slot reference, slot creation or slot deletion endpoint.

## Work catalog

The catalog is the logical layer above the inventory: works identified by their
work number, the source metadata imported for them, the directories bound to
them, and the single user's data about them. The directory name is the only
authority on which work a directory holds, so there is no route that creates a
work or rebinds a copy — scanning does that (ADR 0011 §2). Every work number in
a path or body is accepted in either case and in its 6- or 8-digit spelling, and
resolves to the canonical 8-digit form; `RJ123456` and `RJ00123456` are the same
work, while `RJ1657200` (7 digits) is refused.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/works` | 200 `{"works":[…],"next_cursor":"…"}`. Filters and paging below. |
| GET | `/api/v1/works/{rj}` | 200 the work in full: metadata, personal data, resolved relation, language editions and every copy. A work declared by a relation but never found on disk answers `"placeholder":true` and is absent from the listing. |
| GET | `/api/v1/inventory/media-facts/events` | Bearer-authenticated SSE; `changed` with `{}` after a committed measurement batch, and once on connection. See below. |
| GET | `/api/v1/works/{rj}/content` | 200 the work's file manifest (ADR 0016 §2): every bound copy's tree, below. |
| GET | `/api/v1/works/{rj}/content/{copyId}/{path...}` | 200 one manifest file's bytes (ADR 0016 §3): `path` is the copy-root-relative manifest path. See below. |
| GET | `/api/v1/works/{rj}/thumbs/{copyId}/{path...}?h=240&v=…` | 200 one manifest image's thumbnail (ADR 0015 §3). See below. |
| PUT | `/api/v1/works/{rj}/metadata` | Body: up to four independent data groups (below). 200 `{"applied":[…],"ignored":[…],"warnings":[…],"created_placeholders":[…]}`. |
| PATCH | `/api/v1/works/{rj}/user-data` | Body `{"rating":8,"favorite":true,"note":"…"}`; only the fields present are applied, and `"rating":null` clears it. 200 the resulting state. |
| PUT | `/api/v1/works/{rj}/user-tags` | Body `{"tag_ids":["…"]}`; replaces the work's tag set with exactly these. 200 `{"user_tags":[…]}`. Unknown ids are `USER_TAG_NOT_FOUND` (404). |
| GET | `/api/v1/circles` | 200 `{"circles":[{"id":"…","name":"…","work_count":3}]}`: every circle a listed work carries, ordered by name. `library` (comma-separated ids) scopes the circles and their counts to works with a copy there; any other parameter is `INVALID_ARGUMENT` (400). |
| GET | `/api/v1/voice-actors` | 200 `{"voice_actors":[…]}`, the same shape and scoping for voice actors. Each `id` is what the listing's `circle` / `voice_actor` parameter takes. |
| GET | `/api/v1/user-tags` | 200 `{"user_tags":[{"id":"…","name":"…"}]}`. |
| POST | `/api/v1/user-tags` | Body `{"name":"…"}`; 200 the tag, **creating it if new and returning the existing one otherwise** — creating by name is idempotent. |
| PATCH | `/api/v1/user-tags/{id}` | Body `{"name":"…"}`; 200 the tag, `USER_TAG_EXISTS` (409) on a name another tag holds. |
| DELETE | `/api/v1/user-tags/{id}` | 204; the tag's relations go with it, no work is touched. |
| DELETE | `/api/v1/works/{rj}` | 204; `WORK_IN_USE` (409) with `details[]` while a copy is bound or another work's evidence points at it. |

Ratings are integers 1–10 (`null` means unrated, which is not 0). A work that
exists only as a relation placeholder holds no personal data: writing it is
`WORK_IS_PLACEHOLDER` (409). User tag names are trimmed, must be non-empty, and
are unique as exact text — case is not folded and source tags are a separate
thing.

### Work content manifest

`GET /api/v1/works/{rj}/content` answers the work's files from the **stored
inventory** — never a live walk — as one tree per bound copy (ADR 0016 §2):

```json
{
  "rj": "RJ01000001",
  "copies": [
    { "copy_id": "…", "library_id": "…", "dir_id": "…", "online": true,
      "tree": { "name": "RJ01000001", "rel_path": "", "children": [
        { "name": "表紙", "rel_path": "表紙", "children": [
          { "name": "cover_main.jpg", "rel_path": "表紙/cover_main.jpg",
            "kind": "img", "size": 4096, "format": null, "bitrate": null,
            "width": 1200, "height": 900, "duration_sec": null,
            "thumb_v": "9f86d081884c7d65" } ] },
        { "name": "01_導入.mp3", "rel_path": "01_導入.mp3",
          "kind": "aud", "size": 5120, "format": "MP3", "bitrate": 320,
          "width": null, "height": null, "duration_sec": 185.5,
          "thumb_v": null } ] } }
  ]
}
```

`dir_id` is the copy root's navigation identity (ADR 0003), resolved
server-side. A copy whose member directory no longer resolves on disk answers
`"online": false` with no `tree` — stale is worse than absent. A placeholder
work answers an empty `copies` array, not 404. `kind` classifies each file
(`aud`, `img`, `txt`, `lrc`, `bin`); directories carry `children` (empty
included) and files carry no `children`, always carry `size`, and carry
`format` only for audio, as the container name the track list aggregates by.
`width`/`height`/`duration_sec`/`bitrate` are the inventory's facts (ADR 0015
§2): `null` means unmeasured, not zero. Dimensions and durations are measured
in the background after a scan completes, so a freshly scanned work can answer
`null` for a while. `thumb_v` is an image's opaque
thumbnail version (`null` for every other kind), the `v` its thumbnail address
carries.

`GET /api/v1/works/{rj}/content/{copyId}/{path...}` serves one manifest
file's bytes. The route is read-only and takes no admission slot: only paths
the manifest holds are served, resolved inside the member under the same
containment checks direct file management applies (out-of-bounds segments and
symlink traversal is refused; an unscanned file on disk is 404, not a guess).
Directories are refused, and the opened object must still be a regular file.

- Besides the `Authorization` header, this route (and only this one) accepts
  the token as `?access_token=…`, so an `<img>` or `<audio>` element can load
  it by address. Only a completely absent Authorization header permits this
  fallback: malformed, empty or incorrect header credentials are refused.
- `ETag` keys on `(size, mtime, content_rev)`; `Cache-Control: no-cache`
  revalidates before reuse, so a rescan that changed the bytes changes the
  tag and a match answers 304. Original image addresses may carry optional
  `v=thumb_v` to reload a changed path; this does not change file cache semantics.
- `Accept-Ranges: bytes` with single-range 206 (`Range: bytes=0-99`). A
  multi-range request is answered with the whole file; an unsatisfiable range
  answers 416 with `Content-Range: bytes */<size>`.
- Images carry the type recognized from their bytes; unrecognized image
  bytes use `application/octet-stream`, regardless of extension. Audio can
  fall back to the container extension when byte sniffing is inconclusive.
  Text (`txt`/`lrc`) declares the character set the server
  sniffed (`UTF-8` → `Shift_JIS` → `GB18030` candidates) and the body is
  served exactly as stored — decoding happens once, at the client, per the
  header, using `arrayBuffer()` and `TextDecoder` (not `Response.text()`).
  A multibyte character cut off at the sample boundary is not an invalid
  encoding; truncated characters at actual EOF remain invalid.

`GET /api/v1/works/{rj}/thumbs/{copyId}/{path...}?h=…&v=…` serves one
manifest image scaled to a height (ADR 0015 §3). The source goes through the
same copy binding, containment and whitelist as the file route; a non-image
path is `NOT_AN_IMAGE` (404). Read-only, no admission slot.

- `h` (required, positive) snaps up to a served height: 120, 240, 480 or 960.
  A source shorter than that is never upscaled.
- `v` is the image's `thumb_v` from the manifest, a digest of the file's size,
  mtime, `content_rev` and the engine version. Successful responses and their
  matching 304 at the current version have
  `Cache-Control: private, max-age=31536000, immutable`; any other
  `v` (or none) still answers, but `no-cache`, so a stale address never pins
  old bytes. `ETag` is `"<v>-<height>"`; a match answers 304.
- Opaque sources answer `image/jpeg`, sources with transparency `image/png`.
- `BUSY` (503) with `Retry-After` when the render queue is full — back off and
  retry. `IMAGE_TOO_LARGE` (422) over the pixel limit (about 100M pixels,
  checked from the header), `IMAGE_UNDECODABLE` (422) for bytes that do not
  decode; the page draws its hatched placeholder for both. Errors (including
  500) use `Cache-Control: no-store` and carry no successful-content ETag.
  Allowed cross-origin responses expose `Retry-After` to the client.

### Background media facts notifications

`GET /api/v1/inventory/media-facts/events` is a bearer-authenticated SSE
connection; query-token authentication is not accepted. It responds with
`Content-Type: text/event-stream` and `Cache-Control: no-cache`.

```text
event: changed
data: {}

```

A successful batch commit sends one notification; failed writes send none.
Each subscriber has only one pending notification, so slow clients coalesce
changes without blocking measurement. Every connection receives an initial
notification to cover changes while disconnected. A `: keep-alive` comment
is sent every 15 seconds; disconnection unregisters the subscriber. There are
no event IDs, replay history or persistent event fields.

The root application maintains one connection, reconnecting after 1, 2, 4
seconds up to 30 seconds; 401 stops retries, and unmount cancels both the stream
and pending wait. Notifications cancel any older in-flight reads, refresh
active work-content and member-tree queries, and remove inactive caches.
Scanning and file/library mutations also refresh or remove work lists, details
and manifests. Rebinding a manifest preserves reading, lyric pairing and
playback. Thumbnail blobs are keyed by copy, path and `thumb_v`; changed
versions revoke the old blob and reload both thumbnail and original image.

### Source metadata import

The body carries up to four **independent** groups; a group that is absent is
left exactly as it was, and a group that is present is a complete snapshot of
that group — an unknown scalar is an explicit `null`, and an empty array means
the set is empty. A submission with no group at all is refused.

```json
{
  "core": {
    "source": "dlsite",
    "fetched_at": "2026-09-27T10:00:00.000000000Z",
    "title": "…",
    "circle": { "source_circle_id": "RG01038998", "name": "mete0r" },
    "nsfw": true,
    "release_date": "2025-06-25",
    "source_url": "https://…",
    "cover_url": "…", "thumbnail_url": "…", "sample_cover_url": "…",
    "series_id": null, "series_name": null,
    "voice_actors": [{ "source_voice_actor_id": "17180", "name": "伊ヶ崎綾香" }],
    "tags": [{ "source_tag_id": "497", "name": "ASMR" }]
  },
  "statistics": { "fetched_at": "2026-09-27T10:00:00.000000000Z", "dl_count": 12 },
  "relations": {
    "source": "dlsite",
    "fetched_at": "2026-09-27T10:00:00.000000000Z",
    "role": "child",
    "language": "CHI_HANT",
    "original_rj": "RJ01276487",
    "parent_rj": "RJ01412616",
    "child_rjs": []
  },
  "editions": {
    "source": "dlsite",
    "fetched_at": "2026-09-27T10:00:00.000000000Z",
    "edition_id": "51904",
    "items": [{ "version_root_rj": "RJ01506610", "language": "JPN", "label": "日本語", "display_order": 1 }]
  }
}
```

`relations` and `editions` are separate groups on purpose: `translation_info`
is available on DLsite's dynamic endpoint, `language_editions` only on its
static one, so a crawler refreshing sales cheaply must not be forced to
resubmit — or worse, silently blank — the group it did not fetch
(ADR 0011 §4). `statistics` carries no `source`: sales are only ever reported
by DLsite. `fetched_at` must be UTC with nine fractional digits; other shapes
are `INVALID_TIMESTAMP` (400).

Within a group, a higher-priority source (DLsite) replaces a lower-priority one
(ASMR.one) outright, and the same source only moves forward: an equal or older
fetch is skipped, reported in `ignored[]` with `STALE_FETCHED_AT` or
`LOWER_PRIORITY_SOURCE`. `warnings[]` is a different thing — those groups
*were* written, and carry `RELATION_CONFLICT` or `RELATION_AMBIGUOUS` when a
declaration disagrees with the work's own relation report (the work's own
report always wins). Anything structurally illegal — a self-reference, an
original chain, a cycle among effective parents, a malformed number — refuses
the **whole** submission as a 400 and writes nothing. An import never touches
personal data or copy bindings, and a work it names but the catalog has not
seen on disk is created as a relation placeholder, listed in
`created_placeholders[]`.

### Listing filters, ordering and paging

`GET /api/v1/works` accepts exactly these parameters; anything else is
`INVALID_ARGUMENT` (400), so a typo cannot silently widen the answer.

| Parameter | Meaning |
| --- | --- |
| `q` | Literal substring of the work number or the title. `%` and `_` are ordinary characters, not wildcards. A query that *is* a work number also matches that work exactly, so the short spelling finds it. |
| `circle`, `language`, `library` | Comma-separated id lists; a work matches if it has any of them. |
| `voice_actor` / `source_tag` / `user_tag` | Comma-separated id lists, each **requiring** its `…_match=all\|any` companion (`MATCH_MODE_REQUIRED`, 400, if absent). |
| `nsfw` | `true`, `false` or `unknown` (a work that never reported the flag). |
| `released_from` / `released_to` | `YYYY-MM-DD`, inclusive; works with an unknown release date are not in the range. |
| `rating_from` / `rating_to` | Integer 1–10, inclusive. |
| `unrated` | `true`/`false`. It **unions** with the rating range inside that dimension: `rating_from=8&unrated=true` means "8 or more, or not rated". |
| `favorite`, `has_copy` | `true`/`false`. |
| `sort` / `direction` | `rj` / `release_date` / `collected_at` / `rating`, and `asc` / `desc`. |
| `limit`, `cursor` | Page size and continuation (below). |

Different dimensions intersect. An absent `limit` is 50; a supplied one must be
an integer 1–200, so `limit=abc`, `0`, `-1`, `201` and `1.5` are all
`INVALID_PAGE` (400) rather than being replaced or clamped.

The cursor is opaque and carries the sort position plus a fingerprint of the
query. Changing a filter or the ordering invalidates it
(`CURSOR_QUERY_MISMATCH`, 400) instead of paging through a different sequence;
changing only `limit` does not. Unknown release dates and unrated works always
sort last, in either direction, and equal sort values are broken by work number
so a page boundary cannot repeat or skip a row. There is no cross-request snapshot: a
concurrent metadata change can move a later page's contents.

The listing returns the work number, title, thumbnail, circle, language, adult
flag, release date, **sales**, personal rating, favorite flag, user tags,
whether a note exists, when it was first collected, and its copy count. It does
not return the note text, the full edition list, relation details or the file
tree.

## Paths: the one rule

**The frontend echoes paths verbatim; the backend normalizes them, and only
the backend resolves them.**

The Vue frontend displays and sends the original user strings — it never
touches path separators, never joins or resolves paths, and never uses any
`path` module. Absolute paths (`root_path`) are normalized to POSIX form in Go
(`backend/go/internal/pathnorm`) before they reach the storage adapter or the planning rules.

Paths *inside* a library are relative, and the backend resolves them: a
directory listing returns `rel_path`, a member tree is read by `?dir=<dir_id>`
(the identity derived from that path — see *Directories and member trees*), and
file management addresses items relative to their member. A relative path is validated as a plain descendant chain before it is
joined to a root — absolute paths, drive or device prefixes, backslashes,
`..`, `.` and empty segments are refused — and every write re-checks that the
resolved path is still inside the member it was resolved against.

## Shutdown

On `SIGINT`/`SIGTERM` (or stdin EOF), the backend drains the HTTP server
(in-flight SSE scans included) under a single 5-second forced-exit guard.
