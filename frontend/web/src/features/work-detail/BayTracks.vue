<script setup lang="ts">
import { computed } from 'vue'
import { fmtT } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'
import { usePlayerStore } from '@/stores/player'

// The one track list (v3): every track of the work, the format chosen by the
// bay's chips. A track missing the browsed format stays listed, dimmed and
// tagged with its only format, and clicking it plays that one. The 2px bar
// after the name is the track's share of the whole work; the ¶ marks a
// paired lyric.

const store = useWorkDetailStore()
const player = usePlayerStore()

const maxDuration = computed(() => Math.max(...store.tracks.map((track) => track.duration ?? 0), 0))

function row(t: number) {
  const track = store.tracks[t]
  const missing = track !== undefined && track.files[store.fmt] === undefined
  const only = track?.formats.length === 1 ? track.formats[0] : null
  const now = player.rj === store.rj && player.trackIndex === t
  return { missing, only, now }
}

const barWidth = (t: number): string => {
  const track = store.tracks[t]
  return `${Math.round(((track?.duration ?? 0) / (maxDuration.value || 1)) * 160)}px`
}
const duration = (t: number): string => fmtT(store.tracks[t]?.duration ?? 0)
const hasLrc = (t: number): boolean => store.lrcFor(t) !== null
</script>

<template>
  <div class="list" lang="ja">
    <button v-for="(track, t) in store.tracks" :key="track.stem" class="trk" type="button"
      :class="{ missing: row(t).missing, now: row(t).now }" :aria-current="row(t).now ? 'true' : undefined"
      @click="store.startTrack(t)">
      <span class="no lbl">{{ String(t + 1).padStart(2, '0') }}</span>
      <span class="nm-wrap">
        <span class="nm">{{ track.stem }}</span>
        <span v-if="row(t).only && !track.files[store.fmt]" class="only">{{ row(t).only }} ONLY</span>
        <span v-if="hasLrc(t)" class="has-lrc" title="有歌词">¶</span>
        <span class="bar" :style="{ width: barWidth(t) }"></span>
      </span>
      <span class="dur">{{ duration(t) }}</span>
    </button>
  </div>
</template>

<style scoped>
.list { overflow-y: auto; flex: 1; min-height: 0; scrollbar-width: thin; padding-bottom: var(--sp-2); font-family: var(--font-ja); }

/* The duration bar is the track's weight in the whole work; the right dock and
   the phone drop it (v3). */
.trk { display: grid; grid-template-columns: 28px minmax(0, 1fr) 48px; align-items: center; gap: var(--sp-3); height: 28px; padding: 0 var(--sp-6) 0 var(--sp-5); border-bottom: 1px solid var(--wire-fine); cursor: pointer; width: 100%; text-align: left; background: none; color: var(--fg-base); border-left: none; border-right: none; border-top: none; font-family: inherit; }
.trk:hover { background: var(--bg-subtle); }
.trk .lbl { letter-spacing: .04em; }
.trk .nm-wrap { display: flex; align-items: center; gap: var(--sp-3); min-width: 0; }
.trk .nm { font-size: var(--fs-s); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.trk .bar { flex: 0 0 auto; height: 2px; background: var(--wire-mid); }
.trk .only { flex-shrink: 0; font-family: var(--font-mono); font-size: 9px; letter-spacing: .12em; padding: 0 4px; border: 1px solid var(--wire-mid); color: var(--fg-muted); }
.trk .has-lrc { flex-shrink: 0; font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-meta); }
.trk .dur { text-align: right; font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
.trk.missing { color: var(--fg-meta); }
.trk.now { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.trk.now .lbl, .trk.now .dur, .trk.now .has-lrc { color: var(--invert-fg); }
.trk.now .only { border-color: var(--invert-fg); color: var(--invert-fg); }
.trk.now .bar { background: var(--now); }
.trk .no { position: relative; }
.trk.now .no::after { content: ''; position: absolute; left: 18px; top: 3px; width: 6px; height: 6px; background: var(--now); }

[data-dock='right'] .trk { grid-template-columns: 24px minmax(0, 1fr) 44px; padding: 0 var(--sp-4); }
[data-dock='right'] .trk .bar { display: none; }

@media (max-width: 768px) {
  .trk { grid-template-columns: 24px minmax(0, 1fr) 44px; padding: 0 var(--sp-4); }
  .trk .bar { display: none; }
}
</style>
