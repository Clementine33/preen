import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClientContract } from '@/lib/api/types'
import { usePlayerStore, type QueueItem } from './player'

/**
 * The silent path's state machine: the clock only runs while
 * playing, and the now-playing work is the one thing Space can never touch.
 * What plays and what the wall has selected are two different states — that
 * decoupling is the store's whole contract with the page.
 */
describe('player store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('plays a work and advances the elapsed clock only while playing', () => {
    const store = usePlayerStore()
    expect(store.rj).toBeNull()

    store.play('RJ01113012', [{ title: '双子の妹たちの夜伽メルヘン' }])
    expect(store.rj).toBe('RJ01113012')
    expect(store.paused).toBe(false)

    vi.advanceTimersByTime(3_000)
    expect(store.elapsed).toBe(3)

    store.togglePause()
    vi.advanceTimersByTime(3_000)
    expect(store.elapsed).toBe(3) // the clock stops while paused

    store.togglePause()
    vi.advanceTimersByTime(1_000)
    expect(store.elapsed).toBe(4)
  })

  it('ignores pause and resume while nothing plays', () => {
    const store = usePlayerStore()
    store.togglePause()
    expect(store.paused).toBe(true)
    expect(store.rj).toBeNull()
    vi.advanceTimersByTime(5_000)
    expect(store.elapsed).toBe(0)
  })

  it('replaces the now-playing work with a fresh clock, never a second one', () => {
    const store = usePlayerStore()
    store.play('RJ01113012', [{ title: 'A' }])
    vi.advanceTimersByTime(2_000)
    store.play('RJ01098411', [{ title: 'B' }])

    expect(store.rj).toBe('RJ01098411')
    expect(store.title).toBe('B')
    expect(store.elapsed).toBe(0)

    vi.advanceTimersByTime(2_000)
    expect(store.elapsed).toBe(2) // one clock: no double ticking after a replay
  })

  it('keeps the now-playing work while pausing and resuming', () => {
    const store = usePlayerStore()
    store.play('RJ01113012', [{ title: 'A' }])
    store.togglePause()
    expect(store.paused).toBe(true)
    expect(store.rj).toBe('RJ01113012')
    store.togglePause()
    expect(store.paused).toBe(false)
    expect(store.rj).toBe('RJ01113012')
  })
})

/** A stand-in element: events are dispatched by hand, play() is controllable. */
class FakeAudio extends EventTarget {
  static made: FakeAudio[] = []
  src = ''
  currentTime = 0
  duration = Number.NaN
  paused = true
  volume = 1
  muted = false
  constructor() {
    super()
    FakeAudio.made.push(this)
  }
  play(): Promise<void> {
    this.paused = false
    this.dispatchEvent(new Event('play'))
    return Promise.resolve()
  }
  pause(): void {
    this.paused = true
    this.dispatchEvent(new Event('pause'))
  }
  load(): void {}
  removeAttribute(): void {
    this.src = ''
  }
}

