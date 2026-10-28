<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useReducedMotion } from '@/composables/use-motion'

// The picture box of the collections page: a cover rebuilt by an interlaced
// raster scan, done in CSS (clip-path plus a line mask). The old frame powers
// down to black, then the odd and even lines are drawn, overlapping, each pass
// led by a beam with a short phosphor afterglow. No url shows a blank
// panel; `veiled` Gaussian-blurs the picture instead. Reduced motion, or a burst of switching, swaps the cover at once.
const props = defineProps<{ url: string | null, instant?: boolean, veiled?: boolean }>()
defineEmits<{ redraw: [] }>()

const reduce = useReducedMotion()
const SETTLE_MS = 400 // second pass delay (120) + pass (260) + a frame

const cur = ref<string | null>(null)
const next = ref<string | null>(null)
// Each layer is blurred by its own picture's veil, so a switch never shows the old one sharp.
const curV = ref(false)
const nextV = ref(false)
const scanning = ref(false)
const ready = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

function css(url: string | null) {
  return url
    ? `url("${url.replace(/"/g, '%22')}")`
    : 'none'
}

function settle(url: string | null, veiled: boolean) {
  clearTimeout(timer)
  scanning.value = false
  cur.value = next.value = url
  curV.value = nextV.value = veiled
}

watch(() => props.url, (url) => {
  if (!ready.value || reduce.value || props.instant || !url) {
    ready.value = true
    return settle(url, props.veiled === true)
  }
  settle(next.value, nextV.value) // an interrupted scan lands first
  next.value = url
  nextV.value = props.veiled === true
  scanning.value = false
  // Restart the animations: one frame with the class off, then on.
  requestAnimationFrame(() => {
    scanning.value = true
    timer = setTimeout(settle, SETTLE_MS, url, nextV.value)
  })
}, { immediate: true })
// The veil toggled on the picture already shown.
watch(() => props.veiled, (veiled) => {
  nextV.value = veiled === true
  // A url switch in the same tick leaves cur behind: the old picture keeps its own veil.
  if (next.value === cur.value)
    curV.value = nextV.value
})

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div
    class="screen" :class="{ scanning, empty: !cur && !scanning }" data-testid="collection-screen"
    :style="{ '--cur': css(cur), '--new': css(next) }" role="button" tabindex="-1" title="换一张"
    @click="$emit('redraw')"
  >
    <div class="scr-base" :class="{ blur: curV }" />
    <div class="scr-pass p1" :class="{ blur: nextV }" />
    <div class="scr-pass p2" :class="{ blur: nextV }" />
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
.scr-base, .scr-pass { --blur: blur(0); --zoom: 1; filter: var(--blur); transform: scale(var(--zoom)); }
.blur { --blur: blur(28px); --zoom: 1.15; } /* the zoom hides the blur's faded edge */
.scr-base { background: var(--cur) center / cover no-repeat; }
/* No cover: the bare panel colour, not the dark device. */
.screen.empty { background: var(--bg-panel); }
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
@keyframes decay { 40% { filter: brightness(.55) contrast(1.3) var(--blur); } to { filter: brightness(0) var(--blur); } }
@keyframes reveal { to { clip-path: inset(0 0 0 0); } }
@keyframes head {
  0% { transform: translateY(-100%); opacity: var(--glow, 1); }
  92% { opacity: var(--glow, 1); }
  100% { transform: translateY(2px); opacity: 0; }
}
@media (max-width: 900px) { .screen { width: 100%; height: auto; } }
@media (prefers-reduced-motion: reduce) { .screen * { animation: none !important; } }
</style>
