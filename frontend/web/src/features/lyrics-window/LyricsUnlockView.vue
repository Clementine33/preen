<script setup lang="ts">
import { LOCK_ICON } from '@/features/lyrics-window/lyrics-icons'

// The unlock chip (ADR 0016): the one control of the small window the desktop
// shell floats over a locked, click-through Lyrics Window. Never removed, so a
// locked window can be found again (it fades to a faint chip at rest, lights up
// on hover); the host decides what unlocking does.

defineEmits<{ unlock: [] }>()
</script>

<template>
  <button class="unlock" type="button" aria-label="解锁歌词窗口" title="解锁歌词窗口" @click="$emit('unlock')">
    <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="LOCK_ICON" fill-rule="evenodd" /></svg>
  </button>
</template>

<style scoped>
:global(html), :global(body) { background: transparent; overflow: hidden; }
.unlock { position: fixed; inset: 0; display: grid; place-items: center; width: 100%; height: 100%; color: #fff; background: #141518cc; border: 0; outline: 0; opacity: .12; cursor: pointer; transition: opacity 400ms linear; animation: locate 2s linear; }
.unlock:hover, .unlock:focus-visible { opacity: 1; transition-duration: 120ms; animation: none; }
/* fully lit for the first 1.5s so a fresh lock shows where the chip is; never 0, since a fully transparent pixel may not take the mouse */
@keyframes locate { 0%, 75% { opacity: 1; } }
.unlock svg { width: 12px; height: 12px; fill: currentColor; }
</style>
