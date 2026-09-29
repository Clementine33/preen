import { defineStore } from 'pinia'

/**
 * The silent player (ADR 0012 §4): the full state machine of the bottom
 * baseline with no audio source yet — the UI and the PR say so, and no fake
 * waveform or fabricated duration is ever shown. Elapsed time is real (a
 * clock while playing); the total and the track list stay unknown until an
 * audio backend lands, so the progress laser rests at its origin.
 *
 * Playback is fully decoupled from the wall's selection: the inspector follows
 * the selected work, this store follows the playing one, and neither changing
 * touches the other.
 */
/** The 1s clock handle: owned by the store, but a handle, not state. */
let clockTimer: number | null = null

export const usePlayerStore = defineStore('player', {
  state: () => ({
    /** The work now playing; null = the baseline is idle. */
    rj: null as string | null,
    title: '' as string,
    trackIndex: 0,
    paused: true,
    /** Seconds since this work started playing; only moves while playing. */
    elapsed: 0,
  }),
  actions: {
    play(work: { rj: string; title: string }, track = 0) {
      this.rj = work.rj
      this.title = work.title
      this.trackIndex = track
      this.elapsed = 0
      this.paused = false
      this.runClock()
    },
    /** Space on the wall pauses and resumes; it never changes what plays. */
    togglePause() {
      if (this.rj === null) return
      this.paused = !this.paused
      this.paused ? this.stopClock() : this.runClock()
    },
    runClock() {
      this.stopClock()
      // ponytail: one global 1s clock; per-work clocks when real playback exists.
      clockTimer = window.setInterval(() => {
        this.elapsed += 1
      }, 1_000)
    },
    stopClock() {
      if (clockTimer !== null) {
        window.clearInterval(clockTimer)
        clockTimer = null
      }
    },
  },
})
