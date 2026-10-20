<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { useRouter } from 'vue-router'
import { useToast } from '@/composables/use-toast'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import { useMediaQuery } from '@/composables/use-media-query'
import { useReducedMotion, useSwitchBurst } from '@/composables/use-motion'
import CollectionScreen from '@/features/collections/CollectionScreen.vue'
import { fmtT } from '@/features/work-detail/derive'
import { useApiClient } from '@/lib/api/client'
import type { WorkSummary } from '@/lib/api/types'
import { collectionDetailQueryOptions, newCollectionName, useCollectionManage, useCollectionTitles, useCollections } from '@/queries/collections'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'

// The collections entry page (prototype: NieR "Intel" menu). A plain list panel
// on the left, a detail deck on the right: a 4:3 cover drawn at random from the
// selected collection and, beside it, the newest titles that fit.
//
// Data (no new endpoints): the list is `GET /collections`; a selection reads
// two small windows of `GET /collections/{id}` — the newest titles (as many as the column can hold)
// (`direction=desc`) and one random member for the cover (`limit=1`, a random
// page). Saved order has no added-at, so "newest" is the end of that order.
// The selection is view-local: this page is the entry, not a filtered list.
// Enter (or a second click, or a tap on a phone) opens the collection; the
// selected id is kept on this history entry, so Back restores it and any other
// arrival selects the first.

const api = useApiClient()
const queryClient = useQueryClient()
const router = useRouter()
const libraryUi = useLibraryUiStore()
const player = usePlayerStore()
const { collections } = useCollections()
const manage = useCollectionManage()
const toast = useToast()

// + NEW: an empty collection with a default name, opened at once (its page renames and fills it).
async function createNew() {
  try {
    const made = await manage.create(newCollectionName(collections.value))
    await router.push({ name: 'collection-detail', params: { id: made.id } })
  } catch (e) { toast.show({ text: e instanceof Error ? e.message : String(e) }) }
}

const TITLE_LINE = 22 // a title's line height; a one-line title is the shortest item
const TITLE_GAP = 10
const TITLE_FALLBACK = 20 // no layout to measure (tests)
const titleLimit = ref<number | null>(null)

const sel = ref(0)
const shown = ref(0) // the collection the detail follows; trails sel while a key is held
const current = computed(() => collections.value[shown.value] ?? null)
// Phones show the list alone (the CSS hides the deck), so the deck's requests are not made.
const narrow = useMediaQuery('(max-width: 900px)')
const { query: titlesQuery, titles } = useCollectionTitles(() => current.value?.id ?? null, () => (narrow.value ? null : titleLimit.value))

// ── Cover: one random member per switch, never the one on screen ──
const coverWork = ref<WorkSummary | null>(null)
let lastPage = 0
let draw = 0
async function drawCover(redraw = false) {
  const col = current.value
  const mine = ++draw
  if (narrow.value) return
  if (!col || col.work_count === 0) return void (coverWork.value = null)
  let page = 1 + Math.floor(Math.random() * col.work_count)
  if (col.work_count > 1 && (redraw || page === lastPage)) {
    while (page === lastPage) page = 1 + Math.floor(Math.random() * col.work_count)
  }
  try {
    const detail = await queryClient.fetchQuery({
      ...collectionDetailQueryOptions(api, col.id, { limit: 1, page }),
      staleTime: 60_000,
    })
    if (mine !== draw) return
    lastPage = page
    coverWork.value = detail.works[0] ?? null
  } catch {
    if (mine === draw) coverWork.value = null
  }
}

// ponytail: no failed-load fallback to the remote link (the screen paints a
// CSS background, which cannot report an error); the wall's useCoverSrc can.
const coverUrl = computed(() => {
  const w = coverWork.value
  if (!w || (libraryUi.veil && w.nsfw === true)) return null
  if (w.local_cover) {
    const { copy_id: copyId, rel_path: relPath, version } = w.local_cover
    return api.workFileUrl({ rj: w.rj, copyId, relPath }, version)
  }
  return w.thumbnail_url
})

