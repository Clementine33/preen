<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import type { WorkSummary } from '@/lib/api/types'
import WorkCard from './WorkCard.vue'

// The wall: a virtualized grid of works with roving focus and one square
// cursor (ADR 0012 §4). The column count is computed here and also drives the
// grid's inline style, so the layout and the JS that moves the cursor and the
// keyboard can never disagree. A selection change only toggles classes; the
// wall itself is never rebuilt.

const props = defineProps<{
  works: WorkSummary[]
  selectedRj: string | null
  playingRj: string | null
}>()

const emit = defineEmits<{
  select: [rj: string]
  open: [rj: string]
  /** The wall scrolled near its end: the page fetches the next keyset page. */
  loadMore: []
}>()

const MIN_COL = 184
const GAP_X = 20
const GAP_Y = 28
const MOBILE_GAP_X = 12
/** The caption block under a cover: margin 8 + padding 4 + meta 12 + title 32 + 6. */
const CAP_HEIGHT = 62

const wallEl = ref<HTMLElement | null>(null)
const gridEl = ref<HTMLElement | null>(null)
const gridWidth = ref(0)
const isMobile = ref(false)

let resizeObserver: ResizeObserver | null = null
let mobileQuery: MediaQueryList | null = null
function syncMobile() {
  isMobile.value = mobileQuery?.matches ?? false
}

onMounted(() => {
  // Read at mount, not at import: the query reflects the media state when the
  // wall actually appears.
  mobileQuery = typeof matchMedia === 'function' ? matchMedia('(max-width: 768px)') : null
  syncMobile()
  mobileQuery?.addEventListener('change', onMediaChange)
  // jsdom has no ResizeObserver; the same guard use-container-tier carries.
  if (gridEl.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver((entries) => {
      gridWidth.value = entries[0]?.contentRect.width ?? 0
    })
    resizeObserver.observe(gridEl.value)
  }
})
function onMediaChange() {
  syncMobile()
}
onBeforeUnmount(() => {
  mobileQuery?.removeEventListener('change', onMediaChange)
  resizeObserver?.disconnect()
})

// The prototype wall is repeat(auto-fill, minmax(184px, 1fr)); with the gutter
// known, the count a browser would pick is deterministic, so we compute it.
// Mobile is pinned to the prototype's 2 columns.
const columns = computed(() =>
  isMobile.value ? 2 : Math.max(1, Math.floor((gridWidth.value + GAP_X) / (MIN_COL + GAP_X))),
)
const gapX = computed(() => (isMobile.value ? MOBILE_GAP_X : GAP_X))
const colWidth = computed(
  () => (gridWidth.value - (columns.value - 1) * gapX.value) / columns.value,
)
const coverHeight = computed(() => (colWidth.value * 3) / 4)
const rowHeight = computed(() => coverHeight.value + CAP_HEIGHT + GAP_Y)

const rows = computed(() => {
  const out: WorkSummary[][] = []
  for (let i = 0; i < props.works.length; i += columns.value) {
    out.push(props.works.slice(i, i + columns.value))
  }
  return out
})

const virtualizer = useVirtualizer(
  computed(() => ({
    count: rows.value.length,
    getScrollElement: () => wallEl.value,
    estimateSize: () => rowHeight.value,
    overscan: 3,
  })),
)
const virtualRows = computed(() => virtualizer.value.getVirtualItems())
const totalSize = computed(() => virtualizer.value.getTotalSize())

const selectedIndex = computed(() =>
  props.selectedRj === null ? -1 : props.works.findIndex((work) => work.rj === props.selectedRj),
)

/** Column-aware movement for the page's keyboard dispatch: ±1 or ±columns. */
function move(delta: number): void {
  if (selectedIndex.value < 0 || props.works.length === 0) return
  const next = Math.min(props.works.length - 1, Math.max(0, selectedIndex.value + delta))
  if (next !== selectedIndex.value) emit('select', props.works[next].rj)
}

defineExpose({ columns, move })

