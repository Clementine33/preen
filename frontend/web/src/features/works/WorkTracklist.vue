<script setup lang="ts">
import { computed } from 'vue'
import { deriveTracks, fmtLong, fmtT, pickFormat, playableCopy, trackFormats } from '@/features/work-detail/derive'
import { usePlayWork } from '@/features/works/use-play-work'
import { usePreferences } from '@/queries/preferences'
import { useWorkContent } from '@/queries/works'
import { usePlayerStore } from '@/stores/player'

// One work's tracklist, wherever a work is shown without its detail page (the
// wall's inspector, a collection's expanded row): the detail page's track list
// (deriveTracks over the first online copy) under a TRACKLIST strip; a row
// plays the work from that track. The Format Preference narrows the list here so
// every tracklist narrows the same way; a work shipping none of the preferred
// formats keeps the merged list.

const props = defineProps<{
  rj: string | null
  title: string
  /** Dimmed while the owner is about to swap in another work. */
  dim?: boolean
}>()

const player = usePlayerStore()
const { content, query: contentQuery } = useWorkContent(() => props.rj)
const allTracks = computed(() => {
  const tree = playableCopy(content.value)?.tree
  return tree
    ? deriveTracks(tree)
    : []
})
const { trackFormats: preference } = usePreferences()
const codec = computed(() => pickFormat(trackFormats(allTracks.value), preference.value))
const tracks = computed(() =>
  codec.value
    ? allTracks.value.filter(track => track.files[codec.value!])
    : allTracks.value,
)
// A variant header only helps when the work has more than one variant.
const multiVariant = computed(() => new Set(tracks.value.map(track => track.variant)).size > 1)
function showHeader(t: number): boolean {
  return multiVariant.value && tracks.value[t].variant !== tracks.value[t - 1]?.variant
}
const tracksSummary = computed(() => {
  if (tracks.value.length === 0)
    return '—'
  const total = tracks.value.reduce((sum, track) => sum + (track.duration ?? 0), 0)
  return total > 0
    ? `${tracks.value.length} · ${fmtLong(total)}`
    : String(tracks.value.length)
})

const { playFrom } = usePlayWork()
const isPlaying = (t: number): boolean => player.rj === props.rj && player.trackKey === tracks.value[t]?.key
function playTrack(t: number) {
  if (props.rj === null)
    return
  const index = allTracks.value.indexOf(tracks.value[t])
  playFrom({ rj: props.rj, title: props.title }, content.value, index, codec.value ?? '')
}
</script>

<template>
  <div class="tracklist">
    <div class="strip">
      <span class="strip-name lbl">TRACKLIST</span>
      <span class="lbl">
        <span v-if="codec" class="fmt" data-testid="tracklist-format">{{ codec }}</span>{{ tracksSummary }}
      </span>
    </div>
    <div class="tracks" :class="{ dim }" lang="ja">
      <template v-for="(track, t) in tracks" :key="track.key">
        <div v-if="showHeader(t)" class="var lbl">{{ track.variant || '/' }}</div>
        <button
          class="trk" :class="{ now: isPlaying(t) }"
          type="button" :aria-current="isPlaying(t) || undefined" @click="playTrack(t)"
        >
          <span class="no lbl">{{ String(t + 1).padStart(2, '0') }}</span>
          <span class="nm">{{ track.stem }}</span>
          <span class="dur">{{ track.duration === null ? '—' : fmtT(track.duration) }}</span>
        </button>
      </template>
      <p v-if="tracks.length === 0 && !contentQuery.isPending.value" class="no-tracks lbl">NO TRACK DATA</p>
    </div>
  </div>
</template>

<style scoped>
.tracklist { display: flex; flex-direction: column; min-height: 0; }
.tracks { margin-top: var(--sp-2); overflow-y: auto; transition: opacity 80ms linear; }
.dim { opacity: .3; }
.fmt { margin-right: var(--sp-2); font-family: var(--font-mono); }
.trk {
  display: grid; grid-template-columns: 24px minmax(0, 1fr) 44px; align-items: center; gap: var(--sp-3);
  width: 100%; height: 28px; padding: 0 var(--sp-2); border-bottom: 1px solid var(--wire-fine);
  text-align: left; color: var(--fg-base); font-family: inherit; cursor: pointer;
}
/* A variant is a folder name, usually Japanese: the mono face has no CJK, so it reads in the UI face. */
.var { padding: var(--sp-2) var(--sp-2) var(--sp-1); font-family: var(--font-ui); letter-spacing: 0; font-size: var(--fs-xs); color: var(--fg-meta); border-bottom: 1px solid var(--wire-fine); }
.trk:hover { background: var(--bg-subtle); }
.trk:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: -1px; }
.trk .lbl { letter-spacing: .04em; }
.trk .nm { font-size: var(--fs-s); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.trk .dur { text-align: right; font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
/* The playing track: inverted with the bay's ■ marker ( --now). */
.trk.now { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.trk.now .lbl, .trk.now .dur { color: var(--invert-fg); }
.trk .no { position: relative; }
.trk.now .no::after { content: ''; position: absolute; left: 16px; top: 3px; width: 6px; height: 6px; background: var(--now); }
.no-tracks {
  display: flex; align-items: center; height: 28px;
  padding: 0 var(--sp-2); border-bottom: 1px solid var(--wire-fine);
  letter-spacing: .04em; text-transform: none;
}
</style>
