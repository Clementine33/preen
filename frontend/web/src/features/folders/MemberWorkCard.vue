<script setup lang="ts">
import { computed } from 'vue'
import { fmtSize } from '@/features/work-detail/derive'
import { remoteThumb, useCoverSrc } from '@/features/works/use-cover-src'
import { useWorkDetail } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'

// The work a member directory holds, above its files: cover, title, circle and
// voice actors on one line, the copy's absolute path and size. It answers nothing while the work is loading or
// gone — the file list is the page, this only says what it belongs to.
/** `size`: the bytes of the copy being browsed, summed from its tree by the page. */
const props = defineProps<{ rj: string, size?: number | null, path?: string | null }>()

const libraryUi = useLibraryUiStore()
const { work } = useWorkDetail(() => props.rj)
const { src: realCover, failed } = useCoverSrc(() =>
  work.value
    ? {
        rj: work.value.rj,
        local: work.value.local_cover,
        remote: remoteThumb(work.value.cover_url),
      }
    : null,
)

/** An R18 work's cover stays hatched while the veil is on, as on the wall. */
const veiled = computed(() => libraryUi.veil && work.value?.nsfw === true)
const cover = computed(() => (veiled.value ? null : realCover.value))
const voiceActors = computed(() => work.value?.voice_actors.map(va => va.name).join(' / ') || '—')
</script>

<template>
  <div v-if="work" class="work-card" data-testid="member-work-card">
    <div class="cover" :class="[cover ? '' : 'hatch', { veiled }]">
      <img v-if="cover" :src="cover" alt="" loading="lazy" decoding="async" @error="failed(cover)">
    </div>
    <div class="info">
      <div class="title" data-testid="work-title">{{ work.title ?? '—' }}</div>
      <div class="row">
        <span class="cell"><span class="lbl">社团</span><span data-testid="work-circle">{{ work.circle ?? '—' }}</span></span>
        <span class="cell"><span class="lbl">声优</span><span data-testid="work-voice">{{ voiceActors }}</span></span>
      </div>
      <div v-if="path" class="row"><span class="lbl">路径</span><span class="meta" data-testid="work-path" :title="path">{{ path }}</span></div>
      <div v-if="size != null" class="row"><span class="lbl">大小</span><span class="meta" data-testid="work-size">{{ fmtSize(size) }}B</span></div>
    </div>
  </div>
</template>

<style scoped>
/* The card sizes to the space it is given, not the viewport: the files page
   sits beside other panes. Wide: cover beside text; narrow: cover on top. */
.work-card {
  container-type: inline-size;
  display: grid; grid-template-columns: clamp(72px, 28%, 160px) 1fr;
  gap: var(--sp-3, 12px); padding: var(--sp-2, 8px) var(--sp-3, 12px);
}
.cover {
  width: 100%; aspect-ratio: var(--cover-ratio); position: relative; overflow: hidden;
  box-shadow: inset 0 0 0 1px var(--wire-fine);
}
.cover img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
.info {
  min-width: 0; display: flex; flex-direction: column; justify-content: center; gap: 6px;
  font-size: var(--fs-s); line-height: var(--lh-s);
}
.lbl { flex: none; }
.title {
  font-weight: 700; font-size: var(--fs-l); line-height: var(--lh-l);
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
/* Baseline-aligned: the label is a smaller face than its value. The first label
   of every line shares one width, so the values of 社团 / 路径 / 大小 start on one edge. */
.row { display: flex; align-items: baseline; gap: var(--sp-2, 8px); min-width: 0; }
.row > :first-child, .cell:first-child > .lbl { min-width: 3em; }
.row > span:last-child, .cell > span:last-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* Circle and voice actors share a line: each takes what it needs, both shrink. */
.cell { display: flex; align-items: baseline; gap: var(--sp-2, 8px); min-width: 0; }
.cell + .cell { margin-left: var(--sp-4, 16px); }
.cell > span:last-child { font-weight: 500; }
/* Path and size are reference, not content: smaller, quieter, ellipsised with the full text on hover. */
.meta { font-size: var(--fs-xs); color: var(--fg-muted); }
/* Mobile keeps the compact caption sizing and drops the path. */
@media (max-width: 768px) {
  .info { justify-content: flex-start; gap: 2px; }
  .title { font-size: inherit; line-height: inherit; }
  .row:has(> [data-testid="work-path"]) { display: none; }
  /* Too little width for both on one line: circle, then voice actors. */
  .row:has(> .cell) { flex-direction: column; align-items: stretch; gap: 2px; }
  .cell + .cell { margin-left: 0; }
  .cell > .lbl { min-width: 3em; }
}
@container (max-width: 360px) {
  .work-card { grid-template-columns: 1fr; }
}
</style>