describe('player store, real track', () => {
  const work = { rj: 'RJ01000001', title: '01_導入' }
  const source = { src: '/content/a.mp3', durationSec: 252, format: 'MP3' }

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    FakeAudio.made = []
    vi.stubGlobal('Audio', FakeAudio)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('follows the element: position, length, pause, end', () => {
    const store = usePlayerStore()
    store.play(work.rj, [{ title: work.title, source }])
    const el = FakeAudio.made[0]
    expect(el.src).toBe('/content/a.mp3')
    expect(store.duration).toBe(252) // the scan's fact until the element knows
    expect(store.format).toBe('MP3')

    el.currentTime = 12.5
    el.dispatchEvent(new Event('timeupdate'))
    el.duration = 251.9
    el.dispatchEvent(new Event('durationchange'))
    expect(store.elapsed).toBe(12.5)
    expect(store.duration).toBe(251.9)

    // The silent clock never runs beside the element.
    vi.advanceTimersByTime(3_000)
    expect(store.elapsed).toBe(12.5)

    store.togglePause()
    expect(store.paused).toBe(true)
    store.togglePause()
    expect(store.paused).toBe(false)
    el.dispatchEvent(new Event('ended'))
    expect(store.paused).toBe(true) // the last track of its queue
  })

  it('steps through the queue and starts the next track when one ends', () => {
    const store = usePlayerStore()
    const queue = ['a', 'b', 'c'].map((name) => ({ title: name, source: { ...source, src: `/content/${name}.mp3` } }))
    store.play(work.rj, queue)
    expect([store.hasPrev, store.hasNext]).toEqual([false, true])

    store.step(-1) // nothing before the first
    expect(FakeAudio.made).toHaveLength(1)
    FakeAudio.made[0].dispatchEvent(new Event('ended'))
    expect(store.trackIndex).toBe(1)
    expect(store.title).toBe('b')
    expect(FakeAudio.made[1].src).toBe('/content/b.mp3')
    expect(store.paused).toBe(false)

    store.step(1)
    expect([store.hasPrev, store.hasNext]).toEqual([true, false])
    store.step(1) // nothing after the last
    expect(store.trackIndex).toBe(2)
    store.step(-2)
    expect(store.title).toBe('a')
    expect(FakeAudio.made.at(-1)?.src).toBe('/content/a.mp3')
  })

  it('keeps the volume across tracks and visits, and mutes without losing it', () => {
    localStorage.clear()
    const store = usePlayerStore()
    expect([store.volume, store.muted]).toEqual([1, false])
    store.play(work.rj, [{ title: 'a', source }, { title: 'b', source }])
    store.setVolume(0.4)
    store.toggleMute()
    const first = FakeAudio.made[0]
    expect([first.volume, first.muted]).toEqual([0.4, true])

    store.step(1) // the next element starts at the same level
    const second = FakeAudio.made[1]
    expect([second.volume, second.muted]).toEqual([0.4, true])
    store.setVolume(0.6) // raising the fader unmutes
    expect(second.muted).toBe(false)

    setActivePinia(createPinia())
    expect([usePlayerStore().volume, usePlayerStore().muted]).toEqual([0.6, false])
    localStorage.clear()
  })

  it('seeks the element', () => {
    const store = usePlayerStore()
    store.play(work.rj, [{ title: work.title, source }])
    store.seek(60)
    expect(FakeAudio.made[0].currentTime).toBe(60)
    expect(store.elapsed).toBe(60)
  })

  it('ignores the superseded track, and pauses on a real failure', async () => {
    const store = usePlayerStore()
    // The first play() settles only after the second track started.
    let abort: (err: unknown) => void = () => {}
    vi.spyOn(FakeAudio.prototype, 'play').mockImplementationOnce(function (this: FakeAudio) {
      this.paused = false
      return new Promise<void>((_resolve, reject) => {
        abort = reject
      })
    })
    store.play(work.rj, [{ title: work.title, source }])
    const first = FakeAudio.made[0]
    store.play(work.rj, [{ title: 'a', source }, { title: 'b', source: { ...source, src: '/content/b.mp3' } }], 1)
    abort(new DOMException('replaced', 'AbortError'))
    // Late events from the released element change nothing either.
    first.dispatchEvent(new Event('error'))
    await vi.waitFor(() => Promise.resolve())
    expect(store.paused).toBe(false)
    expect(store.trackIndex).toBe(1)

    const second = FakeAudio.made[1]
    second.dispatchEvent(new Event('error'))
    expect(store.paused).toBe(true)
  })

  it('pauses when the browser refuses to play', async () => {
    const store = usePlayerStore()
    const refused = Promise.reject(new DOMException('blocked', 'NotAllowedError'))
    refused.catch(() => {})
    vi.spyOn(FakeAudio.prototype, 'play').mockReturnValue(refused)
    store.play(work.rj, [{ title: work.title, source }])
    await vi.waitFor(() => expect(store.paused).toBe(true))
  })

  it('a silent play releases the element and runs the clock', () => {
    const store = usePlayerStore()
    store.play(work.rj, [{ title: work.title, source }])
    store.play('RJ01113012', [{ title: 'wall' }])
    expect(FakeAudio.made[0].src).toBe('')
    expect(store.duration).toBeNull()
    expect(store.format).toBeNull()
    vi.advanceTimersByTime(2_000)
    expect(store.elapsed).toBe(2)
  })
})

/**
 * The Listening Position the store writes: the 10 s gate, the 15 s throttle, the
 * flush points, the per-work serial chain and the pending/acknowledged
 * bookkeeping. The element is driven by hand, one second per timeupdate.
 */
