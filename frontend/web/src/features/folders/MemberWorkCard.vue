<script setup lang="ts">
import { computed } from 'vue'
import { useCoverSrc } from '@/features/works/use-cover-src'
import { useWorkDetail } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'

// The work a member directory holds, above its files: cover, title, circle and
// voice actors, nothing more. It answers nothing while the work is loading or
// gone — the file list is the page, this only says what it belongs to.
const props = defineProps<{ rj: string }>()

const libraryUi = useLibraryUiStore()
const { work } = useWorkDetail(() => props.rj)
const { src: realCover, failed } = useCoverSrc(() =>
  work.value
    ? { rj: work.value.rj, local: work.value.local_cover, remote: work.value.thumbnail_url ?? null }
    : null,
)

/** An R18 work's cover stays hatched while the veil is on, as on the wall. */
const veiled = computed(() => libraryUi.veil && work.value?.nsfw === true)
const cover = computed(() => (veiled.value ? null : realCover.value))
const voiceActors = computed(() => work.value?.voice_actors.map((va) => va.name).join(' / ') || '—')
</script>

<template>
  <div v-if="work" class="work-card" data-testid="member-work-card">
    <div class="cover" :class="[cover ? '' : 'hatch', { veiled }]">
      <img v-if="cover" :src="cover" alt="" loading="lazy" decoding="async" @error="failed(cover)" />
    </div>
    <div class="info">
      <div class="title" data-testid="work-title">{{ work.title ?? '—' }}</div>
      <div class="row"><span class="lbl">社团</span><span data-testid="work-circle">{{ work.circle ?? '—' }}</span></div>
      <div class="row"><span class="lbl">声优</span><span data-testid="work-voice">{{ voiceActors }}</span></div>
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
  min-width: 0; display: flex; flex-direction: column; justify-content: center; gap: 4px;
  font-size: var(--fs-m); line-height: var(--lh-m);
}
.lbl { flex: none; }
.title {
  font-weight: 700; font-size: 16px; line-height: 24px;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
/* Baseline-aligned: the label is a smaller face than its value. */
.row { display: flex; align-items: baseline; gap: var(--sp-2, 8px); min-width: 0; }
.row > span:last-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* Mobile keeps the compact caption sizing. */
@media (max-width: 768px) {
  .info { justify-content: flex-start; gap: 2px; font-size: var(--fs-s); line-height: var(--lh-s); }
  .title { font-size: inherit; line-height: inherit; }
}
@container (max-width: 360px) {
  .work-card { grid-template-columns: 1fr; }
}
</style>
