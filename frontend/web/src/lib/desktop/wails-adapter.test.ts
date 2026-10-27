import { afterEach, expect, it, vi } from 'vitest'
import { createDesktopAdapter } from './host-adapter'

vi.mock('@wailsio/runtime', () => ({
  Call: { ByName: vi.fn(async () => null) },
}))

afterEach(() => vi.unstubAllGlobals())

it('selects the browser host without a native message channel', async () => {
  expect((await createDesktopAdapter()).platform).toBe('browser')
})

it('selects the desktop host, preserves folder cancellation and hides the Lyrics Window', async () => {
  vi.stubGlobal('webkit', { messageHandlers: { external: { postMessage() {} } } })
  const adapter = await createDesktopAdapter()
  expect(adapter.platform).toBe('desktop')
  expect(await adapter.pickFolder()).toBeNull()
  expect(adapter.lyricsWindow).toBe(false)
  expect(await adapter.openLyricsWindow(() => { throw new Error('unsupported') }, () => {}, () => {})).toBeNull()
})
