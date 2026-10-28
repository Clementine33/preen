import type { LyricsCommand, LyricsFrame } from './desktop-adapter'
import { Call, Events, System } from '@wailsio/runtime'
import { createApp, h, shallowRef } from 'vue'
import { LOCK_ICON } from '@/features/lyrics-window/lyrics-icons'
import LyricsWindowView from '@/features/lyrics-window/LyricsWindowView.vue'
import { LYRICS_EVENTS } from './wails-adapter'

// The desktop Lyrics Window's page (ADR 0016): main.ts routes /lyrics-window
// here before any application bootstrap. It mounts the shared view over the
// frames the main window pushes and sends commands back; closing and locking
// are host actions that go to the shell, not player commands. Unlocking is the
// shell's separate unlock chip (lyrics-unlock-entry.ts).

const CLOSE_ICON = 'M1 0l4 4 4-4 1 1-4 4 4 4-1 1-4-4-4 4-1-1 4-4-4-4z'
const BACKDROP_ICON = 'M5 0a5 5 0 1 0 0 10A5 5 0 0 0 5 0zm0 1a4 4 0 0 1 0 8z'

/** Windows only: whether the transparent window paints the dark backdrop anyway; this device's choice, transparent by default. */
export const LYRICS_BACKDROP_STORAGE_KEY = 'preen-lyrics-backdrop'
const CLEAR_CLASS = 'lw-clear'

function readDark(): boolean {
  try {
    return localStorage.getItem(LYRICS_BACKDROP_STORAGE_KEY) === 'dark'
  }
  catch {
    return false
  }
}

function toggleBackdrop(): void {
  const dark = !document.documentElement.classList.toggle(CLEAR_CLASS)
  // Best-effort: blocked storage still keeps the choice while the window is open.
  try {
    localStorage.setItem(LYRICS_BACKDROP_STORAGE_KEY, dark ? 'dark' : 'clear')
  }
  catch {
    /* storage unavailable */
  }
}

const blank: LyricsFrame = {
  rj: null,
  title: '',
  paused: true,
  hasPrev: false,
  hasNext: false,
  line: null,
  noLyrics: false,
}

function hostButton(label: string, path: string, onClick: () => void) {
  return h('button', {
    'type': 'button',
    'aria-label': label,
    'title': label,
    onClick,
  }, h('svg', {
    'viewBox': '0 0 10 10',
    'width': 10,
    'height': 10,
    'fill': 'currentColor',
    'aria-hidden': 'true',
  }, h('path', { 'd': path, 'fill-rule': 'evenodd' })))
}

export function mountLyricsWindow(): void {
  // Dark in every theme, like the Picture-in-Picture window; Windows paints nothing behind the lyrics unless the device chose the dark backdrop.
  document.documentElement.classList.remove('dark')
  const windows = System.IsWindows()
  if (windows && !readDark())
    document.documentElement.classList.add(CLEAR_CLASS)
  const frame = shallowRef(blank)
  const locked = shallowRef(false)
  Events.On(LYRICS_EVENTS.frame, (event) => { frame.value = JSON.parse(event.data as string) as LyricsFrame })
  Events.On(LYRICS_EVENTS.locked, (event) => { locked.value = event.data as boolean })
  const app = createApp({
    render: () => h(LyricsWindowView, {
      frame: frame.value,
      locked: locked.value,
      onCommand: (command: LyricsCommand) => void Events.Emit(LYRICS_EVENTS.command, command),
    }, {
      host: () => [
        windows
          ? hostButton('切换透明 / 深色背景', BACKDROP_ICON, toggleBackdrop)
          : null,
        windows
          ? hostButton('锁定歌词窗口', LOCK_ICON, () => void Call.ByName('main.Desktop.SetLyricsLocked', true))
          : null,
        hostButton('关闭歌词窗口', CLOSE_ICON, () => void Call.ByName('main.Desktop.CloseLyricsWindow')),
      ],
    }),
  })
  app.mount('#app')
  void Events.Emit(LYRICS_EVENTS.ready)
}