// ── The square cursor: one for the whole wall. It slides to the current
// caption's left (160ms), stays solid while cruising, and blinks twice about
// 260ms after a stop. Hidden on mobile, where taps route instead. ──
const cursorX = ref(0)
const cursorY = ref(0)
const cursorIdle = ref(false)
let idleTimer: number | null = null

function placeCursor() {
  cursorIdle.value = false
  if (idleTimer !== null) window.clearTimeout(idleTimer)
  if (selectedIndex.value < 0) return
  const rowIndex = Math.floor(selectedIndex.value / columns.value)
  const inRow = selectedIndex.value % columns.value
  // Virtual rows sit at their own offset (all rows are the same height), so the
  // cursor composes the row offset with the in-row offset; offsetTop of the
  // transformed cell would not include the virtualizer's translate.
  cursorX.value = inRow * (colWidth.value + gapX.value) - 14
  cursorY.value = rowIndex * rowHeight.value + coverHeight.value + 14
  idleTimer = window.setTimeout(() => {
    cursorIdle.value = true
  }, 260)
}

// Keep the selected work in view and focus its cell — except on the initial
// mount, which restores the selection from the address without claiming focus.
let mounted = false
watch(selectedIndex, async () => {
  placeCursor()
  if (!mounted) return
  if (selectedIndex.value >= 0) {
    virtualizer.value.scrollToIndex(Math.floor(selectedIndex.value / columns.value), {
      align: 'auto',
    })
    await nextTick()
    gridEl.value
      ?.querySelector<HTMLElement>(`.cell[tabindex="0"]`)
      ?.focus({ preventScroll: true })
  }
})
watch([columns, colWidth], () => placeCursor())
onMounted(() => {
  mounted = true
  placeCursor()
})

function onCellClick(work: WorkSummary) {
  // Mobile has no "inspect the selection": a tap routes straight to the detail
  // page (ADR 0012 §4), so the wall never keeps a selected state there.
  if (isMobile.value) {
    emit('open', work.rj)
    return
  }
  emit('select', work.rj)
}

// Keyset paging: ask for the next page before the scroll reaches the bottom.
function onScroll() {
  const el = wallEl.value
  if (el === null) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 600) emit('loadMore')
}
</script>

<template>
  <section ref="wallEl" class="wall" data-testid="work-wall" @scroll.passive="onScroll">
    <div ref="gridEl" class="grid" role="listbox" aria-label="Works" :style="{
      gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
      columnGap: `${gapX}px`,
      height: `${totalSize}px`,
    }">
      <div class="cursor" :class="{ idle: cursorIdle }" aria-hidden="true"
        :style="{ transform: `translate(${cursorX}px, ${cursorY}px)` }"></div>
      <div v-for="virtualRow in virtualRows" :key="virtualRow.index" class="vrow"
        :style="{ transform: `translateY(${virtualRow.start}px)` }">
        <WorkCard v-for="work in rows[virtualRow.index]" :key="work.rj" :work="work"
          :selected="work.rj === selectedRj" :playing="work.rj === playingRj"
          @click="onCellClick(work)" @dblclick="emit('open', work.rj)" />
      </div>
    </div>
  </section>
</template>

<style scoped>
.wall {
  overflow-y: auto; min-height: 0; height: 100%;
  padding: var(--sp-6) var(--sp-6) 48px;
}
@media (max-width: 768px) {
  .wall { padding: var(--sp-4) var(--sp-4) 40px; }
}

.grid {
  position: relative; display: grid;
}
/* The virtual row mirrors the grid's own columns and gutters, so the JS column
   count is the only authority and cards size themselves inside it. */
.vrow {
  position: absolute; top: 0; left: 0; width: 100%;
  display: grid;
  grid-template-columns: inherit;
  column-gap: inherit;
}

.cursor {
  position: absolute; left: 0; top: 0; width: 8px; height: 8px;
  background: var(--invert-bg); pointer-events: none; z-index: 3;
  transition: transform var(--dur-cursor) var(--ease-out);
  will-change: transform;
}
.cursor.idle { animation: blink 200ms steps(1, end) 2; }
@keyframes blink { 50% { opacity: 0; } }

@media (max-width: 768px) {
  .cursor { display: none; }
}
</style>
