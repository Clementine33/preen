import { beforeEach, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { LYRICS_BACKDROP_STORAGE_KEY, mountLyricsWindow } from './lyrics-window-entry'

const windows = vi.fn(() => true)
vi.mock('@wailsio/runtime', () => ({
  Call: {
    ByName: vi.fn(async () => null),
  },
  Events: {
    On: vi.fn(() => () => {}),
    Emit: vi.fn(),
  },
  System: {
    IsWindows: () => windows(),
  },
}))

const root = document.documentElement
const backdrop = () => document.querySelector<HTMLButtonElement>('[aria-label="切换透明 / 深色背景"]')

async function mount() {
  document.body.innerHTML = '<div id="app"></div>'
  mountLyricsWindow()
  await nextTick()
}

beforeEach(() => {
  localStorage.clear()
  root.className = ''
  windows.mockReturnValue(true)
})

it('starts transparent on Windows and remembers the dark backdrop', async () => {
  await mount()
  expect(root.classList.contains('lw-clear')).toBe(true)
  backdrop()!.click()
  expect(root.classList.contains('lw-clear')).toBe(false)
  expect(localStorage.getItem(LYRICS_BACKDROP_STORAGE_KEY)).toBe('dark')

  root.className = ''
  await mount()
  expect(root.classList.contains('lw-clear')).toBe(false)
  backdrop()!.click()
  expect(root.classList.contains('lw-clear')).toBe(true)
  expect(localStorage.getItem(LYRICS_BACKDROP_STORAGE_KEY)).toBe('clear')
})

it('offers no backdrop switch off Windows', async () => {
  windows.mockReturnValue(false)
  await mount()
  expect(root.classList.contains('lw-clear')).toBe(false)
  expect(backdrop()).toBeNull()
})
