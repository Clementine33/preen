<script setup lang="ts">
import { computed } from 'vue'
import { usePlayerStore } from '@/stores/player'

// The bottom baseline: the thick top line is the progress track,
// and the second double-bar edge of the page. A real track fills the laser to
// its position and shows its length and format; a silent play (from the wall,
// with no file behind it) leaves the laser at its origin and both as
// em-dashes. Prev/next step through the work's track queue, and the laser's
// line is a seek slider while the track's length is known. A mouse gets a
// volume control; a touch screen gets larger controls instead, and its own
// hardware volume (iOS ignores an element's volume anyway).

const player = usePlayerStore()

const active = computed(() => player.rj !== null)
const playingTitle = computed(() => (active.value ? `${player.title}` : ''))

function mmss(raw: number): string {
  const seconds = Math.floor(raw)
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const elapsed = computed(() => (active.value ? mmss(player.elapsed) : '--:--'))
const total = computed(() => (active.value && player.duration !== null ? mmss(player.duration) : '—'))
const progress = computed(() =>
  active.value && player.duration ? `${Math.min(100, (player.elapsed / player.duration) * 100)}%` : '0%',
)

const seekable = computed(() => active.value && player.duration !== null)
function onSeek(event: Event): void {
  player.seek(Number((event.target as HTMLInputElement).value))
}

function onVolume(event: Event): void {
  player.setVolume(Number((event.target as HTMLInputElement).value))
}
const silent = computed(() => player.muted || player.volume === 0)
const volumeFill = computed(() => `${(silent.value ? 0 : player.volume) * 100}%`)

const PREV_ICON = 'M10 0L3 5l7 5zM0 0h2v10H0z'
const NEXT_ICON = 'M0 0l7 5-7 5zM8 0h2v10H8z'
const PLAY_ICON = 'M1 0l9 5-9 5z'
const PAUSE_ICON = 'M1 0h3v10H1zM6 0h3v10H6z'
const SPEAKER_ICON = 'M0 3.5h2.2L5 1v8L2.2 6.5H0z'
const WAVES_ICON = 'M6.3 3.6h1v2.8h-1zM8.3 2.2h1v5.6h-1z'
const MUTED_ICON = 'M6 3.7l.7-.7L8 4.3 9.3 3l.7.7L8.7 5 10 6.3l-.7.7L8 5.7 6.7 7 6 6.3 7.3 5z'
</script>

<template>
  <footer class="baseline substrate-panel" :class="{ paused: player.paused || !active }">
    <i class="bl-laser" :style="{ width: progress }" aria-hidden="true"></i>
    <input class="bl-seek" type="range" aria-label="Seek" min="0" step="any"
      :max="player.duration ?? 0" :value="player.elapsed" :disabled="!seekable" @input="onSeek" />
    <div class="bl-left">
      <span class="bl-dot"></span>
      <RouterLink v-if="player.rj" class="bl-rj" :to="`/works/${player.rj}`" :title="`打开 ${player.rj}`">{{ player.rj }}</RouterLink>
      <span class="bl-title">{{ playingTitle }}</span>
    </div>
    <div class="bl-center">
      <span class="t pos">{{ elapsed }}</span>
      <div class="bl-ctrls">
        <button class="btn btn-icon" type="button" aria-label="Previous track" title="Previous track"
          :disabled="!player.hasPrev" @click="player.step(-1)">
          <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="PREV_ICON" /></svg>
        </button>
        <button class="btn btn-icon btn-main" type="button"
          :aria-label="player.paused ? 'Resume' : 'Pause'" :title="player.paused ? 'Resume' : 'Pause'"
          :disabled="!active" @click="player.togglePause()">
          <svg viewBox="0 0 10 10" aria-hidden="true">
            <path :d="player.paused ? PLAY_ICON : PAUSE_ICON" />
          </svg>
        </button>
        <button class="btn btn-icon" type="button" aria-label="Next track" title="Next track"
          :disabled="!player.hasNext" @click="player.step(1)">
          <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="NEXT_ICON" /></svg>
        </button>
      </div>
      <span class="t">{{ total }}</span>
    </div>
    <div class="bl-right">
      <div class="bl-vol">
        <button class="btn btn-icon bl-mute" type="button" :aria-pressed="player.muted"
          aria-label="Mute" title="Mute" @click="player.toggleMute()">
          <svg viewBox="0 0 10 10" aria-hidden="true">
            <path :d="SPEAKER_ICON" /><path :d="silent ? MUTED_ICON : WAVES_ICON" />
          </svg>
        </button>
        <input class="bl-volume" type="range" aria-label="Volume" min="0" max="1" step="0.05"
          :value="player.muted ? 0 : player.volume" :style="{ '--fill': volumeFill }" @input="onVolume" />
      </div>
      <span class="lbl">{{ (active && player.format) || '—' }}</span>
    </div>
  </footer>
</template>

<style scoped>
.baseline {
  height: 28px; position: relative; display: grid; grid-template-columns: 1fr auto 1fr; align-items: center;
  gap: var(--sp-4); padding: 0 var(--sp-6);
}
/* The second double-bar edge: the thick line doubles as the laser's track. */
.baseline::before {
  content: ''; position: absolute; left: 0; right: 0; top: 0; height: 4px;
  border-top: 2px solid var(--laser-track); border-bottom: 1px solid var(--wire-fine);
}
.bl-laser {
  position: absolute; left: 0; top: 0; height: 2px; background: var(--laser);
  box-shadow: 0 0 6px var(--laser-glow); z-index: 1; pointer-events: none;
}
/* The seek slider: transparent over the laser's line, a taller hit area. */
.bl-seek {
  position: absolute; left: 0; right: 0; top: -4px; height: 12px; margin: 0; z-index: 2;
  appearance: none; background: transparent; cursor: pointer;
}
.bl-seek:disabled { pointer-events: none; }
.bl-seek:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: -1px; }
.bl-seek::-webkit-slider-runnable-track { height: 12px; background: transparent; }
.bl-seek::-webkit-slider-thumb { appearance: none; width: 2px; height: 12px; background: transparent; }
.bl-seek::-moz-range-track { height: 12px; background: transparent; }
.bl-seek::-moz-range-thumb { width: 2px; height: 12px; border: 0; background: transparent; }
.bl-left { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; padding-top: 2px; }
.bl-dot { width: 6px; height: 6px; background: var(--now); flex-shrink: 0; }
.paused .bl-dot { background: var(--fg-meta); }
.bl-rj {
  font-family: var(--font-mono); font-size: var(--fs-xs); font-weight: 600;
  letter-spacing: .08em; color: var(--fg-base); flex-shrink: 0; text-decoration: none;
}
.bl-rj:hover { text-decoration: underline; text-underline-offset: 4px; }
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

