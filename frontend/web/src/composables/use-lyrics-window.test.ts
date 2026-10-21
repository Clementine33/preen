import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { apiClientKey } from '@/lib/api/client'
import { desktopAdapterKey, type LyricsFrame } from '@/lib/desktop/desktop-adapter'
import { usePlayerStore } from '@/stores/player'
import { createLyricsWindow } from './use-lyrics-window'

beforeEach(() => setActivePinia(createPinia()))
afterEach(() => usePlayerStore().stopClock())

function setup() {
  const close = vi.fn()
  let frame!: () => LyricsFrame
  let onClose!: () => void
  const adapter = {
    lyricsWindow: true,
    openLyricsWindow: vi.fn(async (f: () => LyricsFrame, _c: unknown, c: () => void) => ((frame = f), (onClose = c), close)),
  }
  const api = { getWorkText: vi.fn().mockResolvedValue('[00:01]first\n[00:05]second') }
  let lw!: ReturnType<typeof createLyricsWindow>
  mount(defineComponent({ setup: () => ((lw = createLyricsWindow()), () => null) }), {
    global: { provide: { [desktopAdapterKey as symbol]: adapter, [apiClientKey as symbol]: api } },
  })
  return { adapter, close, lw, frame: () => frame(), closed: () => onClose() }
}

it('is unavailable until the playing track has Timed Lyrics, then follows the player', async () => {
  const { adapter, close, lw, frame, closed } = setup()
  const player = usePlayerStore()
  lw.toggle()
  expect(adapter.openLyricsWindow).not.toHaveBeenCalled()
  player.play('RJ1', [{ title: 'T', lyrics: { copyId: 'c', relPath: 'a.lrc' } }, { title: 'U' }])
  await flushPromises()
  expect(lw.available.value).toBe(true)
  lw.toggle()
  await flushPromises()
  expect(lw.open.value).toBe(true)
  expect(frame().line).toBeNull()
  player.elapsed = 6
  expect(frame().line).toMatchObject({ text: 'second', dim: false })
  // The window stays for a track without lyrics and says so.
  player.step(1)
  expect(frame()).toMatchObject({ title: 'U', line: null, noLyrics: true, hasPrev: true })
  lw.toggle()
  expect(close).toHaveBeenCalled()
  closed()
  expect(lw.open.value).toBe(false)
})

it('marks only a step to the next line as an advance', async () => {
  const { lw, frame } = setup()
  const player = usePlayerStore()
  player.play('RJ1', [{ title: 'T', lyrics: { copyId: 'c', relPath: 'a.lrc' } }, { title: 'U' }])
  await flushPromises()
  lw.toggle()
  await flushPromises()
  expect(frame().line).toBeNull() // title first, so line 0 is an advance
  player.elapsed = 1
  expect(frame().line?.advance).toBe(true)
  player.elapsed = 5
  expect(frame().line?.advance).toBe(true)
  expect(frame().line?.advance).toBe(true)
  player.elapsed = 1
  expect(frame().line?.advance).toBe(false)
  player.step(1)
  expect(frame().line).toBeNull()
})
