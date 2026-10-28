# Desktop operation

The Desktop Shell opens Preen in a native window and runs the same backend used by the [Docker deployment](docker.md). Closing the main window stops the service; a second launch focuses the existing window. Keep the application running while scripts or other devices use its API. The desktop Lyrics Window is not available yet.

## Windows portable zip

Download `preen-<version>-windows-amd64.zip` and `SHA256SUMS` from the corresponding GitHub Release. Verify the ZIP's SHA-256 against `SHA256SUMS` (for example with PowerShell `Get-FileHash -Algorithm SHA256 <zip>`), then extract the entire ZIP to a directory your user can write, such as `C:\Users\you\Apps\Preen`. Run `preen-desktop.exe` from there. Keep its `bin` directory beside it; `bin` contains the backend and bundled FFmpeg/FFprobe.

Install [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) if it is missing. The zip contains neither the runtime nor its installer; Preen displays the download address and exits when it cannot find the runtime. The executable is unsigned, so Windows SmartScreen may warn when you first run it.

Preen creates `data` beside `preen-desktop.exe`. If that directory cannot be created or written, startup displays an error and exits. It does not fall back to AppData or another location: move the whole extracted directory to a writable location. Do not run directly inside the zip or place the portable directory under a protected system directory.

To upgrade, close Preen, back up the complete `data` directory, and extract the new zip over the old program files, **preserving `data`**. Replace the shell and everything in `bin` together; mismatched shell and backend versions refuse to start. A fresh zip contains no user data. Release database migrations run at startup and keep pre-migration database backups under `data/backups`; a backup or migration failure stops startup. Back up media separately, since a database backup cannot undo media changes.

## Arch Linux package

Arch packages are built locally and are not published as tag release assets.

Install a locally built pacman package, for example:

```sh
sudo pacman -U dist/preen-0.2.0-1-x86_64.pkg.tar.zst
```

The package declares five runtime dependencies: `gtk4`, `webkitgtk-6.0`, `ffmpeg`, `gst-plugins-good`, and `gst-libav`. Install missing dependencies from your configured Arch repositories. Linux uses the system FFmpeg/FFprobe and GStreamer media plugins.

Start Preen from the desktop menu or run `preen-desktop`. The package installs the shell at `/usr/bin/preen-desktop` and the private backend at `/usr/lib/preen/preen`. Close Preen and back up its data before installing an upgrade with `pacman -U`; package upgrades preserve the user data directory.

## Data and logs

| Platform | Data directory | Current and previous service logs |
| --- | --- | --- |
| Windows portable | `<portable-directory>\data` | `data\logs\preen.log`, `data\logs\preen.prev.log` |
| Linux, `XDG_DATA_HOME` set | `$XDG_DATA_HOME/preen` | `$XDG_DATA_HOME/preen/logs/preen.log`, `$XDG_DATA_HOME/preen/logs/preen.prev.log` |
| Linux, `XDG_DATA_HOME` unset | `~/.local/share/preen` | `~/.local/share/preen/logs/preen.log`, `~/.local/share/preen/logs/preen.prev.log` |

`config.json`, `cache.db`, caches and migration backups live in that data directory. The shell selects this location and sets `PREEN_DATA_DIR` for the service. For a complete backup, close Preen first and copy the entire data directory; copy media separately.

Both service stdout and stderr are captured from startup, including errors before readiness. Each service start replaces `preen.prev.log` with the previous `preen.log` and creates a new `preen.log`. Only these two runs are retained; there is no size-based rotation. Copy a failing run's log before further restarts overwrite it. Startup or unexpected service exits display a native error dialog with recent output; the shell does not automatically restart the service. Errors before the service can start, such as an unwritable data directory, may only appear in the dialog.

The credential-free `PREEN_BACKEND_READY version=… http_port=…` line identifies a ready service and its port. Set `PREEN_LOG_LEVEL` in the shell's launch environment to `debug`, `info` (default), `warn`, or `error`, then restart; invalid values prevent service startup. **Settings → Maintenance → Diagnostic log → Export** exports recent in-memory diagnostics from the current run. Use the files above for the previous run. Logs can contain media paths; inspect them before sharing.

## Fixed credentials, scripts and LAN access

By default the service listens on `127.0.0.1`, uses a system-assigned port, and the shell generates a fresh 32-byte random access key for each launch. The random key stays in memory. The page and service always receive the same effective key, selected in this order:

1. Nonempty `PREEN_TOKEN` in the shell's launch environment.
2. Nonempty `server.token` in the data directory's `config.json`.
3. A fresh random key, only when `server.allow_lan` is false and neither source supplies a key.

An empty environment value leaves the file key in effect. LAN mode never receives a generated fallback key. A malformed configuration still causes the backend to reject startup.

First launch and close Preen to create its configuration. To give a script on this computer a stable API address, edit the existing `server` object in `config.json`, preserving the other settings:

