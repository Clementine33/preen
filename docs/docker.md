# Docker deployment

Preen supports one `linux/amd64` container on a trusted LAN. The image contains the web app, backend, and the FFmpeg/FFprobe version pinned in `mise.toml`. Published images use `ghcr.io/clementine33/preen:<version>`; choose a completed GitHub Release and set its exact version as `PREEN_VERSION` in `.env`. The Release records the image digest and delivers the matching Windows portable package.

## First deployment

Copy [compose.yaml](../compose.yaml) and [.env.example](../.env.example) into your deployment directory, renaming the latter to `.env`.

1. Set `PREEN_VERSION` in `.env` to a published release version. Generate a long random access key (for example `openssl rand -hex 32`), set `PREEN_TOKEN` in `.env`, and run `chmod 600 .env`. Keep the file out of version control. Docker administrators can inspect container environment variables.
2. Edit `user: "1000:1000"` to match a non-root host UID/GID with write access to your application data and media. Create `./data` owned by that user before starting; Docker-created bind directories otherwise default to root ownership.
3. Replace `/srv/media/audio` with your host media directory. Add other explicit mounts under `/media/<name>` as needed. Mount the work directories **and their parent**, so Preen can create sibling `Delete/` recovery directories. Do not mount the host root or Docker socket.
4. The sample publishes `127.0.0.1:8765`. To connect from your LAN, replace `127.0.0.1` with your NAS's chosen LAN IP. Keep the container port `8765`; the host port can change.
5. Run `docker compose up -d`, then open `http://<selected-address>:8765` and enter the shared access key. Empty `/data` is initialized automatically, including default classifications and conversion settings. Missing `PREEN_VERSION` or `PREEN_TOKEN` prevents startup.

Keep container media mount points fixed: library roots store **container paths**, such as `/media/audio`. Changing a mount point invalidates directory identities. Only one instance may use a given `/data`; stop the previous container before starting a replacement.

The root filesystem is read-only. `/data`, the explicit media mounts are writable; scratch files live under `/data/tmp`, so no tmpfs is needed. The image defaults to UID/GID `1000:1000`; the Compose configuration drops all capabilities and disallows privilege escalation.

HTTP exposes credentials and media to anyone able to observe network traffic. Use your existing HTTPS reverse proxy on shared or untrusted networks; this deployment does not support direct public exposure. Changing `.env` requires `docker compose up -d --force-recreate`; `restart` alone does not re-inject its values. Changing the access key invalidates existing browser sessions.

The outbound proxy for metadata and cover downloads is set in the Settings page (Network). Its default, Follow system, reads the container's `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`; Manual and Direct ignore them.

## Logs

Preen writes one logfmt line per event to stdout (`time=… level=… msg="…" key=value …`); read them with `docker compose logs preen`. The sample Compose file rotates them with the `json-file` driver (three files of 10 MB); without that option Docker keeps container output forever. The `PREEN_BACKEND_READY` readiness line is the only line that is not logfmt.

Set `PREEN_LOG_LEVEL` to `debug`, `info` (default), `warn` or `error` in Compose and recreate the container; any other value prevents startup. `debug` adds one line per HTTP request.

To report a problem, use **Settings → Maintenance → Diagnostic log → Export**. It downloads the most recent lines (up to 2 MiB) together with the version and platform. It holds only lines since the last start, so use `docker compose logs` for anything earlier. Logs never contain the access key or proxy credentials, but they do contain media paths.

## Upgrade

1. Stop the current instance with `docker compose down`.
2. Back up the complete stopped `/data` and separately back up media. Database backups do not undo file changes.
3. Set `PREEN_VERSION` in `.env` to the chosen exact release version, run `docker compose pull`, then `docker compose up -d` using the same mounts and UID/GID.
4. Inspect `docker compose logs preen`. The credential-free `PREEN_BACKEND_READY version=… http_port=8765` line signals readiness. Existing release databases are backed up under `/data/backups/` before pending migrations; a backup or migration error stops startup. Configuration remains user-owned; new optional fields use defaults.

Migration backups are retained without automatic cleanup. A failed migration may leave earlier successful migrations applied; after fixing the cause, restart the same new image to retry the remaining migrations. Never run multiple instances to work around an upgrade failure.

## Restore or roll back

