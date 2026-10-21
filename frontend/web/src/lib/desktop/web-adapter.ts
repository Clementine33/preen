import { createApp, h } from 'vue'
import LyricsWindowView from '@/features/lyrics-window/LyricsWindowView.vue'
import type { DesktopAdapter, LyricsCommand, LyricsFrame } from './desktop-adapter'

// The browser host: the Lyrics Window is a Document Picture-in-Picture window
// (Chromium 116+, ADR 0016) holding its own Vue app, fed by the main window.

interface DocumentPip {
  requestWindow(options: { width: number; height: number }): Promise<Window>
}
const documentPip = (): DocumentPip | undefined => (window as unknown as { documentPictureInPicture?: DocumentPip }).documentPictureInPicture

/** The opener's look: its html attributes (theme class, data-theme) and every stylesheet. */
function copyLook(target: Document): void {
  for (const { name, value } of document.documentElement.attributes) target.documentElement.setAttribute(name, value)
  for (const sheet of document.styleSheets) {
    try {
      const style = target.createElement('style')
      style.textContent = [...sheet.cssRules].map((rule) => rule.cssText).join('\n')
      target.head.append(style)
    } catch {
      // A cross-origin sheet's rules are unreadable; link it instead.
      if (!sheet.href) continue
      const link = target.createElement('link')
      link.rel = 'stylesheet'
      link.href = sheet.href
      target.head.append(link)
    }
  }
}

export class WebDesktopAdapter implements DesktopAdapter {
  readonly platform = 'browser' as const
  readonly lyricsWindow = typeof window !== 'undefined' && 'documentPictureInPicture' in window
  private closeLyrics: (() => void) | null = null

  async resolveApi(): Promise<{ baseUrl: string; token: string | null }> {
    return {
      baseUrl: '/api/v1',
      token: null,
    }
  }

  async pickFolder(): Promise<null> {
    return null
  }

  async openLyricsWindow(frame: () => LyricsFrame, onCommand: (command: LyricsCommand) => void, onClose: () => void): Promise<(() => void) | null> {
    const pip = documentPip()
    if (!pip) return null
    if (this.closeLyrics) return this.closeLyrics
    let win: Window
    try {
      win = await pip.requestWindow({ width: 520, height: 220 })
    } catch {
      return null
    }
    copyLook(win.document)
    // The window is dark in every theme: the light theme's invert pair is the dark one.
    win.document.documentElement.classList.remove('dark')
    const app = createApp({ render: () => h(LyricsWindowView, { frame: frame(), onCommand }) })
    app.mount(win.document.body.appendChild(win.document.createElement('div')))
    const close = () => win.close()
    this.closeLyrics = close
    win.addEventListener('pagehide', () => {
      app.unmount()
      this.closeLyrics = null
      onClose()
    })
    return close
  }
}

export function createDesktopAdapter(): DesktopAdapter {
  return new WebDesktopAdapter()
}
