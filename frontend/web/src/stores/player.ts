import type { ApiClientContract, PutWorkListeningParams, WorkListening } from '@/lib/api/types'
import { defineStore } from 'pinia'

/**
 * The bottom baseline's player. A work plays as a queue of its tracks: each
 * real file through an audio element loading the content route by address
 * (ADR 0013, single-range 206 for seeking); the end of one starts the next.
 * A work with no audio file runs the silent state machine: a
 * real elapsed clock, an unknown total, no fabricated waveform.
 *
 * The Listening Position of a real play is written to the server (ADR 0009,
 * docs/api/works.md `PUT /works/{rj}/listening`): nothing before 10 s of real
 * playback of the Rendition, then every 15 s while playing and at once on
 * pause, track change, work change and pagehide. A work's requests go out one
 * at a time; a failed one stays pending and is resent by the next write.
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

/** One entry of the playing work's queue; without a source it plays silently. */
export interface QueueItem {
  /** The track's identity in the page's list; the queue may be a subset of it. */
  key?: string
  title: string
  /** The Rendition the track belongs to and its place among that Rendition's tracks. */
  rendition?: string
  index?: number
  count?: number
  source?: PlaySource
  /** Where the track's Timed Lyrics are, in the copy it plays from. */
  lyrics?: { copyId: string, relPath: string }
}

/** The listener's volume, a per-browser convenience: unreadable storage means full volume. */
const VOLUME_KEY = 'preen.player.volume'
function readVolume(): { volume: number, muted: boolean } {
  try {
    const saved = JSON.parse(localStorage.getItem(VOLUME_KEY) ?? 'null') as { volume?: unknown, muted?: unknown } | null
    const volume = typeof saved?.volume === 'number'
      ? Math.min(1, Math.max(0, saved.volume))
      : 1
    return { volume, muted: saved?.muted === true }
  }
  catch {
    return { volume: 1, muted: false }
  }
}

const GATE_SEC = 10
const WRITE_EVERY_SEC = 15

type Snapshot = Omit<PutWorkListeningParams, 'client' | 'seq'>

/**
 * The Listening Position bookkeeping: handles owned by the store, not state.
 * `pending` holds what the server has not acknowledged (per work and
 * Rendition); `acknowledged` is the last snapshot it did, which is what a new
 * snapshot is compared against.
 */
