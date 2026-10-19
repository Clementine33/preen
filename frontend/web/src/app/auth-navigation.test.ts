import { afterEach, describe, expect, it, vi } from 'vitest'
import { router } from './router'
import { cleanedQuery } from './route-params'
import { loginReturnAddress, redirectToLogin } from './auth-navigation'
import { ApiClient } from '@/lib/api/client'

afterEach(() => vi.unstubAllGlobals())

describe('browser authentication navigation', () => {
  it.each(['//host', '/\\host', '/\t/host', '/\n/host', 'https://host', '/./login', '/LOGIN', '/login/'])('refuses unsafe return address %j', (value) => {
    expect(loginReturnAddress(value)).toBe('/')
  })
  it('returns the normalized same-origin address and retains the login parameter', () => {
    expect(loginReturnAddress('/works/../collections?sort=rj#row')).toBe('/collections?sort=rj#row')
    expect(cleanedQuery(router.resolve('/login?redirect=%2Fworks'))).toBeNull()
  })
  it('does not leave a login route after an unauthorized request', async () => {
    const replace = vi.fn()
    vi.stubGlobal('location', { origin: 'http://localhost:3000', pathname: '/LOGIN/', search: '?redirect=%2Fworks', hash: '', replace })
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 401 })))
    const client = new ApiClient({ baseUrl: '/api/v1', token: null, onUnauthorized: redirectToLogin })
    await expect(client.listLibraries()).rejects.toMatchObject({ status: 401 })
    expect(replace).not.toHaveBeenCalled()
  })
  it('redirects concurrent API and SSE failures once with the original address', async () => {
    const replace = vi.fn()
    vi.stubGlobal('location', { origin: 'http://localhost:3000', pathname: '/works', search: '?q=one', hash: '#row', replace })
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 401 })))
    const client = new ApiClient({ baseUrl: '/api/v1', token: null, onUnauthorized: redirectToLogin })
    await Promise.allSettled([client.listLibraries(), client.listWorks(), client.streamMediaFactsEvents(new AbortController().signal).next()])
    expect(replace).toHaveBeenCalledExactlyOnceWith('/login?redirect=%2Fworks%3Fq%3Done%23row')
  })
})
