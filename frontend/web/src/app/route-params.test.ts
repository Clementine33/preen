import type { Router } from 'vue-router'
import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { canonicalRj, cleanedQuery, installAddressHygiene, readMemberFilter } from './route-params'

/**
 * Address hygiene (ADR 0003): a page keeps only the parameters it
 * declares, with values from a closed vocabulary, and a cleaned address is
 * rewritten in place — no extra history entry, no redirect loop.
 *
 * Folder names and search terms are what an address must never carry, so the
 * retired `folder` parameter and a `q` are the cases that matter here.
 */

const DIR_ID = 'a1b2c3d4e5f60718293a4b5c6d7e8f90'

function guardRouter(): Router {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/worksets',
        name: 'worksets',
        component: { template: '<div />' },
      },
      {
        path: '/works',
        name: 'works',
        component: { template: '<div />' },
      },
      {
        path: '/collections/:id',
        name: 'collection-detail',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId',
        name: 'workbench-overview',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/f/:dirId',
        name: 'overview-files',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/conversion',
        name: 'conversion',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/conversion/settings',
        name: 'conversion-settings',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/conversion/:memberId/files',
        name: 'conversion-member-files',
        component: { template: '<div />' },
      },
      {
        path: '/:pathMatch(.*)*',
        name: 'not-found',
        component: { template: '<div />' },
      },
    ],
  })
  installAddressHygiene(router)
  return router
}

function routeOf(router: Router, fullPath: string) {
  return router.resolve(fullPath)
}

describe('cleaned query', () => {
  it('keeps a collection view sort and nothing else', () => {
    const router = guardRouter()
    expect(cleanedQuery(routeOf(router, '/collections/c1?sort=collected_at&direction=desc'))).toBeNull()
    expect(cleanedQuery(routeOf(router, '/collections/c1?sort=bogus&direction=asc&q=x'))).toEqual({ direction: 'asc' })
  })

  it('keeps a page parameter that carries an allowed value', () => {
    expect(cleanedQuery(routeOf(guardRouter(), `/worksets/lib-1/conversion?filter=warn`))).toBeNull()
    expect(cleanedQuery(routeOf(guardRouter(), `/worksets/lib-1/f/${DIR_ID}?nope=1`))).toEqual({})
  })

  it('drops the retired folder parameter and any other extra, keeping what is declared', () => {
    const route = routeOf(guardRouter(), `/worksets/lib-1/conversion?filter=change&folder=albumA&q=hot`)
    expect(cleanedQuery(route)).toEqual({ filter: 'change' })
  })

  it('drops a declared parameter whose value is outside its vocabulary', () => {
    const route = routeOf(guardRouter(), '/worksets/lib-1/conversion?filter=anything&view=plan')
    expect(cleanedQuery(route)).toEqual({})
  })

  it('keeps no parameter on a page that declares none', () => {
    const route = routeOf(guardRouter(), `/worksets/lib-1/f/${DIR_ID}?view=plan&q=x`)
    expect(cleanedQuery(route)).toEqual({})
  })

  it('removes a fragment even when the query is already clean', () => {
    const route = routeOf(guardRouter(), '/worksets/lib-1/conversion#frag')
    expect(cleanedQuery(route)).toEqual({})
  })

  it('reports an already clean address as clean', () => {
    expect(cleanedQuery(routeOf(guardRouter(), '/worksets/lib-1/conversion?filter=all'))).toBeNull()
  })

  it('reads the filter of an address, falling back to the unfiltered list', () => {
    expect(readMemberFilter('blocked')).toBe('blocked')
    expect(readMemberFilter('anything')).toBe('all')
    expect(readMemberFilter(undefined)).toBe('all')
  })
})