// ── Message line (the Intel menu's bottom sentence): what is playing, else the selection ──
const playing = computed(() => player.rj !== null)
const playClock = computed(() => `${fmtT(player.elapsed)} / ${player.duration === null ? '--:--' : fmtT(player.duration)}`)
const message = computed(() => {
  const col = current.value
  if (!col) return '还没有合集。'
  if (col.work_count === 0) return `${col.name} 还是空的。`
  return `${col.name} 收录 ${col.work_count} 部作品。`
})

// ── Titles: render them all, show the leading run that fits whole; the rest stay hidden ──
const titlesEl = ref<HTMLElement | null>(null)
const fitCount = ref(Infinity)
function measureTitles() {
  const box = titlesEl.value
  if (!box) return
  // Most titles that could fit: all one line. Taller ones fit fewer, hidden below.
  titleLimit.value = box.clientHeight > 0 ? Math.min(200, Math.max(1, Math.floor((box.clientHeight + TITLE_GAP) / (TITLE_LINE + TITLE_GAP)))) : TITLE_FALLBACK
  const items = Array.from(box.children) as HTMLElement[]
  const n = items.findIndex((el) => el.offsetTop + el.offsetHeight > box.clientHeight)
  fitCount.value = n === -1 ? items.length : n
}

// Terminal print: the newest TYPED_LINES titles type out character by character, one after another;
// the rest cascade in one line per tick. Every title is laid out in full (the unprinted part is only
// hidden), so measuring never depends on how far the print has got.
const TYPED_LINES = 3
const TYPE_STEP = 3 // characters per 16ms tick, about 190 per second
const titleText = (w: { title: string | null; rj: string }) => w.title ?? w.rj
const tick = ref(0)
const typedTicks = computed(() => {
  const lens = titles.value.slice(0, TYPED_LINES).map((w) => Math.ceil(titleText(w).length / TYPE_STEP))
  return lens.reduce<number[]>((ends, n) => [...ends, (ends.at(-1) ?? 0) + n], [])
})
const typedOf = (i: number, w: { title: string | null; rj: string }) => {
  const len = titleText(w).length
  if (i < TYPED_LINES) return Math.max(0, Math.min(len, (tick.value - (typedTicks.value[i - 1] ?? 0)) * TYPE_STEP))
  return tick.value - (typedTicks.value.at(-1) ?? 0) >= i - TYPED_LINES + 1 ? len : 0
}
let typeTimer: ReturnType<typeof setInterval> | undefined
let typedKey = ''

// Degradation: switching more than twice in quick succession (a held key, a fast scroll) prints
// instantly, and the animation only comes back once the switching has paused.
const { rapid: switching, note } = useSwitchBurst()
function noteSwitch() {
  note()
  if (switching.value) { clearInterval(typeTimer); tick.value = Infinity }
}

function startTyping() {
  clearInterval(typeTimer)
  if (switching.value || reduced()) return void (tick.value = Infinity)
  tick.value = 0
  typeTimer = setInterval(() => {
    tick.value++
    if (tick.value >= (typedTicks.value.at(-1) ?? 0) + fitCount.value) clearInterval(typeTimer)
  }, 16)
}
// A resize re-measures but keeps what is printed; a new collection (or newest work) prints again.
watch(titles, () => void nextTick(() => {
  measureTitles()
  const key = `${titlesQuery.data.value?.id}:${titles.value[0]?.rj}`
  if (key !== typedKey) { typedKey = key; startTyping() }
}), { flush: 'post' })

// ── List: the cursor mirrors the selected row and the scroll offset ──
const listEl = ref<HTMLElement | null>(null)
const cursorY = ref<number | null>(null)
const glide = ref(false)
const rowEls = ref<HTMLElement[]>([])
const pressed = ref(-1)

function syncChrome() {
  const list = listEl.value, row = rowEls.value[sel.value]
  if (!list || !row) return
  const y = list.offsetTop + row.offsetTop - list.scrollTop + row.offsetHeight / 2
  cursorY.value = y > list.offsetTop && y < list.offsetTop + list.clientHeight ? y : null
}

