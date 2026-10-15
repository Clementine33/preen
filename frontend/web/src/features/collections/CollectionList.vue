<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { REDUCED_MOTION_QUERY, useMediaQuery } from '@/composables/use-media-query'
import { usePlayWork } from '@/features/works/use-play-work'
import { useApiClient } from '@/lib/api/client'
import type { WorkSummary } from '@/lib/api/types'
import { queryKeys } from '@/queries/query-keys'
import { usePlayerStore } from '@/stores/player'
import { landingIndex, moveIntent } from './move-intent'
import CollectionRow from './CollectionRow.vue'

// A collection's members as one continuously scrolling ordered list. The page
// owns the data (chunked, see useCollectionPages) and the view sort; this list
// owns the cursor and the one unfolded row: a click selects and unfolds (a
// second click folds it again), double-click, Enter or the fold's OPEN opens
// the work, the play button plays it, Space pauses, Esc folds. The next chunk
// is requested as the end of the list scrolls into view. In edit mode members
// are arranged instead: rows neither unfold, open nor play; the handle drags a
// row to a gap, the row menu moves it (custom order only), the menu or Delete
// removes it, and Esc leaves the mode. The page does the writing and the
// cursor follows the row. Moves are shown by an insertion line, not by rows
// sliding.
const props = defineProps<{
  works: WorkSummary[]
  total: number
  /** Whether the rows are in saved order, where `#` is the saved position. */
  custom: boolean
  direction: 'asc' | 'desc'
  hasMore: boolean
  editing: boolean
}>()
const emit = defineEmits<{ more: []; move: [rj: string, gap: number]; remove: [rj: string]; done: [] }>()

const router = useRouter()
const api = useApiClient()
const client = useQueryClient()
const player = usePlayerStore()
const { playWork } = usePlayWork()
const reducedMotion = useMediaQuery(REDUCED_MOTION_QUERY)

const sel = ref(0)
const scroller = ref<HTMLElement | null>(null)
const sentinel = ref<HTMLElement | null>(null)

const position = (i: number) => (props.custom && props.direction === 'desc' ? props.total - i : i + 1)

function open(work: WorkSummary) {
  if (props.editing) return
  void router.push({ name: 'work-detail', params: { rj: work.rj } })
}
function play(work: WorkSummary) {
  void playWork({ rj: work.rj, title: work.title ?? work.rj })
}

// ── The fold: the cursor moves at once, the tracklist follows it. Leaving a
// row folds it (the rows below close up with it); the row the cursor settles
// on unfolds FOLD_DELAY later (a click: at once), and only once its manifest is
// in hand, so it opens at full height in one motion and cruising with the keys
// fetches only the rows it stops on. ──
const FOLD_DELAY = 200
const unfolded = ref<string | null>(null)
let foldTimer: ReturnType<typeof setTimeout> | undefined
let foldTicket = 0

function fold() {
  ++foldTicket
  clearTimeout(foldTimer)
  unfolded.value = null
}
function unfoldLater(rj: string, delay: number) {
  if (props.editing) return
  const ticket = foldTicket
  foldTimer = setTimeout(async () => {
    await client.ensureQueryData({
      queryKey: queryKeys.works.content(rj),
      queryFn: ({ signal }) => api.getWorkContent(rj, signal),
    }).catch(() => undefined)
    if (ticket === foldTicket) unfolded.value = rj
  }, delay)
}
function select(i: number, delay = FOLD_DELAY) {
  const to = Math.max(0, Math.min(props.works.length - 1, i))
  const rj = props.works[to]?.rj
  if (!rj) return
  if (to === sel.value && unfolded.value === rj) return
  fold()
  sel.value = to
  unfoldLater(rj, delay)
}
function click(i: number) {
  if (i === sel.value && unfolded.value === props.works[i]?.rj) fold()
  else select(i, 0)
}
function move(to: number) {
  select(to)
  if (sel.value >= props.works.length - 5 && props.hasMore) emit('more')
  void nextTick(() => scroller.value?.querySelector('.row.sel')?.scrollIntoView?.({ block: 'nearest' }))
}
/** A row that has just unfolded is brought into view whole, if it fits. */
function reveal(rj: string) {
  scroller.value?.querySelector(`[data-rj="${rj}"]`)?.parentElement
    ?.scrollIntoView?.({ block: 'nearest', behavior: reducedMotion.value ? 'auto' : 'smooth' })
}

// A different order is a different list: the cursor goes back to its top, and
// the first row unfolds once there is one.
watch(() => [props.custom, props.direction], () => {
  fold()
  sel.value = 0
})
watch(() => props.works[0]?.rj, (rj) => {
  if (rj && sel.value === 0 && unfolded.value === null) unfoldLater(rj, 0)
}, { immediate: true })

// ── Editing: gaps are 0..n, gap g lying above row g. ──
const dragging = ref<string | null>(null)
const gap = ref<number | null>(null)

function moveTo(from: number, to: number) {
  const rj = props.works[from]?.rj
  if (!rj || !moveIntent(props.works.map((w) => w.rj), from, to, props.direction, !props.hasMore)) return
  sel.value = landingIndex(from, to)
  emit('move', rj, to)
}
function menuMove(i: number, to: 'top' | 'up' | 'down' | 'bottom') {
  moveTo(i, { top: 0, up: i - 1, down: i + 2, bottom: props.works.length }[to])
}
function removeAt(i: number) {
  const rj = props.works[i]?.rj
  if (!rj) return
  const next = props.works[i + 1] ?? props.works[i - 1]
  fold()
  sel.value = Math.max(0, Math.min(i, props.works.length - 2))
  if (next) unfoldLater(next.rj, FOLD_DELAY)
  emit('remove', rj)
}

