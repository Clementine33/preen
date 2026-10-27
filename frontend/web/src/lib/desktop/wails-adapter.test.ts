import type { LyricsFrame } from './desktop-adapter'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { createDesktopAdapter } from './host-adapter'
import { WailsDesktopAdapter } from './wails-adapter'

const handlers = new Map<string, (event: { data?: unknown }) => void>()
const emit = vi.fn()
const call = vi.fn(async (_name: string, ..._args: unknown[]): Promise<unknown> => null)
vi.mock('@wailsio/runtime', () => ({
  Call: {
    ByName: (name: string, ...args: unknown[]) => call(name, ...args),
  },
  Events: {
    On: (name: string, cb: (event: { data?: unknown }) => void) => (handlers.set(name, cb), () => handlers.delete(name)),
    Emit: (...args: unknown[]) => emit(...args),
  },
}))

afterEach(() => vi.unstubAllGlobals())

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
  emit.mockClear()
  call.mockClear()
})

const base: LyricsFrame = {
  rj: 'RJ1',
  title: 'T',
  paused: false,
  hasPrev: false,
  hasNext: false,
  line: null,
  noLyrics: false,
}

async function open() {
  const elapsed = ref(0)
  const frame = vi.fn(() => ({
    ...base,
    title: elapsed.value >= 100
      ? 'U'
      : 'T',
  }))
  const onCommand = vi.fn()
  const onClose = vi.fn()
  const onLock = vi.fn()
  const adapter = new WailsDesktopAdapter()
  const close = await adapter.openLyricsWindow(() => (elapsed.value, frame()), onCommand, onClose, onLock)
  return {
    adapter,
    close,
    elapsed,
    onCommand,
    onClose,
    onLock,
  }
}

it('pushes the frame only when its serialized value changes, and resends it when the window is ready', async () => {
  const { elapsed } = await open()
  elapsed.value = 1
  await nextTick()
  expect(emit).not.toHaveBeenCalled()
  elapsed.value = 100
  await nextTick()
  expect(emit).toHaveBeenCalledTimes(1)
  expect(JSON.parse(emit.mock.calls[0][1])).toMatchObject({ title: 'U' })
  handlers.get('lyrics:ready')!({})
  expect(emit).toHaveBeenCalledTimes(2)
  expect(JSON.parse(emit.mock.calls[1][1])).toMatchObject({ title: 'U' })
})

it('routes commands and lock changes, and reports the window closing', async () => {
  const { close, onCommand, onClose, onLock } = await open()
  expect(call).toHaveBeenCalledWith('main.Desktop.OpenLyricsWindow')
  handlers.get('lyrics:command')!({ data: 'next' })
  expect(onCommand).toHaveBeenCalledWith('next')
  handlers.get('lyrics:locked')!({ data: true })
  expect(onLock).toHaveBeenCalledWith(true)
  close!()
  expect(call).toHaveBeenCalledWith('main.Desktop.CloseLyricsWindow')
  handlers.get('lyrics:closed')!({})
  expect(onClose).toHaveBeenCalled()
  expect(handlers.size).toBe(0)
})

it('opens once at a time, can unlock, and gives up when the shell refuses', async () => {
  const { adapter, close } = await open()
  expect(await adapter.openLyricsWindow(() => base, vi.fn(), vi.fn())).toBe(close)
  adapter.setLyricsLocked(false)
  expect(call).toHaveBeenCalledWith('main.Desktop.SetLyricsLocked', false)
  handlers.get('lyrics:closed')!({})
  call.mockRejectedValueOnce(new Error('no'))
  expect(await adapter.openLyricsWindow(() => base, vi.fn(), vi.fn())).toBeNull()
  expect(handlers.size).toBe(0)
})

it('selects the browser host without a native message channel', async () => {
  expect((await createDesktopAdapter()).platform).toBe('browser')
})

it('selects the desktop host, preserves folder cancellation and offers the Lyrics Window', async () => {
  vi.stubGlobal('webkit', {
    messageHandlers: {
      external: {
        postMessage() {},
      },
    },
  })
  const adapter = await createDesktopAdapter()
  expect(adapter.platform).toBe('desktop')
  expect(await adapter.pickFolder()).toBeNull()
  expect(adapter.lyricsWindow).toBe(true)
})
