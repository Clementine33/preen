# Docker deployment

Preen supports one `linux/amd64` container on a trusted LAN. The image contains the web app, backend, and the FFmpeg/FFprobe version pinned in `mise.toml`. Published images use `ghcr.io/clementine33/preen:<version>`; choose an available exact release tag. The sample uses `0.2.0`; image publication is a separate release step.

## First deployment

Copy [compose.yaml](../compose.yaml) and [.env.example](../.env.example) into your deployment directory, renaming the latter to `.env`.

1. Generate a long random access key (for example `openssl rand -hex 32`), set `PREEN_TOKEN` in `.env`, and run `chmod 600 .env`. Keep the file out of version control. Docker administrators can inspect container environment variables.
2. Edit `user: "1000:1000"` to match a non-root host UID/GID with write access to your application data and media. Create `./data` owned by that user before starting; Docker-created bind directories otherwise default to root ownership.
3. Replace `/srv/media/audio` with your host media directory. Add other explicit mounts under `/media/<name>` as needed. Mount the work directories **and their parent**, so Preen can create sibling `Delete/` recovery directories. Do not mount the host root or Docker socket.
4. The sample publishes `127.0.0.1:8765`. To connect from your LAN, replace `127.0.0.1` with your NAS's chosen LAN IP. Keep the container port `8765`; the host port can change.
5. Run `docker compose up -d`, then open `http://<selected-address>:8765` and enter the shared access key. Empty `/data` is initialized automatically, including default classifications and conversion settings. Missing `PREEN_TOKEN` prevents startup.

Keep container media mount points fixed: library roots store **container paths**, such as `/media/audio`. Changing a mount point invalidates directory identities. Only one instance may use a given `/data`; stop the previous container before starting a replacement.

The root filesystem is read-only. `/data`, the explicit media mounts, and the bounded `/tmp` tmpfs are writable. The image defaults to UID/GID `1000:1000`; the Compose configuration drops all capabilities and disallows privilege escalation. Increase the tmpfs size in Compose if your workload requires it.

HTTP exposes credentials and media to anyone able to observe network traffic. Use your existing HTTPS reverse proxy on shared or untrusted networks; this deployment does not support direct public exposure. Changing `.env` requires `docker compose up -d --force-recreate`; `restart` alone does not re-inject its values. Changing the access key invalidates existing browser sessions.

## Upgrade

1. Stop the current instance with `docker compose down`.
2. Back up the complete stopped `/data` and separately back up media. Database backups do not undo file changes.
3. Change `image:` to the chosen exact release version, run `docker compose pull`, then `docker compose up -d` using the same mounts and UID/GID.
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

The smoke test uses Node's built-in SQLite, the host FFmpeg installed by mise to generate independent fixtures, and a Docker-compatible CLI. It creates uniquely named disposable containers and volumes, then removes only those resources. It probes all six input formats (including embedded MP3/M4A covers) and converts each into all five outputs over real `fd:` handles, then decodes and probes the results. It also checks login, read-only/non-root operation, scan, thumbnails, application conversion, file move, sibling recycling, persistence, missing-key rejection, graceful SIGTERM, and upgrade backup/failure behavior. To use Podman: `CONTAINER_ENGINE=podman node scripts/docker-smoke.mjs <local-image>`.


## Publish a release (maintainers)

The actual GitHub repository is `Clementine33/dorganizer`; its name does not determine the image name. The workflow publishes `ghcr.io/clementine33/preen` and labels it with the actual `${GITHUB_REPOSITORY}` source URL so GitHub links the package and grants the repository access. The publish job alone receives `packages: write` and uses its ephemeral `GITHUB_TOKEN`; no registry credential reaches the build or runtime container. If a package already exists without inherited access, grant this repository Actions write access in the package settings first.

Push a new SemVer Git tag such as `v0.2.0-rc.1` to test a prerelease, or `v0.2.0` for a stable release. The CI workflow always runs the complete quality suite for tags, then builds `linux/amd64` with that version, runs the container smoke test, and publishes that exact tested image as `0.2.0-rc.1` or `0.2.0`. Only stable releases update `latest`. Branch and pull request runs do not publish images. Deploy using the exact version rather than `latest`.

On the first publication, GitHub creates packages as **private**, even for public repositories. In the `preen` package settings under `Clementine33`, check the source repository and Actions access, then change package visibility to **Public**. This change is irreversible in GitHub. Confirm an unauthenticated pull of `ghcr.io/clementine33/preen:0.2.0-rc.1` using a clean Docker credential directory before declaring the release ready. See [GitHub package visibility and access](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility).

The publication job serializes versions and checks the registry before pushing: an existing version, an authentication failure, or an indeterminate registry response prevents publication. Re-running a published tag therefore fails without changing it. This protects tags written through this workflow; package administrators must also avoid manual overwrites or deletions. A stable release that fails after its version push but before its `latest` push needs maintainer inspection; rerunning cannot overwrite the existing version.

For the first prerelease acceptance check, record the current `latest` digest (or its absence), push the new prerelease tag, wait for every quality/build/smoke/publish job to pass, verify anonymous pulling and the health response's exact version, and confirm `latest` is unchanged. Re-run the same tag's workflow and confirm the publish job refuses the existing version and its digest remains unchanged.
