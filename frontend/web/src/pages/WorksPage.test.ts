import type { QueryClient } from '@tanstack/vue-query'
import type { VueWrapper } from '@vue/test-utils'
import type { LocationQuery, Router } from 'vue-router'
import type { Library, WorkSummary } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest'
import { h as vh } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { installAddressHygiene } from '@/app/route-params'
import { apiClientKey } from '@/lib/api/client'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'
import { apiStub } from '@/test/api-stub'
import { mountPlayerKeys } from '@/test/player-keys'
import { installTestQueryPlugin } from '@/test/query-client'
import WorksPage from './WorksPage.vue'

/**
 * The page-level contracts of the wall: the address owns what
 * the wall shows — library, filters, FIND, order, page, page size and the
 * inspected work — and the history entry owns where the wall was when a work
 * was opened. The keyboard dispatch is the prototype's: J/K and the arrows
 * move, P plays, Space only pauses, and the library menu owns J/K while it is
 * open.
 *
 * jsdom has no layout: the inspector's strip is where the selection is
 * observed, and the wall's own geometry is covered by the WorkWall test.
 */

/** Real work numbers: the address only keeps a canonical `RJ` + 8 digits. */
function rjOf(n: number): string {
  return `RJ${String(n).padStart(8, '0')}`
}

function makeWorks(base: number, count: number): WorkSummary[] {
  return Array.from({ length: count }, (_, i) => ({
    rj: rjOf(base + i),
    title: `Title ${rjOf(base + i)}`,
    cover_url: null,
    circle: 'CIRCLE',
    language: null,
    nsfw: false,
    age_category: 1,
    release_date: '2024-03-08',
    dl_count: null,
    rating: null,
    favorite: false,
    user_tags: [],
    has_note: false,
    collected_at: '2026-01-01T00:00:00Z',
    copy_count: 1,
    listening: {
      state: 'none',
      track_index: null,
      track_count: null,
      updated_at: null,
    },
  }))
}

const LIB_A = 'lib-a'
const LIB_B = 'lib-b'
// Below RJ01000000, so the listing's numbers also have a 6-digit spelling.
// Forty works: two pages at the smallest page size.
const worksA = makeWorks(100_000, 40)
const worksB = makeWorks(200_000, 2)
const [A0, A1, A2, A3] = worksA.map(work => work.rj)

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
    storage_class: 'hdd',
  }
}

function routerFor(): Router {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/works', name: 'works', component: WorksPage },
      {
        path: '/works/:rj',
        name: 'work-detail',
        component: { template: '<div />' },
      },
      {
        path: '/collections',
        name: 'collections',
        component: { template: '<div />' },
      },
      {
        path: '/worksets',
        name: 'worksets',
        component: { template: '<div />' },
      },
    ],
  })
  installAddressHygiene(router)
  return router
}

interface Harness {
  wrapper: VueWrapper
  router: Router
  queryClient: QueryClient
  listWorks: ReturnType<typeof vi.fn>
  listWorkFacets: ReturnType<typeof vi.fn>
}

/** One page of the library's works, the way the listing pages them. */
function pageOf(params: { library?: string, page?: number, limit?: number }) {
  const all = params.library === LIB_B
    ? worksB
    : worksA
  const limit = params.limit ?? 50
  const start = ((params.page ?? 1) - 1) * limit
  return {
    works: all.slice(start, start + limit),
    total: all.length,
  }
}

async function mountWorks(
  initialPath = '/works',
  attach = false,
  reuse: {
    listWorks?: ReturnType<typeof vi.fn>
    listWorkFacets?: ReturnType<typeof vi.fn>
    listLibraries?: ReturnType<typeof vi.fn>
    /** Runs on the store before the page mounts: what an earlier page left there. */
    seed?: (ui: ReturnType<typeof useLibraryUiStore>) => void
  } = {},
): Promise<Harness> {
  const listWorks = reuse.listWorks ?? vi.fn((params: { library?: string, page?: number, limit?: number }) =>
    Promise.resolve(pageOf(params)))
  const listWorkFacets = reuse.listWorkFacets ?? vi.fn((kind: string) =>
    Promise.resolve(
      kind === 'voice_actor'
        ? [
            { id: 'va-1', name: '伊ヶ崎綾香', work_count: 3 },
          ]
        : [
            { id: 'ci-1', name: 'mete0r', work_count: 2 },
          ],
    ),
  )
  const api = apiStub({
    listLibraries: reuse.listLibraries ?? vi.fn().mockResolvedValue([
      library(LIB_A, 'Archive', '/music'),
      library(LIB_B, 'Brazil', '/brazil'),
    ]),
    listWorks,
    getWork: vi.fn(async (rj: string) => ({
      ...makeWorks(Number(rj.slice(2)), 1)[0]!,
      source_url: null,
      cover_url: null,
      sample_cover_url: null,
      series_id: null,
      series_name: null,
      voice_actors: [],
      source_tags: [],
      note: '',
      language_editions: null,
      edition_tree: null,
      relation: {
        role: null,
        language: null,
        original_rj: null,
        parent_rj: null,
      },
      copies: [],
      relation_warnings: [],
      relation_nodes: [],
      listening: [],
    })),
    listWorkFacets,
    listTagFacets: vi.fn().mockResolvedValue({
      source_tags: [
        { id: 's-asmr', name: 'ASMR', work_count: 4 },
      ],
      user_tags: [
        { id: 'u-asmr', name: 'ASMR', work_count: 1 },
      ],
    }),
    // Every work ships two tracks, so P has a real queue to play.
    getWorkContent: vi.fn(async (rj: string) => ({
      rj,
      copies: [
        {
          copy_id: 'c',
          library_id: LIB_A,
          dir_id: 'd',
          online: true,
          tree: {
            name: '',
            rel_path: '',
            children: ['01.mp3', '02.mp3'].map(name => ({
              name,
              rel_path: name,
              kind: 'aud',
              size: 1,
              format: 'MP3',
              bitrate: null,
              width: null,
              height: null,
              duration_sec: 120,
              thumb_v: null,
            })),
          },
        },
      ],
    })),
  } as never)
  const router = routerFor()
  await router.push(initialPath)
  await router.isReady()
  const query = installTestQueryPlugin()
  const pinia = createPinia()
  setActivePinia(pinia)
  reuse.seed?.(useLibraryUiStore(pinia))
  // Under a RouterView, so the page's leave guard is registered as in the app.
  const wrapper = mount({
    render: () => vh(RouterView),
  }, {
    ...(attach
      ? {
          attachTo: document.body.appendChild(document.createElement('div')),
        }
      : {}),
    global: {
      plugins: [
        pinia,
        router,
        query,
      ],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return {
    wrapper,
    router,
    queryClient: query[1].queryClient,
    listWorks,
    listWorkFacets,
  }
}

/** The page listens on the window, so the keyboard arrives as real events. */
function press(key: string): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key }))
}