describe('player store, Listening Position', () => {
  const RJ = 'RJ01000001'
  const source = { src: '/content/a.mp3', durationSec: 252, format: 'MP3' }
  const tracks = (count: number, shown = count): QueueItem[] =>
    Array.from({ length: shown }, (_, index) => ({
      key: `t${index}`, title: `t${index}`, rendition: '', index, count, source,
    }))
  let put: ReturnType<typeof vi.fn>
  let del: ReturnType<typeof vi.fn>

  /** Plays `seconds` of steady progress on the current element. */
  function playFor(seconds: number, from = FakeAudio.made.at(-1)!.currentTime) {
    const el = FakeAudio.made.at(-1)!
    for (let t = from + 1; t <= from + seconds; t++) {
      el.currentTime = t
      el.dispatchEvent(new Event('timeupdate'))
    }
    return el
  }
  const bodies = () => put.mock.calls.map(([, body]) => body)

  beforeEach(() => {
    vi.restoreAllMocks() // the element tests above leave play() spied
    setActivePinia(createPinia())
    FakeAudio.made = []
    vi.stubGlobal('Audio', FakeAudio)
    put = vi.fn().mockResolvedValue({})
    del = vi.fn().mockResolvedValue(undefined)
    usePlayerStore().bindClient({ putWorkListening: put, deleteWorkListening: del } as unknown as ApiClientContract)
  })
  afterEach(async () => {
    await flushPromises() // a request still in flight must not leak into the next test
    vi.unstubAllGlobals()
  })

  it('writes nothing before 10 s of real playback, and a seek does not count', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(3))
    const el = playFor(9)
    el.currentTime = 200 // a seek: a jump, not playback
    el.dispatchEvent(new Event('timeupdate'))
    store.togglePause() // a pause flush inside the gate writes nothing either
    await flushPromises()
    expect(put).not.toHaveBeenCalled()

    store.togglePause()
    playFor(1, 200)
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0]![0]).toBe(RJ)
    expect(bodies()[0]).toMatchObject({
      rendition: '', track_key: 't0', position_sec: 201, track_index: 0, track_count: 3,
    })
    expect(typeof bodies()[0].client).toBe('string')
  })

  it('writes every 15 s while playing, each snapshot with a higher seq', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(3))
    playFor(10)
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    playFor(14, 10)
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    playFor(1, 24)
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(2)
    expect(bodies()[1].seq).toBeGreaterThan(bodies()[0].seq)
    expect(bodies()[1].client).toBe(bodies()[0].client)
  })

  it('flushes on pause, track change and work change', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(3))
    playFor(10)
    await flushPromises()
    playFor(3, 10)
    store.togglePause()
    await flushPromises()
    expect(bodies().map((b) => [b.track_key, b.position_sec])).toEqual([['t0', 10], ['t0', 13]])

    store.togglePause()
    playFor(1, 13)
    store.step(1)
    await flushPromises()
    expect(bodies().at(-1)).toMatchObject({ track_key: 't0', position_sec: 14 })

    playFor(1, 0)
    store.play('RJ01000002', tracks(2))
    await flushPromises()
    expect(put.mock.calls.at(-1)![0]).toBe(RJ)
    expect(bodies().at(-1)).toMatchObject({ track_key: 't1', position_sec: 1 })
  })

  it('never writes for a silent play', async () => {
    const store = usePlayerStore()
    vi.useFakeTimers()
    store.play(RJ, [{ title: 'wall' }])
    store.play(RJ, [{ key: 'a', title: 'a', rendition: '', index: 0, count: 1 }]) // no file behind it
    vi.advanceTimersByTime(60_000)
    store.togglePause()
    vi.useRealTimers()
    await flushPromises()
    expect(put).not.toHaveBeenCalled()
  })

  it('resends a failed save on the next flush, and on pagehide', async () => {
    const store = usePlayerStore()
    put.mockRejectedValueOnce(new Error('offline'))
    store.play(RJ, tracks(3))
    playFor(12)
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    store.togglePause() // the pause flush sends the newer snapshot
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(2)
    expect(bodies()[1]).toMatchObject({ position_sec: 12 })

    // Everything fails now: pagehide still sends the pending snapshot, kept alive.
    put.mockRejectedValue(new Error('offline'))
    store.togglePause()
    playFor(1, 12)
    store.togglePause()
    await flushPromises()
    window.dispatchEvent(new Event('pagehide'))
    const last = put.mock.calls.at(-1)!
    expect(last[0]).toBe(RJ)
    expect(last[2]).toBe(true)
    expect(last[1]).toMatchObject({ position_sec: 13 })
  })

  it('does not resend a snapshot the server already acknowledged', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(3))
    playFor(10)
    await flushPromises()
    store.togglePause() // same position as the acknowledged write
    store.togglePause()
    store.togglePause()
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
  })

  it('keeps a pending Finished when a position follows', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(1))
    const el = playFor(10)
    await flushPromises()
    put.mockRejectedValueOnce(new Error('offline'))
    el.dispatchEvent(new Event('ended')) // Finished, but the save fails
    await flushPromises()
    expect(bodies().at(-1)).toMatchObject({ finished: true })

    store.togglePause()
    playFor(1, 10)
    store.togglePause() // a position snapshot while the Finished is pending
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(3)
    expect(bodies()[2]).toMatchObject({ finished: true, seq: bodies()[1].seq })
  })

  it('finishes only when the Rendition’s last track ends', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(3))
    playFor(10)
    FakeAudio.made.at(-1)!.dispatchEvent(new Event('ended')) // t0 -> t1
    FakeAudio.made.at(-1)!.dispatchEvent(new Event('ended')) // t1 -> t2
    await flushPromises()
    expect(bodies().some((b) => b.finished)).toBe(false)
    FakeAudio.made.at(-1)!.dispatchEvent(new Event('ended')) // t2 is the last
    await flushPromises()
    expect(bodies().at(-1)).toMatchObject({ track_key: 't2', track_index: 2, track_count: 3, finished: true })

    // A queue cut short (the format is missing from the last track) is not finished.
    put.mockClear()
    store.play(RJ, tracks(3, 2))
    playFor(10)
    FakeAudio.made.at(-1)!.dispatchEvent(new Event('ended'))
    FakeAudio.made.at(-1)!.dispatchEvent(new Event('ended'))
    await flushPromises()
    expect(bodies().some((b) => b.finished)).toBe(false)
  })

  it('finishes past the gate when a resume ends the last track within 10 s', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(1))
    playFor(3)
    FakeAudio.made.at(-1)!.dispatchEvent(new Event('ended'))
    await flushPromises()
    expect(bodies()).toHaveLength(1)
    expect(bodies()[0]).toMatchObject({ track_key: 't0', finished: true })
  })

  it('writes no position after the Rendition finished, on a work change or pagehide', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(1))
    const el = playFor(10)
    await flushPromises()
    Object.assign(el, { ended: true, paused: true })
    el.dispatchEvent(new Event('ended'))
    await flushPromises()
    expect(bodies().at(-1)).toMatchObject({ finished: true })
    const sent = put.mock.calls.length

    window.dispatchEvent(new Event('pagehide'))
    store.play('RJ01000002', tracks(1)) // 继续 / another work flushes the ended element first
    await flushPromises()
    expect(put.mock.calls.length).toBe(sent)
  })

  it('tells the wall only about a first write, a Finished and a clear', async () => {
    const store = usePlayerStore()
    store.play(RJ, tracks(1))
    const el = playFor(10)
    await flushPromises()
    expect(store.saved).toMatchObject({ rj: RJ, lists: true })
    playFor(15, 10)
    await flushPromises()
    expect(store.saved).toMatchObject({ rj: RJ, lists: false })
    el.dispatchEvent(new Event('ended'))
    await flushPromises()
    expect(store.saved).toMatchObject({ rj: RJ, lists: true })
    store.saved = null
    await store.clearListening(RJ)
    expect(store.saved).toMatchObject({ rj: RJ, lists: true })
  })

  it('clears behind an in-flight write, drops what is pending and restarts the gate', async () => {
    const store = usePlayerStore()
    let settle: () => void = () => {}
    put.mockImplementationOnce(() => new Promise<void>((resolve) => { settle = resolve }))
    store.play(RJ, tracks(3))
    playFor(10)
    await flushPromises() // the first write is in flight
    const cleared = store.clearListening(RJ)
    await flushPromises()
    expect(del).not.toHaveBeenCalled() // waits for the request in flight
    settle()
    await cleared
    expect(del).toHaveBeenCalledWith(RJ)
    expect(put.mock.invocationCallOrder[0]).toBeLessThan(del.mock.invocationCallOrder[0]!)

    // Playing on records nothing until 10 more seconds have really played.
    playFor(9, 10)
    store.togglePause()
    store.togglePause()
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    playFor(1, 19)
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(2)
  })
})
