# Upstream Proxy

[API reference](../api.md) · [Common conventions](common.md)

Authenticated routes for the deployment-wide outbound proxy the server uses to
reach metadata sources (DLsite, api.asmr.one) and the cover CDN. There is no
ETag or `If-Match`: the last write wins.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/upstream-proxy` | 200 `{"url":"socks5://127.0.0.1:7890"}`; `{"url":""}` when never saved. |
| PUT | `/api/v1/upstream-proxy` | Same body; 200 with the normalized object. |

An empty `url` means a direct connection. The proxy comes only from this
setting: the server does not read `HTTP_PROXY`, `HTTPS_PROXY` or `NO_PROXY`.
A saved value applies to the next upstream request without a restart; requests
already in flight are unaffected. If the stored value cannot be read, upstream
requests fail rather than connecting directly. The credentials in
`user:pass@` are stored as plain text and returned as saved.

PUT strictly accepts only `url` (unknown keys return `INVALID_ARGUMENT`, 400).
`url` is trimmed. A non-empty value needs a scheme of `http`, `https`, `socks5`
or `socks5h` and a host; the port is optional, `user:pass@` is allowed, and a
path other than `/`, a query or a fragment is refused. A missing `url` or an
invalid value returns `INVALID_UPSTREAM_PROXY` (400); nothing is stored on
refusal.

The payload is stored in the `common_settings` row `upstream_proxy`, which is
not a Task kind, so `/api/v1/tasks/{type}/settings` answers it as an unknown type.
