<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { isScrollShape } from '@/features/work-detail/derive'
import WorkColophon from '@/features/work-detail/WorkColophon.vue'
import { useMediaQuery, WALL_MOBILE_QUERY } from '@/composables/use-media-query'
import { useWorkDetail } from '@/queries/works'
import { useWorkDetailStore } from '@/stores/work-detail'
import type { ContentFileRef, WorkContentFile } from '@/lib/api/types'

// The specimen deck (v3): a plate that shrink-wraps the current image's true
// ratio and never upscales past native width, ghost frames behind it showing
// the set's remaining depth, the corner tick flipping it to its reverse (the
// compact colophon), a justified filmstrip below, and the R18 veil for every
// set but the cover's.

const store = useWorkDetailStore()
const api = useApiClient()
const { work } = useWorkDetail(() => store.rj)
const isMobile = useMediaQuery(WALL_MOBILE_QUERY)

const set = computed(() => store.currentSet)
const file = computed(() => store.currentFile)
/** R18 veil: a viewer option, off by default; the cover set is never hatched (v3). */
const veiled = computed(() => store.veil && !store.unveiled && work.value?.nsfw === true && !(set.value?.cover ?? false))

function refOf(item: WorkContentFile | null): ContentFileRef | null {
  if (!item || !store.rj || !store.activeCopy) return null
  return { rj: store.rj, copyId: store.activeCopy.copy_id, relPath: item.rel_path }
}
const frontSrc = computed(() => {
  const ref = refOf(file.value)
  return ref ? api.workFileUrl(ref, file.value?.thumb_v ?? undefined) : ''
})
// Double quotes: encodeURIComponent leaves an apostrophe in a file name as is.
const frontStyle = computed(() => ({ backgroundImage: `url("${frontSrc.value}")` }))

const mode = computed(() => (store.flipped ? 'REVERSE' : set.value?.cover && store.idx === 0 ? 'PLATE' : 'VIEW'))
const count = computed(() =>
  store.flipped ? 'COLOPHON' : `${String(store.idx + 1).padStart(2, '0')} / ${String(set.value?.files.length ?? 0).padStart(2, '0')}`,
)

