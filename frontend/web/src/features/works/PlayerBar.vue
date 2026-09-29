<script setup lang="ts">
import { computed } from 'vue'
import { usePlayerStore } from '@/stores/player'

// The bottom baseline (ADR 0012 §4): the thick top line is the progress track,
// and the second double-bar edge of the page. The player is silent — no audio
// source exists yet — so the laser rests at its origin, the total time and the
// format stay em-dashes, and prev/next render disabled rather than pretending.
// Elapsed time is real: the store's clock only runs while playing.

const player = usePlayerStore()

const active = computed(() => player.rj !== null)
const playingTitle = computed(() => (active.value ? `${player.title}` : ''))

function mmss(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const elapsed = computed(() => (active.value ? mmss(player.elapsed) : '--:--'))

const PREV_ICON = 'M10 0L3 5l7 5zM0 0h2v10H0z'
const NEXT_ICON = 'M0 0l7 5-7 5zM8 0h2v10H8z'
const PLAY_ICON = 'M1 0l9 5-9 5z'
const PAUSE_ICON = 'M1 0h3v10H1zM6 0h3v10H6z'
</script>

<template>
  <footer class="baseline substrate-panel" :class="{ paused: player.paused || !active }">
    <div class="bl-left">
      <span class="bl-dot"></span>
      <span class="bl-rj">{{ player.rj ?? '' }}</span>
      <span class="bl-title">{{ playingTitle }}</span>
    </div>
    <div class="bl-center">
      <span class="t pos">{{ elapsed }}</span>
      <div class="bl-ctrls">
        <button class="btn btn-icon" type="button" aria-label="Previous track" title="Previous track"
          :disabled="!active">
          <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="PREV_ICON" /></svg>
        </button>
        <button class="btn btn-icon btn-main" type="button"
          :aria-label="player.paused ? 'Resume' : 'Pause'" :title="player.paused ? 'Resume' : 'Pause'"
          :disabled="!active" @click="player.togglePause()">
          <svg viewBox="0 0 10 10" aria-hidden="true">
            <path :d="player.paused ? PLAY_ICON : PAUSE_ICON" />
          </svg>
        </button>
        <button class="btn btn-icon" type="button" aria-label="Next track" title="Next track" :disabled="!active">
          <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="NEXT_ICON" /></svg>
        </button>
      </div>
      <span class="t">—</span>
    </div>
    <div class="bl-right lbl"><span>—</span></div>
  </footer>
</template>

<style scoped>
.baseline {
  position: relative; display: grid; grid-template-columns: 1fr auto 1fr; align-items: center;
  gap: var(--sp-4); padding: 0 var(--sp-6);
}
/* The second double-bar edge: the thick line doubles as the laser's track. */
.baseline::before {
  content: ''; position: absolute; left: 0; right: 0; top: 0; height: 4px;
  border-top: 2px solid var(--laser-track); border-bottom: 1px solid var(--wire-fine);
}
.bl-left { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; padding-top: 2px; }
.bl-dot { width: 6px; height: 6px; background: var(--now); flex-shrink: 0; }
.paused .bl-dot { background: var(--fg-meta); }
.bl-rj {
  font-family: var(--font-mono); font-size: var(--fs-xs); font-weight: 600;
  letter-spacing: .08em; color: var(--fg-base); flex-shrink: 0;
}
.bl-title { font-size: var(--fs-xs); color: var(--fg-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

.bl-center { display: flex; align-items: center; gap: var(--sp-3); padding-top: 2px; }
.bl-center .t {
  font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .04em;
  color: var(--fg-muted); min-width: 36px;
}
.bl-center .t.pos { text-align: right; color: var(--fg-base); font-weight: 600; }
.bl-ctrls { display: flex; gap: var(--sp-1); }
.bl-ctrls .btn-main { width: 32px; border-color: var(--wire-mid); }
.bl-ctrls .btn:disabled { cursor: not-allowed; opacity: .5; }

.bl-right { display: flex; justify-content: flex-end; padding-top: 2px; }

@media (max-width: 768px) {
  .baseline { padding: 0 var(--sp-4); grid-template-columns: 1fr auto; }
  .bl-right { display: none; }
  .bl-center .t { display: none; }
}
</style>