```json
{
  "server": {
    "allow_lan": false,
    "port": 8765,
    "token": "REPLACE_WITH_A_LONG_RANDOM_SECRET"
  }
}
```

Replace the example token with a generated secret (for example `openssl rand -hex 32` where OpenSSL is available). Keys must use visible ASCII without spaces. Restart Preen, then use `http://127.0.0.1:8765/api/v1` with `Authorization: Bearer <your-key>`. For example, a script can GET `/api/v1/libraries` with that header. Userscripts using `GM_xmlhttpRequest` can call this local endpoint without opening LAN access. An ordinary cross-origin browser page additionally needs an explicit `PREEN_CORS_ORIGINS` allowlist; see [API common conventions](api/common.md#cors).

After local script access works, set `allow_lan` to `true`, retaining the fixed port and key, and restart. The service then binds `0.0.0.0`; other devices use `http://<desktop-lan-ip>:8765/api/v1` and the same Bearer key. Allow the selected port through your firewall as needed. The shell serves its embedded UI separately, so this API endpoint does not serve the browser application; use the Docker deployment for browser access from other devices.

LAN mode requires a nonzero fixed port and a nonempty effective key. Missing credentials, an invalid port or key, a malformed/unreadable configuration, or a port already in use prevent startup; the service never silently chooses another port. **Every configuration or launch-environment change requires closing and restarting Preen.** Changing the key invalidates existing credentials. LAN access is intended for a trusted network; HTTP does not encrypt the key or media. Full configuration and authentication rules are in [common conventions](api/common.md#server-configuration).

## Linux rendering problems

If WebKit shows a blank window or crashes with an NVIDIA driver, close Preen and try launching from a terminal with:

```sh
WEBKIT_DISABLE_DMABUF_RENDERER=1 preen-desktop
```

This disables the DMA-BUF renderer, which makes rendering slower and can make CSS `backdrop-filter` effects unavailable, changing translucent blur appearance. Otherwise the shell keeps the DMA-BUF renderer enabled on every GPU, overriding the toolkit's automatic NVIDIA workaround; a value you set explicitly is always kept. Keep this workaround only if it fixes the problem; remove it to return to the default renderer.

## Lyrics Window

The Lyrics Window opens from the player bar as its own window titled `Preen 歌词` and closes from its hover bar or with the main window. On Windows it has a transparent background and stays on top; its backdrop button switches to a dark background and back, remembered on this device; its lock button makes it click-through and floats a small unlock button over its top edge, which unlocks it again (the player bar's lyrics button and `L` only open and close the window, locked or not). On Linux it keeps a dark background and asks to stay on top, which only X11 honours: Wayland has no protocol for it.

On KDE Plasma under Wayland, keep it on top with a window rule: **System Settings → Window Management → Window Rules → Add New**, set **Window title** to `Preen 歌词` (Exact Match), then add **Keep above other windows**, set to **Force** and **Yes**. Other compositors offer equivalent rules matched on the window title.

## Build artifacts locally

Use the pinned tools in [mise.toml](../mise.toml), including nFPM 2.48.0, and install frontend dependencies with `pnpm install --frozen-lockfile`. Linux shell compilation also needs GTK4 and WebKitGTK 6.0 development libraries. Release tasks require `RAW_TAG=vX.Y.Z` or `RAW_TAG=vX.Y.Z-rc.N`; development versions, other prerelease channels and build metadata are rejected. Both binaries receive the same version.

On Arch Linux:

```sh
task desktop:package:arch RAW_TAG=v0.2.0
```

This uses [desktop/nfpm.yaml](../desktop/nfpm.yaml) and produces `dist/preen-0.2.0-1-x86_64.pkg.tar.zst` on x86_64. `task desktop:build RAW_TAG=v0.2.0` builds without packaging, with Linux binaries at `dist/desktop/linux-amd64/bin/preen-desktop` and `dist/desktop/linux-amd64/lib/preen/preen` on amd64. `task desktop:dev` builds and runs a development version with Linux data isolated under `.dev_data/desktop/preen`.

Windows delivery is automatic from a version tag through [.github/workflows/ci.yml](../.github/workflows/ci.yml). It builds audio tools on Ubuntu, then the shell/backend and ZIP on Windows, reusing the frontend quality job's production assets. The ZIP and its checksum are attached to the same GitHub Release as the container reference after all checks pass; see [release publication](docker.md#publish-a-release-maintainers) for the procedure.

For local Windows packaging, run `desktop:ffmpeg` (requires Docker; exports `dist/desktop-ffmpeg/{bin,THIRD_PARTY_NOTICES.txt}`), `desktop:build` on Windows, then `desktop:package:windows` with the same `RAW_TAG`. The packager is [scripts/package-windows.mjs](../scripts/package-windows.mjs), and its output is `dist/preen-0.2.0-windows-amd64.zip` for `RAW_TAG=v0.2.0`.