function dragStart(e: DragEvent, rj: string) {
  dragging.value = rj
  if (!e.dataTransfer) return
  e.dataTransfer.effectAllowed = 'move'
  e.dataTransfer.setData('text/plain', rj)
  const row = (e.target as HTMLElement | null)?.closest('.row')
  if (row) e.dataTransfer.setDragImage?.(row, 16, 28)
}
/** The gap under the pointer: the upper half of a row is above it, the lower half below. */
function gapAt(e: DragEvent): number | null {
  const row = (e.target as HTMLElement | null)?.closest<HTMLElement>('.row')
  const i = props.works.findIndex((w) => w.rj === row?.dataset.rj)
  if (!row || i < 0) return null
  const box = row.getBoundingClientRect()
  return i + (e.clientY > box.top + box.height / 2 ? 1 : 0)
}
function dragOver(e: DragEvent) {
  if (dragging.value === null) return
  e.preventDefault()
  gap.value = gapAt(e)
}
function drop(e: DragEvent) {
  if (dragging.value === null) return
  e.preventDefault()
  const to = gapAt(e) ?? gap.value
  const from = props.works.findIndex((w) => w.rj === dragging.value)
  dragEnd()
  if (to !== null && from >= 0) moveTo(from, to)
}
function dragEnd() { dragging.value = null; gap.value = null }
/** The insertion line sits above the row at the gap, or below the last row. */
const dropMark = (i: number) => (gap.value === i ? 'above' : gap.value === i + 1 && i === props.works.length - 1 ? 'below' : null)

// Entering edit mode folds the open row; leaving it unfolds the selected one again.
watch(() => props.editing, (on) => {
  fold()
  const rj = props.works[sel.value]?.rj
  if (!on && rj) unfoldLater(rj, 0)
})

/** A text field or an open menu keeps every key it receives. */
function ownsKeys(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.closest('[data-own-keys], input, textarea, select, [contenteditable]') !== null
}
/** Enter and Space already mean something on a link or a button. */
function activates(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.closest('a, button') !== null
}
function onKeydown(e: KeyboardEvent) {
  if (ownsKeys(e.target) || e.ctrlKey || e.metaKey || e.altKey) return
  const keys: Record<string, () => void> = {
    ArrowUp: () => move(sel.value - 1), ArrowDown: () => move(sel.value + 1),
    Home: () => move(0), End: () => move(props.works.length - 1),
    Escape: () => (props.editing ? emit('done') : fold()),
    ...(props.editing ? { Delete: () => removeAt(sel.value) } : {}),
  }
  if (!activates(e.target)) {
    keys.Enter = () => props.works[sel.value] && open(props.works[sel.value]!)
    keys[' '] = () => player.togglePause()
  }
  const run = keys[e.key]
  if (run) { e.preventDefault(); run() }
}

let observer: IntersectionObserver | undefined
onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  // jsdom has no IntersectionObserver; the keys still ask for more near the end.
  if (typeof IntersectionObserver === 'undefined' || !sentinel.value) return
  observer = new IntersectionObserver((entries) => { if (entries.some((e) => e.isIntersecting) && props.hasMore) emit('more') }, { root: scroller.value, rootMargin: '600px' })
  observer.observe(sentinel.value)
})
onBeforeUnmount(() => {
  clearTimeout(foldTimer)
  document.removeEventListener('keydown', onKeydown)
  observer?.disconnect()
})
// A chunk that does not fill the viewport leaves the sentinel in view without a new
// intersection change; re-observing reports it again.
watch(() => props.works.length, () => void nextTick(() => {
  if (!observer || !sentinel.value) return
  observer.unobserve(sentinel.value)
  observer.observe(sentinel.value)
}))

const active = computed(() => props.works[sel.value]?.rj)
</script>

<template>
  <div ref="scroller" class="list" role="listbox" aria-label="合集作品" tabindex="-1"
    :aria-activedescendant="active ? `cw-${active}` : undefined" @dragover="dragOver" @drop="drop">
    <CollectionRow v-for="(w, i) in works" :key="w.rj" :work="w" :position="position(i)" :selected="i === sel"
      :expanded="unfolded === w.rj" :playing="player.rj === w.rj"
      :editing="editing" :movable="editing && custom" :first="i === 0" :last="i === works.length - 1 && !hasMore" :drop="dropMark(i)"
      @select="click(i)" @open="open(w)" @play="play(w)" @unfolded="reveal(w.rj)"
      @dragstart="dragStart($event, w.rj)" @dragend="dragEnd" @move="menuMove(i, $event)" @remove="removeAt(i)" />
    <div ref="sentinel" class="sentinel" aria-hidden="true" />
  </div>
</template>

<style scoped>
.list { min-height: 0; overflow-y: auto; padding: var(--sp-2) var(--sp-6); outline: none; }
.sentinel { height: 1px; }
@media (max-width: 900px) { .list { padding-inline: var(--sp-4); } }
</style>
