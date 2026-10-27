<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useMediaQuery, WALL_MOBILE_QUERY } from '@/composables/use-media-query'
import { useSwitchBurst } from '@/composables/use-motion'
import type { WorkSummary } from '@/lib/api/types'
import WorkCard from './WorkCard.vue'

// The wall: one page of works as a plain grid, with roving focus and one
// square cursor. The page owns paging; the wall only reports a
// keyboard move off either end of the page. The column count is computed here
// and also drives the grid's inline style, so the layout and the JS that moves
// the cursor and the keyboard can never disagree. A selection change only
// toggles classes; the grid itself is never rebuilt.

const props = defineProps<{
  works: WorkSummary[]
  selectedRj: string | null
  playingRj: string | null
}>()

const emit = defineEmits<{
  select: [rj: string]
  open: [rj: string]
  /** A ±1 keyboard move ran off the page's first (-1) or last (+1) work. */
  edge: [direction: -1 | 1]
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
const isMobile = useMediaQuery(WALL_MOBILE_QUERY)

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  // jsdom has no ResizeObserver; the same guard use-container-tier carries.
  if (gridEl.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver((entries) => {
      gridWidth.value = entries[0]?.contentRect.width ?? 0
    })
    resizeObserver.observe(gridEl.value)
  }
})
onBeforeUnmount(() => {
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

const selectedIndex = computed(() =>
  props.selectedRj === null ? -1 : props.works.findIndex((work) => work.rj === props.selectedRj),
)

// Set by a keyboard move: only then does the new selection take focus. Any
// other selection change — a filter answering, a page landing, the address
// restoring a work — must leave focus wherever the user put it.
let focusOnSelect = false

/**
 * Column-aware movement for the page's keyboard dispatch: ±1 or ±columns.
 * With nothing on the wall selected, the first move lands on the first work. A
 * ±1 move past either end asks the page for the neighbouring page instead.
 */
function move(delta: number): void {
  if (props.works.length === 0) return
  const raw = selectedIndex.value < 0 ? 0 : selectedIndex.value + delta
  if (selectedIndex.value >= 0 && Math.abs(delta) === 1 && (raw < 0 || raw >= props.works.length)) {
    emit('edge', raw < 0 ? -1 : 1)
    return
  }
  const next = Math.min(props.works.length - 1, Math.max(0, raw))
  if (next === selectedIndex.value) return
  focusOnSelect = true
  emit('select', props.works[next].rj)
}

/** The next selection takes focus: a page turned from the keyboard. */
function claimFocus(): void {
  focusOnSelect = true
}

function cellFor(rj: string): HTMLElement | null {
  return gridEl.value?.querySelector<HTMLElement>(`.cell[data-rj="${CSS.escape(rj)}"]`) ?? null
}

defineExpose({
  columns,
  move,
  claimFocus,
  cellFor,
  get scroller() {
    return wallEl.value
  },
})

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
  // Every row is the same height, so the cursor is placed by arithmetic rather
  // than by measuring the cell.
  cursorX.value = inRow * (colWidth.value + gapX.value) - 14
  cursorY.value = rowIndex * rowHeight.value + coverHeight.value + 14
  idleTimer = window.setTimeout(() => {
    cursorIdle.value = true
  }, 260)
}

// A new page starts at its top; the selection then scrolls itself into view.
watch(() => props.works, () => {
  if (wallEl.value) wallEl.value.scrollTop = 0
})

// Keep the selected work in view; focus its cell only after a keyboard move.
let mounted = false
const { rapid: switching, note } = useSwitchBurst()
// The work and its index both count: a page turn can land on the same index.
watch([() => props.selectedRj, selectedIndex], async () => {
  note()
  placeCursor()
  if (!mounted || selectedIndex.value < 0) return
  const claim = focusOnSelect
  focusOnSelect = false
  await nextTick()
  const cell = props.selectedRj === null ? null : cellFor(props.selectedRj)
  cell?.scrollIntoView?.({ block: 'nearest' })
  if (claim) cell?.focus({ preventScroll: true })
}, { flush: 'post' })
watch([columns, colWidth], () => placeCursor())
onMounted(() => {
  mounted = true
  placeCursor()
})

function onCellClick(work: WorkSummary) {
  // Mobile has no "inspect the selection": a tap routes straight to the detail
  // page, so the wall never keeps a selected state there.
  if (isMobile.value) {
    emit('open', work.rj)
    return
  }
  emit('select', work.rj)
}
</script>

<template>
  <section ref="wallEl" class="wall" data-testid="work-wall">
    <div ref="gridEl" class="grid" :class="{ 'wall-rapid': switching }" role="listbox" aria-label="Works" :style="{
      gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
      columnGap: `${gapX}px`,
      rowGap: `${GAP_Y}px`,
    }">
      <div class="cursor" :class="{ idle: cursorIdle }" aria-hidden="true"
        :style="{ transform: `translate(${cursorX}px, ${cursorY}px)` }"></div>
      <WorkCard v-for="(work, index) in works" :key="work.rj" :work="work" :index="index"
        :selected="work.rj === selectedRj" :playing="work.rj === playingRj"
        @click="onCellClick(work)" @dblclick="emit('open', work.rj)" />
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

/* The JS column count is the only authority; cards size themselves inside it. */
.grid {
  position: relative; display: grid; align-items: start;
}

.cursor {
  position: absolute; left: 0; top: 0; width: 8px; height: 8px;
  background: var(--invert-bg); pointer-events: none; z-index: 3;
  transition: transform var(--dur-cursor) var(--ease-out);
  will-change: transform;
}
.wall-rapid .cursor { transition: none; }
.cursor.idle { animation: blink 200ms steps(1, end) 2; }
@keyframes blink { 50% { opacity: 0; } }

@media (max-width: 768px) {
  .cursor { display: none; }
}
</style>