/** The inspected work, as the inspector's strip states it. */
function inspected(wrapper: VueWrapper): string {
  return wrapper.get('[aria-label="Inspector"] .strip .lbl + .lbl').text()
}

/** The listing parameters of the most recent request. */
function lastParams(listWorks: ReturnType<typeof vi.fn>): Record<string, unknown> {
  return listWorks.mock.calls.at(-1)?.[0] as Record<string, unknown>
}

/** Picks a library in the switcher by its name (`ALL LIBRARIES` for the aggregate). */
async function chooseLibrary(wrapper: VueWrapper, name: string): Promise<void> {
  await wrapper.get('.lib-trigger').trigger('click')
  await wrapper.findAll('.lib-item').find(item => item.find('.nm').text() === name)!.trigger('click')
  await flushPromises()
}

/** Opens one rail select by its label and picks an option by its text. */
async function pick(wrapper: VueWrapper, label: string, option: string): Promise<void> {
  const select = wrapper.findAll('.rs').find(candidate => candidate.find('.rs-lbl').text() === label)!
  await select.get('.rs-trigger').trigger('click')
  const item = select.findAll('[role="option"]').find(candidate => candidate.find('.rs-name').text() === option)!
  await item.trigger('click')
  await flushPromises()
}

// jsdom has no layout: the rail's virtualized option lists measure their
// scroller's outer or client dimensions, so both report the panel's size.
const offsetHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetHeight')!
const offsetWidth = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetWidth')!
const clientHeight = Object.getOwnPropertyDescriptor(Element.prototype, 'clientHeight')!
const clientWidth = Object.getOwnPropertyDescriptor(Element.prototype, 'clientWidth')!
beforeAll(() => {
  Object.defineProperties(HTMLElement.prototype, {
    clientHeight: {
      configurable: true,
      get(this: HTMLElement) { return this.offsetHeight },
    },
    clientWidth: {
      configurable: true,
      get(this: HTMLElement) { return this.offsetWidth },
    },
    offsetHeight: {
      configurable: true,
      get(this: HTMLElement) {
        return this.classList.contains('rs-list')
          ? 320
          : 0
      },
    },
    offsetWidth: {
      configurable: true,
      get(this: HTMLElement) {
        return this.classList.contains('rs-list')
          ? 280
          : 0
      },
    },
  })
})
afterAll(() => {
  Object.defineProperties(HTMLElement.prototype, {
    offsetHeight,
    offsetWidth,
    clientHeight,
    clientWidth,
  })
})

beforeEach(() => {
  localStorage.clear()
  document.body.innerHTML = ''
})

enableAutoUnmount(afterEach)

