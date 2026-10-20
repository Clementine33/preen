<script setup lang="ts">
import { computed } from 'vue'
import type { WorkSummary } from '@/lib/api/types'
import { useLibraryUiStore } from '@/stores/library-ui'
import { useCoverSrc } from './use-cover-src'

// One cell of the wall: a 4:3 cover — hatched when the catalog
// has none — over a caption base that the selection wipes in from the left.
// The playing work carries the terracotta NOW tag; the cover itself is never
// tinted or darkened.
const props = defineProps<{
  work: WorkSummary
  selected: boolean
  playing: boolean
}>()

const libraryUi = useLibraryUiStore()
const { src: realCover, failed } = useCoverSrc(() => ({
  rj: props.work.rj,
  local: props.work.local_cover,
  remote: props.work.thumbnail_url ?? null,
}))

/** An R18 work's cover stays hatched while the veil is on. */
const cover = computed(() => (veiled.value ? null : realCover.value))

const veiled = computed(() => libraryUi.veil && props.work.nsfw === true)

/** `YYYY.MM` from the catalog's `YYYY-MM-DD`, or the empty-state dash. */
const release = computed(() =>
  props.work.release_date ? props.work.release_date.slice(0, 7).replace('-', '.') : '—',
)
</script>

<template>
  <div class="cell" :class="{ active: selected }" role="option" :aria-selected="selected"
    :tabindex="selected ? 0 : -1" :data-rj="work.rj">
    <!-- Lazy and fill-and-crop: an off-screen cover costs nothing until it nears
         the viewport, and every cover fills its 4:3 frame the same way. -->
    <div class="cover" :class="[cover ? '' : 'hatch', { veiled }]">
      <img v-if="cover" :src="cover" alt="" loading="lazy" decoding="async" @error="failed(cover)" />
    </div>
    <span v-if="playing" class="playing-tag">▶ NOW</span>
    <div class="cap">
      <div class="cap-meta">
        <span class="lbl">{{ work.rj }}</span>
        <span class="lbl">{{ release }}</span>
      </div>
      <div class="cap-title">{{ work.title ?? '—' }}</div>
    </div>
  </div>
</template>

<style scoped>
.cell {
  position: relative; cursor: pointer; outline: none;
}
.cover {
  width: 100%; aspect-ratio: var(--cover-ratio); position: relative; overflow: hidden;
  box-shadow: inset 0 0 0 1px var(--wire-fine);
  transition: box-shadow var(--dur-out) linear;
}
.cover img {
  position: absolute; inset: 0; width: 100%; height: 100%;
  object-fit: cover; object-position: center;
}
/* The frame's inset rule draws over the picture, as it did over the old
   background image; it follows the cover's hover and focus states. */
.cover::after {
  content: ''; position: absolute; inset: 0; pointer-events: none;
  box-shadow: inherit;
}
.cover.veiled::before { content: none; }
.cell:hover .cover { box-shadow: inset 0 0 0 1px var(--wire-strong); }
.cell:not(.active):hover .cap { background: var(--bg-subtle); }
.cell:focus-visible { outline: none; }
.cell:focus-visible .cover { box-shadow: inset 0 0 0 2px var(--wire-strong); }

.playing-tag {
  position: absolute; right: 0; top: 0; height: 16px; padding: 0 6px;
  display: inline-flex; align-items: center; gap: 6px;
  background: var(--now); color: var(--now-fg);
  font-family: var(--font-mono); font-size: var(--fs-xs); font-weight: 700; letter-spacing: .12em;
}

/* The caption base: rest is quiet; the bone block wipes in from the left
   (160ms ease-out), the text flips 60ms in, and a 1px ghost frame steps out
   3px down-right 70ms later. */
.cap {
  position: relative; isolation: isolate;
  margin-top: var(--sp-2); padding: var(--sp-1) var(--sp-2) 6px;
  color: var(--fg-base);
}
.cap::before {
  content: ''; position: absolute; inset: 0; z-index: -1;
  background: var(--invert-bg);
  clip-path: inset(0 100% 0 0);
  transition: clip-path var(--dur-out) var(--ease-out);
}
.cap::after {
  content: ''; position: absolute; inset: 0; z-index: -2;
  border: 1px solid var(--wire-mid);
  opacity: 0; transform: translate(0, 0);
  transition: opacity 60ms linear, transform 60ms linear;
}
.cap-meta { display: flex; justify-content: space-between; gap: var(--sp-2); }
.cap-meta .lbl, .cap-title { transition: color 0s; }
.cap-title {
  font-family: var(--font-mincho);
  font-size: var(--fs-s); line-height: var(--lh-s); font-weight: 500; height: 32px;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
.cell.active .cap::before { clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out); }
.cell.active .cap::after {
  opacity: 1; transform: translate(3px, 3px);
  transition: opacity 120ms linear var(--dur-ghost-delay), transform 180ms var(--ease-out) var(--dur-ghost-delay);
}
.cell.active .cap-title, .cell.active .cap-meta .lbl {
  color: var(--invert-fg); transition: color 0s linear 60ms;
}
.cell.active .cap-title { font-weight: 700; }

/* Mobile has no "inspect the selection": a tap routes straight to the detail
   page, so the inverted state never shows there. */
@media (max-width: 768px) {
  .cell.active .cap::before { clip-path: inset(0 100% 0 0); }
  .cell.active .cap::after { opacity: 0; }
  .cell.active .cap-title, .cell.active .cap-meta .lbl { color: inherit; font-weight: 500; }
  .cell.active .cap-meta .lbl { color: var(--fg-meta); }
}
/* A burst on the wall (DESIGN.md §7): the caption inverts at once. The wall's class
   is outside this component, so it stays unscoped; only the last compound is scoped. */
.wall-rapid .cell .cap::before,
.wall-rapid .cell .cap::after,
.wall-rapid .cell .cap-title,
.wall-rapid .cell .cap-meta .lbl { transition: none; }
</style>
