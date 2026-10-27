import type { InjectionKey } from 'vue'
import { inject } from 'vue'

/** What the Lyrics Window shows: a serializable snapshot the main window pushes, so a separate-realm desktop window can carry the same contract. */
export interface LyricsFrame {
  rj: string | null
  title: string
  paused: boolean
  hasPrev: boolean
  hasNext: boolean
  /**
   * The line on show; `dim` keeps a finished WebVTT cue up, faded, until the
   * next one; `advance` marks a step to the next line of the same track (the
   * only change that scrolls).
   */
  line: { key: string, text: string, dim?: boolean, advance?: boolean } | null
  /** The playing track has no Timed Lyrics. */
  noLyrics: boolean
}

export type LyricsCommand = 'prev' | 'toggle' | 'next'

export interface DesktopAdapter {
  platform: 'browser' | 'desktop'
  /** Whether this host can show a Lyrics Window; the entry is hidden otherwise. */
  lyricsWindow: boolean
  /**
   * Opens the Lyrics Window, reading `frame()` reactively and reporting
   * commands; `onClose` fires when it goes away. Must be called from a user
   * gesture. Resolves to a close function, or null when unsupported or declined.
   * `onLock` reports the window's click-through lock (desktop only), which
   * the window itself can no longer undo once set: unlock with `setLyricsLocked`.
   */
  openLyricsWindow: (
    frame: () => LyricsFrame,
    onCommand: (command: LyricsCommand) => void,
    onClose: () => void,
    onLock?: (locked: boolean) => void,
  ) => Promise<(() => void) | null>
  setLyricsLocked: (locked: boolean) => void
  resolveApi: () => Promise<{ baseUrl: string, token: string | null }>
  pickFolder: () => Promise<string | null>
}

export const desktopAdapterKey: InjectionKey<DesktopAdapter> = Symbol('desktop-adapter')

export function useDesktopAdapter(): DesktopAdapter {
  const adapter = inject(desktopAdapterKey)
  if (!adapter)
    throw new Error('DesktopAdapter was not provided')
  return adapter
}