function reveal(row: HTMLElement) {
  const list = listEl.value
  if (!list) return
  const pad = 18, top = row.offsetTop - pad, bottom = row.offsetTop + row.offsetHeight + pad
  if (top < list.scrollTop) list.scrollTop = Math.max(0, top)
  else if (bottom > list.scrollTop + list.clientHeight) list.scrollTop = bottom - list.clientHeight
}

const reducedMotion = useReducedMotion()
const reduced = () => reducedMotion.value
let switchTimer: ReturnType<typeof setTimeout> | undefined
function select(i: number) {
  if (!collections.value.length) return
  sel.value = Math.max(0, Math.min(collections.value.length - 1, i))
  noteSwitch()
  glide.value = true
  void nextTick(() => { reveal(rowEls.value[sel.value]); syncChrome() })
  // Holding ↓ must not redraw per row: only the row you stop on switches the detail.
  clearTimeout(switchTimer)
  switchTimer = setTimeout(() => { shown.value = sel.value }, reduced() ? 0 : 140)
}
const RETURN_KEY = 'collectionReturn'
function pressSelected() {
  pressed.value = sel.value
  setTimeout(() => (pressed.value = -1), 120)
  const col = collections.value[sel.value]
  if (!col) return
  router.options.history.replace(router.currentRoute.value.fullPath, { [RETURN_KEY]: col.id })
  void router.push({ name: 'collection-detail', params: { id: col.id } })
}

// Back from a collection: the list may arrive after mount, so wait for it once.
let returnId = router.options.history.state[RETURN_KEY] as string | null | undefined
watch(collections, (list) => {
  if (!returnId || !list.length) return
  const i = list.findIndex((c) => c.id === returnId)
  returnId = null
  router.options.history.replace(router.currentRoute.value.fullPath, { [RETURN_KEY]: null })
  if (i < 0) return
  sel.value = shown.value = i
  void nextTick(() => { const row = rowEls.value[i]; if (row) reveal(row); syncChrome() })
}, { immediate: true })

// A switch (or the list arriving after mount) draws a new cover.
watch(() => current.value?.id, () => void drawCover())
watch(narrow, (n) => { if (!n) void drawCover() })
watch(collections, () => {
  if (sel.value >= collections.value.length) select(0)
  void nextTick(syncChrome)
})
// The veil toggles in the wall; here it only re-evaluates coverUrl.

// ── Keys ──
function onKeydown(e: KeyboardEvent) {
  const target = e.target instanceof Element ? e.target : null
  if (target?.closest('input, textarea, select, [contenteditable], [data-own-keys]') || e.ctrlKey || e.metaKey || e.altKey) return
  if (target?.closest('button, a') && (e.key === 'Enter' || e.key === ' ')) return
  const keys: Record<string, () => void> = {
    arrowup: () => select(sel.value - 1), arrowdown: () => select(sel.value + 1),
    w: () => select(sel.value - 1), s: () => select(sel.value + 1),
    pageup: () => select(sel.value - 5), pagedown: () => select(sel.value + 5),
    home: () => select(0), end: () => select(collections.value.length - 1),
    enter: pressSelected, r: () => void drawCover(true), ' ': () => player.togglePause(),
  }
  const run = keys[e.key.toLowerCase()]
  if (run) { e.preventDefault(); run() }
}

// ── Fit: the 4:3 cover fills the panel body; its height is the smaller of what the stage
//    allows and what leaves the text column its minimum width ──
const stage = ref<HTMLElement | null>(null)
const listWrap = ref<HTMLElement | null>(null)
const coverH = ref<number | null>(null)
function fit() {
  const el = stage.value
  if (!el) return
  if (innerWidth <= 900) return void (coverH.value = null)
  const cs = getComputedStyle(el)
  const inner = el.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight)
  const byW = ((inner - (listWrap.value?.offsetWidth ?? 0) - parseFloat(cs.columnGap) - 48 - 20 - 260) * 3) / 4
  const byH = el.clientHeight - parseFloat(cs.paddingTop) - 88
  coverH.value = Math.max(240, Math.floor(Math.min(byW, byH)))
}

