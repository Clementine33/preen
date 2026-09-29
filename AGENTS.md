# AGENTS.md

## Architecture
- Go backend (`backend/go`, entry `cmd/onsei-organizer-backend/main.go`): a single net/http API on an ephemeral loopback port; startup prints an `ONSEI_BACKEND_READY token=… version=… http_port=…` handshake that web dev consumes via `VITE_API_BASE`.
- Vue 3 + TS web frontend (`frontend/web`, pnpm workspace), entry `src/main.ts`; API client in `frontend/web/src/lib/api/`, server/cache state in `frontend/web/src/queries/` (Vue Query), UI/selection state in `frontend/web/src/stores/` (Pinia) — with one documented exception: the transient SSE lifecycles stay in `frontend/web/src/stores/{scan,workset-generation,workset-execution}.ts` (streaming process state, not a cacheable resource; the matching `src/composables/use-*` file owns cache coordination).
- `scripts/dev-web.mjs` boots backend + Vite together (`ONSEI_DATA_DIR` → `<repo>/.dev_data`).

## Go module structure & engineering rules
- **Layout**: business packages own their types, their use cases and the ports they need — `internal/{library,inventory,workset,conversion,fileops}` (+ `conversion/{reconcile,execute}`); `internal/adapters/{sqlite,httpapi,filesystem,ffmpeg,settings}` implement those ports; `internal/app` is the assembly root and the only place that knows every adapter at once; `cmd/…/main.go` is signals, the data directory and one call. The rules are asserted in `internal/arch` and recorded in [ADR 0009](docs/adr/0009-business-packages-and-dependency-direction.md).
- **Core goal**: Low coupling across modules, high cohesion within each file. Each file has one clear responsibility.
- **Dependency direction**: `app → adapters → business → pathnorm`. A business package never imports `internal/adapters/…` or `internal/app`; a port is declared by its consumer and implements only the methods that consumer calls.
- **Service decomposition**: `service.go` holds only types, constructors, and DI wiring (≤80 lines); use cases split into separate files named by business verb (e.g., `run.go`, `persist.go`, `load_plan.go`).
- **No semantic-less files**: Never create `helpers.go`, `util.go`, or `common.go` dumps.
- **Split heuristics**: Split when a file hosts ≥2 responsibilities that change at different frequencies, or when a struct + method cluster exceeds single-screen readability. Never split purely to satisfy line limits.
- **Line limits (guidelines)**: Functions ≤100 lines; files ≤400 lines (comfortable) / >600 (should split).
- **Linter & CI gate**: `task lint:go` runs `golangci-lint` (based on maratori config, in `backend/go/.golangci.yml`). Gated in CI (`ci:quality`). Mechanical fixes: `task lint:go:fix`.

### Go test rules (enforced)
- New test files default to `package <pkg>_test`; same-package tests are allowed only where the test really exercises internals, and must keep the `//nolint:testpackage` note explaining why.
- No dedicated tests for unexported helpers/fields — cover behavior through the exported API. When an internal seam must be exposed to tests, funnel it through an `export_test.go`; never bulk re-export.
- Never generate mocks for interfaces defined in this repo; prefer real dependencies or small handwritten fakes.
- Don't assert call counts or call order unless the count/order is itself the contract (retry cap, idempotency, fail-fast).
- Prefer `testing/synctest` over sleeping when synchronizing in-process concurrency; `time.Sleep` is acceptable when waiting on real OS/process/network events.
- Coverage is a diagnostic signal, not a KPI. Test deletion and production refactors never land in the same commit.

## Versions: mise is the single source
- node/go/pnpm/task are pinned in `mise.toml`; CI provisions from the same file via `jdx/mise-action`. Bump versions in `mise.toml` only — `package.json` intentionally has no `packageManager` (pnpm's self-download once corrupted the store).
- Keep `go.mod`'s `go` directive aligned with the mise pin. After editing `mise.toml` run `mise install`; a fresh checkout needs `mise trust` once.

## Database schema changes
- Before changing database structure, read [ADR 0013](docs/adr/0013-development-schema-and-release-migrations.md) for the development/release boundary.
- During development, edit `backend/go/internal/adapters/sqlite/schema.sql` or local tables freely; commit the final structure in `schema.sql`. Stop the backend and run `task db:rebuild:dev` when a clean database is needed.
- Unreleased SQL migrations may be edited, merged, deleted, or renumbered; retain only the final changes needed between releases.
- Once distributed (including an external RC), keep migration filenames, numbers, and contents unchanged. Append a migration for subsequent changes.
- Before release, add the migration from the previous release to the final schema and verify fresh initialization, upgrades, and data preservation. Release startup uses goose only; schema repairs belong in migrations.
- Development databases and release databases stay separate. See [ADR 0013 §3](docs/adr/0013-development-schema-and-release-migrations.md#3-备份重建与恢复) for rebuilding and restoring backups.

## Commands (verified)
- Quality, same shape as CI: `task test:go` · `task lint:go` · `task typecheck:web` · `task test:web` · `task build:web` · `task ci:quality`
- Backend (from root): `task -d backend/go test` · `task -d backend/go lint` · `test:e2e`; focused test: `go test ./<pkg> -run <Test>`
- Web (from `frontend/web`): `pnpm typecheck` · `pnpm test` · `pnpm build`; dev: `task dev:web`; e2e: `task e2e:web` (install Chromium once: `pnpm exec playwright install chromium`)

## CI facts
- `.github/workflows/ci.yml`: main jobs on ubuntu-latest run Go quality, web quality and an optional Playwright smoke; `windows-smoke` on windows-latest gates the Windows path (`task build:go:windows-x64`, a blocking connection-pragma test, + optional e2e).
- PRs reviewed by CodeRabbit (`.github/coderabbit.yaml`).

## Agent skills

### Issue tracker

Issues and specs live as GitHub issues, driven via the `gh` CLI.

### Triage labels

Five canonical labels, each label string equal to its name: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix.

### Domain docs

Single-context: [CONTEXT.md](CONTEXT.md) defines domain language, `docs/adr/` holds the architecture records, and [docs/api.md](docs/api.md) describes HTTP/SSE contracts. `docs/` contains only ADRs and the API reference — a task-control spec is not a repository artifact.

The record set and each record's status live in [docs/adr/0000-index.md](docs/adr/0000-index.md).

### ADR charter

- **A status is mandatory.** Every record opens with `Status:` — one of `Draft`, `Accepted`, `Superseded` — and its date. A record without a status is not a source of truth: fix it or delete it, never cite it.
- **Before the branch merges, every ADR is a Draft.** A Draft records intent and reasoning, not system facts. Only a decision on the mainline is `Accepted`, so a reader can always tell a proposal from a commitment.
- **Keep published ADRs historical.** Add a compatible decision in a new Draft ADR and cite the earlier record there; do not edit an Accepted or Superseded ADR. Only a conflicting decision permits a narrow edit to the older record. If it replaces that record's decision, mark the old record `Superseded`, point to the replacement ADR, and update 0000 in the same commit.
- **Distil drafts; do not chain drafts.** Refactors and fixes before merge converge in the affected Draft ADR: rewrite that draft and fold overturned reasons into its "Rejected alternatives".
- **Cite the record, not the task.** Source comments cite `ADR NNNN §n` or `docs/api.md`; a task-control spec is not a repository artifact and is never cited from the tree.
- **0000 is the index, not a decision.** It lists the records and mirrors their statuses, carries no `Status` line of its own, and is updated in the same commit that changes a record's status.
