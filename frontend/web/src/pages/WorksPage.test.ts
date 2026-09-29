import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { createPinia } from 'pinia'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import WorksPage from './WorksPage.vue'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'
import type { Library, WorkSummary } from '@/lib/api/types'

/**
 * The page-level contracts of the wall (ADR 0012 §4): the address carries
 * exactly one fact — the inspected work in `?rj=` — and every filter is
 * transient page state. The keyboard dispatch is the prototype's: J/K and the
 * arrows move, P plays, Space only pauses, and the library menu owns J/K while
 * it is open.
 *
 * jsdom has no layout, so the wall's virtualizer renders an empty window and
 * the inspector's strip is where the selection is observed. The wall's own
 * geometry is covered by the WorkWall test.
 */

function makeWorks(prefix: string, count: number): WorkSummary[] {
  return Array.from({ length: count }, (_, i) => ({
    rj: `${prefix}${i}`,
    title: `Title ${prefix}${i}`,
    thumbnail_url: null,
    circle: 'CIRCLE',
    language: null,
    nsfw: false,
    release_date: '2024-03-08',
    dl_count: null,
    rating: null,
    favorite: false,
    user_tags: [],
    has_note: false,
    collected_at: '2026-01-01T00:00:00Z',
    copy_count: 1,
  }))
}

const LIB_A = 'lib-a'
const LIB_B = 'lib-b'
const worksA = makeWorks('RJA', 6)
const worksB = makeWorks('RJB', 2)

function library(id: string, name: string, root: string): Library {
  return {
    id,
    name,
    root_path: root,
    created_at: '',
    updated_at: '',
    last_scan_at: null,
    last_scan_status: 'completed',
    last_scan_error: '',
  }
}

function routerFor(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/works', name: 'works', component: WorksPage },
      { path: '/works/:rj', name: 'work-detail', component: { template: '<div />' } },
      { path: '/worksets', name: 'worksets', component: { template: '<div />' } },
    ],
  })
}

async function mountWorks(initialPath = '/works'): Promise<{
  wrapper: VueWrapper
  router: Router
  listWorks: ReturnType<typeof vi.fn>
}> {
  const listWorks = vi.fn((params: { library?: string }) =>
    Promise.resolve({ works: params.library === LIB_B ? worksB : worksA }),
  )
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue([library(LIB_A, 'Archive', '/music'), library(LIB_B, 'Brazil', '/brazil')]),
    listWorks,
  } as never)
  const router = routerFor()
  await router.push(initialPath)
  await router.isReady()
  const wrapper = mount(WorksPage, {
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, router, listWorks }
}

/** The page listens on the window, so the keyboard arrives as real events. */
function press(key: string): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key }))
}

/** The inspected work, as the inspector's strip states it. */
function inspected(wrapper: VueWrapper): string {
  return wrapper.get('[aria-label="Inspector"] .strip .lbl + .lbl').text()
}

beforeEach(() => {
  localStorage.clear()
})

describe('works page', () => {
  it('restores the inspected work from ?rj= and follows the keyboard in the address', async () => {
    const { wrapper, router } = await mountWorks('/works?rj=RJA2')

    // The address's work survived the first listing: it is what is inspected.
    expect(inspected(wrapper)).toBe('RJA2')

    press('j')
    await flushPromises()
    expect(inspected(wrapper)).toBe('RJA3')
    // Replace, not push: inspecting a neighbour is one state, not a page in
    // history.
    expect(router.currentRoute.value.query.rj).toBe('RJA3')
  })

  it('defaults the selection to the first work and writes it to the address', async () => {
    const { router } = await mountWorks('/works')

    expect(router.currentRoute.value.query.rj).toBe(worksA[0].rj)
  })

  it('resets the selection when switching libraries', async () => {
    const { wrapper, router } = await mountWorks('/works?rj=RJA3')

    useLibraryUiStore().setActiveLibrary(LIB_B)
    await flushPromises()

    // The other library's listing no longer holds the selected work, so the
    // selection falls back to its first item — and the address follows.
    expect(inspected(wrapper)).toBe(worksB[0].rj)
    expect(router.currentRoute.value.query.rj).toBe(worksB[0].rj)
  })

  it('sends the filter tab straight to the listing parameters', async () => {
    const { wrapper, listWorks } = await mountWorks('/works')

    await wrapper.findAll('.tab')[3].trigger('click') // R18
    await flushPromises()

    const last = listWorks.mock.calls.at(-1)?.[0] as { nsfw?: boolean }
    expect(last.nsfw).toBe(true)
  })

  it('plays the selected work with P and never lets Space change what plays', async () => {
    const { wrapper } = await mountWorks('/works?rj=RJA1')

    press('p')
    await flushPromises()
    const player = usePlayerStore()
    expect(player.rj).toBe('RJA1')
    expect(player.paused).toBe(false)

    press('j') // browse away from the playing work
    await flushPromises()
    expect(inspected(wrapper)).toBe('RJA2')
    expect(player.rj).toBe('RJA1') // browsing never interrupts playback

    press(' ')
    await flushPromises()
    expect(player.paused).toBe(true)
    expect(player.rj).toBe('RJA1') // pausing never changes the work either
  })

  it('opens the detail page with Enter', async () => {
    const { router } = await mountWorks('/works?rj=RJA2')

    press('Enter')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('work-detail')
    expect(router.currentRoute.value.params.rj).toBe('RJA2')
  })
})
