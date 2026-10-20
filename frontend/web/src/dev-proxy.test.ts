import { createServer as createHTTPServer } from 'node:http'
import { once } from 'node:events'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { AddressInfo } from 'node:net'
import { createServer, type ViteDevServer } from 'vite'
import { expect, it, vi } from 'vitest'
import { WebDesktopAdapter } from './lib/desktop/web-adapter'

it('keeps browser API configuration same-origin and credential-free', async () => {
  vi.stubEnv('PREEN_TOKEN', 'proxy-only-secret')
  try {
    expect(await new WebDesktopAdapter().resolveApi()).toEqual({ baseUrl: '/api/v1', token: null })
  } finally {
    vi.unstubAllEnvs()
  }
})

it('proxies API, original files, thumbnails and event streams with server-side credentials', async () => {
  const received: Array<{ path: string | undefined; authorization: string | undefined }> = []
  const backend = createHTTPServer((request, response) => {
    received.push({ path: request.url, authorization: request.headers.authorization })
    response.setHeader('Content-Type', request.url?.endsWith('/events') ? 'text/event-stream' : 'application/json')
    response.end('ok')
  })
  const cacheDir = await mkdtemp(join(tmpdir(), 'preen-proxy-vite-'))
  let vite: ViteDevServer | undefined
  try {
    backend.listen(0, '127.0.0.1')
    await once(backend, 'listening')
    vi.stubEnv('PREEN_BACKEND_URL', `http://127.0.0.1:${(backend.address() as AddressInfo).port}`)
    vi.stubEnv('PREEN_TOKEN', 'proxy-only-secret')
    vite = await createServer({ cacheDir, server: { host: '127.0.0.1', port: 0 }, logLevel: 'silent' })
    await vite.listen()
    const origin = `http://127.0.0.1:${(vite.httpServer!.address() as AddressInfo).port}`
    const paths = ['/api/v1/health', '/api/v1/works/RJ000001/content/copy/test.mp3', '/api/v1/works/RJ000001/thumbs/copy/cover.jpg', '/api/v1/inventory/media-facts/events']
    for (const path of paths) {
      const response = await fetch(origin + path)
      expect(await response.text()).toBe('ok')
    }
    expect(received).toEqual(paths.map((path) => ({ path, authorization: 'Bearer proxy-only-secret' })))
    const module = await vite.transformRequest('/src/lib/desktop/web-adapter.ts')
    expect(module?.code).not.toContain('proxy-only-secret')
    expect(module?.code).not.toContain(process.env.PREEN_BACKEND_URL)
  } finally {
    try {
      await vite?.close()
    } finally {
      backend.closeAllConnections()
      await new Promise<void>((resolve) => backend.close(() => resolve()))
      vi.unstubAllEnvs()
      await rm(cacheDir, { recursive: true, force: true })
    }
  }
}, 20_000)
