# AGENTS.md

## Scope
- When changing backend code, apply [backend/go/AGENTS.md](backend/go/AGENTS.md); when changing web code, apply [frontend/web/AGENTS.md](frontend/web/AGENTS.md). Cross-stack changes apply both.
- When changing the Desktop Shell, apply [desktop/AGENTS.md](desktop/AGENTS.md).
- `scripts/dev-web.mjs` starts the backend and Vite together; `PREEN_DATA_DIR` defaults to `<repo>/.dev_data`.

## Documentation responsibilities
- AGENTS.md files own current working rules and responsibility-placement decisions. Keep shared rules here and stack-specific rules in the corresponding scoped file.
- Package and module comments describe current responsibilities, public seams and important boundaries. Update them when those responsibilities change; keep declarations and implementation details in code.
- ADRs record architectural decisions, reasons, rejected alternatives and trade-offs. Current coding rules belong in AGENTS.md; external contracts belong in docs/api/*.md.
- [CONTEXT.md](CONTEXT.md) defines domain language. When changing an HTTP/SSE contract, read its reference via [docs/api.md](docs/api.md) and the [common conventions](docs/api/common.md), and update the affected reference with the implementation.
- `docs/` holds ADRs and the API reference. Local task-control specs and evidence live in `docs/issues/` until published as GitHub issues. Tasks in `docs/issues/` are never cited from source tree code. Record statuses are indexed in [docs/adr/0000-index.md](docs/adr/0000-index.md).

## Versions: mise is the single source
- node/go/pnpm/task are pinned in `mise.toml`; CI provisions from the same file via `jdx/mise-action`. Bump versions in `mise.toml` only — `package.json` intentionally has no `packageManager` (pnpm's self-download once corrupted the store).
- Keep `backend/go/go.mod`'s `go` directive aligned with the mise pin. After editing `mise.toml` run `mise install`; a fresh checkout needs `mise trust` once.

## Integration checks
- `task ci:quality` runs the combined quality gate; focused checks and test conventions live in the scoped AGENTS.md files.
- `.github/workflows/ci.yml` also gates the Windows backend path with a Windows build and connection-pragma test; validate that path when changing platform-dependent behavior.
- PRs are reviewed by CodeRabbit (`.github/coderabbit.yaml`).

## Issues
- Record unpublished issues and specs in `docs/issues/`; publish to GitHub via `gh` only when requested.
- Five canonical triage labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix.

## ADR charter

- **A status is mandatory.** Every record opens with `Status:` — one of `Draft` (on working branch) or `Accepted` (on mainline) — and its date. A record without a status is not a source of truth: fix it or delete it, never cite it.
- **Distil decisions into the current record set.** The ADR collection is the distilled set of current architectural decisions. When an architectural decision evolves, rewrite the corresponding ADR directly and fold overturned reasons into its "Rejected alternatives" section. Git history preserves past versions.
- **Renumbering and restructuring.** Structural renumbering or consolidation across the ADR set is permitted when agreed, provided that `docs/adr/0000-index.md` maintains a historical mapping table and all codebase citations are updated in the same commit.
- **Cite reason or contract.** Source comments may cite `ADR NNNN` (without section numbers `§n`) for architectural rationale, or `docs/api/*.md#anchor` for external wire/contract behavior. State local responsibilities in module/package comments rather than referencing ADRs repeatedly across internal functions. Files under `docs/issues/` are transient task/measurement materials and must never be cited by source code.
- **0000 is the index, not a decision.** It lists the records, mirrors their statuses, and maps historical numbers. It carries no `Status` line of its own and is updated in the same commit that changes any ADR.