/** True while READ owns the stage: the deck steps down to a postage stamp (v3). */
const reading = computed(() => store.leaf === 'script')
const chipsEl = ref<HTMLElement | null>(null)
watch(() => store.set, async () => {
  await nextTick()
  chipsEl.value?.querySelector<HTMLElement>('[aria-pressed="true"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
})

// ── Shrink-wrap geometry: measure the slot, hug the image, never upscale. ──
const slotEl = ref<HTMLElement | null>(null)
const plateW = ref(0)
const plateH = ref(0)
const scrollMode = ref(false)
let observer: ResizeObserver | null = null

function layoutPlate(): void {
  const item = file.value
  if (!item || !slotEl.value) return
  // An unmeasured image (null facts, docs/api.md) still gets a plate: a 4:3
  // frame it is contained in, never upscaled past the frame's native guess.
  const [width, height] = item.width !== null && item.height ? [item.width, item.height] : [1200, 900]
  const rect = slotEl.value.getBoundingClientRect()
  const sw = Math.max(120, rect.width - (isMobile.value ? 72 : 96))
  const sh = Math.max(120, rect.height - 12)
  const ratio = width / height
  let w: number
  let h: number
  if (isScrollShape(width, height)) {
    w = Math.min(sw, 360)
    h = sh
  } else {
    w = Math.min(sw, sh * ratio, width)
    h = w / ratio
  }
  if (store.flipped) {
    w = Math.max(w, Math.min(sw, 380))
    h = Math.max(h, Math.min(sh, 460))
  }
  plateW.value = Math.round(w)
  plateH.value = Math.round(h)
  scrollMode.value = isScrollShape(width, height) && !veiled.value
}

/** Ghost depth: the frames still behind this one, up to three (v3). */
const ghosts = computed(() => {
  const depth = Math.min(3, (set.value?.files.length ?? 1) - 1 - store.idx)
  const shadows: string[] = []
  for (let d = 1; d <= depth; d++) {
    shadows.push(`${d * 4}px ${d * 4}px 0 -1px var(--bg-canvas)`, `${d * 4}px ${d * 4}px 0 0 var(--wire-mid)`)
  }
  return shadows.join(', ') || 'none'
})

// ── Enter wipe: a step reveals the new front left to right (v3): the clip is
//    applied without a transition, then the transition returns and opens it. ──
const entering = ref(false)
const running = ref(true)
watch(
  () => [store.set, store.idx] as const,
  async () => {
    if (store.flipped) return
    running.value = false
    entering.value = true
    await nextTick()
    requestAnimationFrame(() => {
      entering.value = false
      running.value = true
    })
  },
)

// ── Flip wipe: the bone bar sweeps across and the reverse settles in (v3). ──
const flipCount = ref(0)
watch(
  () => store.flipped,
  () => {
    flipCount.value++
  },
)

// ── Filmstrip: 4 thumbnail requests in flight, nearest first; a set switch
//    aborts the batch so the server drops its queued renders; each thumbnail
//    loads once per visit (ADR 0013). ──
const THUMB_HEIGHT = 40
/** Copy, relative path and version identify a loaded thumbnail. */
const thumbs = ref(new Map<string, string>())
let batch: AbortController | null = null

function thumbKey(item: WorkContentFile): string {
  return JSON.stringify([store.activeCopy?.copy_id, item.rel_path, item.thumb_v])
}

function scheduleThumbs(): void {
  batch?.abort()
  batch = new AbortController()
  const signal = batch.signal
  const current = set.value
  const ref = refOf(current?.files[0] ?? null)
  if (!current || !ref || veiled.value) return
  const queue = current.files
    .map((item, i) => ({ item, i, key: thumbKey(item) }))
    .filter(({ item, key }) => item.thumb_v !== null && !thumbs.value.has(key))
    .sort((a, b) => Math.abs(a.i - store.idx) - Math.abs(b.i - store.idx))
  const height = Math.ceil(THUMB_HEIGHT * (window.devicePixelRatio || 1))
  const worker = async (): Promise<void> => {
    for (let entry = queue.shift(); entry && !signal.aborted; entry = queue.shift()) {
      try {
        const blob = await api.getWorkThumb({ ...ref, relPath: entry.item.rel_path }, height, entry.item.thumb_v ?? '', signal)
        if (signal.aborted) return
        thumbs.value = new Map(thumbs.value).set(entry.key, URL.createObjectURL(blob))
      } catch {
        // Aborted, or refused (too large / undecodable): the hatch stays.
      }
    }
  }
  for (let k = 0; k < 4; k++) void worker()
}

function dropThumbs(): void {
  batch?.abort()
  for (const url of thumbs.value.values()) URL.revokeObjectURL(url)
  thumbs.value = new Map()
}

watch(veiled, (now) => {
  if (!now) scheduleThumbs()
})
// Another set is another batch: the previous one is aborted, its loaded
// thumbnails kept for the visit.
watch(set, () => {
  const valid = new Set(store.sets.flatMap((entry) => entry.files.map(thumbKey)))
  for (const [key, url] of thumbs.value) {
    if (valid.has(key)) continue
    URL.revokeObjectURL(url)
    thumbs.value.delete(key)
  }
  scheduleThumbs()
})

onMounted(() => {
  observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => layoutPlate())
  if (slotEl.value && observer) observer.observe(slotEl.value)
  scheduleThumbs()
})
onBeforeUnmount(() => {
  observer?.disconnect()
  dropThumbs()
})
watch(
  () => [store.rj, store.set, veiled.value, store.flipped, file.value, isMobile.value, store.dock] as const,
  () => layoutPlate(),
)
// A new work is a new visit: nothing is pre-loaded.
watch(
  () => store.rj,
  () => {
    dropThumbs()
    scheduleThumbs()
  },
)

function thumbStyle(item: WorkContentFile): { width: string; backgroundImage?: string } {
  const ratio = item.width !== null && item.height !== null && item.height > 0 ? item.width / item.height : 1
  const raw = 40 * ratio
  return {
    width: `${Math.round(Math.max(24, Math.min(110, raw)))}px`,
    ...(thumbs.value.has(thumbKey(item)) && !veiled.value
      ? { backgroundImage: `url("${thumbs.value.get(thumbKey(item))}")` }
      : {}),
  }
}
const isCropped = (item: WorkContentFile): boolean => {
  const ratio = item.width !== null && item.height !== null && item.height > 0 ? item.width / item.height : 1
  const raw = 40 * ratio
  return raw !== Math.max(24, Math.min(110, raw))
}

function openViewer(): void {
  if (veiled.value || store.flipped) return
  store.openViewer()
}

const PREV_ICON = 'M8 0L1 5l7 5z'
const NEXT_ICON = 'M2 0l7 5-7 5z'
</script>