describe('the wall address', () => {
  it('keeps a work number in either spelling the API accepts, and nothing else', () => {
    for (const rj of ['RJ01113012', 'rj01113012', 'RJ123456']) {
      expect(cleanedQuery(routeOf(guardRouter(), `/works?rj=${rj}`)), rj).toBeNull()
    }
    // 7 digits is no work number (docs/api.md), and a term is not an identity.
    for (const rj of ['RJ1657200', 'RJ012345678', 'asmr']) {
      expect(cleanedQuery(routeOf(guardRouter(), `/works?rj=${rj}`)), rj).toEqual({})
    }
  })

  it.each(['CHI_HANS', 'CHI_HANT', 'JPN', 'NM'])('keeps language %s through navigation and reload', async (language) => {
    const router = guardRouter()
    await router.push(`/works?language=${language}`)
    expect(router.currentRoute.value.query.language).toBe(language)
    const reloaded = guardRouter()
    await reloaded.push(router.currentRoute.value.fullPath)
    expect(reloaded.currentRoute.value.query.language).toBe(language)
  })

  it('keeps the whole view of the wall, a long id group included', () => {
    // 300 ids of one name: about 11,000 characters, never dropped for length.
    const ids = Array.from({ length: 300 }, (_, i) => `00000000-0000-4000-8000-${String(i).padStart(12, '0')}`)
    const view = new URLSearchParams({
      library: 'all',
      voice: ids.join(','),
      circle: 'ci-1',
      age: 'r18',
      rating: '8',
      listening: 'listening',
      tag: 's:s-asmr|u:u-asmr,s:s-ear',
      mode: 'any',
      sort: 'listened_at',
      q: '月 下',
      page: '3',
      size: '36',
      rj: 'RJ01113012',
    })
    expect(cleanedQuery(routeOf(guardRouter(), `/works?${view}`))).toBeNull()
  })

  it('drops a wall value of the wrong shape', () => {
    for (const [key, value] of [
      ['page', '0'],
      ['page', '-1'],
      ['size', '50'],
      ['age', 'adult'],
      ['voice', 'a b'],
      ['language', 'invalid'],
      ['rating', '11'],
      ['listening', 'maybe'],
    ]) {
      expect(cleanedQuery(routeOf(guardRouter(), `/works?${key}=${encodeURIComponent(value)}`)), key).toEqual({})
    }
  })

  it('compares a work number in its canonical spelling', () => {
    expect(canonicalRj('RJ123456')).toBe('RJ00123456')
    expect(canonicalRj('rj01113012')).toBe('RJ01113012')
    expect(canonicalRj('RJ1657200')).toBeNull()
  })
})

describe('address hygiene guard', () => {
  it('rewrites an unclean address in place, adding no history entry of its own', async () => {
    const router = guardRouter()
    await router.push(`/worksets/lib-1/f/${DIR_ID}`)
    const cleanDelta = window.history.length
    await router.push(`/worksets/lib-1/f/${DIR_ID}`)
    const cleanSteps = window.history.length - cleanDelta

    const before = window.history.length
    await router.push(`/worksets/lib-1/f/${DIR_ID}?folder=albumA&q=hot#top`)
    expect(router.currentRoute.value.fullPath).toBe(`/worksets/lib-1/f/${DIR_ID}`)
    expect(window.history.length - before).toBe(cleanSteps)
  })

  it('keeps the page parameters a page declares, and settles instead of looping', async () => {
    const router = guardRouter()
    await router.push('/worksets/lib-1/conversion?filter=warn&q=hot')
    expect(router.currentRoute.value.fullPath).toBe('/worksets/lib-1/conversion?filter=warn')

    // A second pass over the same address changes nothing: an idempotent guard
    // is what keeps this from becoming a redirect loop.
    await router.push('/worksets/lib-1/conversion?filter=warn')
    expect(router.currentRoute.value.fullPath).toBe('/worksets/lib-1/conversion?filter=warn')
    expect(router.currentRoute.value.name).toBe('conversion')
  })

  it('leaves an unknown address alone, so the not-found page keeps its path', async () => {
    const router = guardRouter()
    await router.push('/worksets/libraries/lib-1?folder=albumA')
    expect(router.currentRoute.value.name).toBe('not-found')
    expect(router.currentRoute.value.fullPath).toBe('/worksets/libraries/lib-1')
  })
})
