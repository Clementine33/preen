# Diagnostics

[API reference](../api.md) · [Common conventions](common.md)

## Output

The backend writes its Diagnostic Log to stdout, one logfmt record per line
(`time=… level=… msg="…" key=value …`). The only other stdout line is the
[startup handshake](common.md#startup-handshake). Messages are stable phrases;
variable facts are fields, and an error is always `err`. Credentials (the
access key, `Authorization` and `Cookie` headers, the outbound proxy URL) are
never logged; media paths are.

`PREEN_LOG_LEVEL` selects the lowest level written: `debug`, `info` (default),
`warn` or `error`, case-insensitive. Any other value fails startup. It is read
once at startup and has no `config.json` field.

Every HTTP request is logged at `debug` with `method`, `path`, `status` and
`duration_ms`; a 5xx is logged at `error` and carries the failure behind it as
`err` on the same line (the response still says only `INTERNAL`). The query
string and headers are never logged. A handler panic is logged at `error` with its stack and answered
with `500` / `INTERNAL`.

Task failures are logged because the records that carry them are replaced by the
next run. A failed execution logs `execution failed` at `error` with
`execution_id`, `plan_id`, `workset_id`, `operation_type`, `error_code` and
`error_message`; a component that failed on its own logs
`execution component failed` at `warn` with the same identifiers plus
`component_index`, `component_id`, `root_path` and `stage`; a failed plan
generation logs `generation failed` at `error` with `generation_id`,
`workset_id`, `operation_type`, `error_code` and `error_message`. `err` is added
when the cause is not already in the message. A cancellation or shutdown is not
a failure and is not logged, except `execution canceled without a cancel
request` at `warn`, which marks an error that looked like a cancellation while
nothing asked to cancel.

A Library Scan logs `scan skipped unaddressable name` at `warn` for each file
or directory name that is not valid UTF-8, with `root` (the Library root) and
`path` (library-relative, forward slashes, invalid bytes written as Go escapes
such as `\x93\xfa`). The scan's `completed` event carries only the count.

The Media Facts Pass logs `media facts unmeasurable` at `warn` for each file it
visits but cannot measure, with `path` (absolute), `format` and, when the probe
itself failed, `err`. Every pass revisits these files, so the record repeats per
pass; the [status event](works.md#background-media-facts-notifications) carries
only the per-library count.

## Log Export

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/diagnostics/log` | 200 `text/plain; charset=utf-8` attachment |

Authenticated like every protected route. The response carries
`Content-Disposition: attachment; filename="preen-<version>-<YYYYMMDDTHHMMSSZ>.log"`
(the export time in UTC) and `Cache-Control: no-store`. The body starts with
four header lines:

```
# preen <version> <go version> <os>/<arch>
# started <process start, RFC 3339 UTC>
# exported <export time, RFC 3339 UTC>
# log level=<PREEN_LOG_LEVEL> dropped=<lines> skipped=<records>
```

`dropped` counts the oldest lines pushed out since start and `skipped` the
records too large to keep; both `0` means the export holds every line since
start. Lines below `level` were never written: at the default `info` the
per-request lines are absent, so reproduce with `debug` to get them.

followed by the newest Diagnostic Log lines, byte-for-byte as written to
stdout at the current level: the newest whole lines whose total fits 2 MiB.
The oldest lines are dropped first; a single record larger than 2 MiB is left
out of the export (stdout still has it). The tail lives in memory only: a restart starts it empty, and older lines are wherever the
process's stdout went (for Docker, `docker compose logs`).