<template>
  <div class="deck" :class="{ reading }" aria-label="Plate deck">
    <div class="strip">
      <span class="strip-name lbl">{{ mode }}</span>
      <span v-show="!reading" class="strip-side">
        <span ref="chipsEl" class="chips">
          <button v-for="(entry, i) in store.sets" :key="entry.path" class="chip" type="button"
            :aria-pressed="i === store.set" :title="entry.path || '根目录图片'"
            :aria-label="entry.path ? undefined : `根目录图片，${entry.files.length} 张`"
            @click="store.showSet(i)"><span class="jp">{{ entry.path ? entry.id : '封面' }}</span>{{ entry.files.length }}</button>
        </span>
        <span class="lbl">{{ count }}</span>
      </span>
    </div>

    <div ref="slotEl" class="slot">
      <div v-if="file" class="plate" :class="{ flipped: store.flipped, 'is-scroll': scrollMode }"
        :style="{ width: `${plateW}px`, height: `${plateH}px`, boxShadow: ghosts }">
        <div class="face front" :class="{ enter: entering, run: running, veil: veiled, scroll: scrollMode }"
          :style="veiled || scrollMode ? {} : frontStyle"
          :role="veiled ? 'button' : undefined" :tabindex="veiled ? 0 : undefined" @click="veiled ? store.unveil() : openViewer()">
          <img v-if="scrollMode && !veiled" :src="frontSrc" :alt="file.name" />
        </div>
        <div class="face back"><WorkColophon compact /></div>
        <div v-if="flipCount > 0" :key="flipCount" class="wipe"></div>
        <span class="scroll-tag">SCROLL</span>
        <i class="tick tl"></i><i class="tick tr"></i><i class="tick bl"></i>
        <button class="tick br" type="button" title="翻面：奥付" aria-label="翻面：奥付" @click="store.flip()"></button>
        <span class="flip-hint lbl">{{ store.flipped ? 'FRONT' : 'REVERSE' }}</span>
        <button v-show="!reading && !store.flipped && store.idx > 0" class="btn btn-icon step prev" type="button"
          aria-label="上一张" @click="store.step(-1)"><svg viewBox="0 0 10 10" aria-hidden="true"><path :d="PREV_ICON" /></svg></button>
        <button v-show="!reading && !store.flipped && file && store.idx < (set?.files.length ?? 1) - 1"
          class="btn btn-icon step next" type="button" aria-label="下一张" @click="store.step(1)"><svg viewBox="0 0 10 10" aria-hidden="true"><path :d="NEXT_ICON" /></svg></button>
      </div>
    </div>

    <div v-show="!reading" v-if="set && set.files.length >= 2" class="film">
      <button v-for="(item, i) in set.files" :key="item.rel_path" class="thumb"
        :class="{ crop: isCropped(item), pending: !thumbs.has(thumbKey(item)) && !veiled, veiled }" type="button"
        :style="thumbStyle(item)" :aria-current="i === store.idx ? 'true' : undefined" :title="item.name"
        @click="store.showImg(store.set, i)"></button>
    </div>
  </div>
</template>

<style scoped>
.deck { display: flex; flex-direction: column; min-height: 0; min-width: 0; }
.strip { gap: var(--sp-3); }

