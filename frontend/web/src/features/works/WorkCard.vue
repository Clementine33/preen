<script setup lang="ts">
import type { WorkSummary } from '@/lib/api/types'
import { computed } from 'vue'
import { useLibraryUiStore } from '@/stores/library-ui'
import { remoteThumb, useCoverSrc } from './use-cover-src'

// One cell of the wall: a 4:3 cover — hatched when the catalog
// has none — over a caption base that the selection wipes in from the left.
// The playing work carries the terracotta NOW tag; the cover itself is never
// tinted or darkened.
const props = defineProps<{
  work: WorkSummary
  selected: boolean
  playing: boolean
  /** Position on the page: the first covers load eagerly, the rest lazily. */
  index: number
}>()

/** The covers of the first rows are on screen at load, so lazy loading only delays them. */
const EAGER_COVERS = 18

const libraryUi = useLibraryUiStore()
const { src: realCover, failed } = useCoverSrc(() => ({
  rj: props.work.rj,
  local: props.work.local_cover,
  remote: remoteThumb(props.work.cover_url),
}))

/** An R18 work's cover stays hatched while the veil is on. */
const cover = computed(() => (veiled.value ? null : realCover.value))

/**
 * The dot's tooltip: the track the work is on, from the newest Rendition with a
 * position — also on a finished work being heard again in another Rendition.
 */
const listeningTitle = computed(() => {
  const { state, track_index: index, track_count: count } = props.work.listening
  return `${state === 'finished'
    ? '重听'
    : '在听'} · ${(index ?? 0) + 1}/${count ?? 0}`
})

const veiled = computed(() => libraryUi.veil && props.work.nsfw === true)
</script>

<template>
  <div
    class="cell" :class="{ active: selected }" role="option" :aria-selected="selected"
    :tabindex="selected ? 0 : -1" :data-rj="work.rj"
  >
    <!-- Lazy past the first rows and fill-and-crop: an off-screen cover costs
         nothing until it nears the viewport, and every cover fills its 4:3 frame
         the same way. The first cover is the likeliest largest paint. -->
    <div class="cover" :class="[cover ? '' : 'hatch', { veiled }]">
      <img
        v-if="cover" :src="cover" alt="" :loading="index < EAGER_COVERS ? 'eager' : 'lazy'"
        :fetchpriority="index === 0 ? 'high' : undefined" decoding="async" @error="failed(cover)"
      >
    </div>
    <i
      v-if="work.listening.track_index !== null" class="listening-dot" :title="listeningTitle" role="img"
      :aria-label="listeningTitle"
    />
    <span v-if="playing" class="playing-tag">▶ NOW</span>
    <div class="cap">
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

/* A work with a Listening Position: a small solid dot on the cover's corner, no text. */
.listening-dot {
  position: absolute; left: 6px; top: 6px; width: 8px; height: 8px; border-radius: 50%;
  background: var(--now); box-shadow: 0 0 0 1px var(--now-fg);
}

.playing-tag {
  position: absolute; right: 0; top: 0; height: 16px; padding: 0 6px;
  display: inline-flex; align-items: center; gap: 6px;
  background: var(--now); color: var(--now-fg);
  font-family: var(--font-mono); font-size: var(--text-2xs); font-weight: 700; letter-spacing: var(--track-caps);
}

/* The caption base: rest is quiet; the bone block wipes in from the left
   (160ms ease-out) and the text flips 60ms in. */
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
.cap-title { transition: color 0s; }
.cap-title {
  font-family: var(--font-mincho);
  font-size: var(--text-sm); line-height: var(--text-sm--line-height); font-weight: 500; height: calc(2 * var(--text-sm) * var(--text-sm--line-height));
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
.cell.active .cap::before { clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out); }
.cell.active .cap-title {
  color: var(--invert-fg); transition: color 0s linear 60ms;
}
.cell.active .cap-title { font-weight: 700; }

/* Mobile has no "inspect the selection": a tap routes straight to the detail
   page, so the inverted state never shows there. */
@media (max-width: 768px) {
  .cell.active .cap::before { clip-path: inset(0 100% 0 0); }
  .cell.active .cap-title { color: inherit; font-weight: 500; }
}
/* A burst on the wall (DESIGN.md §7): the caption inverts at once. The wall's class
   is outside this component, so it stays unscoped; only the last compound is scoped. */
.wall-rapid .cell .cap::before,
.wall-rapid .cell .cap-title { transition: none; }
</style>
