<script setup lang="ts">
import type { LyricsCommand, LyricsFrame } from '@/lib/desktop/desktop-adapter'
import { computed } from 'vue'
import { NEXT_ICON, PAUSE_ICON, PLAY_ICON, PREV_ICON } from '@/features/works/player-icons'

// The Lyrics Window's content (ADR 0016): the current line of the playing
// track's Timed Lyrics, scrolling in place, under a control bar that appears on hover. Pure
// display of a serializable frame; it runs in an app of its own (a Document
// Picture-in-Picture document, or a desktop window), so no Pinia, router or
// injection here, and no keyboard handling. The bar is also the desktop
// window's drag region, and its `host` slot carries the buttons only a host
// can act on (close and lock in the desktop window; Picture-in-Picture has the
// system close button and leaves it empty).

const props = defineProps<{ frame: LyricsFrame }>()
defineEmits<{ command: [LyricsCommand] }>()

/** What the stage shows: the line, else the title; a second text line is the translation. */
const shown = computed(() => props.frame.line ?? { key: 'title', text: props.frame.title, dim: false })
const rows = computed(() => shown.value.text.split('\n').slice(0, 2))
</script>

<template>
  <div class="lw" lang="ja" :class="{ paused: frame.paused }">
    <div class="bar">
      <button type="button" aria-label="Previous track" :disabled="!frame.hasPrev" @click="$emit('command', 'prev')">
        <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="PREV_ICON" /></svg>
      </button>
      <button type="button" :aria-label="frame.paused ? 'Resume' : 'Pause'" :disabled="frame.rj === null" @click="$emit('command', 'toggle')">
        <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="frame.paused ? PLAY_ICON : PAUSE_ICON" /></svg>
      </button>
      <button type="button" aria-label="Next track" :disabled="!frame.hasNext" @click="$emit('command', 'next')">
        <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="NEXT_ICON" /></svg>
      </button>
      <slot name="host" />
    </div>
    <div class="stage">
      <Transition name="lw-line" :css="shown.advance === true">
        <p :key="shown.key" class="line" :class="{ dim: shown.dim }">
          <span v-for="(row, i) in rows" :key="i" :class="i === 0 ? 'main' : 'sub'">{{ row }}</span>
        </p>
      </Transition>
      <p v-if="frame.noLyrics && !frame.line" class="none">无歌词</p>
    </div>
  </div>
</template>

<style scoped>
.lw {
  position: fixed; inset: 0; display: flex; flex-direction: column;
  background: var(--invert-bg); color: var(--invert-fg); font-family: var(--font-ui);
  /* Readable on whatever lies behind a transparent window. */
  text-shadow: 0 0 2px #000, 0 1px 6px #000a;
}
/* The desktop entry marks a transparent window: nothing paints behind the lyrics. */
:global(html.lw-clear), :global(html.lw-clear body), :global(html.lw-clear) .lw { background: transparent; }
.stage { container-type: size; position: relative; flex: 1; min-height: 0; overflow: hidden; transition: opacity var(--dur-in) linear; }
.paused .stage { opacity: .7; }
.line, .none { position: absolute; left: 0; right: 0; margin: 0; padding: 0 var(--sp-4); text-align: center; }
.line { top: 50%; translate: 0 -50%; display: flex; flex-direction: column; gap: .25em; font-size: clamp(14px, min(6cqw, 30cqh), 36px); font-weight: 700; line-height: 1.3; overflow-wrap: anywhere; transition: opacity var(--dur-in) var(--ease-out); }
.line.dim { opacity: .45; }
.sub { font-size: .8em; font-weight: 400; }
.none { bottom: var(--sp-3); font-size: var(--fs-xs); opacity: .6; }

/* A step to the next line scrolls: the old line leaves by its own height as the new one follows from below. */
.lw-line-enter-active { transition: transform calc(var(--dur-in) * 2) var(--ease-out), opacity calc(var(--dur-in) * 2) var(--ease-out); }
.lw-line-leave-active { transition: transform calc(var(--dur-in) * 2) var(--ease-out), opacity calc(var(--dur-in) * 1.2) var(--ease-out); }
.lw-line-enter-from { transform: translateY(100%); opacity: 0; }
.lw-line-leave-to { transform: translateY(-100%); opacity: 0; }

/* Desktop-lyrics convention: the controls float over the top edge only while the pointer (or focus) is in the window. */
.bar { position: absolute; top: 0; left: 0; right: 0; z-index: 1; display: flex; justify-content: center; gap: var(--sp-2); padding: var(--sp-2); background: linear-gradient(color-mix(in srgb, var(--invert-bg) 80%, transparent) 60%, transparent); --wails-draggable: drag; opacity: 0; transition: opacity var(--dur-in) linear; }
.lw:hover .bar, .bar:focus-within { opacity: 1; }
.bar button, .bar :slotted(button) { --wails-draggable: no-drag; width: 36px; height: 28px; display: grid; place-items: center; color: inherit; background: none; border: 1px solid color-mix(in srgb, currentColor 40%, transparent); }
.bar button:hover:not(:disabled), .bar :slotted(button:hover:not(:disabled)) { background: var(--invert-fg); color: var(--invert-bg); }
.bar button:focus-visible, .bar :slotted(button:focus-visible) { outline: 1px solid currentColor; outline-offset: 2px; }
.bar button:disabled { opacity: .35; cursor: not-allowed; }
.bar svg { width: 10px; height: 10px; fill: currentColor; }
</style>
