import type { DesktopAdapter } from './desktop-adapter'
import { WebDesktopAdapter } from './web-adapter'

interface NativeHost { chrome?: { webview?: unknown }, webkit?: { messageHandlers?: { external?: unknown } } }

/**
 * Select the host before bootstrap; loading Wails has runtime side effects.
 * The shell's message channel exists from document start, unlike `_wails`,
 * which Wails injects only after the page finishes loading.
 */
export async function createDesktopAdapter(): Promise<DesktopAdapter> {
  const host = window as NativeHost
  if (host.chrome?.webview || host.webkit?.messageHandlers?.external) {
    const { WailsDesktopAdapter } = await import('./wails-adapter')
    return new WailsDesktopAdapter()
  }
  return new WebDesktopAdapter()
}
