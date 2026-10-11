<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { fmtT, parseLyrics, type LrcLine } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'
import { usePlayerStore } from '@/stores/player'

// LISTEN (v3): the paired lyric, framed like subtitles (30% above, 60% below),
// the current line inverted with its ■ marker and scrolled to 38% of the pane,
// past lines dimmed, sound-effect lines smaller. Clicking a line seeks the
// playing track.

const store = useWorkDetailStore()
const api = useApiClient()
const player = usePlayerStore()

const lrcRel = computed(() => store.lrcFor(store.track))
const lines = ref<LrcLine[]>([])
const bodyEl = ref<HTMLElement | null>(null)
const currentTrack = computed(() => player.rj === store.rj && player.trackKey === store.tracks[store.track]?.key)
const status = ref('')

watch(
  [() => store.rj, () => store.activeCopy?.copy_id, lrcRel],
  async ([rj, copyId, rel], _, onCleanup) => {
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    lines.value = []
    status.value = ''
    if (!rel || !rj || !copyId) return
    status.value = '正在读取歌词…'
    let text: string
    try {
      text = await api.getWorkText({ rj, copyId, relPath: rel }, controller.signal)
    } catch {
      if (!controller.signal.aborted) status.value = '无法读取歌词文件'
      return
    }
    if (controller.signal.aborted) return
    lines.value = parseLyrics(text, rel)
    status.value = lines.value.length ? '' : '未找到有效的时间轴歌词'
  },
  { immediate: true },
)

/** The last line at or before the clock — what "now" is. */
const nowIndex = computed(() => {
  if (!currentTrack.value) return -1
  let index = -1
  lines.value.forEach((line, i) => {
    if (player.elapsed >= line.time && (line.endTime === undefined || player.elapsed < line.endTime)) index = i
  })
  return index
})

function sync(jump: boolean): void {
  const body = bodyEl.value
  const now = body?.querySelector<HTMLElement>('.lrc-line.now')
  if (!body || !now) return
  body.scrollTo({ top: now.offsetTop - body.clientHeight * 0.38, behavior: jump ? 'auto' : 'smooth' })
}
watch([lines, nowIndex], (_, previous) => sync(previous[0] !== lines.value), { flush: 'post' })

function seek(line: LrcLine): void {
  if (currentTrack.value) player.seek(line.time)
}

const lineTime = (line: LrcLine): string => fmtT(line.time)
</script>

<template>
  <div ref="bodyEl" class="lrc-scroll">
    <p v-if="status" role="status">{{ status }}</p>
    <div class="lrc" lang="ja">
      <button v-for="(line, i) in lines" :key="`${i}-${line.time}`" class="lrc-line" type="button"
        :disabled="!currentTrack" :class="{ now: i === nowIndex, past: currentTrack && i !== nowIndex && player.elapsed >= (line.endTime ?? line.time), sfx: line.sfx }" @click="seek(line)">
        <span class="t">{{ lineTime(line) }}</span>
        <span>{{ line.text }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.lrc-scroll { height: 100%; overflow-y: auto; }
.lrc { padding: 30% var(--sp-2) 60%; display: flex; flex-direction: column; gap: 2px; font-family: var(--font-ja); }
.lrc-line { display: grid; grid-template-columns: 44px 1fr; gap: var(--sp-3); padding: 6px var(--sp-2); font-size: var(--fs-m); line-height: 22px; color: var(--fg-meta); transition: color var(--dur-out) linear; position: relative; background: none; border: none; cursor: pointer; text-align: left; font-family: inherit; white-space: pre-line; }
.lrc-line:disabled { cursor: default; }
.lrc-scroll > p { padding: var(--sp-3); color: var(--fg-meta); }
.lrc-line .t { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .04em; padding-top: 5px; }
.lrc-line.past { color: var(--fg-muted); }
.lrc-line.now { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.lrc-line.now::before { content: ''; position: absolute; left: -14px; top: 14px; width: 6px; height: 6px; background: var(--invert-bg); }
.lrc-line.sfx { font-size: var(--fs-s); }

@media (max-width: 768px) {
  .lrc { padding: var(--sp-2) var(--sp-2) var(--sp-4); }
}
</style>
