import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, createWebHistory } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { CollectionDetail, WorkSummary } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { sampleWorkContent } from '@/test/work-content-fixture'
import RailSelect from '@/features/works/RailSelect.vue'
import CollectionDetailPage from './CollectionDetailPage.vue'

const playWork = vi.fn()
const playFrom = vi.fn()
vi.mock('@/features/works/use-play-work', () => ({ usePlayWork: () => ({ playWork, playFrom }) }))

enableAutoUnmount(afterEach)

const work = (n: number): WorkSummary => ({
  rj: `RJ${String(n).padStart(8, '0')}`, title: `作品${n}`, thumbnail_url: null, circle: `社${n}`, language: null,
  nsfw: false, age_category: 1, release_date: null, dl_count: null, rating: null,
} as WorkSummary)

const ALL = Array.from({ length: 205 }, (_, i) => work(i + 1))

function setup(total = ALL.length) {
  const getCollection = vi.fn(async (_id: string, w: { limit?: number; page?: number } = {}) => {
    const start = ((w.page ?? 1) - 1) * (w.limit ?? 200)
    return { id: 'c1', name: 'Sleep', work_count: total, created_at: '', updated_at: '', total, works: ALL.slice(start, start + (w.limit ?? 200)).slice(0, total) } as CollectionDetail
  })
  return { getCollection, api: apiStub({ getCollection }) }
}

async function mountPage(api: ReturnType<typeof apiStub>, path = '/collections/c1') {
  const stub = { template: '<i />' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/collections', name: 'collections', component: stub },
      { path: '/collections/:id', name: 'collection-detail', component: CollectionDetailPage },
      { path: '/works', name: 'works', component: stub },
      { path: '/works/:rj', name: 'work-detail', component: stub },
    ],
  })
  await router.push(path)
  const wrapper = mount(CollectionDetailPage, {
    attachTo: document.body.appendChild(document.createElement('div')),
    global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, router }
}

const press = (key: string) => document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
const rows = (w: ReturnType<typeof mount>) => w.findAll('[data-testid="collection-work"]')

beforeEach(() => {
  playWork.mockClear()
  playFrom.mockClear()
  vi.unstubAllGlobals()
})

