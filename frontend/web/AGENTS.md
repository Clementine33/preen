# Vue web frontend

## Responsibility placement
- Keep HTTP/SSE transport, wire types and request encoding in `src/lib/api/`; callers consume this client rather than defining another transport path.
- Keep cacheable server resources, query keys, mutations and invalidation in `src/queries/` (Vue Query). Keep UI and selection state in `src/stores/` (Pinia); derive resource views from the query cache.
- Transient SSE lifecycles stay in `src/stores/{scan,workset-generation,workset-execution}.ts`: these represent streaming process state. The matching `src/composables/use-*` file coordinates cache updates.
- Keep reusable interaction orchestration in composables and rendering/event handling in components. Place new behavior with its state owner; extract a module when it has a distinct responsibility and useful interface.
- A list view owns its view state in the URL (what is listed, the page, the page size, the selection) and writes it only through one composable's explicit actions, which carry the reset rules (e.g. `use-wall-query.ts`). The return position (scroll, focus target) lives in the history entry's state; localStorage holds only first-visit preferences; drafts, staged picks and open menus stay in components.
- Document a module's responsibility and important seams beside its code when the boundary needs explanation; update that explanation when ownership changes. ADRs supply decision history, while these rules guide new code.
- Shared visual foundation follows the YoRHa theme: tokens live in `theme-yorha.css` and `style.css`; works pages provide exhibition/playback while worksets retain dense tabular forms and review workflows without flattening them into exhibition style. Contrast rules, motion reduction (`prefers-reduced-motion`), and accessibility take precedence over decorative parity. See `DESIGN.md` for UI tokens, font matrix, and interaction principles.

## Runtime and checks
- Vue 3 + TypeScript, entry `src/main.ts`, in the pnpm workspace. Tool versions are pinned in the root `mise.toml`.
- From this directory: `pnpm typecheck`, `pnpm test`, `pnpm build`.
- From the repository root: `task typecheck:web`, `task test:web`, `task build:web`; `task dev:web` starts backend + Vite and `task e2e:web` runs Playwright.
- Keep full-stack browser tests for cross-stack user journeys. Backend disk/codec/session contracts belong in `backend/go/tests/e2e/`; browser-only layout/input checks run with `task test:web:browser` (Vite only).
- Install Chromium once when needed for e2e: `pnpm exec playwright install chromium` from this directory.
