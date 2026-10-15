<script setup lang="ts">
import { computed } from 'vue'
import type { WorkSummary } from '@/lib/api/types'
import { useCoverSrc } from '@/features/works/use-cover-src'
import WorkTracklist from '@/features/works/WorkTracklist.vue'
import { useLibraryUiStore } from '@/stores/library-ui'

// One row of a collection's ordered list. Desktop: a reserved drag-handle slot
// (member moving comes later), the position, a small cover, title, circle, work
// number, release date and the play button. A phone keeps cover, a two-line title,
// the circle beneath it and the play button; the CSS hides the rest. An
// expanded row unfolds below into the work's tracklist and an OPEN link, the
// one way into the work on a phone; it opens and folds with a height reveal.
const props = defineProps<{
  work: WorkSummary
  /** The `#` column: the saved position, or the row number under a view sort. */
  position: number
  selected: boolean
  expanded: boolean
  playing: boolean
}>()
defineEmits<{ select: []; open: []; play: []; unfolded: [] }>()

const libraryUi = useLibraryUiStore()
const { src, failed } = useCoverSrc(() => ({ rj: props.work.rj, local: props.work.local_cover, remote: props.work.thumbnail_url ?? null }))
// An R18 work's cover stays hatched while the veil is on.
const cover = computed(() => (libraryUi.veil && props.work.nsfw === true ? null : src.value))
</script>

<template>
  <div class="item">
    <div :id="`cw-${work.rj}`" class="row" :class="{ sel: selected }" role="option" :aria-selected="selected"
      :data-rj="work.rj" data-testid="collection-work" @click="$emit('select')" @dblclick="$emit('open')">
      <span class="handle" aria-hidden="true" />
      <span class="lbl pos">{{ position }}</span>
      <span class="cover" :class="{ hatch: !cover }">
        <img v-if="cover" :src="cover" alt="" loading="lazy" decoding="async" @error="failed(cover)" />
      </span>
      <span class="main">
        <span class="title" lang="ja">{{ work.title ?? '—' }}</span>
        <span class="lbl circle sub">{{ work.circle }}</span>
      </span>
      <span class="circle col">{{ work.circle }}</span>
      <span class="lbl rj">{{ work.rj }}</span>
      <span class="lbl date">{{ work.release_date ?? '—' }}</span>
      <span class="end">
        <span v-if="playing" class="playing-tag" data-testid="collection-playing">▶ NOW</span>
        <button class="play" type="button" aria-label="播放" title="播放" data-testid="collection-play"
          @click.stop="$emit('play')" @dblclick.stop>▶</button>
      </span>
    </div>
    <Transition name="fold" @after-enter="$emit('unfolded')">
      <div v-if="expanded" class="fold-wrap" data-testid="collection-fold">
        <div class="fold-clip">
          <div class="fold">
            <WorkTracklist :rj="work.rj" :title="work.title ?? work.rj" />
            <button class="btn btn-s open" type="button" data-testid="collection-open" @click="$emit('open')">
              OPEN<svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 4.5h7.2L4.6 1.9l.7-.7L9.1 5 5.3 8.8l-.7-.7 2.6-2.6H0z" /></svg>
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.row {
  display: grid; align-items: center; column-gap: var(--sp-3); height: 56px; padding: 0 var(--sp-3); cursor: pointer;
  grid-template-columns: 16px 44px 56px minmax(0, 1fr) minmax(0, 200px) 100px 90px 88px;
  box-shadow: inset 0 -1px 0 var(--wire-fine);
}
.row:hover { background: var(--bg-subtle); }
.row.sel { background: var(--invert-bg); color: var(--invert-fg); }
.row.sel .lbl { color: var(--invert-fg); }
.pos { text-align: right; }
.cover { position: relative; width: 56px; height: 42px; overflow: hidden; box-shadow: inset 0 0 0 1px var(--wire-fine); }
.cover img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
.main { min-width: 0; display: grid; }
.title { font-family: var(--font-mincho); font-size: var(--fs-m); font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.circle { min-width: 0; font-size: var(--fs-s); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sub { display: none; }
.end { display: flex; align-items: center; justify-content: flex-end; gap: var(--sp-2); }
.playing-tag {
  height: 16px; padding: 0 6px; display: inline-flex; align-items: center;
  background: var(--now); color: var(--now-fg); font-family: var(--font-mono); font-size: var(--fs-xs); font-weight: 700; letter-spacing: .12em;
}
.play {
  width: 28px; height: 28px; border: 1px solid var(--wire-control); background: transparent; color: inherit;
  font-size: var(--fs-xs); cursor: pointer; opacity: 0;
}
.row:hover .play, .row.sel .play, .play:focus-visible { opacity: 1; }
.play:hover { border-color: var(--wire-strong); }
/* The fold sits under the title column; a long tracklist scrolls inside it.
   It opens and folds by sizing its one grid track from / to 0fr, so the reveal
   follows the content's real height; the clip keeps the padding from showing at 0.
   Folding is quicker than opening: the list is already moving on. */
.fold-wrap { display: grid; grid-template-rows: 1fr; box-shadow: inset 0 -1px 0 var(--wire-fine); }
.fold-clip { min-height: 0; overflow: hidden; }
.fold-enter-active { transition: grid-template-rows 220ms cubic-bezier(.2, .7, .2, 1); }
.fold-enter-active .fold { transition: opacity 160ms linear 60ms, transform 220ms cubic-bezier(.2, .7, .2, 1); }
.fold-leave-active { transition: grid-template-rows 180ms cubic-bezier(.4, 0, .6, 1); }
.fold-leave-active .fold { transition: opacity 120ms linear; }
.fold-enter-from, .fold-leave-to { grid-template-rows: 0fr; }
.fold-enter-from .fold { opacity: 0; transform: translateY(-6px); }
.fold-leave-to .fold { opacity: 0; }
@media (prefers-reduced-motion: reduce) {
  .fold-enter-active, .fold-enter-active .fold, .fold-leave-active, .fold-leave-active .fold { transition: none; }
}
.fold { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: end; gap: var(--sp-4);
  padding: var(--sp-3) var(--sp-3) var(--sp-4) calc(16px + 44px + 56px + var(--sp-3) * 4); }
.fold :deep(.tracks) { max-height: 336px; }

@media (max-width: 900px) {
  .row { grid-template-columns: 56px minmax(0, 1fr) 28px; height: 64px; }
  .handle, .pos, .col, .rj, .date, .playing-tag { display: none; }
  .title { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .sub { display: block; }
  .play { opacity: 1; }
  .fold { grid-template-columns: minmax(0, 1fr); padding-left: var(--sp-3); }
  .open { justify-self: end; }
}
</style>
