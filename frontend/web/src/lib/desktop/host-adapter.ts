import type { DesktopAdapter } from './desktop-adapter'
import { WebDesktopAdapter } from './web-adapter'

/** Select the host before bootstrap; loading Wails has runtime side effects. */
export async function createDesktopAdapter(): Promise<DesktopAdapter> {
  if ('_wails' in window) {
    const { WailsDesktopAdapter } = await import('./wails-adapter')
    return new WailsDesktopAdapter()
  }
  return new WebDesktopAdapter()
}
