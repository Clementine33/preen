# Go backend

## Responsibility placement
- Place a new use case with the business package that owns the affected state and invariants; keep its types, validation and lifecycle together. Read that package's doc comment and public seam before extending it.
- Keep business policy and coordination in business packages; place HTTP/SQL, external tools and filesystem mechanisms in `internal/adapters`. Classify technical capabilities by their contract, rather than treating every worker or queue as business policy.
- Declare storage, filesystem, tool and network ports at their business consumer, with only the methods it calls; adapters implement them. Keep each atomic storage operation as one port method so callers do not reconstruct a transaction from separate reads and writes.
- Dependencies point from assembly to adapters to business and lower-level capabilities. Business packages import neither `internal/adapters` nor `internal/app`; `internal/app` assembles dependencies and `cmd` handles process startup. `internal/arch` checks the enforced dependency boundaries.
- Keep generic workset lifecycle independent of task kinds: a task implements the workset's seam and owns its payload and policy; the workset owns operation/session identity, versions and scheduling.
- Extend an existing owner first; create a package or local subpackage when it has a distinct responsibility and a useful interface. Give new packages a responsibility comment, and update comments at both sides when moving a responsibility.

## Engineering rules
- Each file has one clear responsibility. Split when responsibilities change independently or a struct and its methods become difficult to read; line counts alone do not justify a split.
- `service.go` holds only types, constructors and DI wiring (≤80 lines); put use cases in files named by business verb, such as `run.go`, `persist.go` or `load_plan.go`.
- Use responsibility-bearing filenames rather than `helpers.go`, `util.go` or `common.go` dumps.
- Line-count guidelines: functions ≤100 lines; files ≤400 lines comfortably, >600 calls for reviewing the split.
- A structural refactor preserves stored payloads, hashes, admission timing and HTTP/SSE behavior. Change those contracts explicitly when they are part of the requested scope.

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
