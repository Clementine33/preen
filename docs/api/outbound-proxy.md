# Outbound Proxy

[API reference](../api.md) · [Common conventions](common.md)

Authenticated routes for how the server reaches metadata sources (DLsite,
api.asmr.one) and the cover CDN. There is no ETag or `If-Match`: the last
write wins.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/outbound-proxy` | 200 `{"mode":"manual","url":"socks5://127.0.0.1:7890"}`; `{"mode":"system","url":""}` when never saved. |
| PUT | `/api/v1/outbound-proxy` | Same body; 200 with the normalized object. |

`mode` is one of:

- `system`: follow the proxy settings of the computer the server runs on,
  resolved for each request's target. Windows reads the current user's
  Internet Options, evaluating auto-detection and PAC scripts through WinHTTP.
  Linux reads KDE's `kioslaverc` or GNOME's `gsettings` when the process runs
  in a desktop session, otherwise `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`;
  a Linux desktop set to PAC or auto-detection fails the request. Loopback
  targets are always direct.
- `manual`: connect through `url`.
- `direct`: connect directly.

A setting stored before modes existed reads as `manual` when it holds a URL and
`direct` otherwise. A saved value applies to the next upstream request without
a restart, and changes to the system settings apply the same way; requests
already in flight are unaffected. If the stored value or the system settings
cannot be read, upstream requests fail rather than connecting directly. The
credentials in `user:pass@` are stored as plain text and returned as saved.

PUT strictly accepts only `mode` and `url` (unknown keys return
`INVALID_ARGUMENT`, 400). `url` is optional outside `manual`, where it is kept
for switching back. It is trimmed; a non-empty value needs a scheme of `http`,
`https`, `socks5` or `socks5h` and a host; the port is optional, `user:pass@`
is allowed, and a path other than `/`, a query or a fragment is refused. An
unknown `mode`, a `manual` mode without `url`, or an invalid `url` returns
`INVALID_OUTBOUND_PROXY` (400); nothing is stored on refusal.

The payload is stored in the `common_settings` row `outbound_proxy`, which is
not a Task kind, so `/api/v1/tasks/{type}/settings` answers it as an unknown type.