.bl-right { display: flex; align-items: center; justify-content: flex-end; gap: var(--sp-4); padding-top: 2px; }

/* Volume: the mute toggle and a 72px fader, its fill drawn like the laser. */
.bl-vol { display: flex; align-items: center; gap: var(--sp-2); }
.bl-mute { width: 22px; border-color: transparent; }
.bl-mute svg { width: 10px; height: 10px; }
.bl-volume {
  width: 72px; height: 18px; margin: 0; appearance: none; background: transparent; cursor: pointer;
}
.bl-volume:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }
.bl-volume::-webkit-slider-runnable-track {
  height: 2px; background: linear-gradient(to right, var(--fg-base) var(--fill), var(--wire-mid) var(--fill));
}
.bl-volume::-webkit-slider-thumb {
  appearance: none; width: 4px; height: 10px; margin-top: -4px; background: var(--fg-base);
}
.bl-volume::-moz-range-track { height: 2px; background: var(--wire-mid); }
.bl-volume::-moz-range-progress { height: 2px; background: var(--fg-base); }
.bl-volume::-moz-range-thumb { width: 4px; height: 10px; border: 0; border-radius: 0; background: var(--fg-base); }

/* A finger needs room: a taller bar, 44px-wide buttons and a seek strip the
   height of a fingertip. Touch volume is the device's own. */
@media (pointer: coarse) {
  .baseline { height: 52px; }
  .bl-seek { top: -8px; height: 24px; }
  .bl-seek::-webkit-slider-runnable-track, .bl-seek::-webkit-slider-thumb { height: 24px; }
  .bl-seek::-moz-range-track, .bl-seek::-moz-range-thumb { height: 24px; }
  .bl-ctrls { gap: var(--sp-2); }
  .bl-ctrls .btn-icon { width: 44px; height: 40px; }
  .bl-ctrls .btn-main { width: 56px; }
  .bl-ctrls .btn-icon svg { width: 12px; height: 12px; }
  .bl-vol { display: none; }
}

@media (max-width: 768px) {
  .baseline { padding: 0 var(--sp-4); grid-template-columns: 1fr auto; }
  .bl-right { display: none; }
  .bl-center .t { display: none; }
}
</style>
