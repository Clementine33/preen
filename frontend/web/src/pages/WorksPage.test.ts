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

/** Real work numbers: the address only keeps a canonical `RJ` + 8 digits. */
function rjOf(n: number): string {
  return `RJ${String(n).padStart(8, '0')}`
}

function makeWorks(base: number, count: number): WorkSummary[] {
  return Array.from({ length: count }, (_, i) => ({
    rj: rjOf(base + i),
    title: `Title ${rjOf(base + i)}`,
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
// Below RJ01000000, so the listing's numbers also have a 6-digit spelling.
const worksA = makeWorks(100_000, 6)
const worksB = makeWorks(200_000, 2)
const [A0, A1, A2, A3] = worksA.map((work) => work.rj)

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

async function mountWorks(
  initialPath = '/works',
  attach = false,
): Promise<{
  wrapper: VueWrapper
  router: Router
  listWorks: ReturnType<typeof vi.fn>
  listWorkFacets: ReturnType<typeof vi.fn>
}> {
  const listWorks = vi.fn((params: { library?: string }) =>
    Promise.resolve({ works: params.library === LIB_B ? worksB : worksA }),
  )
  const listWorkFacets = vi.fn((kind: string) =>
    Promise.resolve(
      kind === 'voice_actor'
        ? [{ id: 'va-1', name: '伊ヶ崎綾香', work_count: 3 }]
        : [{ id: 'ci-1', name: 'mete0r', work_count: 2 }],
    ),
  )
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue([library(LIB_A, 'Archive', '/music'), library(LIB_B, 'Brazil', '/brazil')]),
    listWorks,
    listWorkFacets,
  } as never)
  const router = routerFor()
  await router.push(initialPath)
  await router.isReady()
  const wrapper = mount(WorksPage, {
    ...(attach ? { attachTo: document.body.appendChild(document.createElement('div')) } : {}),
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, router, listWorks, listWorkFacets }
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

/** Opens one rail select by its label and picks an option by its text. */
async function pick(wrapper: VueWrapper, label: string, option: string): Promise<void> {
  const select = wrapper.findAll('.rs').find((candidate) => candidate.find('.rs-lbl').text() === label)!
  await select.get('.rs-trigger').trigger('click')
  const item = select.findAll('[role="option"]').find((candidate) => candidate.find('.rs-name').text() === option)!
  await item.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  localStorage.clear()
  document.body.innerHTML = ''
})

describe('works page', () => {
  it('restores the inspected work from ?rj= and follows the keyboard in the address', async () => {
    const { wrapper, router } = await mountWorks(`/works?rj=${A2}`)

    // The address's work survived the first listing: it is what is inspected.
    expect(inspected(wrapper)).toBe(A2)

    press('j')
    await flushPromises()
    expect(inspected(wrapper)).toBe(A3)
    // Replace, not push: inspecting a neighbour is one state, not a page in
    // history.
    expect(router.currentRoute.value.query.rj).toBe(A3)
  })

  it('reads the 6-digit spelling of a work number as the canonical one', async () => {
    // RJ00100002 is also spelled rj100002.
    const { wrapper } = await mountWorks(`/works?rj=rj${A2.slice(4)}`)

    expect(inspected(wrapper)).toBe(A2)
  })

  it('keeps an address work beyond the loaded pages inspected until the filters change', async () => {
    const beyond = rjOf(9_000_000)
    const { wrapper, listWorks } = await mountWorks(`/works?rj=${beyond}`)

    // Not on the first page, still the inspected work: a refresh keeps it.
    expect(inspected(wrapper)).toBe(beyond)

    // The first move lands on the wall's first work.
    press('j')
    await flushPromises()
    expect(inspected(wrapper)).toBe(A0)
    expect(listWorks).toHaveBeenCalled()
  })

  it('defaults the selection to the first work and writes it to the address', async () => {
    const { router } = await mountWorks('/works')

    expect(router.currentRoute.value.query.rj).toBe(A0)
  })

  it('resets the selection when switching libraries', async () => {
    const { wrapper, router } = await mountWorks(`/works?rj=${A3}`)

    useLibraryUiStore().setActiveLibrary(LIB_B)
    await flushPromises()

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
    await pick(wrapper, 'SORT', '添加时间')
    expect(lastParams(listWorks)).toMatchObject({ sort: 'collected_at', direction: 'desc' })
  })

  it('sends each rail filter straight to its listing parameter', async () => {
    const { wrapper, listWorks } = await mountWorks('/works')

    await pick(wrapper, 'AGE', 'R18')
    expect(lastParams(listWorks)).toMatchObject({ nsfw: true })
    await pick(wrapper, 'AGE', 'UNKNOWN')
    expect(lastParams(listWorks)).toMatchObject({ nsfw: 'unknown' })

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

  it('offers a name held by several per-work entities once, asking for all of them', async () => {
    const { wrapper, listWorks, listWorkFacets } = await mountWorks('/works')
    listWorkFacets.mockResolvedValue([
      { id: 'va-7', name: '悠木碧', work_count: 1 },
      { id: 'va-9', name: '悠木碧', work_count: 2 },
    ])
    useLibraryUiStore().setActiveLibrary(LIB_B)
    await flushPromises()

    await pick(wrapper, 'VOICE', '悠木碧')
    expect(lastParams(listWorks)).toMatchObject({ voice_actor: 'va-7,va-9', voice_actor_match: 'any' })
  })

  it('narrows a long facet list by name', async () => {
    const { wrapper } = await mountWorks('/works')
    const select = wrapper.findAll('.rs').find((candidate) => candidate.find('.rs-lbl').text() === 'VOICE')!
    await select.get('.rs-trigger').trigger('click')

    await select.get('input[type="search"]').setValue('nobody')
    // The empty entry stays reachable, so clearing is still one step.
    expect(select.findAll('[role="option"]').map((item) => item.find('.rs-name').text())).toEqual(['ALL'])
  })

  it('drops the voice actor and circle of the previous library', async () => {
    const { wrapper, listWorks } = await mountWorks('/works')
    await pick(wrapper, 'VOICE', '伊ヶ崎綾香')

    useLibraryUiStore().setActiveLibrary(LIB_B)
    await flushPromises()

    expect(lastParams(listWorks)).toMatchObject({ library: LIB_B })
    expect(lastParams(listWorks)).not.toHaveProperty('voice_actor')
  })

  it('plays the selected work with P and never lets Space change what plays', async () => {
    const { wrapper } = await mountWorks(`/works?rj=${A1}`)

    press('p')
    await flushPromises()
    const player = usePlayerStore()
    expect(player.rj).toBe(A1)
    expect(player.paused).toBe(false)

    press('j') // browse away from the playing work
    await flushPromises()
    expect(inspected(wrapper)).toBe(A2)
    expect(player.rj).toBe(A1) // browsing never interrupts playback

    press(' ')
    await flushPromises()
    expect(player.paused).toBe(true)
    expect(player.rj).toBe(A1) // pausing never changes the work either
  })

  it('opens the detail page with Enter', async () => {
    const { router } = await mountWorks(`/works?rj=${A2}`)

    press('Enter')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('work-detail')
    expect(router.currentRoute.value.params.rj).toBe(A2)
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
})
