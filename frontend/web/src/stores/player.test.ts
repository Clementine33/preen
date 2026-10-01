import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePlayerStore } from './player'

/**
 * The silent path's state machine (ADR 0012 §4): the clock only runs while
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

    store.play({ rj: 'RJ01113012', title: '双子の妹たちの夜伽メルヘン' })
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
    store.play({ rj: 'RJ01113012', title: 'A' })
    vi.advanceTimersByTime(2_000)
    store.play({ rj: 'RJ01098411', title: 'B' })

    expect(store.rj).toBe('RJ01098411')
    expect(store.title).toBe('B')
    expect(store.elapsed).toBe(0)

    vi.advanceTimersByTime(2_000)
    expect(store.elapsed).toBe(2) // one clock: no double ticking after a replay
  })

  it('keeps the now-playing work while pausing and resuming', () => {
    const store = usePlayerStore()
    store.play({ rj: 'RJ01113012', title: 'A' })
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
    store.play(work, 0, source)
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
    expect(store.paused).toBe(true)
  })

  it('seeks the element', () => {
    const store = usePlayerStore()
    store.play(work, 0, source)
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
    store.play(work, 0, source)
    const first = FakeAudio.made[0]
    store.play(work, 1, { ...source, src: '/content/b.mp3' })
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
    store.play(work, 0, source)
    await vi.waitFor(() => expect(store.paused).toBe(true))
  })

  it('a silent play releases the element and runs the clock', () => {
    const store = usePlayerStore()
    store.play(work, 0, source)
    store.play({ rj: 'RJ01113012', title: 'wall' })
    expect(FakeAudio.made[0].src).toBe('')
    expect(store.duration).toBeNull()
    expect(store.format).toBeNull()
    vi.advanceTimersByTime(2_000)
    expect(store.elapsed).toBe(2)
  })
})