describe('works page', () => {
  it('restores the inspected work from ?rj= and follows the keyboard in the address', async () => {
    const { wrapper, router } = await mountWorks(`/works?rj=${A2}`)

    // The address's work survived the first listing: it is what is inspected.
    expect(inspected(wrapper)).toBe(A2)
    expect(wrapper.get('.insp-title').text()).toBe(`Title ${A2}`)

    press('ArrowRight')
    await flushPromises()
    expect(inspected(wrapper)).toBe(A3)
    // Replace, not push: inspecting a neighbour is one state, not a page in
    // history.
    expect(router.currentRoute.value.query.rj).toBe(A3)
  })

  it('leaves J and K alone: movement is the arrows', async () => {
    const { wrapper } = await mountWorks(`/works?rj=${A2}`)
    press('j')
    press('k')
    await flushPromises()
    expect(inspected(wrapper)).toBe(A2)
  })

  it('reads the 6-digit spelling of a work number as the canonical one', async () => {
    // RJ00100002 is also spelled rj100002.
    const { wrapper } = await mountWorks(`/works?rj=rj${A2.slice(4)}`)

    expect(inspected(wrapper)).toBe(A2)
  })

  it('falls back to the first work of the page when the address names a work not on it', async () => {
    // A deleted work, one an order moved away, or a link to another page: the
    // wall does not go looking for it, and the inspector never shows a work
    // the wall does not.
    const { wrapper, router } = await mountWorks(`/works?rj=${rjOf(9_000_000)}`)

    expect(inspected(wrapper)).toBe(A0)
    expect(router.currentRoute.value.query.rj).toBe(A0)
  })

  it('defaults the selection to the first work and writes it to the address', async () => {
    const { router } = await mountWorks('/works')

    expect(router.currentRoute.value.query.rj).toBe(A0)
  })

  it('keeps the address library across a reload instead of the first one', async () => {
    const { router, listWorks } = await mountWorks(`/works?library=${LIB_B}`)

    expect(lastParams(listWorks)).toMatchObject({ library: LIB_B })
    expect(router.currentRoute.value.query.library).toBe(LIB_B)
  })

  it('lists every library at once under the ALL scope and leaves it for one library', async () => {
    const { wrapper, router, listWorks } = await mountWorks('/works')

    await chooseLibrary(wrapper, 'ALL LIBRARIES')
    expect(lastParams(listWorks)).toMatchObject({ library: `${LIB_A},${LIB_B}` })
    expect(router.currentRoute.value.query.library).toBe('all')

    await chooseLibrary(wrapper, 'Brazil')
    expect(lastParams(listWorks)).toMatchObject({ library: LIB_B })
    expect(router.currentRoute.value.query.library).toBe(LIB_B)
  })

  it('keeps the ALL scope across a reload', async () => {
    const { router, listWorks } = await mountWorks('/works?library=all')

    expect(lastParams(listWorks)).toMatchObject({ library: `${LIB_A},${LIB_B}` })
    expect(router.currentRoute.value.query.library).toBe('all')
  })

  it('lists by the applied tags and drops them with the library', async () => {
    const { wrapper, listWorks } = await mountWorks('/works', true)

    await wrapper.get('.ti-trigger').trigger('click')
    await flushPromises()
    const chip = [
      ...document.querySelectorAll<HTMLButtonElement>('.ti-chips .chip'),
    ][0]
    chip.click()
    await flushPromises()
    ;[
      ...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button'),
    ]
      .find(button => button.textContent === 'APPLY')!
      .click()
    await flushPromises()
    // One name is one group: both vocabularies' ids, either one matches.
    expect(lastParams(listWorks)).toMatchObject({ tag: 's:s-asmr|u:u-asmr', tag_match: 'all' })

    await chooseLibrary(wrapper, 'Brazil')
    expect(lastParams(listWorks)).toMatchObject({ library: LIB_B })
    expect(lastParams(listWorks)).not.toHaveProperty('tag')
  })

  it('resets the selection when switching libraries', async () => {
    const { wrapper, router } = await mountWorks(`/works?rj=${A3}`)

    await chooseLibrary(wrapper, 'Brazil')

    // The other library's listing no longer holds the selected work, so the
    // selection falls back to its first item — and the address follows.
    expect(inspected(wrapper)).toBe(worksB[0].rj)
    expect(router.currentRoute.value.query.rj).toBe(worksB[0].rj)
  })

  it('lists newest releases first until another order is chosen', async () => {
    const { wrapper, listWorks } = await mountWorks('/works')

    expect(lastParams(listWorks)).toMatchObject({ sort: 'release_date', direction: 'desc' })

    await pick(wrapper, 'SORT', '个人评分')
    expect(lastParams(listWorks)).toMatchObject({ sort: 'rating', direction: 'desc' })
    await pick(wrapper, 'SORT', '入库时间')
    expect(lastParams(listWorks)).toMatchObject({ sort: 'collected_at', direction: 'desc' })
    await pick(wrapper, 'SORT', '最近收听')
    expect(lastParams(listWorks)).toMatchObject({ sort: 'listened_at', direction: 'desc' })
  })

  it('sends each rail filter straight to its listing parameter', async () => {
    const { wrapper, listWorks, router } = await mountWorks('/works')

    await pick(wrapper, 'LANG', '无语言限制')
    expect(lastParams(listWorks)).toMatchObject({ language: 'NM' })
    await pick(wrapper, 'LANG', '简中')
    expect(lastParams(listWorks)).toMatchObject({ language: 'CHI_HANS' })
    await pick(wrapper, 'LANG', '多版在库')
    expect(lastParams(listWorks)).toMatchObject({ multi_edition: true })
    expect(lastParams(listWorks)).not.toHaveProperty('language')
    await pick(wrapper, 'LANG', 'ALL')
    expect(router.currentRoute.value.query.language).toBeUndefined()
    await pick(wrapper, 'AGE', 'R18')
    expect(lastParams(listWorks)).toMatchObject({ age_category: 3 })
    await pick(wrapper, 'AGE', 'R15')
    expect(lastParams(listWorks)).toMatchObject({ age_category: 2 })
    await pick(wrapper, 'AGE', '全年龄')
    expect(lastParams(listWorks)).toMatchObject({ age_category: 1 })
    expect(wrapper.text()).not.toContain('UNKNOWN')

    // A voice actor filter always states its match mode, which the API requires.
    await pick(wrapper, 'VOICE', '伊ヶ崎綾香')
    expect(lastParams(listWorks)).toMatchObject({ voice_actor: 'va-1', voice_actor_match: 'any' })
    await pick(wrapper, 'CIRCLE', 'mete0r')
    expect(lastParams(listWorks)).toMatchObject({ circle: 'ci-1', voice_actor: 'va-1' })

    // ALL clears a dimension instead of sending an empty value.
    await pick(wrapper, 'VOICE', 'ALL')
    expect(lastParams(listWorks)).not.toHaveProperty('voice_actor')
    expect(lastParams(listWorks)).not.toHaveProperty('voice_actor_match')
  })

  it('maps the personal rating bar to an inclusive range, the unrated set, or nothing', async () => {
    const { wrapper, listWorks, router } = await mountWorks('/works?rating=7-9')
    expect(lastParams(listWorks)).toMatchObject({ rating_from: 7, rating_to: 9 })
    const select = wrapper.findAll('.rs').find(candidate => candidate.find('.rs-lbl').text() === 'RATING')!
    expect(select.get('.rs-value').text()).toBe('7–9')

    // Two presses in either order pick the range; one cell twice is exact.
    const cell = (score: number) => select.findAll('.rb-cell').find(candidate => candidate.text() === String(score))!
    await select.get('.rs-trigger').trigger('click')
    await cell(10).trigger('click')
    await cell(6).trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.rating).toBe('6-10')
    expect(lastParams(listWorks)).toMatchObject({ rating_from: 6, rating_to: 10 })
    await select.get('.rs-trigger').trigger('click')
    await cell(8).trigger('click')
    await cell(8).trigger('click')
    await flushPromises()
    expect(lastParams(listWorks)).toMatchObject({ rating_from: 8, rating_to: 8 })
    expect(select.get('.rs-value').text()).toBe('8')

    await pick(wrapper, 'RATING', '未评分')
    expect(lastParams(listWorks)).toMatchObject({ unrated: true })
    expect(lastParams(listWorks)).not.toHaveProperty('rating_from')

    // An address value outside the bar, an old floor included, constrains nothing.
    for (const address of ['/works?rating=bogus', '/works?rating=8', '/works?rating=9-7']) {
      const other = await mountWorks(address)
      expect(lastParams(other.listWorks)).not.toHaveProperty('rating_from')
      expect(lastParams(other.listWorks)).not.toHaveProperty('unrated')
    }
  })

  it('maps the listening filter to its listing parameter, or to nothing', async () => {
    const { wrapper, listWorks, router } = await mountWorks('/works')
    expect(lastParams(listWorks)).not.toHaveProperty('listening')

    await pick(wrapper, 'PROG', '在听')
    expect(lastParams(listWorks)).toMatchObject({ listening: 'listening' })
    expect(router.currentRoute.value.query.listening).toBe('listening')
    await pick(wrapper, 'PROG', '听完')
    expect(lastParams(listWorks)).toMatchObject({ listening: 'finished' })
    await pick(wrapper, 'PROG', '未听')
    expect(lastParams(listWorks)).toMatchObject({ listening: 'none' })

    const bogus = await mountWorks('/works?listening=bogus')
    expect(lastParams(bogus.listWorks)).not.toHaveProperty('listening')
  })

  it('offers a name held by several per-work entities once, asking for all of them', async () => {
    const { wrapper, listWorks, listWorkFacets } = await mountWorks('/works')
    listWorkFacets.mockResolvedValue([
      { id: 'va-7', name: '悠木碧', work_count: 1 },
      { id: 'va-9', name: '悠木碧', work_count: 2 },
    ])
    await chooseLibrary(wrapper, 'Brazil')

    await pick(wrapper, 'VOICE', '悠木碧')
    expect(lastParams(listWorks)).toMatchObject({ voice_actor: 'va-7,va-9', voice_actor_match: 'any' })
  })

  it('narrows every other rail to what the wall can still combine with', async () => {
    const { wrapper, listWorkFacets } = await mountWorks('/works')
    const lastScope = (kind: string) =>
      listWorkFacets.mock.calls.filter(call => call[0] === kind).at(-1)?.[1] as Record<string, unknown>

    await pick(wrapper, 'VOICE', '伊ヶ崎綾香')
    await pick(wrapper, 'AGE', 'R18')

    // The circles follow the voice actor and the age; the voice actors follow
    // the age but not their own pick, so the alternatives stay on offer.
    expect(lastScope('circle')).toMatchObject({ library: LIB_A, voice_actor: 'va-1', age_category: 3 })
    expect(lastScope('voice_actor')).toMatchObject({ library: LIB_A, age_category: 3 })
    expect(lastScope('voice_actor')).not.toHaveProperty('voice_actor')
    expect(lastScope('circle')).not.toHaveProperty('sort')
  })

  it('keeps a pick the other filters narrowed away on its tab', async () => {
    const { wrapper, listWorkFacets } = await mountWorks('/works')
    await pick(wrapper, 'VOICE', '伊ヶ崎綾香')

    listWorkFacets.mockResolvedValue([])
    await pick(wrapper, 'AGE', 'R18')

    const select = wrapper.findAll('.rs').find(candidate => candidate.find('.rs-lbl').text() === 'VOICE')!
    expect(select.get('.rs-value').text()).toBe('伊ヶ崎綾香')
  })

  it('narrows a long facet list by name', async () => {
    const { wrapper } = await mountWorks('/works')
    const select = wrapper.findAll('.rs').find(candidate => candidate.find('.rs-lbl').text() === 'VOICE')!
    await select.get('.rs-trigger').trigger('click')

    await select.get('input[type="search"]').setValue('nobody')
    // The empty entry stays reachable, so clearing is still one step.
    expect(select.findAll('[role="option"]').map(item => item.find('.rs-name').text())).toEqual(['ALL'])
  })

  it('drops the voice actor and circle of the previous library', async () => {
    const { wrapper, listWorks } = await mountWorks('/works')
    await pick(wrapper, 'VOICE', '伊ヶ崎綾香')

    await chooseLibrary(wrapper, 'Brazil')

    expect(lastParams(listWorks)).toMatchObject({ library: LIB_B })
    expect(lastParams(listWorks)).not.toHaveProperty('voice_actor')
  })

  it('plays the selected work with P and never lets Space change what plays', async () => {
    const { wrapper } = await mountWorks(`/works?rj=${A1}`)
    mountPlayerKeys()

    press('p')
    await flushPromises()
    const player = usePlayerStore()
    expect(player.rj).toBe(A1)
    expect(player.paused).toBe(false)
    // The work's first track, by its address, with the rest queued behind it.
    expect(player.queue.map(item => item.source?.src)).toEqual(['/files/01.mp3', '/files/02.mp3'])
    expect(player.trackIndex).toBe(0)

    press('ArrowRight') // browse away from the playing work
    await flushPromises()
    expect(inspected(wrapper)).toBe(A2)
    expect(player.rj).toBe(A1) // browsing never interrupts playback

    press(' ')
    await flushPromises()
    expect(player.paused).toBe(true)
    expect(player.rj).toBe(A1) // pausing never changes the work either
  })

  it('steps and seeks the playing track from the baseline', async () => {
    const { wrapper } = await mountWorks(`/works?rj=${A1}`)
    press('p')
    await flushPromises()
    const player = usePlayerStore()
    const prev = wrapper.get('[aria-label="Previous track"]')
    const next = wrapper.get('[aria-label="Next track"]')
    expect([
      prev.attributes('disabled'),
      next.attributes('disabled'),
    ]).toEqual(['', undefined])

    await next.trigger('click')
    expect(player.trackIndex).toBe(1)
    expect(player.title).toBe('02')
    expect(next.attributes('disabled')).toBe('')

    const seek = wrapper.get<HTMLInputElement>('input[aria-label="Seek"]')
    await seek.setValue('42')
    expect(player.elapsed).toBe(42)

    await wrapper.get<HTMLInputElement>('input[aria-label="Volume"]').setValue('0.3')
    expect(player.volume).toBe(0.3)
    await wrapper.get('[aria-label="Mute"]').trigger('click')
    expect(player.muted).toBe(true)
  })

  it('opens the detail page with Enter', async () => {
    const { router } = await mountWorks(`/works?rj=${A2}`)

    press('Enter')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('work-detail')
    expect(router.currentRoute.value.params.rj).toBe(A2)
  })

  it('opens the sole collected translation when FIND submits a parent number', async () => {
    const { wrapper, router, listWorks } = await mountWorks()
    listWorks.mockResolvedValue({
      works: makeWorks(1_526_629, 1),
      total: 1,
    })
    await wrapper.get('.find input').setValue('RJ01526615')
    await wrapper.get('.find input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(lastParams(listWorks)).toMatchObject({ q: 'RJ01526615' })
    expect(router.currentRoute.value.params.rj).toBe('RJ01526629')
    wrapper.unmount()
  })

  it('keeps multiple parent matches on the wall', async () => {
    const { wrapper, router, listWorks } = await mountWorks()
    listWorks.mockResolvedValue({
      works: makeWorks(1_526_629, 2),
      total: 2,
    })
    await wrapper.get('.find input').setValue('RJ01526615')
    await wrapper.get('.find input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('works')
    wrapper.unmount()
  })

  it('discards a pending jump when the input changes', async () => {
    const { wrapper, router, listWorks } = await mountWorks()
    let resolve!: (value: { works: WorkSummary[], total: number }) => void
    listWorks.mockImplementation(() => new Promise((done) => { resolve = done }))
    await wrapper.get('.find input').setValue('RJ01526615')
    await wrapper.get('.find input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    await wrapper.get('.find input').setValue('another query')
    resolve({
      works: makeWorks(1_526_629, 1),
      total: 1,
    })
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('works')
    wrapper.unmount()
  })

  it('does not submit a composing Enter', async () => {
    const { wrapper, router, listWorks } = await mountWorks()
    listWorks.mockResolvedValue({
      works: makeWorks(1_526_629, 1),
      total: 1,
    })
    await wrapper.get('.find input').setValue('RJ01526615')
    await wrapper.get('.find input').trigger('keydown', { key: 'Enter', isComposing: true })
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('works')
    wrapper.unmount()
  })

  it('leaves keys typed into FIND to the text box', async () => {
    const { wrapper } = await mountWorks(`/works?rj=${A1}`, true)
    const find = wrapper.get<HTMLInputElement>('.find input').element

    find.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true }))
    find.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    expect(inspected(wrapper)).toBe(A1)
    wrapper.unmount()
  })

  it('never turns Enter on a link into opening the selected work', async () => {
    const { wrapper, router } = await mountWorks(`/works?rj=${A1}`, true)
    const link = wrapper.get('.lib-foot a').element

    link.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('works')
    wrapper.unmount()
  })

  it('has no inspector on a phone, where a tap routes to the detail page', async () => {
    vi.spyOn(window, 'matchMedia').mockImplementation((query: string) => ({
      matches: query === '(max-width: 768px)',
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: () => false,
    }))
    const { wrapper } = await mountWorks('/works')

    expect(wrapper.find('[aria-label="Inspector"]').exists()).toBe(false)
    vi.restoreAllMocks()
  })

  describe('the address owns the view', () => {
    it('writes every filter to the address and reads it back after a reload', async () => {
      const first = await mountWorks('/works')
      await pick(first.wrapper, 'VOICE', '伊ヶ崎綾香')
      await pick(first.wrapper, 'AGE', 'R18')
      const address = first.router.currentRoute.value.fullPath
      expect(first.router.currentRoute.value.query).toMatchObject({ voice: 'va-1', age: 'r18', library: LIB_A })
      first.wrapper.unmount()

      // A fresh page (new store, new cache) on the same address.
      const reloaded = await mountWorks(address)
      expect(lastParams(reloaded.listWorks)).toMatchObject({
        library: LIB_A,
        voice_actor: 'va-1',
        voice_actor_match: 'any',
        age_category: 3,
      })
      const tab = (label: string) =>
        reloaded.wrapper.findAll('.rs').find(candidate => candidate.find('.rs-lbl').text() === label)!
      expect(tab('VOICE').get('.rs-value').text()).toBe('伊ヶ崎綾香')
      expect(tab('AGE').get('.rs-value').text()).toBe('R18')
    })

    it('restores an address with another library without dropping its filters', async () => {
      const { listWorks } = await mountWorks(`/works?library=${LIB_B}&voice=va-1`)

      expect(lastParams(listWorks)).toMatchObject({ library: LIB_B, voice_actor: 'va-1' })
    })

    it('follows a change to its own address made from outside the page', async () => {
      const { router, listWorks } = await mountWorks('/works')

      await router.replace({
        query: { ...router.currentRoute.value.query, voice: 'va-1', q: 'moon' },
      })
      await flushPromises()
      expect(lastParams(listWorks)).toMatchObject({ voice_actor: 'va-1', q: 'moon' })
    })

    it('lets an address change replace the FIND draft and cancel its pending write', async () => {
      vi.useFakeTimers()
      try {
        const { wrapper, router } = await mountWorks('/works')
        await wrapper.get('.find input').setValue('typed')
        await router.replace({
          query: { ...router.currentRoute.value.query, q: 'linked' },
        })
        await flushPromises()
        expect(wrapper.get<HTMLInputElement>('.find input').element.value).toBe('linked')

        vi.advanceTimersByTime(500)
        await flushPromises()
        expect(router.currentRoute.value.query.q).toBe('linked')
      }
      finally {
        vi.useRealTimers()
      }
    })
  })

  describe('pages', () => {
    it('turns pages keeping the filters, and starts again at page 1 when they change', async () => {
      const { wrapper, router, listWorks } = await mountWorks('/works?size=36')
      await pick(wrapper, 'AGE', 'R18')

      await wrapper.get('[aria-label="Page 2"]').trigger('click')
      await flushPromises()
      expect(lastParams(listWorks)).toMatchObject({ page: 2, limit: 36, age_category: 3 })
      expect(router.currentRoute.value.query.page).toBe('2')
      // The page's first work is selected, not one from page 1.
      expect(inspected(wrapper)).toBe(worksA[36].rj)

      await pick(wrapper, 'AGE', 'R15')
      expect(lastParams(listWorks)).toMatchObject({ page: 1, age_category: 2 })
      expect(router.currentRoute.value.query.page).toBeUndefined()

      await wrapper.get('[aria-label="Page 2"]').trigger('click')
      await flushPromises()
      await pick(wrapper, 'SIZE', '60')
      expect(lastParams(listWorks)).toMatchObject({ page: 1, limit: 60 })
    })

    it('reads the same page range from one link whatever the local size preference', async () => {
      localStorage.setItem('works.pageSize', '120')
      const named = await mountWorks('/works?page=2')
      // A named page with no size is the fixed default, never the preference.
      expect(named.listWorks.mock.calls[0][0]).toMatchObject({ page: 2, limit: 60 })
      // Correcting the page past the end writes the size in effect, not the preference.
      expect(named.router.currentRoute.value.query.size).toBe('60')
      named.wrapper.unmount()

      // A fresh visit takes the preference once, and the address carries it.
      const fresh = await mountWorks('/works')
      expect(fresh.router.currentRoute.value.query.size).toBe('120')
      expect(lastParams(fresh.listWorks)).toMatchObject({ limit: 120 })
    })

    it('corrects a page past the end only once the listing has answered', async () => {
      let answer!: (value: ReturnType<typeof pageOf>) => void
      const listWorks = vi.fn((params: { page?: number, limit?: number }) =>
        params.page === 9
          ? new Promise((done) => { answer = done })
          : Promise.resolve(pageOf(params)))
      const { router } = await mountWorks('/works?size=36&page=9', false, { listWorks })

      // Still waiting: nothing is corrected from what is not yet known.
      expect(router.currentRoute.value.query.page).toBe('9')
      answer({ works: [], total: worksA.length })
      await flushPromises()
      expect(router.currentRoute.value.query.page).toBe('2')
      expect(lastParams(listWorks)).toMatchObject({ page: 2 })
    })

    it('says so when nothing matches, and selects nothing', async () => {
      const listWorks = vi.fn(() => Promise.resolve({ works: [], total: 0 }))
      const { wrapper, router } = await mountWorks(`/works?rj=${A1}`, false, { listWorks })

      expect(wrapper.text()).toContain('没有作品')
      expect(router.currentRoute.value.query.rj).toBeUndefined()
    })

    it('turns the page from the keyboard off either end of it', async () => {
      const { wrapper, router, listWorks } = await mountWorks(`/works?size=36&rj=${worksA[35].rj}`)

      press('ArrowRight')
      await flushPromises()
      expect(lastParams(listWorks)).toMatchObject({ page: 2 })
      expect(inspected(wrapper)).toBe(worksA[36].rj)

      press('ArrowLeft')
      await flushPromises()
      // Page 1 answers from the cache; the address says where the wall is.
      expect(router.currentRoute.value.query.page).toBeUndefined()
      // Back off the first work lands on the previous page's last.
      expect(inspected(wrapper)).toBe(worksA[35].rj)

      press('PageDown')
      await flushPromises()
      expect(inspected(wrapper)).toBe(worksA[36].rj)
    })
  })

  describe('leaving the wall and coming back', () => {
    const scrollerOf = (h: Harness) => h.wrapper.get('[data-testid="work-wall"]').element as HTMLElement

    /**
     * Leaves the wall by `leave`, then steps back to its entry. With `cold` the
     * listing is gone from the cache and its next request stays pending until
     * the returned `answer` is called.
     */
    async function leaveAndReturn(h: Harness, leave: () => Promise<unknown> | unknown, cold = false) {
      await leave()
      await flushPromises()
      expect(h.router.currentRoute.value.path).not.toBe('/works')
      let answer = (_value: ReturnType<typeof pageOf>) => {}
      if (cold) {
        h.listWorks.mockImplementation(() => new Promise((done) => { answer = done }))
        h.queryClient.clear()
      }
      h.router.back()
      await flushPromises()
      return (value: ReturnType<typeof pageOf>) => answer(value)
    }

    it('commits the work and restores the wall\'s position, from every entry', async () => {
      for (const open of [
        (h: Harness) => h.wrapper.get(`[data-rj="${A3}"]`).trigger('dblclick'),
        (h: Harness) => {
          void h.wrapper.get(`[data-rj="${A3}"]`).trigger('click')
          return flushPromises().then(() => press('Enter'))
        },
      ]) {
        const h = await mountWorks('/works?size=36')
        scrollerOf(h).scrollTop = 120
        await leaveAndReturn(h, () => open(h))
        expect(h.router.currentRoute.value.query.rj).toBe(A3)
        expect(scrollerOf(h).scrollTop).toBe(120)
        h.wrapper.unmount()
      }
    })

    it('restores the position after any other way off the wall', async () => {
      const h = await mountWorks('/works?size=36')
      scrollerOf(h).scrollTop = 80
      await leaveAndReturn(h, () => h.router.push('/collections'))
      expect(scrollerOf(h).scrollTop).toBe(80)
    })

    it('does not write the wall\'s address over the destination when leaving by BACK', async () => {
      const h = await mountWorks('/collections')
      await h.router.push('/works?size=60')
      await flushPromises()
      expect(h.wrapper.find('[data-testid="work-wall"]').exists()).toBe(true)

      h.router.back()
      await flushPromises()
      expect(h.router.currentRoute.value.fullPath).toBe('/collections')
      expect(h.router.options.history.location).toBe('/collections')
      expect(h.router.options.history.state.wallReturn).toBeUndefined()
    })

    it('restores exactly the scroll offset and focuses the selection once a slow listing lands', async () => {
      const h = await mountWorks('/works?size=36', true)
      scrollerOf(h).scrollTop = 120
      // The spy records where the scroller stood when the cell was brought into view.
      const seen: Array<{ rj?: string, top: number }> = []
      HTMLElement.prototype.scrollIntoView = function (this: HTMLElement) {
        seen.push({ rj: this.dataset.rj, top: scrollerOf(h).scrollTop })
      }
      onTestFinished(() => { delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView })
      const answer = await leaveAndReturn(h, () => h.wrapper.get(`[data-rj="${A3}"]`).trigger('dblclick'), true)
      expect(document.activeElement).toBe(document.body)

      answer(pageOf({ limit: 36 }))
      await flushPromises()
      expect(scrollerOf(h).scrollTop).toBe(120)
      // The saved offset is the user's place: the restore does not pull the selection into view.
      expect(seen).not.toContainEqual({ rj: A3, top: 120 })
      expect((document.activeElement as HTMLElement).dataset.rj).toBe(A3)
      // Used once: the entry no longer asks for a restore.
      expect(h.router.options.history.state.wallReturn).toBeNull()
    })

    it('does not take focus when something else already holds it', async () => {
      const h = await mountWorks('/works?size=36', true)
      const answer = await leaveAndReturn(h, () => h.wrapper.get(`[data-rj="${A3}"]`).trigger('dblclick'), true)
      const find = h.wrapper.get<HTMLInputElement>('.find input').element
      find.focus()

      answer(pageOf({ limit: 36 }))
      await flushPromises()
      expect(document.activeElement).toBe(find)
    })
  })

  describe('the wall\'s library comes from its address', () => {
    it('fills a bare address with the remembered wall\'s library, not the workbench\'s', async () => {
      const { router, listWorks } = await mountWorks('/works?voice=va-1', false, {
        seed: (ui) => {
          ui.setActiveLibrary(LIB_B)
          ui.wallQuery = { library: LIB_A }
        },
      })

      expect(router.currentRoute.value.query).toMatchObject({ library: LIB_A, voice: 'va-1' })
      expect(lastParams(listWorks)).toMatchObject({ library: LIB_A })
    })

    it('fills with the remembered ALL scope as well', async () => {
      const { router } = await mountWorks('/works', false, {
        seed: (ui) => { ui.wallQuery = { library: 'all' } },
      })

      expect(router.currentRoute.value.query.library).toBe('all')
    })

    it('fills with the workbench\'s active library without a memory, or when the remembered one is gone', async () => {
      const memories: LocationQuery[] = [
        {},
        { library: 'removed' },
      ]
      for (const wallQuery of memories) {
        const { router } = await mountWorks('/works?voice=va-1', false, {
          seed: (ui) => {
            ui.setActiveLibrary(LIB_B)
            ui.wallQuery = wallQuery
          },
        })
        expect(router.currentRoute.value.query.library).toBe(LIB_B)
      }
    })

    it('fills with the first library when there is neither a memory nor an active library', async () => {
      const { router } = await mountWorks('/works')

      expect(router.currentRoute.value.query.library).toBe(LIB_A)
    })

    it('never changes the workbench\'s active library or selection', async () => {
      const { wrapper } = await mountWorks(`/works?library=${LIB_A}`, false, {
        seed: (ui) => {
          ui.setActiveLibrary(LIB_B)
          ui.selectedDirPaths = ['Alpha']
          ui.overviewScroll = 40
        },
      })
      const ui = useLibraryUiStore()
      await chooseLibrary(wrapper, 'ALL LIBRARIES')
      await chooseLibrary(wrapper, 'Archive')

      expect(ui.activeLibraryId).toBe(LIB_B)
      expect(ui.selectedDirPaths).toEqual(['Alpha'])
      expect(ui.overviewScroll).toBe(40)
    })

    it('writes the switcher\'s pick to the address and drops the previous library\'s picks', async () => {
      const { wrapper, router } = await mountWorks(`/works?library=${LIB_A}&voice=va-1&circle=ci-1&tag=s:s-asmr&mode=any&page=2&age=r18&size=36`)

      await chooseLibrary(wrapper, 'Brazil')

      const query = router.currentRoute.value.query
      expect(query).toMatchObject({ library: LIB_B, age: 'r18', size: '36' })
      for (const key of [
        'voice',
        'circle',
        'tag',
        'mode',
        'page',
      ])
        expect(query).not.toHaveProperty(key)
    })

    it('corrects a library that no longer exists to the first one, dropping its picks', async () => {
      const { router } = await mountWorks('/works?library=removed&voice=va-1&age=r18')

      expect(router.currentRoute.value.query).toMatchObject({ library: LIB_A, age: 'r18' })
      expect(router.currentRoute.value.query).not.toHaveProperty('voice')
    })

    it('lists nothing until the address names a library, and says nothing is too long meanwhile', async () => {
      const { wrapper, router, listWorks, listWorkFacets } = await mountWorks('/works', false, {
        listLibraries: vi.fn(() => new Promise(() => {})),
      })

      expect(router.currentRoute.value.query.library).toBeUndefined()
      expect(listWorks).not.toHaveBeenCalled()
      expect(listWorkFacets).not.toHaveBeenCalled()
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    })

    it('shows the empty state and requests nothing when there are no libraries', async () => {
      const { wrapper, router, listWorks, listWorkFacets } = await mountWorks('/works', false, {
        listLibraries: vi.fn().mockResolvedValue([]),
      })

      expect(wrapper.text()).toContain('没有作品')
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(router.currentRoute.value.query.library).toBeUndefined()
      expect(listWorks).not.toHaveBeenCalled()
      expect(listWorkFacets).not.toHaveBeenCalled()
    })
  })

  describe('the remembered Wall View', () => {
    it('remembers an untouched wall loaded with a full query', async () => {
      await mountWorks(`/works?library=${LIB_B}&size=36&page=1&voice=va-1`)

      expect(useLibraryUiStore().wallQuery).toMatchObject({ library: LIB_B, size: '36', voice: 'va-1' })
    })

    it('is not overwritten by an address that names no library', async () => {
      const { router } = await mountWorks(`/works?library=${LIB_B}&voice=va-1`)
      const ui = useLibraryUiStore()

      await router.replace({
        name: 'works',
        query: { voice: 'va-2' },
      })
      expect(ui.wallQuery).toMatchObject({ library: LIB_B, voice: 'va-1' })
      // The fill then names the remembered library.
      await flushPromises()
      expect(router.currentRoute.value.query).toMatchObject({ library: LIB_B, voice: 'va-2' })
    })

    it('brings the wall back with its filters, page and work after a trip to another page', async () => {
      const { wrapper, router, listWorks } = await mountWorks('/works?size=36')
      await pick(wrapper, 'AGE', 'R18')
      await wrapper.get('[aria-label="Page 2"]').trigger('click')
      await flushPromises()
      const before = { ...router.currentRoute.value.query }
      expect(before).toMatchObject({
        library: LIB_A,
        age: 'r18',
        page: '2',
        rj: worksA[36].rj,
      })

      await router.push('/collections')
      await flushPromises()
      await router.push({ name: 'works', query: useLibraryUiStore().wallQuery })
      await flushPromises()

      expect(router.currentRoute.value.query).toEqual(before)
      expect(lastParams(listWorks)).toMatchObject({
        library: LIB_A,
        age_category: 3,
        page: 2,
        limit: 36,
      })
      expect(inspected(wrapper)).toBe(worksA[36].rj)
    })
  })

  describe('picks that outlive their options', () => {
    it('keeps a narrowed-away voice actor named, after a reload, and clearable', async () => {
      // With R18 applied no work in scope has this voice actor; the library has.
      const listWorkFacets = vi.fn((kind: string, scope: { age_category?: number }) =>
        Promise.resolve(kind === 'voice_actor' && !scope.age_category
          ? [
              { id: 'va-1', name: '伊ヶ崎綾香', work_count: 3 },
            ]
          : []))
      const { wrapper, listWorks } = await mountWorks('/works?age=r18&voice=va-1', false, { listWorkFacets })

      const tab = wrapper.findAll('.rs').find(candidate => candidate.find('.rs-lbl').text() === 'VOICE')!
      expect(tab.get('.rs-value').text()).toBe('伊ヶ崎綾香')
      expect(lastParams(listWorks)).toMatchObject({ voice_actor: 'va-1', age_category: 3 })
      await pick(wrapper, 'VOICE', 'ALL')
      expect(lastParams(listWorks)).not.toHaveProperty('voice_actor')
    })

    it('refuses a pick too long for the address out loud instead of widening the wall', async () => {
      const { wrapper, router, listWorkFacets, listWorks } = await mountWorks('/works')
      // One name without a source id, one id per work: 2,000 of them.
      listWorkFacets.mockResolvedValue(Array.from({ length: 2000 }, (_, i) => ({
        id: `00000000-0000-4000-8000-${String(i).padStart(12, '0')}`,
        name: '同名',
        work_count: 1,
      })))
      await router.replace({
        query: { ...router.currentRoute.value.query, age: 'r15' },
      })
      await flushPromises()
      const calls = listWorks.mock.calls.length

      await pick(wrapper, 'VOICE', '同名')
      expect(wrapper.text()).toContain('筛选条件过多，未应用')
      expect(router.currentRoute.value.query.voice).toBeUndefined()
      expect(listWorks.mock.calls.length).toBe(calls)
    })
  })
})
