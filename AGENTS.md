# AGENTS.md

## Scope
- When changing backend code, apply [backend/go/AGENTS.md](backend/go/AGENTS.md); when changing web code, apply [frontend/web/AGENTS.md](frontend/web/AGENTS.md). Cross-stack changes apply both.
- `scripts/dev-web.mjs` starts the backend and Vite together; `ONSEI_DATA_DIR` defaults to `<repo>/.dev_data`.

## Documentation responsibilities
- AGENTS.md files own current working rules and responsibility-placement decisions. Keep shared rules here and stack-specific rules in the corresponding scoped file.
- Package and module comments describe current responsibilities, public seams and important boundaries. Update them when those responsibilities change; keep declarations and implementation details in code.
- ADRs record decisions, reasons and rejected alternatives. Consult them for the relevant decision's history; current coding rules belong in AGENTS.md. Keep ownership descriptions beside the code rather than in a separate package-placement inventory.
- [CONTEXT.md](CONTEXT.md) defines domain language. When changing an HTTP/SSE contract, read its reference via [docs/api.md](docs/api.md) and the [common conventions](docs/api/common.md), and update the affected reference with the implementation.
- `docs/` holds ADRs and the API reference; task-control specs live in issues rather than in the repository. Record statuses are indexed in [docs/adr/0000-index.md](docs/adr/0000-index.md).

## Versions: mise is the single source
- node/go/pnpm/task are pinned in `mise.toml`; CI provisions from the same file via `jdx/mise-action`. Bump versions in `mise.toml` only — `package.json` intentionally has no `packageManager` (pnpm's self-download once corrupted the store).
- Keep `backend/go/go.mod`'s `go` directive aligned with the mise pin. After editing `mise.toml` run `mise install`; a fresh checkout needs `mise trust` once.

## Integration checks
- `task ci:quality` runs the combined quality gate; focused checks and test conventions live in the scoped AGENTS.md files.
- `.github/workflows/ci.yml` also gates the Windows backend path with a Windows build and connection-pragma test; validate that path when changing platform-dependent behavior.
- PRs are reviewed by CodeRabbit (`.github/coderabbit.yaml`).

## Issues
- Issues and specs live as GitHub issues, driven via the `gh` CLI.
- Five canonical triage labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix.

## ADR charter

- **A status is mandatory.** Every record opens with `Status:` — one of `Draft`, `Accepted`, `Superseded` — and its date. A record without a status is not a source of truth: fix it or delete it, never cite it.
- **Before the branch merges, every ADR is a Draft.** A Draft records intent and reasoning, not system facts. Only a decision on the mainline is `Accepted`, so a reader can always tell a proposal from a commitment.
- **Keep published ADRs historical.** Add a compatible decision in a new Draft ADR and cite the earlier record there; do not edit an Accepted or Superseded ADR. Only a conflicting decision permits a narrow edit to the older record. If it replaces that record's decision, mark the old record `Superseded`, point to the replacement ADR, and update 0000 in the same commit.
- **Distil drafts; do not chain drafts.** Refactors and fixes before merge converge in the affected Draft ADR: rewrite that draft and fold overturned reasons into its "Rejected alternatives".
- **Cite the reason or contract.** Source comments may cite `ADR NNNN §n` for a decision's rationale or `docs/api/*.md` for a contract; state the current responsibility locally. A task-control spec is never cited from the tree.
- **0000 is the index, not a decision.** It lists the records and mirrors their statuses, carries no `Status` line of its own, and is updated in the same commit that changes a record's status.