let api: ApiClientContract | null = null
let onPageHide: (() => void) | null = null
const clientId = Math.random().toString(36).slice(2) + Date.now().toString(36)
let seq = 0
/** Seconds really played per work and Rendition: the 10 s gate. */
const listened = new Map<string, number>()
/** Seconds played since the last write. */
let unsaved = 0
const pending = new Map<string, { rj: string, body: PutWorkListeningParams }>()
const acknowledged = new Map<string, string>()
const chains = new Map<string, Promise<void>>()
/** Works whose acknowledged writes the wall has already been told about. */
const announced = new Set<string>()
const keyOf = (rj: string, rendition: string) => `${rj}\n${rendition}`
const signature = (body: Snapshot) => JSON.stringify([body.track_key, body.position_sec, body.finished === true])

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
    /** The playing work's tracks, in order; prev/next and the end step through it. */
    queue: [] as QueueItem[],
    /** 0–1, kept across tracks and visits; muting leaves it as it was. */
    ...readVolume(),
    /**
     * The last server-acknowledged Listening Position write, for the cache
     * sync: the row the server now holds (null after a clear), and whether the
     * work's wall state may have changed with it.
     */
    saved: null as { rj: string, row: WorkListening | null, lists: boolean } | null,
  }),
  getters: {
    trackKey: (state): string | null => (state.rj !== null
      ? (state.queue[state.trackIndex]?.key ?? null)
      : null),
    hasPrev: state => state.rj !== null && state.trackIndex > 0,
    hasNext: state => state.rj !== null && state.trackIndex < state.queue.length - 1,
  },
  actions: {
    /**
     * Start a work at one of its tracks. Called synchronously from the click,
     * so the browser treats the element's play() as user-initiated.
     */
    play(rj: string, queue: QueueItem[], index = 0, resumeAt = 0) {
      this.flush()
      this.rj = rj
      this.queue = queue
      this.start(index, resumeAt)
    },
    /** Prev / next within the queue; past either end nothing changes. */
    step(delta: number) {
      const index = this.trackIndex + delta
      if (this.rj === null || index < 0 || index >= this.queue.length)
        return
      this.flush()
      this.start(index)
    },
    /** With a source the file plays (from `resumeAt` once playable); without one the silent clock runs. */
    start(index: number, resumeAt = 0) {
      const source = this.queue[index]?.source
      this.title = this.queue[index]?.title ?? ''
      this.trackIndex = index
      this.elapsed = resumeAt
      this.paused = false
      this.stopClock()
      this.release()
      this.duration = source?.durationSec ?? null
      this.format = source?.format ?? null
      if (source)
        this.attach(source.src, resumeAt)
      else this.runClock()
    },
    /** Space pauses and resumes; it never changes what plays. */
    togglePause() {
      if (this.rj === null)
        return
      this.paused = !this.paused
      // The element's own events confirm (or correct) the state set here.
      if (audio) {
        if (this.paused)
          audio.pause()
        else this.resume(audio)
        return
      }
      this.paused
        ? this.stopClock()
        : this.runClock()
    },
    /** Jump within the playing track (a lyric line click). */
    seek(seconds: number) {
      if (audio)
        audio.currentTime = seconds
      this.elapsed = seconds
    },
    setVolume(volume: number) {
      this.volume = Math.min(1, Math.max(0, volume))
      // Turning the volume up is the way out of mute.
      if (this.volume > 0)
        this.muted = false
      this.applyVolume()
    },
    toggleMute() {
      this.muted = !this.muted
      this.applyVolume()
    },
    applyVolume() {
      if (audio) {
        audio.volume = this.volume
        audio.muted = this.muted
      }
      try {
        localStorage.setItem(VOLUME_KEY, JSON.stringify({ volume: this.volume, muted: this.muted }))
      }
      catch {
        /* storage unavailable: the volume still applies for this visit */
      }
    },
    /** One element per play: every handler checks it is still the current one. */
    attach(src: string, resumeAt = 0) {
      const el = new Audio()
      audio = el
      el.volume = this.volume
      el.muted = this.muted
      const current = () => el === audio
      let last = 0
      if (resumeAt > 0) {
        el.addEventListener('canplay', () => {
          if (current())
            el.currentTime = resumeAt
        }, { once: true })
      }
      el.addEventListener('timeupdate', () => {
        if (!current())
          return
        this.elapsed = el.currentTime
        // A jump (seek, resume) is not playback; only steady progress counts.
        const dt = el.currentTime - last
        last = el.currentTime
        if (dt > 0 && dt <= 2 && !this.paused)
          this.tick(dt)
      })
      el.addEventListener('durationchange', () => {
        if (current() && Number.isFinite(el.duration))
          this.duration = el.duration
      })
      el.addEventListener('play', () => {
        if (current())
          this.paused = false
      })
      el.addEventListener('ended', () => {
        if (!current())
          return
        if (this.hasNext) {
          this.step(1)
        }
        else {
          this.paused = true
          // Finished is the Rendition's last track ending, not the queue's.
          const item = this.queue[this.trackIndex]
          this.flush(item?.index !== undefined && item.index === (item.count ?? 0) - 1)
        }
      })
      el.addEventListener('pause', () => {
        if (!current())
          return
        this.paused = true
        // The pause that precedes `ended` is not a stopping place.
        if (!el.ended)
          this.flush()
      })
      el.addEventListener('error', () => {
        if (current())
          this.paused = true
      })
      el.src = src
      this.resume(el)
    },
    resume(el: HTMLAudioElement) {
      // A superseded element's play() rejects with AbortError: not this track's
      // failure. Anything else (blocked autoplay, a 401/404 source) pauses —
      // never falling back to the silent clock, which would pretend audio exists.
      Promise.resolve(el.play()).catch((err: unknown) => {
        if (el === audio && !(err instanceof DOMException && err.name === 'AbortError'))
          this.paused = true
      })
    },
    release() {
      if (!audio)
        return
      const el = audio
      audio = null
      el.pause()
      el.removeAttribute('src')
      el.load()
    },
    /** Binds the API client the Listening Position is written through; a fresh binding is a clean session. */
    bindClient(client: ApiClientContract) {
      api = client
      for (const state of [
        listened,
        pending,
        acknowledged,
        chains,
        announced,
      ]) state.clear()
      unsaved = 0
      if (onPageHide)
        window.removeEventListener('pagehide', onPageHide)
      onPageHide = () => this.pageHide()
      window.addEventListener('pagehide', onPageHide)
    },
    /** Real playback of the current track: the first write waits for the gate, then one per interval. */
    tick(dt: number) {
      const item = this.queue[this.trackIndex]
      if (this.rj === null || !item?.source || item.rendition === undefined)
        return
      const key = keyOf(this.rj, item.rendition)
      const before = listened.get(key) ?? 0
      listened.set(key, before + dt)
      if (before + dt < GATE_SEC)
        return
      unsaved += dt
      if (before < GATE_SEC || unsaved >= WRITE_EVERY_SEC)
        this.flush()
    },
    /** Writes the current position (or the Rendition's end) once the gate is open; silent plays never write. */
    flush(finished = false, keepalive = false) {
      const item = this.queue[this.trackIndex]
      if (!audio || this.rj === null || !item?.source || item.key === undefined)
        return
      if (item.rendition === undefined || item.index === undefined || item.count === undefined)
        return
      // Reaching the end is never noise: a resume with seconds left must still finish.
      if (!finished && (listened.get(keyOf(this.rj, item.rendition)) ?? 0) < GATE_SEC)
        return
      // The Rendition's end was reported as Finished; a later flush (work change,
      // pagehide) must not write that end back as a position to resume from.
      if (!finished && audio.ended && item.index === item.count - 1)
        return
      unsaved = 0
      this.save(this.rj, {
        rendition: item.rendition,
        track_key: item.key,
        position_sec: audio.currentTime,
        track_index: item.index,
        track_count: item.count,
        ...(finished
          ? { finished: true }
          : {}),
      }, keepalive)
    },
    /**
     * Records a snapshot as pending and sends it. A snapshot the server already
     * holds is dropped; a newer one replaces a pending position, never a pending Finished.
     */
    save(rj: string, snapshot: Snapshot, keepalive = false) {
      const key = keyOf(rj, snapshot.rendition)
      if (!api)
        return
      if (acknowledged.get(key) === signature(snapshot)) {
        pending.delete(key)
      }
      else if (!(pending.get(key)?.body.finished && !snapshot.finished)) {
        pending.set(key, {
          rj,
          body: { ...snapshot, client: clientId, seq: ++seq },
        })
      }
      // Sending also retries whatever else of this work failed before.
      if (!keepalive)
        void this.enqueue(rj, () => this.send(rj))
    },
    /** One work's requests run one after another. */
    enqueue(rj: string, run: () => Promise<void>): Promise<void> {
      const next = (chains.get(rj) ?? Promise.resolve()).then(run)
      chains.set(rj, next.catch(() => {}))
      return next
    },
    /** Sends the work's pending snapshots; a failed one stays pending for the next write. */
    async send(rj: string) {
      for (const [key, entry] of [...pending]) {
        if (entry.rj !== rj || pending.get(key) !== entry)
          continue
        let row: WorkListening | undefined
        try {
          row = await api?.putWorkListening(rj, entry.body)
        }
        catch {
          continue
        }
        // A snapshot replaced or cleared meanwhile is no longer this one's to acknowledge.
        if (pending.get(key) !== entry)
          continue
        pending.delete(key)
        acknowledged.set(key, signature(entry.body))
        // Only a work's first write here and a Finished can move it on the wall.
        if (row)
          this.saved = { rj, row, lists: !announced.has(rj) || entry.body.finished === true }
        announced.add(rj)
      }
    },
    /** The page is going away: flush now and resend everything pending, past the serial chain. */
    pageHide() {
      this.flush(false, true)
      for (const { rj, body } of pending.values()) void api?.putWorkListening(rj, body, true).catch(() => {})
    },
    /**
     * Forgets the work's Listening Position: pending writes are dropped, the
     * DELETE waits behind any request in flight, and the 10 s gate starts over.
     */
    async clearListening(rj: string) {
      const prefix = `${rj}\n`
      for (const state of [pending, acknowledged, listened]) {
        for (const key of [
          ...state.keys(),
        ]) {
          if (key.startsWith(prefix))
            state.delete(key)
        }
      }
      unsaved = 0
      await this.enqueue(rj, async () => {
        await api?.deleteWorkListening(rj)
      })
      announced.delete(rj)
      this.saved = { rj, row: null, lists: true }
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
