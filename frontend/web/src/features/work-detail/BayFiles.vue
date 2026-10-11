<script setup lang="ts">
import { computed } from 'vue'
import { audioInfo, dirCounts, fmtSize, kindGlyph } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'
import { usePlayerStore } from '@/stores/player'
import type { WorkContentDir, WorkContentFile } from '@/lib/api/types'

// The FILES tab (v3): an OpenList browser of the copy's tree — breadcrumbs,
// kind glyphs, recursive directory counts, no filter (work folders are small).
// Rows act on the whole page: an image locates its set on the deck, lyrics
// and texts open in READ, an audio starts its track.

const store = useWorkDetailStore()
const player = usePlayerStore()

const dir = computed(() => store.cwdDir)
const crumbs = computed(() => (store.copy ? ['/', ...store.cwd] : []))

/** Directories first, order kept (v3's stable sort). */
const entries = computed(() =>
  [...(dir.value?.children ?? [])].sort((a, b) =>
    'children' in a === ('children' in b) ? 0 : 'children' in a ? -1 : 1,
  ),
)

function counts(entry: WorkContentDir): { aud: number; img: number; txt: number } {
  return dirCounts(entry)
}
function info(entry: WorkContentFile): string {
  if (entry.kind === 'aud') return audioInfo(entry)
  if (entry.kind === 'img' && entry.width !== null && entry.height !== null) return `${entry.width}×${entry.height}`
  return ''
}
function sizeOf(entry: WorkContentDir | WorkContentFile): string {
  return 'children' in entry ? String(entry.children.length) : fmtSize(entry.size)
}
function isPlaying(entry: WorkContentDir | WorkContentFile): boolean {
  return (
    !('children' in entry) &&
    entry.kind === 'aud' &&
    player.rj === store.rj &&
    store.tracks.some(
      (track) =>
        player.trackKey === track.key &&
        Object.values(track.files).some((f) => f?.rel_path === entry.rel_path),
    )
  )
}
</script>

<template>
  <div class="files">
    <div class="crumbs">
      <template v-for="(crumb, i) in crumbs" :key="i">
        <span v-if="i > 0" class="crumb-sep" aria-hidden="true">›</span>
        <button class="crumb" type="button" lang="ja" :aria-current="i === crumbs.length - 1 ? 'true' : undefined"
          @click="store.selectDir(store.cwd.slice(0, i))">{{ crumb }}</button>
      </template>
      <button class="btn btn-s copy-pick" type="button" disabled
        :title="`读取副本 ${store.activeCopy?.library_id ?? '—'}（多副本切换未实现）`">{{ store.activeCopy?.library_id ?? '—' }} ▾</button>
    </div>
    <div class="list-head"><span></span><span></span><span class="lbl">NAME</span><span class="lbl">INFO</span><span class="lbl">SIZE</span></div>
    <div class="list" lang="ja">
      <button v-if="store.cwd.length" class="row dir" type="button" @click="store.selectDir(store.cwd.slice(0, -1))">
        <span class="mark"></span><span class="glyph">▴</span><span class="name">..</span><span class="info"></span><span class="size"></span>
      </button>
      <template v-for="entry in entries" :key="entry.rel_path">
        <button v-if="'children' in entry" class="row dir" type="button" @click="store.selectDir([...store.cwd, entry.name])">
          <span class="mark"></span><span class="glyph">{{ kindGlyph('dir') }}</span><span class="name">{{ entry.name }}</span>
          <span class="info"><span class="counts">
            <template v-if="counts(entry).aud">♪{{ counts(entry).aud }}</template>
            <template v-if="counts(entry).img"> ▢{{ counts(entry).img }}</template>
            <template v-if="counts(entry).txt"> ¶{{ counts(entry).txt }}</template>
          </span></span>
          <span class="size">{{ sizeOf(entry) }}</span>
        </button>
        <button v-else class="row" type="button" :class="{ now: isPlaying(entry) }"
          :aria-selected="store.sel === entry.rel_path" @click="store.selectFile(entry)">
          <span class="mark"></span><span class="glyph">{{ kindGlyph(entry.kind) }}</span>
          <span class="name">{{ entry.name }}<span v-if="isPlaying(entry)" class="nowtag">NOW</span></span>
          <span class="info">{{ info(entry) }}</span>
          <span class="size">{{ sizeOf(entry) }}</span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.files { display: flex; flex-direction: column; min-height: 0; flex: 1; }

.crumbs { display: flex; align-items: center; gap: 2px; padding: var(--sp-2) var(--sp-4) var(--sp-2) var(--sp-5); border-bottom: 1px solid var(--wire-fine); flex-shrink: 0; overflow-x: auto; scrollbar-width: none; }
.crumb { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .06em; color: var(--fg-muted); padding: 0 4px; height: 18px; white-space: nowrap; background: none; border: none; cursor: pointer; }
.crumb:hover { color: var(--fg-base); background: var(--bg-subtle); }
.crumb[aria-current='true'] { color: var(--fg-base); font-weight: 600; }
.crumb-sep { color: var(--fg-meta); font-family: var(--font-mono); font-size: var(--fs-xs); }
.crumbs .copy-pick { margin-left: auto; flex-shrink: 0; }

.list-head, .row { display: grid; grid-template-columns: 14px 18px minmax(0, 1fr) auto 64px; align-items: center; gap: var(--sp-2); padding: 0 var(--sp-4) 0 var(--sp-1); }
.list-head { height: 22px; border-bottom: 1px solid var(--wire-strong); flex-shrink: 0; }
.list-head .lbl:nth-child(4), .list-head .lbl:nth-child(5), .row .info, .row .size { text-align: right; }
.list { overflow-y: auto; flex: 1; min-height: 0; padding-bottom: var(--sp-2); }

.row { position: relative; isolation: isolate; height: 28px; border-bottom: 1px solid var(--wire-fine); cursor: pointer; width: 100%; text-align: left; background: none; color: var(--fg-base); border-top: none; border-left: none; border-right: none; font-family: inherit; }
.row::before { content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle); clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-in); }
.row:hover::before { clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out); }
.row .mark { width: 6px; height: 6px; justify-self: center; }
.row:hover .mark { background: var(--wire-mid); }
.row .glyph { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-meta); text-align: center; }
.row .name { font-size: var(--fs-s); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-family: var(--font-ja); }
.row.dir .name { font-weight: 700; }
.row .info, .row .size { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .04em; color: var(--fg-muted); white-space: nowrap; }
.row .counts { display: inline-flex; gap: var(--sp-2); }
.row[aria-selected='true'] { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.row[aria-selected='true'] .mark { background: var(--invert-fg); }
.row[aria-selected='true'] .glyph, .row[aria-selected='true'] .info, .row[aria-selected='true'] .size { color: var(--invert-fg); }
.row.now .mark { background: var(--now); }
.row .nowtag { font-family: var(--font-mono); font-size: 9px; letter-spacing: .12em; background: var(--now); color: var(--now-fg); padding: 0 4px; margin-left: var(--sp-2); font-weight: 700; }

@media (max-width: 768px) {
  .list-head, .row { grid-template-columns: 14px 18px minmax(0, 1fr) 64px; }
  .list-head .lbl:nth-child(4), .row .info { display: none; }
}
</style>
