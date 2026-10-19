import type { DesktopAdapter } from './desktop-adapter'

export class WebDesktopAdapter implements DesktopAdapter {
  readonly platform = 'browser' as const

  async resolveApi(): Promise<{ baseUrl: string; token: string | null }> {
    return {
      baseUrl: '/api/v1',
      token: null,
    }
  }

  async pickFolder(): Promise<null> {
    return null
  }
}

export function createDesktopAdapter(): DesktopAdapter {
  return new WebDesktopAdapter()
}
