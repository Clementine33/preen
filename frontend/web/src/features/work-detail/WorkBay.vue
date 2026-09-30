<script setup lang="ts">
import { computed } from 'vue'
import BayFiles from '@/features/work-detail/BayFiles.vue'
import BayTracks from '@/features/work-detail/BayTracks.vue'
import { dirCounts, fileCount, fmtLong } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'

// The bay (v3): one file chamber, two dockings. Bottom = a lifting drawer
// (32px tab strip; a tab click lifts, the open tab's click closes). Right =
// a pinned column beside a stage re-stacked deck-over-leaf. Both dockings
// are the same two tabs: the track list and the OpenList browser.

const store = useWorkDetailStore()

const trackCount = computed(() => store.tracks.length)
const fileTotal = computed(() => (store.copy ? fileCount(store.copy) : 0))

/** `8 TRACKS · 2H45M · ♪15 ▢31 ¶9`: the work at a glance (hidden pinned/mobile). */
const summary = computed(() => {
  const counts = store.copy ? dirCounts(store.copy) : { aud: 0, img: 0, txt: 0 }
  const browsed = store.tracks.filter((track) => track.files[store.fmt])
  const total = browsed.reduce((sum, track) => sum + (track.duration ?? 0), 0)
  return `${trackCount.value} TRACKS · ${total > 0 ? fmtLong(total) : '—'} · ♪${counts.aud} ▢${counts.img} ¶${counts.txt}`
})

function pin(): void {
  store.setDock(store.dock === 'right' ? 'bottom' : 'right')
}
</script>

<template>
  <aside class="bay substrate-panel" :class="{ open: store.bayOpen }" aria-label="File bay">
    <div class="bay-tabs">
      <span class="bay-grip" aria-hidden="true"></span>
      <button class="tab" type="button" :aria-pressed="store.tab === 'tracks' && (store.bayOpen || store.dock === 'right')"
        @click="store.openBayTab('tracks')"><span class="lbl">TRACKS</span><span class="lbl">{{ trackCount }}</span></button>
      <button class="tab" type="button" :aria-pressed="store.tab === 'files' && (store.bayOpen || store.dock === 'right')"
        @click="store.openBayTab('files')"><span class="lbl">FILES</span><span class="lbl">{{ fileTotal }}</span></button>
      <span v-show="store.tab === 'tracks'" class="chips">
        <button v-for="format in store.formats" :key="format" class="chip" type="button"
          :aria-pressed="store.fmt === format" @click="store.setFmt(format)">{{ format }}</button>
      </span>
      <span class="bay-sum lbl">{{ summary }}</span>
      <span class="grow"></span>
      <button class="btn btn-s pin" type="button" :title="store.dock === 'right' ? '机库停靠底部' : '机库钉在右侧'"
        @click="pin">{{ store.dock === 'right' ? 'DOCK BOTTOM' : 'PIN RIGHT' }}</button>
    </div>
    <div class="bay-body">
      <div class="bay-inner">
        <BayTracks v-if="store.tab === 'tracks'" />
        <BayFiles v-else />
      </div>
    </div>
  </aside>
</template>

<style scoped>
.bay { display: flex; flex-direction: column; min-height: 0; min-width: 0; position: relative; }
[data-dock='bottom'] .bay { border-top: 1px solid var(--wire-mid); }
[data-dock='right'] .bay { border-left: 1px solid var(--wire-mid); }

.bay-tabs { height: 32px; display: flex; align-items: center; gap: var(--sp-1); padding: 0 var(--sp-6) 0 var(--sp-4); flex-shrink: 0; }
[data-dock='right'] .bay-tabs { padding: 0 var(--sp-4); border-bottom: 1px solid var(--wire-fine); }
.bay-grip { width: 24px; height: 4px; border-top: 1px solid var(--wire-mid); border-bottom: 1px solid var(--wire-mid); margin-right: var(--sp-3); flex-shrink: 0; }
[data-dock='right'] .bay-grip { display: none; }

.tab { position: relative; isolation: isolate; height: 24px; padding: 0 var(--sp-3); white-space: nowrap; color: var(--fg-muted); display: inline-flex; align-items: center; gap: var(--sp-2); background: none; border: none; cursor: pointer; }
.tab::before { content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle); clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-in); }
.tab:hover::before { clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out); }
.tab .lbl { color: inherit; }
.tab[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.tab[aria-pressed='true'] .lbl { color: inherit; }

.bay-sum { margin-left: var(--sp-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
[data-dock='right'] .bay-sum { display: none; }
.bay-tabs .grow { flex: 1; }

.bay-body { max-height: 0; overflow: hidden; transition: max-height 220ms var(--ease-out); min-height: 0; display: flex; flex-direction: column; }
.bay-inner { min-height: 0; display: flex; flex-direction: column; }
[data-dock='bottom'] .bay.open .bay-body { max-height: min(300px, 38vh); }
[data-dock='bottom'] .bay-inner { height: min(300px, 38vh); }
[data-dock='right'] .bay-body { max-height: none; flex: 1; }
[data-dock='right'] .bay-inner { height: 100%; }

.chips { display: inline-flex; gap: var(--sp-1); margin-left: var(--sp-3); }
.chip { height: 16px; padding: 0 6px; font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .08em; color: var(--fg-muted); border: 1px solid var(--wire-fine); display: inline-flex; align-items: center; gap: 4px; background: none; cursor: pointer; }
.chip:hover { border-color: var(--wire-mid); color: var(--fg-base); }
.chip[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }

@media (max-width: 768px) {
  .bay-tabs { padding: 0 var(--sp-4); }
  .pin, .bay-sum { display: none; }
  [data-dock='bottom'] .bay-inner { height: 60vh; }
  [data-dock='bottom'] .bay.open .bay-body { max-height: 60vh; }
}
</style>
