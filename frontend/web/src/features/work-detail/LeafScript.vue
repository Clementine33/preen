<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { loadText } from '@/features/work-detail/content-source'
import { parseTxt, type ScriptLine } from '@/features/work-detail/derive'
import { useWorkDetailStore } from '@/stores/work-detail'

// READ (v3): the TXT script as a Japanese book — vertical-rl right-to-left by
// default, Gothic (--font-ja) at line-height 2, scene heads sideways, sound
// effects under （SE）, speech numbered; 横 switches to a horizontal measure.
// The only position instrument is the strip's percentage gauge (no scene
// index, no resume point — both rejected); nothing is persisted.

const emit = defineEmits<{ progress: [percent: string] }>()

const store = useWorkDetailStore()
const txtRel = computed(() => store.activeTxt)
const lines = ref<ScriptLine[]>([])
const wrapEl = ref<HTMLElement | null>(null)

watch(
  txtRel,
  async (rel) => {
    lines.value = []
    if (!rel || !store.rj || !store.activeCopy) return
    const text = await loadText({ rj: store.rj, copyId: store.activeCopy.copy_id, relPath: rel })
    if (txtRel.value !== rel) return
    lines.value = parseTxt(text)
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
  // Vertical reads right-to-left, so rtl containers scroll negative.
  const max = store.vertical ? wrap.scrollWidth - wrap.clientWidth : wrap.scrollHeight - wrap.clientHeight
  const p = max > 0 ? Math.min(1, (store.vertical ? Math.abs(wrap.scrollLeft) : wrap.scrollTop) / max) : 1
  emit('progress', `${Math.round(p * 100)}%`)
}
watch([txtRel, () => store.vertical], () => {
  emit('progress', '0%')
})
</script>

<template>
  <div ref="wrapEl" class="script-wrap" :class="{ 'script-h': !store.vertical }" @scroll.passive="trackReading">
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
.script-wrap { height: 100%; overflow-x: auto; overflow-y: hidden; direction: rtl; scrollbar-width: thin; }
.script {
  direction: ltr; writing-mode: vertical-rl; height: 100%;
  padding: var(--sp-5) var(--sp-4);
  font-family: var(--font-ja); font-size: 15px; line-height: 2; letter-spacing: .04em;
  font-feature-settings: 'vpal'; color: var(--fg-base);
}
.script p { text-indent: 1em; margin: 0; }
.script .sp { text-indent: 0; }
.script .spk { font-weight: 700; font-size: var(--fs-s); color: var(--fg-muted); }
.script .no { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-meta); text-combine-upright: all; letter-spacing: 0; }
.script .scene { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .14em; color: var(--fg-meta); margin: 0 1.2em; text-orientation: sideways; text-indent: 0; }
.script-h { direction: ltr; overflow-y: auto; overflow-x: hidden; }
.script-h .script { writing-mode: horizontal-tb; height: auto; max-width: 38em; line-height: 1.9; }
</style>
