# Update Check

[API reference](../api.md) · [Common conventions](common.md)

One authenticated route reports whether GitHub has published a newer stable
release than the running server. It only informs; nothing is downloaded or
installed.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/update-check` | 200 `{"state":"none"}`; `{"state":"checking"}`; `{"state":"available","latest_version":"v0.3.0","release_url":"https://github.com/Clementine33/preen/releases/tag/v0.3.0"}`. |

`state` is one of:

- `checking`: the check made when the process started has not settled yet.
  Clients poll until it changes.
- `available`: the latest release is newer than the running version.
  `latest_version` is the release's tag as GitHub returns it (for example
  `v0.3.0`) and `release_url` is its release page. Both fields are present
  only in this state.
- `none`: no newer release is known. This covers a running version that is
  the latest or newer, a skipped check, and a failed check: a failure is
  deliberately indistinguishable from "up to date" on the wire.

The server checks once per process, in the background, by reading
`https://api.github.com/repos/Clementine33/preen/releases/latest`, which names
only the latest stable release (never a pre-release or draft). There is no
setting to turn the check off, and no periodic re-check: the result holds until
the process restarts. The result is kept in memory only.

Versions are compared by [semantic version](https://semver.org) precedence with
or without a leading `v`, so `0.2.0` is newer than `0.2.0-rc.2`. A running
development version (a `dev` pre-release) or any version that is not a semantic
version skips the check and reads `none` at once, without a request.

The request goes through the [Outbound Proxy](outbound-proxy.md). Each attempt
times out after 10 seconds. Network errors, HTTP 5xx and 429 are retried, up to
three attempts in all, waiting 2 seconds and then 4 seconds in between. A 404, any other 4xx or a response that
cannot be read ends the check at once. When the check gives up it writes one
`update check failed` line to the [Diagnostic Log](diagnostics.md#output) and
the state becomes `none`.
