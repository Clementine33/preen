<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { fmtT, parseLrc, type LrcLine } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'
import { usePlayerStore } from '@/stores/player'

// LISTEN (v3): the paired lyric, framed like subtitles (30% above, 60% below),
// the current line inverted with its ■ marker and scrolled to 38% of the pane,
// past lines dimmed, sound-effect lines smaller. Clicking a line seeks — to
// the silent clock today, to the audio once playback lands.

const store = useWorkDetailStore()
const api = useApiClient()
const player = usePlayerStore()

const lrcRel = computed(() => store.lrcFor(store.track))
const lines = ref<LrcLine[]>([])
const bodyEl = ref<HTMLElement | null>(null)
let firstPaint = true

watch(
  lrcRel,
  async (rel) => {
    lines.value = []
    if (!rel || !store.rj || !store.activeCopy) return
    const text = await api.getWorkText({ rj: store.rj, copyId: store.activeCopy.copy_id, relPath: rel })
    // The pairing may have moved on while the text loaded.
    if (lrcRel.value !== rel) return
    lines.value = parseLrc(text)
    firstPaint = true
    sync(true)
  },
  { immediate: true },
)

/** The last line at or before the clock — what "now" is. */
const nowIndex = computed(() => {
  let index = 0
  lines.value.forEach((line, i) => {
    if (player.elapsed >= line.time) index = i
  })
  return index
})

function sync(jump: boolean): void {
  const body = bodyEl.value
  const now = body?.querySelector<HTMLElement>('.lrc-line.now')
  if (!body || !now) return
  body.scrollTo({ top: now.offsetTop - body.clientHeight * 0.38, behavior: jump ? 'auto' : 'smooth' })
}
watch(nowIndex, () => {
  sync(firstPaint)
  firstPaint = false
})

function seek(line: LrcLine): void {
  // Phase A: the seek moves the silent clock only — the UI never fakes sound.
  player.elapsed = line.time
}

const lineTime = (line: LrcLine): string => fmtT(line.time)
</script>

<template>
  <div ref="bodyEl" class="lrc-scroll">
    <div class="lrc" lang="ja">
      <button v-for="(line, i) in lines" :key="`${i}-${line.time}`" class="lrc-line" type="button"
        :class="{ now: i === nowIndex, past: i < nowIndex, sfx: line.sfx }" @click="seek(line)">
        <span class="t">{{ lineTime(line) }}</span>
        <span>{{ line.text }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.lrc-scroll { height: 100%; overflow-y: auto; scrollbar-width: thin; }
.lrc { padding: 30% var(--sp-2) 60%; display: flex; flex-direction: column; gap: 2px; font-family: var(--font-ja); }
.lrc-line { display: grid; grid-template-columns: 44px 1fr; gap: var(--sp-3); padding: 6px var(--sp-2); font-size: var(--fs-m); line-height: 22px; color: var(--fg-meta); transition: color var(--dur-out) linear; position: relative; background: none; border: none; cursor: pointer; text-align: left; font-family: inherit; }
.lrc-line .t { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .04em; padding-top: 5px; }
.lrc-line.past { color: var(--fg-muted); }
.lrc-line.now { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.lrc-line.now::before { content: ''; position: absolute; left: -14px; top: 14px; width: 6px; height: 6px; background: var(--invert-bg); }
.lrc-line.sfx { font-size: var(--fs-s); }

@media (max-width: 768px) {
  .lrc { padding: var(--sp-2) var(--sp-2) var(--sp-4); }
}
</style>
