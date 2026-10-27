import { afterEach, beforeEach, expect } from 'vitest'

// Unexpected diagnostics must fail the test even when a library catches the error.
const diagnostics: unknown[][] = []
for (const level of ['warn', 'error'] as const) {
  const original = console[level].bind(console)
  console[level] = (...args: unknown[]) => {
    diagnostics.push([level, ...args])
    original(...args)
  }
}
beforeEach(() => { diagnostics.length = 0 })
afterEach(() => { expect(diagnostics, 'Unexpected console warnings/errors').toEqual([]) })

// Minimal jsdom polyfills for the test environment.
// Vitest's jsdom population does not expose localStorage (jsdom implements
// it on its own realm and the global copy misses it), and Node's built-in
// experimental localStorage is unavailable without --localstorage-file.
// Provide a deterministic in-memory Storage for all tests.
const store = new Map<string, string>()

const memoryStorage: Storage = {
  get length() {
    return store.size
  },
  clear() {
    store.clear()
  },
  getItem(key: string) {
    return store.has(key)
      ? store.get(key)!
      : null
  },
  key(index: number) {
    return [
      ...store.keys(),
    ][index] ?? null
  },
  removeItem(key: string) {
    store.delete(key)
  },
  setItem(key: string, value: string) {
    store.set(String(key), String(value))
  },
}

Object.defineProperty(globalThis, 'localStorage', {
  value: memoryStorage,
  configurable: true,
})

// jsdom does not implement window.matchMedia; provide a no-op stub so
// theme composables that consult the system preference can run in tests.
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  }),
})
// jsdom implements no media playback: play() returns nothing and logs "not
// implemented". Inert stubs keep the player's real path quiet in tests that
// only care about state; player tests substitute their own Audio.
for (const [name, fn] of Object.entries({
  play: (): Promise<void> => Promise.resolve(),
  pause: (): void => undefined,
  load: (): void => undefined,
})) {
  Object.defineProperty(HTMLMediaElement.prototype, name, { configurable: true, writable: true, value: fn })
}