let observers: ResizeObserver[] = []
onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  const make = (cb: () => void, el: Element | null) => {
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(cb)
    ro.observe(el)
    observers.push(ro)
  }
  make(measureTitles, titlesEl.value)
  make(syncChrome, listEl.value)
  make(fit, stage.value)
  fit()
  measureTitles()
  void nextTick(() => { syncChrome(); glide.value = false })
  if (current.value) void drawCover()
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  clearTimeout(switchTimer)
  clearInterval(typeTimer)
  observers.forEach((ro) => ro.disconnect())
  observers = []
  draw++
})

function onRow(i: number) {
  const selected = i === sel.value
  if (!selected) select(i)
  if (selected || narrow.value) pressSelected()
}
</script>

<template>
  <section class="page substrate" aria-label="合集" data-testid="collections-page">
    <header class="flex min-h-14 shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 edge-bottom px-4 py-1.5 rail:px-5">
      <GlobalMenu />
      <div class="min-w-0 flex-1">
        <h1 class="font-heading text-sm font-semibold">合集</h1>
        <p v-if="!narrow" class="truncate text-[11px] text-muted-foreground">
          选择一个合集，随机展示它的封面与最新作品
        </p>
      </div>
      <button class="btn btn-s" type="button" data-testid="collections-new" @click="createNew">+ NEW</button>
    </header>

    <main ref="stage" class="stage" :style="coverH ? { '--cover-h': coverH + 'px' } : undefined">
      <div ref="listWrap" class="listwrap">
        <div class="gutter">
          <i class="cursor" :class="{ glide }" :style="{ transform: `translateY(${cursorY ?? 0}px)`, visibility: cursorY === null ? 'hidden' : 'visible' }" />
        </div>
        <div class="panel">
          <div ref="listEl" class="list" role="listbox" tabindex="0" aria-label="合集"
            :aria-activedescendant="collections[sel] ? `col-${collections[sel].id}` : undefined"
            @scroll.passive="syncChrome" @wheel.passive="glide = false">
            <div v-for="(col, i) in collections" :id="`col-${col.id}`" :key="col.id"
              :ref="(el) => { if (el) rowEls[i] = el as HTMLElement }" class="row" :class="{ press: pressed === i }"
              role="option" :aria-selected="i === sel" :title="col.name" data-testid="collection-row" @click="onRow(i)">
              <i class="sq" /><span class="nm">{{ col.name }}</span>
            </div>
          </div>
        </div>
      </div>

      <section class="deck" aria-label="详情">
        <div class="deck-strip">
          <span :key="current?.id" class="in" :title="current?.name">{{ current?.name }}</span>
        </div>
        <div class="panel">
          <div class="deck-body">
            <CollectionScreen :url="coverUrl" :instant="switching" @redraw="drawCover(true)" />
            <div class="titles-col">
              <div ref="titlesEl" class="titles" lang="ja" data-testid="collection-titles">
                <div v-for="(w, i) in titles" :key="`${current?.id}-${w.rj}`" class="ti" :class="{ off: i >= fitCount }" :title="titleText(w)">
                  <span>{{ titleText(w).slice(0, typedOf(i, w)) }}<span class="rest">{{ titleText(w).slice(typedOf(i, w)) }}</span></span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>
    </main>

    <div class="hint-wrap">
      <div class="hint">
        <RouterLink v-if="playing" class="msg now" :class="{ paused: player.paused }" :to="`/works/${player.rj}`"
          :title="`打开 ${player.rj}`" data-testid="collections-now-playing">
          <i class="st" /><span class="rj">{{ player.rj }}</span><span class="tt" lang="ja">{{ player.title }}</span><span class="clk">{{ playClock }}</span>
        </RouterLink>
        <p v-else class="msg" data-testid="collections-message">{{ message }}</p>
        <div class="keys lbl"><kbd>↑</kbd><kbd>↓</kbd> SELECT<span class="sep">·</span><kbd>R</kbd> REDRAW<template v-if="playing"><span class="sep">·</span><kbd>␣</kbd> {{ player.paused ? 'PLAY' : 'PAUSE' }}</template></div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.page {
  --pad: clamp(24px, 3.2vw, 62px);
  position: relative; height: 100%; display: grid; overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr) auto;
}
.page > :not(header) { padding-inline: var(--pad); position: relative; }

