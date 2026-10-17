# Go backend

## Responsibility placement
- Place use cases with the business module owning their state and invariants; keep types, validation and lifecycle together so one change has one owner. Read its package comment and public seam before extending it.
- Keep business policy and coordination in business packages, and HTTP/SQL, external tools and filesystem mechanisms in `internal/adapters`, so mechanisms can change without rewriting policy. Judge workers and queues by their contract: cover delivery owns persistent per-work outcomes under `works/`; thumbnail owns a disposable derived cache as a technical capability.
- Declare ports at their consumers with only the operations they call, so dependencies express actual needs. A consumer needing only resolved containment takes that single function, not a shared `PathResolver`. Keep atomic storage operations in one method to preserve transaction boundaries; recovery path truncation stays behind the existing Recover seam.
- Dependencies point from assembly to adapters to business and lower-level capabilities, so business stays reusable. Business imports neither adapters nor app; app assembles dependencies and cmd handles startup. Nesting expresses ownership, not Go dependency: child workflows may import their base owner, which remains independent of those workflows.
- Keep workset lifecycle independent of task kinds, so adding a kind does not change generic scheduling. Tasks own their payload, policy and optional scheduling declarations; workset owns operation/session identity, versions and worker lifetimes. Light planning must be explicitly declared, never inferred from an empty input refresh.
- Keep Task integration in its feature package unless that workflow has a real production consumer outside workset; then use a distinct name such as `refreshtask`, so that consumer can use the workflow without importing workset. A generic `task` package obscures which feature callers use.
- Extend an existing owner first; split a package when a distinct responsibility yields a useful interface, so directory layers do not add indirection by themselves. Package comments describe current responsibilities, AGENTS holds working rules, and ADRs retain decisions and reasons; update comments on both sides of a move. Code comments cite `ADR NNNN` without section numbers, and cite `docs/api/*.md#anchor` for wire contracts.
- `internal/arch` classifies every production package under `internal/` as business except `adapters/**`, `app` and `arch`, so new packages cannot escape checks by omission. Pure cores opt in by name (`conversion/reconcile`, `librarypath`): their direct imports forbid `os`, `net/**`, `os/exec`, `database/sql` and `io/fs`; process, database and network dependencies are also checked transitively. This keeps supplied-fact algorithms independent of I/O without declaring all orchestration pure. Every automated rule must explain its purpose.

## Engineering rules
- Each file has one clear responsibility. Split when responsibilities change independently or a struct and its methods become difficult to read; line counts alone do not justify a split.
- `service.go` holds only types, constructors and DI wiring (≤80 lines); put use cases in files named by business verb, such as `run.go`, `persist.go` or `load_plan.go`.
- Use responsibility-bearing filenames rather than `helpers.go`, `util.go` or `common.go` dumps.
- Line-count guidelines: functions ≤100 lines; files ≤400 lines comfortably, >600 calls for reviewing the split.
- A package move preserves stored operation kinds (including `"rename"` and `"metadata_refresh"`), payloads and hashes, error codes, HTTP routes, SSE event names and admission timing: package names are not data contracts. Change these only as an explicit contract change in the requested scope.

## Tests
- New test files default to `package <pkg>_test`; same-package tests are allowed only where the test really exercises internals, and must keep the `//nolint:testpackage` note explaining why.
- Cover behavior through the exported API rather than dedicated tests for unexported helpers or fields. When an internal seam must be exposed to tests, funnel it through a narrow `export_test.go`.
- Prefer real dependencies or small handwritten fakes; never generate mocks for interfaces defined in this repo.
- Assert call counts or order only when they are themselves the contract (retry cap, idempotency, fail-fast).
- Prefer `testing/synctest` for in-process concurrency; `time.Sleep` is acceptable when waiting on real OS/process/network events.
- Coverage is a diagnostic signal, not a KPI. Test deletion and production refactors never land in the same commit.

## Database changes
- Development databases and release databases have separate lifecycles and are not converted into one another. `X.Y.Z-dev` uses the development schema; release versions and external RCs use goose. Build metadata does not select the mode, and program versions are not migration numbers.
- During development, edit `internal/adapters/sqlite/schema.sql` or local tables freely and commit the final structure in `schema.sql`; existing development databases are not automatically repaired.
- Unreleased SQL migrations may be edited, merged, deleted or renumbered; retain only the final changes needed between releases.
- Once distributed, including an external RC, migration filenames, numbers and contents are immutable. Append a migration for subsequent changes; release startup uses goose only, so schema repairs belong in migrations.
- Before release, add SQL from the previous release to the final schema in `internal/adapters/sqlite/migrations/`, starting with `-- +goose Up`; add a migration only when the schema changes. Verify fresh initialization, successive and cross-version upgrades, data preservation, and matching structure and read/write behavior between development schema and release migrations; run backend tests and lint.
- The default development version lives in `cmd/onsei-organizer-backend/main.go`. For a release build, use `task -d backend/go build RAW_TAG=v0.2.0` from the repository root (or set `RAW_TAG` in the environment).

### Rebuild and restore
- Rebuild a development database: stop the backend, then run `task db:rebuild:dev` from the repository root. It always targets `.dev_data/cache.db`, ignores `ONSEI_DATA_DIR`, backs up to `.dev_data/backups/` and replaces the database; configuration and media files stay in place. It can rebuild an old development database and refuses a goose-managed release database.
- Restore a backup: stop every process using the database; move the current `cache.db` and any `cache.db-wal` / `cache.db-shm` into another directory; copy the chosen backup to `cache.db`; start the corresponding old program. Leave no WAL/SHM from the failed database beside the restored file.

## Runtime and checks
- The backend serves one net/http API on an ephemeral loopback port. Startup prints `ONSEI_BACKEND_READY token=… version=… http_port=…`, which the web development launcher consumes through `VITE_API_BASE`.
- From the repository root: `task test:go`, `task lint:go`, `task lint:go:fix`; Go lint configuration is `.golangci.yml` in this directory and is gated by CI.
- From this directory: `task test`, `task lint`, `task test:e2e`; focused tests: `go test ./<pkg> -run <Test>`.
