<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useReducedMotion } from '@/composables/use-motion'

// The picture box of the collections page: a cover rebuilt by an interlaced
// raster scan, done in CSS (clip-path plus a line mask). The old frame powers
// down to black, then the odd and even lines are drawn, overlapping, each pass
// led by a beam with a short phosphor afterglow. No url shows the hatched
// empty state. Reduced motion, or a burst of switching, swaps the cover at once.
const props = defineProps<{ url: string | null; instant?: boolean }>()
defineEmits<{ redraw: [] }>()

const reduce = useReducedMotion()
const SETTLE_MS = 400 // second pass delay (120) + pass (260) + a frame

const cur = ref<string | null>(null)
const next = ref<string | null>(null)
const scanning = ref(false)
const ready = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

const css = (url: string | null) => (url ? `url("${url.replace(/"/g, '%22')}")` : 'none')

function settle(url: string | null) {
  clearTimeout(timer)
  scanning.value = false
  cur.value = next.value = url
}

watch(() => props.url, (url) => {
  if (!ready.value || reduce.value || props.instant || !url) {
    ready.value = true
    return settle(url)
  }
  settle(next.value) // an interrupted scan lands first
  next.value = url
  scanning.value = false
  // Restart the animations: one frame with the class off, then on.
  requestAnimationFrame(() => {
    scanning.value = true
    timer = setTimeout(() => settle(url), SETTLE_MS)
  })
}, { immediate: true })

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="screen" :class="{ scanning, empty: !cur && !scanning }" data-testid="collection-screen"
    :style="{ '--cur': css(cur), '--new': css(next) }" role="button" tabindex="-1" title="换一张"
    @click="$emit('redraw')">
    <div class="scr-base" />
    <div class="scr-pass p1" />
    <div class="scr-pass p2" />
    <i class="scr-head h1" />
    <i class="scr-head h2" />
  </div>
</template>

<style scoped>
/* The picture box is a device: dark in both themes. */
.screen {
  --screen-bg: #07080a; --screen-ink: #ded7c6;
  --t: 260ms; --t2: 120ms;
  position: relative; height: var(--cover-h); aspect-ratio: var(--cover-ratio); overflow: hidden; isolation: isolate;
  background: var(--screen-bg); cursor: pointer; user-select: none;
}
.screen > * { position: absolute; inset: 0; pointer-events: none; }
.scr-base { background: var(--cur) center / cover no-repeat; }
/* The wall's empty/veiled cover: the same 45° hatch on the panel colour, not the dark device. */
.screen.empty { background: repeating-linear-gradient(-45deg, var(--wire-mid) 0 1px, transparent 1px 6px), var(--bg-panel); }
.screen.empty .scr-base { display: none; }
.scr-pass { background: var(--new) center / cover no-repeat; clip-path: inset(0 0 100% 0); }
.scr-pass.p1 { mask: repeating-linear-gradient(to bottom, #000 0 2px, transparent 2px 4px); }
.scr-pass.p2 { mask: repeating-linear-gradient(to bottom, transparent 0 2px, #000 2px 4px); }
/* Scan head = a 2px beam with a short afterglow trailing above it (the lines just drawn). */
.scr-head {
  inset: 0; opacity: 0;
  background:
    linear-gradient(var(--screen-ink), var(--screen-ink)) bottom / 100% 2px no-repeat,
    linear-gradient(to bottom, transparent, color-mix(in srgb, var(--screen-ink) 22%, transparent)) bottom / 100% 36px no-repeat;
  mix-blend-mode: screen;
  transform: translateY(-100%);
}
.h2 { --glow: .55; }
/* The old picture powers down to black first, so the new lines land on a dark screen, not on a ghost. */
.scanning .scr-base { animation: decay 110ms cubic-bezier(.4, 0, 1, 1) forwards; }
.scanning .p1 { animation: reveal var(--t) linear forwards; }
.scanning .p2 { animation: reveal var(--t) linear var(--t2) forwards; }
.scanning .h1 { animation: head var(--t) linear forwards; }
.scanning .h2 { animation: head var(--t) linear var(--t2) forwards; }
@keyframes decay { 40% { filter: brightness(.55) contrast(1.3); } to { filter: brightness(0); } }
@keyframes reveal { to { clip-path: inset(0 0 0 0); } }
@keyframes head {
  0% { transform: translateY(-100%); opacity: var(--glow, 1); }
  92% { opacity: var(--glow, 1); }
  100% { transform: translateY(2px); opacity: 0; }
}
@media (max-width: 900px) { .screen { width: 100%; height: auto; } }
@media (prefers-reduced-motion: reduce) { .screen * { animation: none !important; } }
</style>