.list:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }

/* Stage: list left, detail right, as in the Intel menu */
.stage {
  /* --cover-h is solved by fit(): the 4:3 cover fills the panel body, so both panels size to it
     (never to the stage: a taller panel than its cover leaves dead space under it). The panels
     centre in the stage's height, so what the cover leaves splits above and below them. */
  --cover-h: 480px;
  display: grid; grid-template-columns: clamp(300px, 24vw, 420px) minmax(0, 1fr); gap: clamp(24px, 3vw, 64px);
  align-items: start; align-content: center; min-height: 0; padding-top: var(--sp-6);
}
.listwrap, .deck { height: calc(var(--cover-h) + 88px); }

/* Panels: the panel colour, no frame; a rule near the top and bottom is the only edge line */
.panel { position: relative; min-height: 0; background: var(--bg-panel); }
.panel::before, .panel::after {
  content: ''; position: absolute; left: 12px; right: 12px; z-index: 3; pointer-events: none;
  border-top: 1px solid var(--wire-mid);
}
.panel::before { top: 8px; }
.panel::after { bottom: 8px; }

/* Left: the list */
.listwrap { display: grid; grid-template-columns: 28px minmax(0, 1fr); min-height: 0; }
.listwrap .panel, .deck .panel { height: 100%; }
/* The list floats: a hatched ghost of the panel offset 6px, no outline. Hatched, not solid, so it never
   reads as a matching bar to the gutter's double barline and boxes the panel in. 34px = gutter + offset. */
.listwrap { position: relative; isolation: isolate; }
.listwrap::after {
  content: ''; position: absolute; z-index: -1; left: 34px; top: 4px; right: -4px; bottom: -4px;
  background: repeating-linear-gradient(135deg, var(--wire-fine) 0 1px, transparent 1px 5px);
}
.gutter { position: relative; }
.gutter::before { content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 8px; background: var(--wire-fine); }
.gutter::after { content: ''; position: absolute; left: 12px; top: 0; bottom: 0; width: 3px; background: var(--wire-mid); opacity: .6; }
/* The cursor: a solid pointer that glides to the selected row */
.cursor {
  position: absolute; left: 4px; top: 0; width: 16px; height: 20px; margin-top: -10px; z-index: 2;
  background: var(--wire-strong); clip-path: polygon(0 0, 100% 50%, 0 100%);
}
.cursor.glide { transition: transform 120ms var(--ease-out); }
.list {
  position: absolute; inset: 14px 8px 14px 0; overflow-y: auto; outline: none;
  padding: 12px 14px 0;
}
.row {
  position: relative; height: 48px; margin-bottom: 12px;
  display: flex; align-items: center; gap: var(--sp-3); padding: 0 var(--sp-3); cursor: pointer;
}
.row .sq { flex: none; width: 12px; height: 12px; background: var(--invert-bg); }
.row .nm { min-width: 0; font-size: var(--fs-m); line-height: 20px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row:hover { background: var(--bg-subtle); }
.row[aria-selected="true"] { background: var(--invert-bg); color: var(--invert-fg); }
.row[aria-selected="true"] .sq { background: var(--invert-fg); }
.row[aria-selected="true"]::before, .row[aria-selected="true"]::after {
  content: ''; position: absolute; left: 0; right: 0; height: 2px; background: var(--wire-strong);
}
.row[aria-selected="true"]::before { top: -6px; }
.row[aria-selected="true"]::after { bottom: -6px; }
.row.press { translate: 2px 2px; }

/* Right: detail deck */
.deck { display: flex; flex-direction: column; min-height: 0; }
.deck-strip {
  height: 44px; flex: none; display: flex; align-items: center; gap: var(--sp-3); padding: 0 var(--sp-4);
  background: var(--bg-subtle); box-shadow: inset 0 -1px 0 var(--wire-fine);
  font-size: var(--fs-l); font-weight: 500; white-space: nowrap; overflow: hidden;
}
.deck-strip::before { content: ''; flex: none; width: 14px; height: 14px; background: var(--invert-bg); }
.deck-strip span { overflow: hidden; text-overflow: ellipsis; animation: wipe 200ms var(--ease-out) both; }
@keyframes wipe { from { clip-path: inset(0 100% 0 0); } to { clip-path: inset(0 0 0 0); } }
.deck .panel { flex: 1; height: auto; }
.deck-body { position: absolute; inset: 0; display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--sp-5); padding: 22px 24px; }

