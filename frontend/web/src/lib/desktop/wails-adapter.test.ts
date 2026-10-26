import { afterEach, expect, it, vi } from 'vitest'
import { createDesktopAdapter } from './host-adapter'

vi.mock('@wailsio/runtime', () => ({
  Call: { ByName: vi.fn(async () => null) },
}))

afterEach(() => vi.unstubAllGlobals())

it('selects the desktop host, preserves folder cancellation and hides the Lyrics Window', async () => {
  vi.stubGlobal('_wails', {})
  const adapter = await createDesktopAdapter()
  expect(adapter.platform).toBe('desktop')
  expect(await adapter.pickFolder()).toBeNull()
  expect(adapter.lyricsWindow).toBe(false)
  expect(await adapter.openLyricsWindow(() => { throw new Error('unsupported') }, () => {}, () => {})).toBeNull()
})
