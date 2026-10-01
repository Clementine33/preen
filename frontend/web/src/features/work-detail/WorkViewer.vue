<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type { ContentFileRef } from '@/lib/api/types'
import { useWorkDetailStore } from '@/stores/work-detail'

// The fullscreen viewer (v3): a native <dialog> on a solid substrate — no
// translucency (ADR 0012 removed it). FIT never upscales; 1:1 removes the
// caps and pans by pointer drag. It shares the deck's set/idx, so stepping
// here moves the plate too, and closing leaves it on the last viewed image.

const store = useWorkDetailStore()
const api = useApiClient()
const file = computed(() => store.currentFile)
const count = computed(() =>
  `${String(store.idx + 1).padStart(2, '0')} / ${String(store.currentSet?.files.length ?? 0).padStart(2, '0')}`,
)
const src = computed(() => {
  const item = file.value
  if (!item || !store.rj || !store.activeCopy) return ''
  const ref: ContentFileRef = { rj: store.rj, copyId: store.activeCopy.copy_id, relPath: item.rel_path }
  return api.workFileUrl(ref, item.thumb_v ?? undefined)
})

// ── Open/close: jsdom has no showModal, so tests render the same DOM by the
//    plain open attribute (the open path itself is e2e territory). ──
const dialogEl = ref<HTMLDialogElement | null>(null)
watch(
  () => store.viewerOpen,
  (open) => {
    const el = dialogEl.value
    if (!el) return
    if (open && !el.open) {
      if (typeof el.showModal === 'function') el.showModal()
      else el.setAttribute('open', '')
    } else if (!open && el.open) {
      if (typeof el.close === 'function') el.close()
      else el.removeAttribute('open')
    }
  },
)

// ── FIT / 1:1: an image that fits the viewport has no 1:1 to offer. ──
const real = ref(false)
const imgEl = ref<HTMLImageElement | null>(null)
const bodyEl = ref<HTMLElement | null>(null)
const fits = ref(true)
const big = ref(false)
const realOne = computed(() => real.value && !fits.value)

function applyFit(): void {
  const img = imgEl.value
  if (!img) return
  // jsdom reports natural size 0: everything fits, exactly like a mock image.
  fits.value = img.naturalWidth <= window.innerWidth - 112 && img.naturalHeight <= window.innerHeight - 96
  big.value = !fits.value
  if (real.value && !fits.value && bodyEl.value) {
    bodyEl.value.scrollLeft = (bodyEl.value.scrollWidth - bodyEl.value.clientWidth) / 2
    bodyEl.value.scrollTop = (bodyEl.value.scrollHeight - bodyEl.value.clientHeight) / 2
  }
}

function setReal(on: boolean): void {
  real.value = on
  applyFit()
}

// Every open and every step resets to FIT (v3).
watch(
  () => [store.viewerOpen, store.set, store.idx] as const,
  () => {
    real.value = false
  },
)

function close(): void {
  const el = dialogEl.value
  if (el && typeof el.close === 'function') el.close()
  else {
    el?.removeAttribute('open')
    store.closeViewer()
  }
}

// ── 1:1 drag-pan (v3): the pointer drags the scroll, the image ignores it so
//    the drag never stalls. ──
const dragging = ref(false)
let drag: { x: number; y: number; left: number; top: number } | null = null
function onPointerDown(event: PointerEvent): void {
  if (!realOne.value || !bodyEl.value) return
  drag = { x: event.clientX, y: event.clientY, left: bodyEl.value.scrollLeft, top: bodyEl.value.scrollTop }
  dragging.value = true
  if (typeof bodyEl.value.setPointerCapture === 'function') bodyEl.value.setPointerCapture(event.pointerId)
}
function onPointerMove(event: PointerEvent): void {
  if (!drag || !bodyEl.value) return
  bodyEl.value.scrollLeft = drag.left - (event.clientX - drag.x)
  bodyEl.value.scrollTop = drag.top - (event.clientY - drag.y)
}
function onPointerUp(): void {
  drag = null
  dragging.value = false
}

const PREV_ICON = 'M8 0L1 5l7 5z'
const NEXT_ICON = 'M2 0l7 5-7 5z'
</script>

<template>
  <dialog ref="dialogEl" class="viewer" aria-label="插画全屏查看" @close="store.closeViewer()">
    <div class="strip">
      <span class="strip-name lbl">VIEW</span>
      <span class="strip-side">
        <span class="strip-file" lang="ja">{{ file?.name ?? '—' }}</span>
        <span class="lbl">{{ file?.width ?? '—' }}×{{ file?.height ?? '—' }}</span>
        <span class="lbl">{{ count }}</span>
        <span class="seg">
          <button class="btn btn-s" type="button" :aria-pressed="!realOne" @click="setReal(false)">FIT</button>
          <button class="btn btn-s" type="button" :disabled="fits" :aria-pressed="realOne" @click="setReal(true)">1:1</button>
        </span>
        <button class="btn btn-s" type="button" @click="close">CLOSE</button>
      </span>
    </div>
    <div ref="bodyEl" class="vw-body" :class="{ real: realOne, dragging }"
      @pointerdown="onPointerDown" @pointermove="onPointerMove" @pointerup="onPointerUp" @pointercancel="onPointerUp">
      <img ref="imgEl" :src="src" :class="{ big }" :alt="file?.name ?? ''" @load="applyFit"
        @click="!realOne && big && setReal(true)" />
    </div>
    <button v-show="store.idx > 0" class="btn btn-icon step prev" type="button" aria-label="上一张"
      @click="store.step(-1)"><svg viewBox="0 0 10 10" aria-hidden="true"><path :d="PREV_ICON" /></svg></button>
    <button v-show="store.idx < (store.currentSet?.files.length ?? 1) - 1" class="btn btn-icon step next" type="button"
      aria-label="下一张" @click="store.step(1)"><svg viewBox="0 0 10 10" aria-hidden="true"><path :d="NEXT_ICON" /></svg></button>
  </dialog>
</template>

<style scoped>
.viewer { position: fixed; inset: 0; width: 100vw; height: 100vh; max-width: none; max-height: none; margin: 0; padding: 0; border: 0; background: var(--grain), var(--bg-canvas); color: var(--fg-base); display: none; grid-template-rows: auto minmax(0, 1fr); }
.viewer[open] { display: grid; }
.viewer::backdrop { background: var(--bg-canvas); }
.viewer .strip { margin: var(--sp-3) var(--sp-4) 0; }
.viewer .strip-side { display: flex; align-items: center; gap: var(--sp-3); }
.strip-file { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-base); letter-spacing: .04em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.seg { display: inline-flex; border: 1px solid var(--wire-mid); }
.seg :deep(.btn-s + .btn-s) { border-left: 1px solid var(--wire-fine); }

.vw-body { position: relative; overflow: auto; display: grid; place-items: center; scrollbar-color: var(--wire-mid) transparent; }
.vw-body img { display: block; max-width: calc(100vw - 112px); max-height: calc(100vh - 96px); box-shadow: 0 0 0 1px var(--wire-mid), 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid); }
.vw-body img.big { cursor: zoom-in; }
.vw-body.real { place-items: start; cursor: grab; }
.vw-body.real img { max-width: none; max-height: none; margin: var(--sp-6); cursor: default; }
.vw-body.real.dragging { cursor: grabbing; }
.vw-body.real.dragging img { pointer-events: none; }
.step { position: fixed; top: 50%; transform: translateY(-50%); }
.step.prev { left: var(--sp-4); }
.step.next { right: var(--sp-4); }
</style>