/* The text column: the newest titles, as many lines as fit. Titles are work
   titles, hence the mincho face. */
.titles-col { position: relative; min-height: 0; padding-right: 18px; }
.titles { height: 100%; overflow: hidden; }
.ti.off { visibility: hidden; }
.ti {
  margin-bottom: 10px; line-height: 22px; /* TITLE_GAP, TITLE_LINE */ overflow: hidden; overflow-wrap: anywhere;
  display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 3; line-clamp: 3;
  font-family: var(--font-mincho); font-size: var(--fs-m); font-weight: 500;
}
.ti .rest { visibility: hidden; }

/* Hint bar */
.hint-wrap { padding-top: 22px; padding-bottom: 12px; }
.hint {
  position: relative; height: 52px; display: flex; align-items: center; justify-content: flex-end; gap: var(--sp-5);
  padding: 0 40px 0 28px; border-left: 6px solid var(--invert-bg);
  background: var(--bg-panel); box-shadow: inset 0 0 0 1px var(--wire-fine), inset 0 -2px 0 var(--wire-fine);
}
.hint::after { content: ''; position: absolute; right: 6px; bottom: 6px; width: 8px; height: 8px; background: var(--invert-bg); }
/* The message takes the free width; a long title truncates before the key hints do */
.msg { flex: 1; min-width: 0; margin-right: auto; font-size: var(--fs-m); letter-spacing: .06em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.msg.now { display: flex; align-items: center; gap: var(--sp-3); color: inherit; text-decoration: none; }
.msg.now:hover .tt { text-decoration: underline; text-underline-offset: 4px; }
.msg.now .st { flex: none; width: 10px; height: 10px; background: var(--invert-bg); clip-path: polygon(0 0, 100% 50%, 0 100%); }
.msg.now.paused .st { clip-path: none; background: linear-gradient(90deg, var(--invert-bg) 35%, transparent 35% 65%, var(--invert-bg) 65%); }
.msg.now .rj, .msg.now .clk { flex: none; font-family: var(--font-mono); font-size: var(--fs-s); color: var(--fg-meta); }
.msg.now .tt { min-width: 0; overflow: hidden; text-overflow: ellipsis; font-family: var(--font-mincho); }
/* Same key legend as the work inspector's footer */
.keys { flex: none; }
.keys kbd {
  font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted);
  border: 1px solid var(--wire-fine); padding: 0 4px; margin-right: 2px;
}

@media (max-width: 900px) {
  /* Phones: the list alone, filling the height between header and hint bar. */
  .page { --pad: var(--sp-4); grid-template-rows: auto minmax(0, 1fr) auto; }
  .stage { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); align-items: stretch; padding-top: var(--sp-4); }
  .listwrap { height: auto; }
  .deck { display: none; }
  .keys { display: none; }
  .hint-wrap { padding-top: var(--sp-4); padding-bottom: var(--sp-4); }
}
</style>

