import type { DesktopAdapter, LyricsCommand, LyricsFrame } from './desktop-adapter'
import { Browser, Call, Events } from '@wailsio/runtime'
import { watch } from 'vue'

/** Event names of the Lyrics Window channel; Go interprets none of the frames and commands (ADR 0016). */
export const LYRICS_EVENTS = {
  frame: 'lyrics:frame',
  ready: 'lyrics:ready',
  command: 'lyrics:command',
  closed: 'lyrics:closed',
  locked: 'lyrics:locked',
} as const

/** Native bindings supply credentials, folder selection and the Lyrics Window; HTTP stays in ApiClient. */
export class WailsDesktopAdapter implements DesktopAdapter {
  readonly platform = 'desktop' as const
  readonly lyricsWindow = true
  private closeLyrics: (() => void) | null = null

  constructor() {
    // Wails webviews don't route new-window requests to the system browser, so do it here.
    document.addEventListener('click', (event) => {
      const link = (event.target as Element | null)?.closest<HTMLAnchorElement>('a[target="_blank"]')
      if (link && /^https?:/i.test(link.href)) {
        event.preventDefault()
        void Browser.OpenURL(link.href)
      }
    }, true)
    window.open = (url) => {
      void Browser.OpenURL(String(url))
      return null
    }
  }

  async resolveApi(): Promise<{ baseUrl: string, token: string }> {
    return Call.ByName('main.Desktop.ResolveApi')
  }

  async pickFolder(): Promise<string | null> {
    return await Call.ByName('main.Desktop.PickFolder') || null
  }

  async openLyricsWindow(
    frame: () => LyricsFrame,
    onCommand: (command: LyricsCommand) => void,
    onClose: () => void,
  ): Promise<(() => void) | null> {
    if (this.closeLyrics)
      return this.closeLyrics
    // frame() follows player.elapsed, so push only when the serialized frame changes.
    const push = (json: string) => void Events.Emit(LYRICS_EVENTS.frame, json)
    const stopWatch = watch(() => JSON.stringify(frame()), push)
    const off = [
      // A freshly mounted window asks for the current frame, which may not change again for a while.
      Events.On(LYRICS_EVENTS.ready, () => push(JSON.stringify(frame()))),
      Events.On(LYRICS_EVENTS.command, event => onCommand(event.data as LyricsCommand)),
      Events.On(LYRICS_EVENTS.closed, () => {
        release()
        onClose()
      }),
    ]
    const release = () => {
      stopWatch()
      off.forEach(stop => stop())
      this.closeLyrics = null
    }
    // Set before awaiting: lyrics:closed may arrive before the call returns.
    const close = () => void Call.ByName('main.Desktop.CloseLyricsWindow')
    this.closeLyrics = close
    try {
      await Call.ByName('main.Desktop.OpenLyricsWindow')
    }
    catch {
      release()
      return null
    }
    return close
  }
}
