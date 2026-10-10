# Common conventions

[API reference](../api.md)

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

The work catalog adds: `INVALID_PAGE` / `MATCH_MODE_REQUIRED` / `INVALID_TAG_NAME` /
`INVALID_TIMESTAMP` (400), `WORK_NOT_FOUND` / `USER_TAG_NOT_FOUND` (404),
`WORK_IN_USE` / `USER_TAG_EXISTS` / `WORK_IS_PLACEHOLDER` (409). A refusal may
carry `details[]` naming the references that hold a work in place.

## Paths: the one rule

**The frontend echoes paths verbatim; the backend normalizes them, and only
the backend resolves them.**

The Vue frontend displays and sends the original user strings — it never
touches path separators, never joins or resolves paths, and never uses any
`path` module. Absolute paths (`root_path`) are normalized to POSIX form in Go
(`backend/go/internal/librarypath`) before they reach the storage adapter or the planning rules.

Paths *inside* a library are relative, and the backend resolves them: a
directory listing returns `rel_path`, a member tree is read by `?dir=<dir_id>`
(the identity derived from that path — see [Directories and member trees](libraries.md#directories-and-member-trees)), and
file management addresses items relative to their member. A relative path is validated as a plain descendant chain before it is
joined to a root — absolute paths, drive or device prefixes, backslashes,
`..`, `.` and empty segments are refused — and every write re-checks that the
resolved path is still inside the member it was resolved against.

## Shutdown

On `SIGINT`/`SIGTERM` (or stdin EOF), the backend seals request and worker
admission and cancels active work. Requests arriving after the seal receive
503 / `SHUTTING_DOWN`. HTTP request contexts are canceled, including SSE scans;
connections have four seconds to drain, and closing them does not count as
handler completion. Planning and execution sessions stopped by shutdown become
`interrupted`; an explicit user cancel remains `canceled`.

The backend joins admitted handlers, dispatcher workers, maintenance, media
facts and cover workers before closing SQLite. File publication and its database
finalization finish through their protected boundaries. One five-second
forced-exit guard covers all joins and database Close. At its deadline the
process logs outstanding work and exits with code 1 without closing a database
still used by workers. Startup marks leftover sessions interrupted; a forced
exit can still leave unfinished filesystem changes requiring inspection.

## Health endpoint

| Method | Path | Auth | Status | Response |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/health` | no | 200 | `{"ok":true,"version":"dev"}` |
