<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMediaQuery } from '@/composables/use-media-query'

// The wall's pager: a centred row of page numbers on an even pitch, separated
// by faint dots. The current page is set in Mincho and flanked ❖ 3 ❖, the two
// marks taking the places of its neighbouring dots, so the pitch never breaks.
// A page turn moves the pair staccato — glide, hold, glide — the leading mark
// first, then settles it around the new number; each dot it passes flashes.
// It only reports the page asked for; the address is the page's to change.

const props = defineProps<{ page: number; pageCount: number }>()
const emit = defineEmits<{ go: [page: number] }>()

/** A phone keeps one neighbour either side; a gap is still marked as one. */
const narrow = useMediaQuery('(max-width: 640px)')

/** One place in the row; n is the page number, 0 for the others. */
interface Step { kind: 'prev' | 'next' | 'gap' | 'page'; n: number }
const steps = computed<Step[]>(() => [
  { kind: 'prev', n: 0 },
  ...pagerItems(props.page, props.pageCount, narrow.value ? 1 : 2).map((n): Step => (n === null ? { kind: 'gap', n: 0 } : { kind: 'page', n })),
  { kind: 'next', n: 0 },
])

const row = ref<HTMLElement | null>(null)
/** The current number's centre, and the dots' centre line. */
const markX = ref(0)
const markY = ref(0)
/** Half the current measure plus half a dot slot: where each mark sits. */
const flank = ref(0)
/** A dot and its margins (.dot), the slot each mark takes. */
const DOT_SLOT = 16
const MARK_SIZE = 7

// The turn's timing: each hop glides, then holds — tick, rest, tick. A long
// jump takes at most MAX_HOPS longer hops, so no turn runs past ~250ms. The
// mark on the side of travel leads; the other trails a little behind.
const GLIDE = 30
const HOLD = 15
const MAX_HOPS = 5
const TRAIL = 20
const SETTLE = 120
const EASE = 'cubic-bezier(.2, .8, .2, 1)'

/** The dot before step i, when that dot borders the current page. */
const flanking = (i: number) => [steps.value[i], steps.value[i - 1]].some((s) => s.kind === 'page' && s.n === props.page)
/** Where a mark sits beside a number centred at x, opened by `spread` pixels. */
const markAt = (x: number, side: -1 | 1, spread = 0) =>
  `translate(${Math.round(x + side * flank.value - MARK_SIZE / 2) + side * spread}px, ${Math.round(markY.value - MARK_SIZE / 2)}px)`

function place(): void {
  const current = row.value?.querySelector<HTMLElement>('[aria-current="page"]')
  const dot = row.value?.querySelector<HTMLElement>('.dot')
  if (!current) return
  markX.value = current.offsetLeft + current.offsetWidth / 2
  flank.value = current.offsetWidth / 2 + DOT_SLOT / 2
  if (dot) markY.value = dot.offsetTop + dot.offsetHeight / 2
}

const reduced = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
const centre = (el: HTMLElement) => el.offsetLeft + el.offsetWidth / 2

