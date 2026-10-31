# Libraries, scans and file management

[API reference](../api.md) · [Common conventions](common.md)

## Endpoints

| Method | Path | Auth | Status | Response |
| --- | --- | --- | --- | --- |
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
| PUT | `/api/v1/libraries/:id/operations/:type/current` | yes | 201/200 | record + `skipped[]` (see [Processing records](worksets.md#creating-the-current-record)) |

## Library object

```json
{
  "id": "uuid",
  "name": "Music",
  "root_path": "/home/me/music",
  "storage_class": "hdd",
  "created_at": "2026-08-22T00:00:00Z",
  "updated_at": "2026-08-22T00:00:00Z",
  "last_scan_at": null,
  "last_scan_status": "",
  "last_scan_error": ""
}
```

`POST /api/v1/libraries` body: `{"name":"...","root_path":"/abs/path"}`.
New libraries default to `storage_class: "hdd"`. `PATCH` applies only the fields
present (`name`, `root_path`, `storage_class`). `storage_class` accepts exactly
`"hdd"` or `"ssd"`; any other value, including empty strings and `null`, returns
400 `INVALID_STORAGE_CLASS`. An omitted value preserves the current setting,
including when edits to different fields overlap. Renaming a library
preserves its storage class; deleting it deletes that setting with the library.
Storage-class edits need no idle admission slot: started execution sessions and
media-facts batches keep their captured concurrency, and later work reads the
new value. Queued execution sessions capture it when the worker starts processing
them, before session preparation. Neither worker counts nor a second SSD library
list are file configuration.

Library roots must be disjoint: neither configured root may be a strict
ancestor of another. Creation and a canonical root change return 409
`LIBRARY_ROOT_OVERLAP` for containment in either direction. Canonical duplicates
keep 409 `LIBRARY_EXISTS`. Checks use the normal lexical root identity rules:
POSIX case is significant; drive and UNC paths compare without case; component
boundaries keep `/a`, `/a b` and `/ab` distinct. They do not traverse media or
resolve filesystem aliases.

A pre-existing overlapping configuration is left intact for repair. Names,
storage class and identity-preserving root spelling edits remain available;
deleting a conflicting library or moving its root to a disjoint location
unblocks scanning. Media files are untouched.

Changing
`root_path` clears the scanned inventory of the old root and the prior scan
state; the library must be scanned again before anything is planned from it.
A root change is rejected with `LIBRARY_HAS_WORKSETS` while the library still
has a processing record: delete the library — its record goes with it — and
re-create it at the new root.
Both a root change and a deletion take the direct-file-management slot, so
neither interleaves with file management (ADR 0002).

`DELETE /api/v1/libraries/:id` removes the library **together with its
processing record**: the record, its members, operations, drafts, plans and
execution results become inaccessible in the same transaction, so no orphaned
record survives its library. Their stored payload is then removed in background
batches. The stored inventory and copy bindings of the library root are removed
synchronously. Retirement cancels pending and running cover demands under the
cover publication guard, so a cover publishes before deletion commits or is
blocked at the final demand check. Media files, published covers
and all `Delete/` recovery directories on
disk are never touched. Deletion is blocked by active operation generation
(`GENERATION_IN_PROGRESS`) and by an active execution session
(`EXECUTION_IN_PROGRESS`).

## Scan: POST + SSE

`POST /api/v1/libraries/:id/scans` takes a JSON object body; an empty body is
read like `{}`. The optional `mode` field accepts only `"full"`:
omitting it runs a Library Scan, which reuses eligible unchanged directories;
`{"mode":"full"}` runs a Full Rescan, reading every directory without reuse.
Every other supplied value, including `""`, `null` and non-string values,
returns 400 `INVALID_SCAN_MODE` before admission or SSE starts. Background
task-driven scans use the default Library Scan mode.

An optional `root_path` is retained for
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

`files_scanned` includes both newly observed files and files carried forward
from reused directories; it is the reconciled file count, not a count of
filesystem metadata reads. Directories still count toward `dirs_scanned` when
their files are reused.

Library Scan has two known limits: it cannot detect an external tool's in-place
file rewrite when that directory's mtime stays unchanged, and client filesystem
attribute caches can temporarily hide a directory change. The latter is caught
by a later scan after the cache expires. Use Full Rescan to read the files in
unchanged directories, for example after editing tags in place.

`Delete/` is a reserved recovery directory name at every depth. Such directories
and their descendants are skipped before reading their metadata or traversing
them, including old library-level recovery directories. They are not stored in
the inventory or counted in scan progress. A successful full rescan removes
recovery entries and copy bindings left by older scans. Windows paths compare
the name case-insensitively; POSIX paths compare it exactly.

Library Scans, Full Rescans, member refreshes and selected-folder scans reject an affected
pre-existing overlapping root with `LIBRARY_ROOT_OVERLAP`; unrelated sibling
libraries remain usable. The check applies to the configured library root,
not to the member or selected scope. HTTP scan and tree-refresh admission
return 409 JSON before starting SSE or traversal; refusal leaves inventory
unchanged. A conflict detected after a stream has started retains the same
code in its terminal `error` event. Internal task-driven scans apply the same
validation before traversing media.

A scan is refused with `EXECUTION_IN_PROGRESS` (409) while any record of the
library has a queued/running execution: an execution validates the inventory
the scan would rewrite. It is refused with `GENERATION_IN_PROGRESS` (409) while
the library has a queued/running planning, which refreshes the same inventory
itself, and with `SCAN_IN_PROGRESS` (409) while another scan of the library is
queued, running or merging. Other libraries' planning and scans do not matter.
It is also refused with `BUSY` (409) while direct file
management holds the admission slot, and it takes the scanning side of that
slot itself, so a file operation refuses a running scan in turn (ADR 0002).

## Directories and member trees

`GET /api/v1/libraries/:id/dirs` lists **every scanned work root** of the
library, whether or not it holds audio, with that directory's subtree
counts. Recovery directories at every depth are not members and are left out:

```json
{
  "dirs": [
    { "name": "albumA", "path": "/home/me/music/albumA", "rel_path": "albumA",
      "dir_id": "4b7fa32eebf5b001bcf43b1976c1b163",
      "audio_file_count": 4, "file_count": 7,
      "added_at": "2026-05-01T08:00:00Z", "work_code": null }
  ]
}
```

`added_at` is when a scan first saw the directory; rescans never move it.
`work_code` is the canonical work number (`RJ` + eight digits) when the
directory name carries exactly one, otherwise `null`.

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
file management addresses. `rj` is the work number of the Copy bound to that
directory, or `null` when the directory is unbound (the refresh route answers it
the same way):

```json
{
  "dir_id": "4b7fa32eebf5b001bcf43b1976c1b163",
  "member_path": "albumA",
  "rj": null,
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

The identity is resolved against the library's scanned inventory — the work
roots its listing shows, recovery directory excluded — and only the
single match is then checked on disk. The refusals are distinct:

| Condition | Answer |
|---|---|
| No `dir` parameter | 400 `DIR_ID_REQUIRED` |
| Not 32 lowercase hex characters | 400 `DIR_ID_INVALID` |
| No directory of this library has that identity | 404 `DIRECTORY_NOT_FOUND` |
| More than one directory claims it | 409 `DIRECTORY_AMBIGUOUS` (never the first match) |
| A work root in the inventory that is gone from disk | 404 `MEMBER_MISSING` |
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

## Direct file management

`POST /api/v1/libraries/:id/file-operations` is the second, explicitly
non-plan write path (ADR 0002). It holds the direct-file-management slot
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
| `move` | one item; `target_dir` is an existing directory of the same member, or `"."` for the member root (`INVALID_TARGET_DIR`, `MOVE_INTO_SELF`) |
| `soft_delete` | one or more items, applied in order |

A refused request (nothing ran) answers `400` with the refusing code itself (`MEMBER_MISSING`, `PATH_INVALID`, `SYMLINK`, `OPERATION_DENIED`, …), and `REQUEST_REFUSED` only for other errors. A rename that changes only letter case of the same file is allowed on case-insensitive filesystems (ADR 0002).

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
the work name and the item's member-relative path under
`<work parent>/Delete/<work name>/` and never overwrites media recycled earlier
(a collision becomes `<stem>.N<ext>`). For example, deleting
`分类/RJ000001/disc/track.flac` recovers it as
`分类/Delete/RJ000001/disc/track.flac`;
`recovered_path` is that location relative to the library root, for the user to restore from with their own
file manager. A refresh failure is reported beside the item results and never
replaces them: the files were modified, and both facts are stated.

Work roots may be nested beneath classification directories. The list exposes the
basename as `name` and the full library-relative path as `rel_path`; `dir_id`
keeps the existing identity algorithm. A work's internal RJ directories are not
separate list rows. Browsing, refresh and file operations resolve the same scanned
root and reject symbolic links along its entire ancestor chain, invalid paths,
escapes and any `Delete` subtree.

`completed.scan_id` still identifies this scan session. An entry's stored `scan_id`
is the last scan that actually wrote it; unchanged rescans leave `updated_at`
untouched. Staging records every observed entry for stale deletion. Changed size,
mtime or file/directory type invalidates media facts and advances the content
revision; other observed changes preserve facts and error status.