.strip-name, .strip-side > .lbl { flex-shrink: 0; white-space: nowrap; }
.strip-side { display: flex; align-items: center; gap: var(--sp-3); min-width: 0; }
.chips { display: inline-flex; gap: var(--sp-1); overflow-x: auto; scrollbar-width: none; min-width: 0; }
.chips::-webkit-scrollbar { display: none; }
.chip { height: 16px; padding: 0 6px; font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .08em; color: var(--fg-muted); border: 1px solid var(--wire-fine); display: inline-flex; align-items: center; gap: 4px; background: none; cursor: pointer; flex-shrink: 0; }
.chip:hover { border-color: var(--wire-mid); color: var(--fg-base); }
.chip[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.chip .jp { font-family: var(--font-ui); letter-spacing: 0; max-width: 8em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.slot { position: relative; flex: 1; min-height: 0; display: grid; place-items: center; margin: var(--sp-4) 0 var(--sp-3); }
.plate { position: relative; transition: width 240ms var(--ease-out), height 240ms var(--ease-out), box-shadow 160ms linear; }
.face { position: absolute; inset: 0; overflow: hidden; }
.face.front { clip-path: inset(0 0 0 0); background-size: contain; background-position: center; background-repeat: no-repeat; box-shadow: inset 0 0 0 1px var(--wire-fine); }
.face.front.scroll { overflow-y: auto; background: none; }
.face.front.scroll img { display: block; width: 100%; }
.face.front.enter { clip-path: inset(0 100% 0 0); }
.face.front.run { transition: clip-path 240ms var(--ease-out); }
.face.front:not(.veil):not(.scroll) { cursor: zoom-in; }
.face.front.scroll img { cursor: zoom-in; }
/* The reverse = the colophon: the bone edge wipes in from the left, then the paper settles. */
.face.back { background: var(--grain), var(--bg-panel); border: 1px solid var(--wire-strong); clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-in); overflow-y: auto; }
.plate.flipped .face.back { clip-path: inset(0 0 0 0); transition: clip-path 260ms var(--ease-out) 40ms; }
.wipe { position: absolute; top: 0; bottom: 0; left: 0; width: 100%; background: var(--invert-bg); transform: scaleX(0); transform-origin: left; pointer-events: none; z-index: 2; }
.wipe { animation: wipe 300ms var(--ease-out); }
@keyframes wipe { 0% { transform: scaleX(0); transform-origin: left; } 50% { transform: scaleX(1); transform-origin: left; } 51% { transform-origin: right; } 100% { transform: scaleX(0); transform-origin: right; } }

.tick { position: absolute; width: 8px; height: 8px; border-color: var(--wire-strong); border-style: solid; border-width: 0; z-index: 3; }
.tick.tl { left: -4px; top: -4px; border-left-width: 1px; border-top-width: 1px; }
.tick.tr { right: -4px; top: -4px; border-right-width: 1px; border-top-width: 1px; }
.tick.bl { left: -4px; bottom: -4px; border-left-width: 1px; border-bottom-width: 1px; }
.tick.br { right: -6px; bottom: -6px; width: 14px; height: 14px; border-right-width: 2px; border-bottom-width: 2px; cursor: pointer; background: none; padding: 0; }
.tick.br:hover { border-color: var(--now); }
.scroll-tag { position: absolute; left: 0; top: 0; height: 16px; padding: 0 6px; background: var(--invert-bg); color: var(--invert-fg); font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .12em; font-weight: 700; z-index: 2; display: none; align-items: center; }
.plate.is-scroll .scroll-tag { display: inline-flex; }
.plate.flipped .scroll-tag { display: none; }
.step { position: absolute; top: 50%; margin-top: -9px; z-index: 4; }
.step.prev { left: -38px; }
.step.next { right: -38px; }
.flip-hint { position: absolute; right: -6px; bottom: -24px; opacity: 0; pointer-events: none; z-index: 3; color: var(--fg-muted); transition: opacity var(--dur-out) linear; }
.tick.br:hover + .flip-hint { opacity: 1; transition: opacity var(--dur-in) linear; }

/* The R18 veil: the empty-state hatch, heavier for the thumbs so the two read apart. */
.face.front.veil { background: repeating-linear-gradient(-45deg, var(--wire-fine) 0 1px, transparent 1px 6px), var(--bg-panel); cursor: pointer; display: grid; place-items: center; }
.face.front.veil::after { content: 'R18 · SHOW'; font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .14em; color: var(--fg-muted); background: var(--bg-panel); border: 1px solid var(--wire-mid); padding: 2px var(--sp-2); }
.face.front.veil:hover::after { color: var(--invert-fg); background: var(--invert-bg); border-color: var(--invert-bg); }

/* The filmstrip: equal heights, widths follow the ratio; extremes are cropped with a hatch mark. */
.film { display: flex; gap: 6px; height: 48px; overflow-x: auto; overflow-y: hidden; flex-shrink: 0; padding: 2px 2px 4px; }
.thumb { position: relative; height: 40px; flex-shrink: 0; background-size: cover; background-position: center; box-shadow: inset 0 0 0 1px var(--wire-fine); opacity: .62; transition: opacity var(--dur-out) linear; border: none; padding: 0; cursor: pointer; background-color: var(--bg-panel); }
.thumb:hover { opacity: 1; box-shadow: inset 0 0 0 1px var(--wire-strong); }
.thumb[aria-current='true'] { opacity: 1; box-shadow: 0 0 0 1px var(--wire-strong), 3px 3px 0 0 var(--wire-mid); }
.thumb[aria-current='true']::after { content: ''; position: absolute; left: 50%; bottom: -5px; width: 6px; height: 3px; margin-left: -3px; background: var(--invert-bg); }
.thumb.crop::before { content: ''; position: absolute; right: 0; top: 0; bottom: 0; width: 6px; background: repeating-linear-gradient(-45deg, var(--invert-bg) 0 1px, transparent 1px 3px), var(--bg-panel); }
.thumb.pending, .thumb.veiled { background: repeating-linear-gradient(-45deg, var(--wire-fine) 0 1px, transparent 1px 4px), var(--bg-panel); opacity: 1; }
.thumb.veiled { background: repeating-linear-gradient(-45deg, var(--wire-mid) 0 1px, transparent 1px 4px), var(--bg-panel); }

@media (max-width: 768px) {
  .slot { height: 62vw; flex: none; }
  .deck.reading .slot { height: 34vw; }
  .step.prev { left: -30px; }
  .step.next { right: -30px; }
}
</style>
