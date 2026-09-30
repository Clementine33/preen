<script setup lang="ts">
import { computed, ref } from 'vue'
import LeafLyrics from '@/features/work-detail/LeafLyrics.vue'
import LeafScript from '@/features/work-detail/LeafScript.vue'
import WorkColophon from '@/features/work-detail/WorkColophon.vue'
import { useWorkDetailStore } from '@/stores/work-detail'
import { usePlayerStore } from '@/stores/player'

// The leaf (v3): one of three modes behind one strip, listing only what exists
// right now — LISTEN while the current track has a paired lyric (a live dot
// while it plays), READ once a script has been opened (sticky), the colophon
// always. A track without a pairing offers `LRC ▾` instead of hiding LISTEN.

const store = useWorkDetailStore()
const player = usePlayerStore()

const mode = computed(() =>
  store.leaf === 'lrc' ? 'LISTEN' : store.leaf === 'script' ? 'READ' : 'COLOPHON',
)
const pairedLrc = computed(() => store.lrcFor(store.track))
const pairedByHand = computed(() => store.pairs[store.track] !== undefined)
const playing = computed(() => player.rj === store.rj && !player.paused)

const LISTEN = computed(() => {
  const lrc = pairedLrc.value
  if (!lrc) return null
  return { mode: 'lrc' as const, label: 'LISTEN' }
})
const READ = computed(() => (store.scriptSeen && store.activeTxt !== null ? { mode: 'script' as const, label: 'READ' } : null))
/** The switcher's items, in v3's order: LISTEN, READ, 奥付. */
const items = computed(() => {
  const out: Array<{ mode: 'colophon' | 'lrc' | 'script'; label: string }> = [{ mode: 'colophon', label: '奥付' }]
  if (READ.value) out.unshift(READ.value)
  if (LISTEN.value) out.unshift(LISTEN.value)
  return out
})

/** Every .lrc of the copy, the pair menu's candidates. */
const lrcFiles = computed(() => {
  if (!store.copy) return []
  const out: { name: string; rel_path: string }[] = []
  const walk = (dir: typeof store.copy) => {
    for (const child of dir.children) {
      if ('children' in child) walk(child)
      else if (child.kind === 'lrc') out.push(child)
    }
  }
  walk(store.copy)
  return out
})

/** The READ gauge, fed by the script's scroll (the only position instrument). */
const gauge = ref('')

const side = computed(() => {
  if (store.leaf === 'lrc') {
    const lrc = pairedLrc.value
    return { file: lrc?.slice(lrc.lastIndexOf('/') + 1) ?? '', tag: pairedByHand.value ? 'PAIRED' : 'SYNC' }
  }
  if (store.leaf === 'script') {
    const txt = store.activeTxt
    return { file: txt?.slice(txt.lastIndexOf('/') + 1) ?? '', tag: '' }
  }
  return null
})
</script>

<template>
  <div class="leaf" aria-label="Leaf">
    <div class="strip">
      <span class="strip-name lbl">{{ mode }}</span>
      <span class="strip-side">
        <span v-if="store.leaf === 'script'" class="lbl gauge">{{ gauge }}</span>
        <template v-if="side">
          <span class="strip-file" lang="ja">{{ side.file }}</span>
          <span v-if="side.tag" class="lbl">{{ side.tag }}</span>
          <button v-if="store.leaf === 'script'" class="btn btn-s orient" type="button"
            :aria-pressed="store.vertical" @click="store.vertical = !store.vertical">{{ store.vertical ? '縦' : '横' }}</button>
        </template>
        <span class="seg" role="group" aria-label="页叶">
          <button v-if="!pairedLrc" class="seg-b pick" type="button" :aria-expanded="store.picking"
            title="为当前音轨手动指定一份歌词" @click="store.togglePicking()">LRC ▾</button>
          <button v-for="item in items" :key="item.mode" class="seg-b" type="button"
            :aria-pressed="store.leaf === item.mode" @click="store.setLeaf(item.mode)">
            <span v-if="item.mode === 'lrc' && playing" class="live" aria-hidden="true"></span>
            <span v-if="item.mode === 'colophon'" class="jp">奥付</span>
            <template v-else>{{ item.label }}</template>
          </button>
        </span>
        <span v-if="store.picking" class="menu" role="menu">
          <div class="strip">
            <span class="strip-name lbl">PAIR LYRICS</span>
            <span class="lbl">TRACK {{ String(store.track + 1).padStart(2, '0') }}</span>
          </div>
          <button v-for="lrc in lrcFiles" :key="lrc.rel_path" class="menu-item" type="button" role="menuitem"
            lang="ja" @click="store.pairLrc(store.track, lrc.rel_path)"><span class="glyph">¶</span>{{ lrc.name }}</button>
        </span>
      </span>
    </div>
    <div :key="`${store.leaf}|${pairedLrc}|${store.activeTxt}|${store.vertical}`" class="leaf-body">
      <WorkColophon v-if="store.leaf === 'colophon'" />
      <LeafLyrics v-else-if="store.leaf === 'lrc'" />
      <LeafScript v-else @progress="gauge = $event" />
    </div>
  </div>
</template>

<style scoped>
.leaf { display: flex; flex-direction: column; min-height: 0; min-width: 0; }
.leaf .strip { position: relative; overflow: visible; }
.strip-file { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-base); letter-spacing: .04em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.leaf-body { flex: 1; min-height: 0; overflow: auto; position: relative; margin-top: var(--sp-3); scrollbar-width: thin; animation: fadein 160ms var(--ease-out); }
@keyframes fadein { from { opacity: 0; } to { opacity: 1; } }
.gauge { color: var(--fg-base); white-space: nowrap; }

.seg { display: inline-flex; border: 1px solid var(--wire-mid); }
.seg-b { height: 16px; padding: 0 8px; font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .12em; color: var(--fg-muted); display: inline-flex; align-items: center; gap: 4px; background: none; border: none; cursor: pointer; }
.seg-b + .seg-b { border-left: 1px solid var(--wire-fine); }
.seg-b:hover { background: var(--bg-subtle); color: var(--fg-base); }
.seg-b[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.seg-b .jp { font-family: var(--font-ui); letter-spacing: 0; }
.seg-b .live { width: 5px; height: 5px; background: var(--now); }
.seg-b.pick { color: var(--fg-base); }
.seg-b.pick[aria-expanded='true'] { background: var(--bg-subtle); }

.menu { position: absolute; right: 0; top: calc(100% + 8px); z-index: 20; width: 264px; padding-bottom: var(--sp-2); background: var(--grain), var(--bg-panel); border: 1px solid var(--wire-strong); box-shadow: 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid); }
.menu .strip { margin: var(--sp-2) var(--sp-2) var(--sp-1); }
.menu-item { display: flex; align-items: center; gap: var(--sp-2); width: calc(100% - 16px); margin: 0 var(--sp-2); height: 28px; padding: 0 var(--sp-2) 0 var(--sp-1); border-bottom: 1px solid var(--wire-fine); font-size: var(--fs-s); text-align: left; color: var(--fg-base); background: none; cursor: pointer; }
.menu-item:hover { background: var(--invert-bg); color: var(--invert-fg); }
.menu-item .glyph { width: 14px; text-align: center; font-family: var(--font-mono); font-size: var(--fs-xs); color: inherit; }

/* On a phone the page scrolls, so the leaf opens up until READ caps it. */
@media (max-width: 768px) {
  .leaf-body { max-height: none; overflow: visible; }
  .menu-item { font-family: var(--font-ja); }
}
</style>
