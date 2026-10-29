# Common conventions

[API reference](../api.md)

## Startup handshake

On startup the backend unconditionally prints one readiness line to stdout
(alongside the [Diagnostic Log](diagnostics.md#output), whose lines are
logfmt). It is the only line not written as logfmt and contains no credentials:

```
PREEN_BACKEND_READY version=%s http_port=%d
```

- `http_port` — HTTP port
- `version` — build version stamp (`dev` by default)

The Vue dev script and the Playwright launchers scan stdout for the
`PREEN_BACKEND_READY` line and read the HTTP port from it.

## Server configuration

The backend reads `server` from `config.json` beside its database once at
startup. These settings currently have no frontend controls:

```json
{
  "server": {
    "allow_lan": false,
    "port": 0,
    "token": ""
  }
}
```

- `allow_lan`: false binds `127.0.0.1`; true binds `0.0.0.0` (all local IPv4
  interfaces, with reachability determined by deployment and firewall rules).
- `port`: an integer from 0 to 65535. Zero requests a system-assigned port and
  is allowed only for local access. A configured port that cannot be bound
  fails startup; the backend never silently chooses another port.
- `token`: the single shared bearer key for all protected API callers. A
  nonempty `PREEN_TOKEN` overrides the file key; an empty or absent environment
  value leaves the file key in effect. Keys must be visible ASCII without
  spaces, so they fit HTTP headers.

LAN access requires a fixed nonzero port and a nonempty effective key. For
example, set `allow_lan` to true, `port` to 8765 and `token` to a generated
random secret. Clients then use `http://<server-ip>:8765/api/v1` with that key.
There is no automatic key generation or rotation.

Startup flags override only their corresponding fields when explicitly passed:
`--allow-lan` (or `--allow-lan=false`) overrides `server.allow_lan`, and
`--port=<n>` overrides `server.port`, including an explicit zero. The same
validation applies to the effective settings; `PREEN_TOKEN` still overrides
the file key. For example:

```sh
preen --allow-lan --port=8765 --web-dir=/app/web
```

`--web-dir=<directory>` serves a built application on the API's port. It requires
a directory containing `index.html`; without the flag only the API is served.
Non-API GET requests serve an existing file or fall back to `index.html` for
history routes. Static files require no authentication. `index.html` and other
unversioned files use `Cache-Control: no-cache`; files with a content hash in
their name use `public, max-age=31536000, immutable`. Traversal paths, including
encoded traversal and symlinks leaving the web root, are refused.
Paths under `/api/` never fall back to HTML: unknown paths return a JSON
404 / `NOT_FOUND`, and unsupported methods return a JSON 405 /
`METHOD_NOT_ALLOWED` with an `Allow` header.

A missing configuration file preserves local developer defaults. An unreadable
or malformed file, invalid port, invalid key, or incomplete LAN configuration
fails startup before opening the database. Changes require a backend restart.

## Auth

Protected routes accept credentials in this order; a present invalid higher-priority
credential is refused rather than falling back:

1. `Authorization: Bearer <token>` (desktop and script callers).
2. `preen_token` HttpOnly cookie (same-origin browser callers).
3. `access_token` query parameter, only on the Work file-content and thumbnail
   routes used by `<img>` and `<audio>`; other routes never accept query credentials.

The cookie contains the shared key encoded with unpadded base64url; malformed
encoding is invalid. The decoded key is compared in constant time. The effective
key comes from the server configuration and optional environment override above.
An empty key disables auth only in local-developer mode; LAN mode refuses to
start without a key. Missing or invalid credentials return `401` with
`{"code":"UNAUTHORIZED","message":"..."}`. Keys and cookie values are never logged.

Cookie-authenticated requests other than GET and HEAD require the same origin:
when `Sec-Fetch-Site` is present its value must be `same-origin`. Otherwise,
when `Origin` is present, its parsed host (including the port) must exactly
match the request `Host`. Requests with neither header are treated as
non-browser requests and allowed. Failed checks return `403 FORBIDDEN`.
Bearer authentication and GET/HEAD reads (including media and event streams)
are exempt. `Sec-Fetch-Site` takes precedence over `Origin`.

## Session

| Method | Path | Auth | Status | Request |
| --- | --- | --- | --- | --- |
| POST | `/api/v1/session` | no | 204 | `{"token":"<shared key>"}` |
| DELETE | `/api/v1/session` | no | 204 | none |

A successful login sets `preen_token=<base64url(key)>; HttpOnly; SameSite=Strict;
Path=/api/v1; Max-Age=34560000` (400 days). `Secure` is added when the request
has TLS or `X-Forwarded-Proto: https`. The response has no body. Invalid keys
return `401 UNAUTHORIZED` without setting a cookie; malformed JSON uses the
usual `400 INVALID_ARGUMENT` envelope (oversized bodies return 413).
There is no server-side session state or session-probe endpoint: restarting
preserves login, and changing the shared key invalidates all old cookies.
Login rate limiting is outside the trusted-LAN deployment scope.

DELETE expires `preen_token` with an empty value and `Max-Age=0` at the same
`/api/v1` path, preserving HttpOnly, SameSite and conditional Secure attributes.
It succeeds even when credentials are missing or invalid. It clears this
browser's cookie; it does not rotate the shared key or invalidate other clients.
The settings page waits for logout to succeed, then replaces the entire document
with `/login`, ending playback and event streams and discarding cached data.
Pages restored from the browser's back/forward cache reload before reusing data.

Browser API and event-stream `401` responses replace the whole document with
`/login?redirect=<current address>` once. A successful login replaces the
document with the validated same-origin return address, or `/` if invalid or
resolving to login. This restarts event subscriptions, media and query caches.
Desktop callers continue to use Bearer and file query credentials.

## CORS

`PREEN_CORS_ORIGINS` is a comma-separated allowlist of browser origins. The
default allowlist is empty: no cross-origin browser receives CORS permission
headers unless explicitly configured. Development browsers use same-origin
`/api/v1`; Vite proxies `/api` to the launcher-provided `PREEN_BACKEND_URL`
and adds the Bearer header from its server-side `PREEN_TOKEN` when set.
Neither value enters frontend build output.
Origins outside the allowlist receive no CORS headers and are blocked by the
browser. Preflight (`OPTIONS`) is answered by the CORS middleware before the
auth middleware runs, because browsers do not send the `Authorization` header
on preflight. Allowed origins can read the `ETag` and `Retry-After` response
headers (`Access-Control-Expose-Headers`); an `ETag` is what a later
`If-Match` echoes.

## Error envelope

Non-2xx JSON responses always use the same shape:

```json
{ "code": "LIBRARY_NOT_FOUND", "message": "library not found" }
```

Common codes: `INVALID_ARGUMENT` (400), `UNAUTHORIZED` (401),
`LIBRARY_NOT_FOUND` / `DIRECTORY_NOT_FOUND` / `MEMBER_MISSING` (404),
`LIBRARY_EXISTS` / `LIBRARY_ROOT_OVERLAP` / `BUSY` (409), `INTERNAL` (500). The session routes add their
own guards: `IDEMPOTENCY_KEY_REQUIRED`, `VERSION_REQUIRED`,
`INVALID_IDEMPOTENCY_KEY`, `INVALID_PAGE` (400) and `PAYLOAD_TOO_LARGE` (413).
Codes such as `SCOPE_REQUIRED` are a planning session's `error_code`, never an
HTTP envelope.

The work catalog adds: `INVALID_PAGE` / `MATCH_MODE_REQUIRED` / `INVALID_TAG_NAME` /
`INVALID_TIMESTAMP` / `INVALID_PREFERENCES` / `INVALID_OUTBOUND_PROXY` (400), `WORK_NOT_FOUND` / `USER_TAG_NOT_FOUND` (404),
`WORK_IN_USE` / `USER_TAG_EXISTS` (409). A refusal may
carry `details[]` naming the references that hold a work in place.

Work collections add `COLLECTION_NOT_FOUND` (404), `BUILTIN_COLLECTION` and
`COLLECTION_MEMBER_CONFLICT` (409). Append reuses `WORK_NOT_FOUND` (404), with missing canonical work numbers in `details[]`.
Collection references, including Favorites, also block permanent Work deletion.

Database cleanup adds `CLEANUP_RUN_NOT_FOUND` (404); category validation uses
`INVALID_ARGUMENT` (400), and competing activity uses `BUSY` (409). See
[database record cleanup](maintenance.md).

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

The process runs as a standalone service by default; closing stdin has no
effect. Hosted launchers pass `--watch-stdin` to request graceful shutdown on
stdin EOF. Parent PID polling is not used, and no debug listener is started.

On `SIGINT`/`SIGTERM` (or stdin EOF with `--watch-stdin`), the backend seals request and worker
admission and cancels active work. Requests arriving after the seal receive
503 / `SHUTTING_DOWN`. HTTP request contexts are canceled, including SSE scans;
connections have four seconds to drain, and closing them does not count as
handler completion. Planning and execution sessions stopped by shutdown become
`interrupted`; an explicit user cancel remains `canceled`.

The backend joins admitted handlers, dispatcher workers, maintenance, media
facts, cover workers and manual cleanup before closing SQLite. File publication and its database
finalization finish through their protected boundaries. One five-second
forced-exit guard covers all joins and database Close. At its deadline the
process logs outstanding work and exits with code 1 without closing a database
still used by workers. Startup marks leftover sessions interrupted; a forced
exit can still leave unfinished filesystem changes requiring inspection.

## Health endpoint

| Method | Path | Auth | Status | Response |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/health` | no | 200 | `{"ok":true,"version":"dev"}` |
