import { Call, Events } from '@wailsio/runtime'
import { watch } from 'vue'
import type { DesktopAdapter, LyricsCommand, LyricsFrame } from './desktop-adapter'

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

  async resolveApi(): Promise<{ baseUrl: string; token: string }> {
    return Call.ByName('main.Desktop.ResolveApi')
  }

  async pickFolder(): Promise<string | null> {
    return await Call.ByName('main.Desktop.PickFolder') || null
  }

  setLyricsLocked(locked: boolean): void {
    void Call.ByName('main.Desktop.SetLyricsLocked', locked)
  }

  async openLyricsWindow(
    frame: () => LyricsFrame,
    onCommand: (command: LyricsCommand) => void,
    onClose: () => void,
    onLock?: (locked: boolean) => void,
  ): Promise<(() => void) | null> {
    if (this.closeLyrics) return this.closeLyrics
    // frame() follows player.elapsed, so push only when the serialized frame changes.
    const push = (json: string) => void Events.Emit(LYRICS_EVENTS.frame, json)
    const stopWatch = watch(() => JSON.stringify(frame()), push)
    const off = [
      // A freshly mounted window asks for the current frame, which may not change again for a while.
      Events.On(LYRICS_EVENTS.ready, () => push(JSON.stringify(frame()))),
      Events.On(LYRICS_EVENTS.command, (event) => onCommand(event.data as LyricsCommand)),
      Events.On(LYRICS_EVENTS.locked, (event) => onLock?.(event.data as boolean)),
      Events.On(LYRICS_EVENTS.closed, () => {
        release()
        onClose()
      }),
    ]
    const release = () => {
      stopWatch()
      off.forEach((stop) => stop())
      this.closeLyrics = null
    }
    // Set before awaiting: lyrics:closed may arrive before the call returns.
    const close = () => void Call.ByName('main.Desktop.CloseLyricsWindow')
    this.closeLyrics = close
    try {
      await Call.ByName('main.Desktop.OpenLyricsWindow')
    } catch {
      release()
      return null
    }
    return close
  }
}
