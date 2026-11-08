<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useKeymap } from '@/composables/use-keymap'
import { fmtT } from '@/features/work-detail/derive'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'

// The merged track list keeps its order across format chips. A missing format
// plays the track's first available format. The meter
// left of the duration is the track's weight in the work, eight cells in a
// fixed column so rows compare at a glance; the ¶ marks a
// paired lyric. With the LRC chip on, the list is the copy's lyric files
// instead: a click pairs one with the current track (and may be repeated).
// The keyboard cursor starts on the playing track (in LRC mode, the paired
// lyric file), follows it, and Enter does what a click on that row does.

const store = useWorkDetailStore()
const player = usePlayerStore()

function fileFor(t: number) {
  const track = store.tracks[t]
  return track?.files[store.fmt] ?? track?.files[track.formats[0]]
}
const CELLS = 8
const durations = computed(() => store.tracks.map((_, t) => fileFor(t)?.duration_sec ?? 0))
// A "full cut" track (about the sum of the others) would squash every real
// track to one cell, so it is full and the second longest sets the scale.
const scale = computed(() => {
  const sorted = durations.value.filter(d => d > 0).toSorted((a, b) => b - a)
  const [max = 0, second = 0] = sorted
  const rest = sorted.slice(1).reduce((sum, d) => sum + d, 0)
  return sorted.length >= 4 && max >= rest * 0.7
    ? second
    : max
})
const isPlaying = (t: number): boolean => player.rj === store.rj && player.trackKey === store.tracks[t]?.key
const missing = (t: number): boolean => store.tracks[t]?.files[store.fmt] === undefined

/** Lit cells: none without a duration, else at least one; square-root scaled so short tracks still read. */
function level(t: number): number {
  const d = durations.value[t]
  return d > 0
    ? Math.min(CELLS, Math.max(1, Math.ceil(Math.sqrt(d / scale.value) * CELLS)))
    : 0
}
function duration(t: number): string {
  const seconds = fileFor(t)?.duration_sec
  return seconds == null
    ? '—'
    : fmtT(seconds)
}
// A variant header only helps when the work has more than one variant.
const multiVariant = computed(() => new Set(store.tracks.map(track => track.variant)).size > 1)
function showHeader(t: number): boolean {
  return multiVariant.value && store.tracks[t].variant !== store.tracks[t - 1]?.variant
}
const hasLrc = (t: number): boolean => store.lrcFor(t) !== null
const pairedRel = computed(() => store.lrcFor(store.track))

const list = ref<HTMLElement | null>(null)
const rowCount = computed(() => (store.lrcMode ? store.lrcFiles.length : store.tracks.length))
const home = computed(() =>
  Math.max(0, store.lrcMode
    ? store.lrcFiles.findIndex(lrc => lrc.rel_path === pairedRel.value)
    : store.tracks.findIndex((_, t) => isPlaying(t))),
)
const cursor = ref(home.value)
watch(home, to => (cursor.value = to))
watch(cursor, () => void nextTick(() => list.value?.querySelector('.cur')?.scrollIntoView?.({ block: 'nearest' })))
const at = (t: number): boolean => cursor.value === t

useKeymap({
  'detail.cursor': (event) => {
    cursor.value = Math.min(Math.max(cursor.value + (event.key === 'ArrowDown'
      ? 1
      : -1), 0), rowCount.value - 1)
  },
  'detail.activate': () => {
    if (!store.lrcMode) {
      return store.tracks[cursor.value]
        ? store.startTrack(cursor.value)
        : false
    }
    const lrc = store.lrcFiles[cursor.value]
    return lrc
      ? store.pairLrc(store.track, lrc.rel_path)
      : false
  },
}, () => store.bayVisible)
</script>

