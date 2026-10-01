<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { parseLyrics, parseTxt, type ScriptLine } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'

// READ: horizontal scripts and lyric text, with sound effects under （SE）
// and speech numbered. Timed lyrics are read without their timestamps.
// The only position instrument is the strip's percentage gauge (no scene
// index, no resume point — both rejected); nothing is persisted.

const emit = defineEmits<{ progress: [percent: string] }>()

const store = useWorkDetailStore()
const api = useApiClient()
const txtRel = computed(() => store.activeTxt)
const lines = ref<ScriptLine[]>([])
const wrapEl = ref<HTMLElement | null>(null)

watch(
  txtRel,
  async (rel) => {
    lines.value = []
    if (!rel || !store.rj || !store.activeCopy) return
    let text: string
    try {
      text = await api.getWorkText({ rj: store.rj, copyId: store.activeCopy.copy_id, relPath: rel })
    } catch {
      // Gone since the scan, or unreachable: the leaf stays empty.
      return
    }
    if (txtRel.value !== rel) return
    lines.value = parseTxt(/\.(?:lrc|vtt)$/i.test(rel)
      ? parseLyrics(text, rel).map(line => line.text).join('\n')
      : text)
    // A gauge for text with no overflow reads 100% (v3), so measure after paint.
    requestAnimationFrame(trackReading)
  },
  { immediate: true },
)

/** The speech counter runs across the whole script, like a printed edition. */
const numbered = computed(() => {
  let n = 0
  return lines.value.map((line) => (line.kind === 'sp' ? { ...line, no: ++n } : line))
})

function trackReading(): void {
  const wrap = wrapEl.value
  if (!wrap) return
  const max = wrap.scrollHeight - wrap.clientHeight
  const p = max > 0 ? Math.min(1, wrap.scrollTop / max) : 1
  emit('progress', `${Math.round(p * 100)}%`)
}
watch(txtRel, () => {
  emit('progress', '0%')
})
</script>

<template>
  <div ref="wrapEl" class="script-wrap" @scroll.passive="trackReading">
    <div class="script" lang="ja">
      <template v-for="(line, i) in numbered" :key="i">
        <p v-if="line.kind === 'scene'" class="scene">{{ line.text }}</p>
        <p v-else-if="line.kind === 'sfx'" class="sp"><span class="spk">（SE）</span>{{ line.text }}</p>
        <p v-else><span class="no">{{ line.no }}</span>　{{ line.text }}</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.script-wrap { height: 100%; overflow-y: auto; overflow-x: hidden; }
.script {
  padding: var(--sp-5) var(--sp-4);
  font-family: var(--font-ja); font-size: 15px; line-height: 1.9; letter-spacing: .04em;
  color: var(--fg-base);
}
.script p { text-indent: 1em; margin: 0; }
.script .sp { text-indent: 0; }
.script .spk { font-weight: 700; font-size: var(--fs-s); color: var(--fg-muted); }
.script .no { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-meta); letter-spacing: 0; }
.script .scene { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .14em; color: var(--fg-meta); margin: 0 1.2em; text-indent: 0; }
</style>
