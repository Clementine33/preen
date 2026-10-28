<script setup lang="ts">
import { LOCK_ICON } from '@/features/lyrics-window/lyrics-icons'

// The unlock chip (ADR 0016): the one control of the small window the desktop
// shell floats over a locked, click-through Lyrics Window. Always visible so a
// locked window can be found again; the host decides what unlocking does.

defineEmits<{ unlock: [] }>()
</script>

<template>
  <button class="unlock" type="button" aria-label="解锁歌词窗口" title="解锁歌词窗口" @click="$emit('unlock')">
    <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="LOCK_ICON" fill-rule="evenodd" /></svg>
  </button>
</template>

<style scoped>
:global(html), :global(body) { background: transparent; overflow: hidden; }
.unlock { position: fixed; inset: 0; display: grid; place-items: center; width: 100%; height: 100%; color: #fff; background: #141518; border: 1px solid #fff6; opacity: .55; cursor: pointer; transition: opacity var(--dur-in) linear; }
.unlock:hover, .unlock:focus-visible { opacity: 1; }
.unlock svg { width: 12px; height: 12px; fill: currentColor; }
</style>