<template>
  <div v-if="store.lrcMode" ref="list" class="list" lang="ja">
    <button
      v-for="(lrc, i) in store.lrcFiles" :key="lrc.rel_path" class="trk" type="button"
      :class="{ now: lrc.rel_path === pairedRel, cur: at(i) }" :aria-current="lrc.rel_path === pairedRel ? 'true' : undefined"
      @click="store.pairLrc(store.track, lrc.rel_path)"
    >
      <span class="no lbl">{{ String(i + 1).padStart(2, '0') }}</span>
      <span class="nm-wrap"><span class="nm">{{ lrc.name }}</span></span>
      <span class="dur lbl">{{ lrc.rel_path === pairedRel ? `TRACK ${String(store.track + 1).padStart(2, '0')}` : '' }}</span>
    </button>
  </div>
  <div v-else ref="list" class="list" lang="ja">
    <template v-for="(track, t) in store.tracks" :key="track.key">
      <div v-if="showHeader(t)" class="var lbl">{{ track.variant || '/' }}</div>
      <button
        class="trk" type="button"
        :class="{ missing: missing(t), now: isPlaying(t), cur: at(t) }" :aria-current="isPlaying(t) ? 'true' : undefined"
        @click="store.startTrack(t)"
      >
        <span class="no lbl">{{ String(t + 1).padStart(2, '0') }}</span>
        <span class="nm-wrap">
          <span class="nm">{{ track.stem }}</span>
          <span v-if="missing(t) && track.formats.length === 1" class="only">{{ track.formats[0] }} ONLY</span>
          <span v-if="hasLrc(t)" class="has-lrc" title="有歌词">¶</span>
        </span>
        <span class="tail">
          <span class="meter" aria-hidden="true"><i v-for="c in CELLS" :key="c" :class="{ on: c <= level(t) }" /></span>
          <span class="dur">{{ duration(t) }}</span>
        </span>
      </button>
    </template>
  </div>
</template>

<style scoped>
.list { overflow-y: auto; flex: 1; min-height: 0; padding-bottom: var(--sp-2); }

/* The meter is the track's weight in the whole work; the right dock and the
   phone drop it (v3). */
.trk { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: var(--sp-3); height: 28px; padding: 0 var(--sp-6) 0 var(--sp-5); border-bottom: 1px solid var(--wire-fine); cursor: pointer; width: 100%; text-align: left; background: none; color: var(--fg-base); border-left: none; border-right: none; border-top: none; font-family: inherit; }
/* A variant is a folder name, usually Japanese: the mono face has no CJK, so it reads in the UI face. */
.var { padding: var(--sp-2) var(--sp-6) var(--sp-1) var(--sp-5); font-family: var(--font-ui); letter-spacing: 0; font-size: var(--fs-xs); color: var(--fg-meta); border-bottom: 1px solid var(--wire-fine); }
.trk:hover { background: var(--bg-subtle); }
.trk.cur { outline: 1px solid var(--wire-strong); outline-offset: -1px; }
.trk .lbl { letter-spacing: .04em; }
.trk .nm-wrap { display: flex; align-items: center; gap: var(--sp-3); min-width: 0; }
.trk .nm { font-size: var(--fs-s); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.trk .tail { display: flex; align-items: center; gap: var(--sp-3); }
.trk .meter { display: flex; gap: 2px; }
.trk .meter i { width: 4px; height: 6px; background: var(--wire-fine); }
.trk .meter i.on { background: var(--fg-meta); }
.trk .only { flex-shrink: 0; font-family: var(--font-mono); font-size: 9px; letter-spacing: .12em; padding: 0 4px; border: 1px solid var(--wire-mid); color: var(--fg-muted); }
.trk .has-lrc { flex-shrink: 0; font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-meta); }
.trk .dur { min-width: 40px; text-align: right; font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
.trk.missing { color: var(--fg-meta); }
.trk.now { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.trk.now .lbl, .trk.now .dur, .trk.now .has-lrc { color: var(--invert-fg); }
.trk.now .only { border-color: var(--invert-fg); color: var(--invert-fg); }
.trk.now .meter i { background: color-mix(in srgb, var(--invert-fg) 25%, transparent); }
.trk.now .meter i.on { background: var(--now); }
.trk.missing .meter { opacity: .5; }
.trk .no { position: relative; }
.trk.now .no::after { content: ''; position: absolute; left: 18px; top: 3px; width: 6px; height: 6px; background: var(--now); }

[data-dock='right'] .trk { grid-template-columns: 24px minmax(0, 1fr) 44px; padding: 0 var(--sp-4); }
[data-dock='right'] .trk .meter { display: none; }

@media (max-width: 768px) {
  .trk { grid-template-columns: 24px minmax(0, 1fr) 44px; padding: 0 var(--sp-4); }
  .trk .meter { display: none; }
}
</style>
