import { effectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// The state is module-level, so each case imports a fresh module.
async function load() {
  vi.resetModules()
  return import('./use-motion')
}

function systemReduces(matches: boolean) {
  vi.spyOn(window, 'matchMedia').mockImplementation((query: string) => ({
    matches: matches && query === '(prefers-reduced-motion: reduce)',
    media: query, onchange: null, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(),
  }) as MediaQueryList)
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('reduce-motion')
})
afterEach(() => vi.restoreAllMocks())

describe('useMotion', () => {
  it('falls back to following the system on a missing or invalid stored value', async () => {
    localStorage.setItem('preen-motion', 'full')
    const { useMotion } = await load()
    expect(useMotion().preference.value).toBe('system')
    expect(useMotion().reduced.value).toBe(false)
  })

  it('cannot override a system that asks for reduced motion', async () => {
    systemReduces(true)
    const { useMotion } = await load()
    const motion = useMotion()
    expect(motion.preference.value).toBe('system')
    expect(motion.reduced.value).toBe(true)
    expect(document.documentElement.classList.contains('reduce-motion')).toBe(true)
  })

  it('keeps working for the session when storage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked') })
    const { useMotion } = await load()
    const motion = useMotion()
    motion.preference.value = 'reduce'
    await Promise.resolve()
    expect(motion.reduced.value).toBe(true)
  })
})

describe('useSwitchBurst', () => {
  async function burst() {
    const { useSwitchBurst } = await load()
    return effectScope().run(() => useSwitchBurst())!
  }
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('turns rapid on the third switch', async () => {
    const { rapid, note } = await burst()
    note(); note()
    expect(rapid.value).toBe(false)
    note()
    expect(rapid.value).toBe(true)
  })

  it('turns rapid off 800ms after the last switch', async () => {
    const { rapid, note } = await burst()
    note(); note(); note()
    vi.advanceTimersByTime(799)
    expect(rapid.value).toBe(true)
    vi.advanceTimersByTime(1)
    expect(rapid.value).toBe(false)
  })

  it('restarts the count after a pause', async () => {
    const { rapid, note } = await burst()
    note(); note()
    vi.advanceTimersByTime(800)
    note()
    expect(rapid.value).toBe(false)
  })
})
