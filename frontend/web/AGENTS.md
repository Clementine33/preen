# Vue web frontend

## Responsibility placement
- Keep HTTP/SSE transport, wire types and request encoding in `src/lib/api/`; callers consume this client rather than defining another transport path.
- Keep cacheable server resources, query keys, mutations and invalidation in `src/queries/` (Vue Query). Keep UI and selection state in `src/stores/` (Pinia); derive resource views from the query cache.
- Transient SSE lifecycles stay in `src/stores/{scan,workset-generation,workset-execution}.ts`: these represent streaming process state. The matching `src/composables/use-*` file coordinates cache updates.
- Keep reusable interaction orchestration in composables and rendering/event handling in components. Place new behavior with its state owner; extract a module when it has a distinct responsibility and useful interface.
- A list view owns its view state in the URL (what is listed, the page, the page size, the selection) and writes it only through one composable's explicit actions, which carry the reset rules (e.g. `use-wall-query.ts`). Stores mirror that state one way, from the URL; a store reaches the URL only through the composable's explicit fill (an address naming no library) and correction (a removed library falls back to the first). The view's global nav entry returns to the session's last view, remembered in a Pinia store. The return position is the scroll offset, recorded on the history entry whenever the page is left and restored once on BACK; the selection, not a stored focus target, names the item to bring into view. localStorage holds only first-visit preferences; drafts, staged picks and open menus stay in components.
- A failed user action reports through `useToast().error(e, fallback?)`, which reads the message via `errorDetails()`; only code-specific wording stays at the call site. Background task results (scan, planning, execution, covers) end with `useTaskResultNotice` in `use-task-toast.ts`. `main.ts` catches whatever no caller handled.
- Keyboard shortcuts register through `src/composables/use-keymap.ts` (descriptors in `KEYS` also feed the Settings list); components never listen to `keydown` on window/document.
- Document a module's responsibility and important seams beside its code when the boundary needs explanation; update that explanation when ownership changes. ADRs supply decision history, while these rules guide new code.
- Shared visual foundation follows the YoRHa theme: tokens live in `theme-yorha.css` and `style.css`; works pages provide exhibition/playback while worksets retain dense tabular forms and review workflows without flattening them into exhibition style. UI/UX changes are judged by user-friendliness: the user keeps their place, meets fewer surprises and takes fewer steps. Contrast rules and motion reduction (`prefers-reduced-motion`) take precedence over decorative parity; accessibility beyond native semantics is not a primary deliverable. See `DESIGN.md` for UI tokens, font matrix, and interaction principles.

## Runtime and checks
- Vue 3 + TypeScript, entry `src/main.ts`, in the pnpm workspace. Tool versions are pinned in the root `mise.toml`.
- From this directory: `pnpm lint`, `pnpm typecheck`, `pnpm test`, `pnpm build`.
- From the repository root: `task lint:web`, `task typecheck:web`, `task test:web`, `task build:web`; `task dev:web` starts backend + Vite and `task e2e:web` runs Playwright.
- ESLint with `@antfu/eslint-config` (`eslint.config.js`) is both linter and formatter; there is no Prettier. `pnpm lint:fix` / `task lint:web:fix` applies formatting. Overrides in the config record where project idioms differ from the preset.
- Keep full-stack browser tests for cross-stack user journeys. Backend disk/codec/session contracts belong in `backend/go/tests/e2e/`; browser-only layout/input checks run with `task test:web:browser` (Vite only).
- Install Chromium once when needed for e2e: `pnpm exec playwright install chromium` from this directory.
