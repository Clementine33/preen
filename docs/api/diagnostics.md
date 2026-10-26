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
`duration_ms`; a 5xx is logged at `error`. The query string and headers are
never logged. A handler panic is logged at `error` with its stack and answered
with `500` / `INTERNAL`.

## Log Export

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/diagnostics/log` | 200 `text/plain; charset=utf-8` attachment |

Authenticated like every protected route. The response carries
`Content-Disposition: attachment; filename="preen-<version>-<YYYYMMDDTHHMMSSZ>.log"`
(the export time in UTC) and `Cache-Control: no-store`. The body starts with
three header lines:

```
# preen <version> <go version> <os>/<arch>
# started <process start, RFC 3339 UTC>
# exported <export time, RFC 3339 UTC>
```

followed by the newest Diagnostic Log lines, byte-for-byte as written to
stdout at the current level: the newest whole lines whose total fits 2 MiB.
The oldest lines are dropped first; a single record larger than 2 MiB is left
out of the export (stdout still has it). The tail lives in memory only: a restart starts it empty, and older lines are wherever the
process's stdout went (for Docker, `docker compose logs`).