describe('CollectionDetailPage', () => {
  it('lists the saved order, numbered by position, with the name and count in the header', async () => {
    const { getCollection, api } = setup(3)
    const { wrapper } = await mountPage(api)
    expect(getCollection).toHaveBeenCalledWith('c1', { limit: 200, page: 1, direction: 'asc' }, expect.anything())
    expect(wrapper.get('h1').text()).toBe('Sleep')
    expect(rows(wrapper).map((r) => r.get('.pos').text())).toEqual(['1', '2', '3'])
  })

  it('numbers a descending custom order from the end, and other sorts by row', async () => {
    const { api } = setup(3)
    const desc = await mountPage(api, '/collections/c1?direction=desc')
    expect(rows(desc.wrapper).map((r) => r.get('.pos').text())).toEqual(['3', '2', '1'])
    const sorted = await mountPage(api, '/collections/c1?sort=rj')
    expect(rows(sorted.wrapper).map((r) => r.get('.pos').text())).toEqual(['1', '2', '3'])
  })

  it('writes a sort change to the address and requests it, newest first by default', async () => {
    const { getCollection, api } = setup(3)
    const { wrapper, router } = await mountPage(api)
    // The menu is virtualized (no layout in jsdom): choose through its model.
    wrapper.getComponent(RailSelect).vm.$emit('update:modelValue', 'collected_at')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ sort: 'collected_at' })
    expect(getCollection).toHaveBeenLastCalledWith('c1', { limit: 200, page: 1, direction: 'desc', sort: 'collected_at' }, expect.anything())
    await wrapper.get('[data-testid="collection-direction"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ sort: 'collected_at', direction: 'asc' })
    expect(getCollection).toHaveBeenLastCalledWith('c1', { limit: 200, page: 1, direction: 'asc', sort: 'collected_at' }, expect.anything())
  })

  it('loads the next chunk when the end of the list comes into view', async () => {
    let fire: (entries: { isIntersecting: boolean }[]) => void = () => {}
    vi.stubGlobal('IntersectionObserver', class {
      constructor(cb: typeof fire) { fire = cb }
      observe() {} unobserve() {} disconnect() {}
    })
    const { getCollection, api } = setup()
    const { wrapper } = await mountPage(api)
    expect(rows(wrapper)).toHaveLength(200)
    fire([{ isIntersecting: true }])
    await flushPromises()
    expect(getCollection).toHaveBeenLastCalledWith('c1', { limit: 200, page: 2, direction: 'asc' }, expect.anything())
    expect(rows(wrapper)).toHaveLength(205)
    expect(rows(wrapper)[204]!.get('.pos').text()).toBe('205')
  })

  it('moves the cursor with the arrows and opens the work on Enter or double-click', async () => {
    const { api } = setup(3)
    const { wrapper, router } = await mountPage(api)
    expect(rows(wrapper)[0]!.attributes('aria-selected')).toBe('true')
    press('ArrowDown')
    await flushPromises()
    expect(rows(wrapper)[1]!.attributes('aria-selected')).toBe('true')
    press('Enter')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/works/RJ00000002')
    await router.push('/collections/c1')
    await flushPromises()
    await rows(wrapper)[2]!.trigger('dblclick')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/works/RJ00000003')
  })

  it('plays from the row button without opening or selecting-away', async () => {
    const { api } = setup(3)
    const { wrapper, router } = await mountPage(api)
    await rows(wrapper)[1]!.get('[data-testid="collection-play"]').trigger('click')
    expect(playWork).toHaveBeenCalledWith({ rj: 'RJ00000002', title: '作品2' })
    expect(router.currentRoute.value.path).toBe('/collections/c1')
  })

  it('ignores Space typed into a field', async () => {
    const { api } = setup(3)
    await mountPage(api)
    const input = document.body.appendChild(document.createElement('input'))
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    input.remove()
    expect(document.querySelector('[aria-selected="true"]')!.getAttribute('data-rj')).toBe('RJ00000001')
  })

  it('says an empty collection is empty and points to the wall', async () => {
    const { api } = setup(0)
    const { wrapper } = await mountPage(api)
    const empty = wrapper.get('[data-testid="collection-empty"]')
    expect(empty.get('a').attributes('href')).toBe('/works')
    expect(rows(wrapper)).toHaveLength(0)
  })

  it('BACK steps back to the entry it was opened from, else opens the entry fresh', async () => {
    window.history.replaceState(null, '', '/')
    const stub = { template: '<i />' }
    const router = createRouter({
      history: createWebHistory(),
      routes: [
        { path: '/', component: stub },
        { path: '/collections', name: 'collections', component: stub },
        { path: '/collections/:id', name: 'collection-detail', component: CollectionDetailPage },
      ],
    })
    await router.push('/collections')
    await router.push('/collections/c1')
    const before = window.history.length
    const wrapper = mount(CollectionDetailPage, {
      global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: setup(3).api } },
    })
    await flushPromises()
    await wrapper.get('[data-testid="collection-back"]').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/collections'))
    expect(window.history.length).toBe(before)

    await router.push('/')
    await router.push('/collections/c1')
    await wrapper.get('[data-testid="collection-back"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/collections')
  })

  it('unfolds the settled row into its tracklist: a track plays from there, OPEN opens the work, a second click folds', async () => {
    const { getCollection } = setup(3)
    const getWorkContent = vi.fn(async (rj: string) => sampleWorkContent(rj))
    const { wrapper, router } = await mountPage(apiStub({ getCollection, getWorkContent }))
    const folds = () => wrapper.findAll('[data-testid="collection-fold"]')
    // The first row unfolds on arrival.
    await vi.waitFor(() => expect(folds()).toHaveLength(1))
    await rows(wrapper)[1]!.trigger('click')
    await vi.waitFor(() => expect(wrapper.find('[data-rj="RJ00000002"]').element.parentElement!.querySelector('[data-testid="collection-fold"]')).not.toBeNull())
    expect(folds()).toHaveLength(1)
    const fold = folds()[0]!
    await fold.findAll('.trk')[0]!.trigger('click')
    expect(playFrom).toHaveBeenCalledWith({ rj: 'RJ00000002', title: '作品2' }, expect.objectContaining({ rj: 'RJ00000002' }), 0, '')
    await rows(wrapper)[1]!.trigger('click')
    await flushPromises()
    expect(folds()).toHaveLength(0)
    await rows(wrapper)[1]!.trigger('click')
    await vi.waitFor(() => expect(folds()).toHaveLength(1))
    await folds()[0]!.get('[data-testid="collection-open"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/works/RJ00000002')
  })

  it('folds while the keys cruise and unfolds only the row they settle on, after a pause', async () => {
    const { getCollection } = setup(5)
    const getWorkContent = vi.fn(async (rj: string) => sampleWorkContent(rj))
    const { wrapper } = await mountPage(apiStub({ getCollection, getWorkContent }))
    await vi.waitFor(() => expect(wrapper.findAll('[data-testid="collection-fold"]')).toHaveLength(1))
    vi.useFakeTimers()
    try {
      getWorkContent.mockClear()
      press('ArrowDown'); press('ArrowDown'); press('ArrowDown')
      await flushPromises()
      expect(wrapper.findAll('[data-testid="collection-fold"]')).toHaveLength(0)
      await vi.advanceTimersByTimeAsync(150)
      press('ArrowUp')
      await vi.advanceTimersByTimeAsync(150)
      expect(getWorkContent).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(100)
      expect(getWorkContent.mock.calls.map(([rj]) => rj)).toEqual(['RJ00000003'])
      await flushPromises()
      expect(wrapper.get('[data-rj="RJ00000003"]').element.parentElement!.querySelector('[data-testid="collection-fold"]')).not.toBeNull()
      press('Escape')
      await flushPromises()
      expect(wrapper.findAll('[data-testid="collection-fold"]')).toHaveLength(0)
    } finally {
      vi.useRealTimers()
    }
  })
})