/** The pair hops from the number at fromX to the current one, then settles. */
function travel(el: HTMLElement, fromX: number): void {
  const toX = markX.value
  const distance = toX - fromX
  if (!distance) return
  const dots = [...el.querySelectorAll<HTMLElement>('.dot:not(.flanking)')]
    .filter((dot) => (centre(dot) - fromX) * (centre(dot) - toX) < 0)
  const hops = Math.min(MAX_HOPS, Math.max(1, dots.length + 1))
  const span = hops * GLIDE + (hops - 1) * HOLD
  const forward = distance > 0

  el.querySelectorAll<SVGElement>('.mark').forEach((mark) => {
    const side = Number(mark.dataset.side) as -1 | 1
    const delay = side === 1 === forward ? 0 : TRAIL
    const frames: Keyframe[] = [{ offset: 0, transform: markAt(fromX, side), easing: EASE }]
    for (let k = 1; k <= hops; k++) {
      const x = fromX + (distance * k) / hops
      const landed = (k - 1) * (GLIDE + HOLD) + GLIDE
      frames.push({ offset: landed / span, transform: markAt(x, side), easing: 'linear' })
      if (k < hops) frames.push({ offset: (landed + HOLD) / span, transform: markAt(x, side), easing: EASE })
    }
    mark.animate?.(frames, { duration: span, delay, fill: 'backwards' })
    // The landing: the pair breathes out a pixel and a half around the number.
    mark.animate?.([
      { transform: markAt(toX, side) },
      { transform: markAt(toX, side, 1.5), offset: 0.4 },
      { transform: markAt(toX, side) },
    ], { duration: SETTLE, delay: delay + span, easing: EASE })
  })

  // Each dot the leading mark passes flashes and fades, a short afterglow.
  const style = getComputedStyle(el)
  const bright = style.getPropertyValue('--fg-base').trim()
  const dim = style.getPropertyValue('--wire-mid').trim()
  for (const dot of dots) {
    const at = (Math.abs(centre(dot) - fromX) / Math.abs(distance)) * span
    dot.animate?.([{ backgroundColor: bright }, { backgroundColor: dim }], { duration: 220, delay: at, easing: 'ease-out' })
  }
}

watch(() => [props.page, props.pageCount] as const, ([to], [from]) => {
  const before = markX.value
  place()
  const el = row.value
  if (to === from || !el || reduced()) return
  // Measured in the new row where possible: the window may have shifted under
  // the pair, and the motion should go from the old number to the new one.
  const old = el.querySelector<HTMLElement>(`[aria-label="Page ${from}"]`)
  travel(el, old ? centre(old) : before)
  // The numbers trade faces: the new one rises into Mincho a beat after the
  // pair sets off, the old one fades back into the row.
  el.querySelector<HTMLElement>('.current')?.animate?.(
    [{ opacity: 0, transform: 'translateY(1px) scale(.9)' }, { opacity: 1, transform: 'none' }],
    { duration: 160, delay: 60, easing: EASE, fill: 'backwards' })
  old?.animate?.([{ opacity: 0 }, { opacity: 1 }], { duration: 140, easing: 'ease-out' })
}, { flush: 'post' })

// The row appears once there is a second page, often after the first fetch.
// From then on, a narrower window or any reflow moves the current number, so
// the pair follows the row's size.
watch(row, (el, _, onCleanup) => {
  if (!el) return
  place()
  if (typeof ResizeObserver !== 'function') return
  const observer = new ResizeObserver(place)
  observer.observe(el)
  onCleanup(() => observer.disconnect())
}, { flush: 'post' })
</script>

<script lang="ts">
export function pagerItems(page: number, count: number, reach = 2): (number | null)[] {
  const wanted = new Set([1, count])
  for (let n = page - reach; n <= page + reach; n++) if (n >= 1 && n <= count) wanted.add(n)
  const sorted = [...wanted].sort((a, b) => a - b)
  const out: (number | null)[] = []
  for (const n of sorted) {
    const last = out.at(-1)
    // A gap of exactly one page shows that page rather than an ellipsis.
    if (typeof last === 'number' && n - last === 2) out.push(last + 1)
    else if (typeof last === 'number' && n - last > 2) out.push(null)
    out.push(n)
  }
  return out
}

// The mark, ❖: four small diamonds around an open cross on an 11px grid, set
// so the cross between them is a full pixel wide and reads at one-to-one.
const MARK = 'M5.5.2l2.3 2.3-2.3 2.3-2.3-2.3zM5.5 6.2l2.3 2.3-2.3 2.3-2.3-2.3zM.2 5.5l2.3-2.3 2.3 2.3-2.3 2.3zM6.2 5.5l2.3-2.3 2.3 2.3-2.3 2.3z'
</script>