Stop every instance first. To restore a complete data backup, restore its configuration and database together. For a database-only migration backup, move the failed `cache.db`, `cache.db-wal`, and `cache.db-shm` aside, then copy the selected pre-upgrade backup to `/data/cache.db`. No old WAL/SHM sidecars may remain alongside the restored database. Restore configuration from the matching backup if required, preserve ownership, select the corresponding old image version, and start it. The application does not downgrade databases automatically. Restore media separately if you need to undo conversions, moves, or recycling.

## Build and verify locally

With Docker and the tools from `mise.toml` installed:

```sh
task build:docker RAW_TAG=v0.2.0
task test:docker RAW_TAG=v0.2.0
```

`build:docker` builds only `linux/amd64` and requires an injected release version. `.dockerignore` uses an allowlist to exclude credentials, runtime data, Git metadata, dependencies and build output. Node, pnpm and Go come from `mise.toml`. FFmpeg is built from the matching upstream release source after verifying its signature against the pinned FFmpeg release signing key. LAME and zlib are linked statically; Opus is linked against the Alpine `opus` package, which the runtime installs. The Alpine runtime otherwise receives only the two media executables, backend and web assets. The media build stage is cached independently of application changes; no build tools enter the runtime image.

The media build enables WAV, FLAC, MP3, AAC, M4A and Ogg/Opus inputs, the backend's five output formats, PCM precision variants, and PNG/JPEG cover streams. Keep the Dockerfile's codec/filter/container whitelist and the smoke test aligned when adding backend formats; a source build may take several minutes on a cold cache.

The smoke test uses Node's built-in SQLite, the host FFmpeg to generate independent fixtures, and a Docker-compatible CLI. It creates uniquely named disposable containers and volumes, then removes only those resources. It probes all six input formats (including embedded MP3/M4A covers) and converts each into all five outputs over real `fd:` handles, then decodes and probes the results. It also checks login, read-only/non-root operation, scan, thumbnails, application conversion, file move, sibling recycling, persistence, missing-key rejection, graceful SIGTERM, and migration backup/failure behavior (a successful upgrade is not covered until the first post-baseline migration exists). To use Podman: `CONTAINER_ENGINE=podman node scripts/docker-smoke.mjs <local-image>`.


## Publish a release (maintainers)

The actual GitHub repository is `Clementine33/dorganizer`; its name does not determine the image name. The workflow publishes `ghcr.io/clementine33/preen` and labels it with the actual `${GITHUB_REPOSITORY}` source URL so GitHub links the package and grants the repository access. The publish job alone receives `packages: write` and uses its ephemeral `GITHUB_TOKEN`; no registry credential reaches the build or runtime container. If a package already exists without inherited access, grant this repository Actions write access in the package settings first.

After merging the intended changes, push `vX.Y.Z-rc.N` for an RC or `vX.Y.Z` for a stable release. Tags run the complete quality suite and build the Windows portable ZIP. The publish job then builds, smoke-tests and pushes the linux/amd64 image directly, without transferring an image archive between runners. `gh release create` attaches the ZIP and `SHA256SUMS`, records the source commit and image digest, and publishes the Release. An existing Release stops publication before any image push.

Stable releases update both latest pointers in publication order; RCs are marked as prereleases and update neither. Publish stable versions in increasing order, one at a time. There is no automatic old-version backfill or rollback protection. Branch and pull request runs do not publish images. Deploy using the exact version rather than `latest`.

On the first publication, GitHub creates packages as **private**, even for public repositories. In the `preen` package settings under `Clementine33`, check the source repository and Actions access, then change package visibility to **Public**. This change is irreversible in GitHub. Confirm an unauthenticated pull using a clean Docker credential directory for first-release acceptance; this is a maintainer check. See [GitHub package visibility and access](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility).

Publication has no automatic recovery. If an image has already been published when a release step fails, leave that version intact and release a new patch version. Do not rerun completed releases or overwrite published versions. Intermediate Actions artifacts use fixed names and can be replaced when rebuilding; public release assets are separate.

For the first RC acceptance check, record the current `latest` digest (or its absence), wait for all jobs to pass, verify anonymous pulling and the service's exact version, and confirm `latest` is unchanged. Download the Release's ZIP and `SHA256SUMS`, verify the checksum and launch the extracted package on Windows.
