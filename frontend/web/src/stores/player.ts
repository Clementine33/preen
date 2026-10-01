import { defineStore } from 'pinia'

/**
 * The bottom baseline's player. A work played from the detail page plays its
 * real file: an audio element loading the content route by address (ADR 0016
 * §3, single-range 206 for seeking). A work played from the wall has no
 * manifest to play from, so it runs the silent state machine (ADR 0012 §4): a
 * real elapsed clock, an unknown total, no fabricated waveform.
 *
 * Playback is fully decoupled from the wall's selection, and from the page:
 * the element lives here, never in the DOM, so leaving the detail page keeps
 * the sound going.
 */

/** What a real track plays: its address, and the facts the baseline shows before the element knows them. */
export interface PlaySource {
  src: string
  /** Scan-time duration (docs/api.md), until the element reports its own. */
  durationSec: number | null
  /** Container label for the baseline: 'MP3' | 'WAV' | … */
  format: string | null
}

/** The 1s silent clock and the playing element: handles owned by the store, not state. */
let clockTimer: number | null = null
let audio: HTMLAudioElement | null = null

export const usePlayerStore = defineStore('player', {
  state: () => ({
    /** The work now playing; null = the baseline is idle. */
    rj: null as string | null,
    title: '' as string,
    trackIndex: 0,
    paused: true,
    /** Seconds into the playing track (silent: since the work started). */
    elapsed: 0,
    /** Seconds the track lasts; null while unknown (always, when silent). */
    duration: null as number | null,
    format: null as string | null,
  }),
  actions: {
    /**
     * Start a work. With a source the file plays; without one the silent clock
     * runs. Called synchronously from the click, so the browser treats the
     * element's play() as user-initiated.
     */
    play(work: { rj: string; title: string }, track = 0, source?: PlaySource) {
      this.rj = work.rj
      this.title = work.title
      this.trackIndex = track
      this.elapsed = 0
      this.paused = false
      this.stopClock()
      this.release()
      this.duration = source?.durationSec ?? null
      this.format = source?.format ?? null
      if (source) this.attach(source.src)
      else this.runClock()
    },
    /** Space pauses and resumes; it never changes what plays. */
    togglePause() {
      if (this.rj === null) return
      this.paused = !this.paused
      // The element's own events confirm (or correct) the state set here.
      if (audio) {
        if (this.paused) audio.pause()
        else this.resume(audio)
        return
      }
      this.paused ? this.stopClock() : this.runClock()
    },
    /** Jump within the playing track (a lyric line click). */
    seek(seconds: number) {
      if (audio) audio.currentTime = seconds
      this.elapsed = seconds
    },
    /** One element per play: every handler checks it is still the current one. */
    attach(src: string) {
      const el = new Audio()
      audio = el
      const current = () => el === audio
      el.addEventListener('timeupdate', () => {
        if (current()) this.elapsed = el.currentTime
      })
      el.addEventListener('durationchange', () => {
        if (current() && Number.isFinite(el.duration)) this.duration = el.duration
      })
      el.addEventListener('play', () => {
        if (current()) this.paused = false
      })
      for (const stop of ['pause', 'ended', 'error']) {
        el.addEventListener(stop, () => {
          if (current()) this.paused = true
        })
      }
      el.src = src
      this.resume(el)
    },
    resume(el: HTMLAudioElement) {
      // A superseded element's play() rejects with AbortError: not this track's
      // failure. Anything else (blocked autoplay, a 401/404 source) pauses —
      // never falling back to the silent clock, which would pretend audio exists.
      Promise.resolve(el.play()).catch((err: unknown) => {
        if (el === audio && !(err instanceof DOMException && err.name === 'AbortError')) this.paused = true
      })
    },
    release() {
      if (!audio) return
      const el = audio
      audio = null
      el.pause()
      el.removeAttribute('src')
      el.load()
    },
    runClock() {
      this.stopClock()
      // ponytail: one global 1s clock for the silent path only.
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