<template>
  <nav v-if="pageCount > 1" class="folio" aria-label="Pages" :style="{ '--digits': String(pageCount).length }">
    <div ref="row" class="row">
      <span v-for="(step, i) in steps" :key="step.kind === 'page' ? step.n : `${step.kind}-${i}`" class="step">
        <i v-if="i > 1 && i < steps.length - 1" class="dot" :class="{ flanking: flanking(i) }" aria-hidden="true"></i>
        <button v-if="step.kind === 'prev'" class="cell turn" type="button" aria-label="Previous page"
          :disabled="page <= 1" @click="emit('go', page - 1)">‹</button>
        <button v-else-if="step.kind === 'next'" class="cell turn" type="button" aria-label="Next page"
          :disabled="page >= pageCount" @click="emit('go', page + 1)">›</button>
        <span v-else-if="step.kind === 'gap'" class="cell gap" aria-hidden="true">…</span>
        <button v-else-if="step.n === page" class="cell current" type="button" :aria-label="`Page ${step.n}`"
          aria-current="page" tabindex="-1">{{ step.n }}</button>
        <button v-else class="cell" type="button" :aria-label="`Page ${step.n}`"
          @click="emit('go', step.n)">{{ step.n }}</button>
      </span>
      <svg v-for="side in ([-1, 1] as const)" :key="side" class="mark" :data-side="side" viewBox="0 0 11 11"
        aria-hidden="true" :style="{ transform: markAt(markX, side) }"><path :d="MARK" /></svg>
    </div>
  </nav>
</template>

<style scoped>
/* One line, 4px of air above and below. */
.folio { display: flex; justify-content: center; padding: 4px var(--sp-6); }
.row { position: relative; display: flex; align-items: baseline; height: 20px; }
/* Steps only group a dot with its measure: the row lays out both directly,
   so the numbers share one baseline and the dots one centre line. */
.step { display: contents; }

/* The rhythm: every measure is one width — set by the last page's digits, so
   page 1 and page 128 share it — and every dot sits at the midpoint between
   two measures. The turns stand outside the beat, after a rest. */
.dot {
  /* Sits on the numbers' baseline, lifted to mid x-height. */
  align-self: baseline; position: relative; top: -3px; flex-shrink: 0; width: 2px; height: 2px; margin: 0 7px; background: var(--wire-mid); }
/* A dot beside the current page gives its slot to a mark. */
.dot.flanking { visibility: hidden; }
.cell {
  position: relative; display: inline-flex; justify-content: center; align-items: baseline;
  width: calc(max(var(--digits), 2) * 8px + 4px); height: 20px; padding: 0;
  background: none; border: 0; cursor: pointer;
  font-family: var(--font-mono); font-size: var(--fs-xs); line-height: 20px; letter-spacing: .04em;
  color: var(--fg-meta); transition: color var(--dur-in) var(--ease-out);
}
.cell:hover:not(:disabled) { color: var(--fg-base); }
.cell:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: -1px; }
.turn { width: 20px; margin: 0 20px; font-size: var(--fs-s); }
.turn:disabled { opacity: .35; cursor: default; }
.gap { cursor: default; letter-spacing: 0; }
/* The current number shares the row's baseline. */
.current {
  cursor: default; color: var(--fg-base);
  font-family: var(--font-mincho); font-weight: 600; font-size: 15px; letter-spacing: 0;
  transition: none;
}

/* The marks: ❖ on the dots' centre line, one either side of the current page. */
.mark {
  position: absolute; left: 0; top: 0; width: 7px; height: 7px; pointer-events: none;
  fill: var(--wire-strong);
}

/* A finger gets about 40px around each measure without moving the drawing:
   the hit areas meet at the dots' midpoints. */
@media (pointer: coarse) {
  .cell::after { content: ''; position: absolute; inset: -10px -8px; }
}

/* A phone draws the turns larger and close in. */
@media (max-width: 640px) {
  .folio { padding: 4px var(--sp-4); }
  .turn { width: 32px; margin: 0 2px; font-size: 20px; color: var(--fg-muted); }
  .turn::after { inset: -12px -6px; }
}
</style>
