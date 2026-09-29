import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePlayerStore } from './player'

/**
 * The silent player's state machine (ADR 0012 §4): the clock only runs while
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
