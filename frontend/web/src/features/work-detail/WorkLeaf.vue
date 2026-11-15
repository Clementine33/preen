<script setup lang="ts">
import { computed, ref } from 'vue'
import LeafLyrics from '@/features/work-detail/LeafLyrics.vue'
import LeafScript from '@/features/work-detail/LeafScript.vue'
import WorkColophon from '@/features/work-detail/WorkColophon.vue'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'

// The leaf (v3): one of three modes behind one strip, listing only what exists
// right now — LISTEN while the current track has a paired lyric (a live dot
// while it plays), READ once a script has been opened (sticky), the colophon
// always. Choosing which lyrics a track uses lives in the bay's tab bar.

const store = useWorkDetailStore()
const player = usePlayerStore()

const mode = computed(() =>
  store.leaf === 'lrc'
    ? 'LISTEN'
    : store.leaf === 'script'
      ? 'READ'
      : 'COLOPHON',
)
const pairedLrc = computed(() => store.lrcFor(store.track))
const playing = computed(() => player.rj === store.rj && !player.paused)

const LISTEN = computed(() => {
  const lrc = pairedLrc.value
  if (!lrc)
    return null
  return { mode: 'lrc' as const, label: 'LISTEN' }
})
const READ = computed(() => (store.scriptSeen && store.activeTxt !== null
  ? { mode: 'script' as const, label: 'READ' }
  : null))
/** The switcher's items, in v3's order: LISTEN, READ, COLOPHON. */
const items = computed(() => {
  const out: Array<{ mode: 'colophon' | 'lrc' | 'script', label: string }> = [
    { mode: 'colophon', label: 'COLOPHON' },
  ]
  if (READ.value)
    out.unshift(READ.value)
  if (LISTEN.value)
    out.unshift(LISTEN.value)
  return out
})

/** The READ gauge, fed by the script's scroll (the only position instrument). */
const gauge = ref('')

const side = computed(() => {
  if (store.leaf === 'script') {
    const txt = store.activeTxt
    return { file: txt?.slice(txt.lastIndexOf('/') + 1) ?? '' }
  }
  return null
})
</script>

<template>
  <div class="leaf" aria-label="Leaf">
    <div class="strip">
      <span class="strip-name lbl lbl-xs">{{ mode }}</span>
      <span class="strip-side">
        <span v-if="store.leaf === 'script'" class="lbl gauge">{{ gauge }}</span>
        <template v-if="side">
          <span class="strip-file" lang="ja">{{ side.file }}</span>
        </template>
        <span class="chips" role="group" aria-label="页叶">
          <button
            v-for="item in items" :key="item.mode" class="chip chip-s" type="button"
            :aria-pressed="store.leaf === item.mode" @click="store.setLeaf(item.mode)"
          >
            <span v-if="item.mode === 'lrc' && playing" class="live" aria-hidden="true" />
            {{ item.label }}
          </button>
        </span>
      </span>
    </div>
    <div :key="`${store.leaf}|${pairedLrc}|${store.activeTxt}`" class="leaf-body">
      <WorkColophon v-if="store.leaf === 'colophon'" />
      <LeafLyrics v-else-if="store.leaf === 'lrc'" />
      <LeafScript v-else @progress="gauge = $event" />
    </div>
  </div>
</template>

<style scoped>
.leaf { display: flex; flex-direction: column; min-height: 0; min-width: 0; }
.leaf .strip { position: relative; overflow: visible; }
.strip-file { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--fg-base); letter-spacing: var(--track-data); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.leaf-body { flex: 1; min-height: 0; overflow: auto; position: relative; margin-top: var(--sp-3); animation: fadein 160ms var(--ease-out); }
@keyframes fadein { from { opacity: 0; } to { opacity: 1; } }
.gauge { color: var(--fg-base); white-space: nowrap; }

.chips { display: inline-flex; gap: var(--sp-1); }
.chip .live { width: 5px; height: 5px; background: var(--now); }

/* On a phone the page scrolls, so the leaf opens up until READ caps it. */
@media (max-width: 768px) {
  .leaf-body { max-height: none; overflow: visible; }
  .strip-file, .gauge { display: none; }
}
</style>
