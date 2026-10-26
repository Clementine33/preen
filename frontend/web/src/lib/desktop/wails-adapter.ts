import { Call } from '@wailsio/runtime'
import type { DesktopAdapter } from './desktop-adapter'

/** Native bindings supply credentials and folder selection; HTTP stays in ApiClient. */
export class WailsDesktopAdapter implements DesktopAdapter {
  readonly platform = 'desktop' as const
  readonly lyricsWindow = false

  async resolveApi(): Promise<{ baseUrl: string; token: string }> {
    return Call.ByName('main.Desktop.ResolveApi')
  }

  async pickFolder(): Promise<string | null> {
    return await Call.ByName('main.Desktop.PickFolder') || null
  }

  async openLyricsWindow(): Promise<null> {
    return null
  }
}
