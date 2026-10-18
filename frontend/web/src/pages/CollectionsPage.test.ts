import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, createWebHistory } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { CollectionDetail, WorkCollection, WorkSummary } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'
import CollectionsPage from './CollectionsPage.vue'

enableAutoUnmount(afterEach)

// The page reads its data from the collection list and two windows of the
// detail route; these tests pin which windows it asks for and what it shows.

const collection = (id: string, name: string, count: number): WorkCollection => ({
  id, name, work_count: count, created_at: '', updated_at: '',
})
const work = (n: number, patch: Partial<WorkSummary> = {}): WorkSummary => ({
  rj: `RJ${String(n).padStart(8, '0')}`, title: `作品${n}`, thumbnail_url: `https://img/${n}.jpg`,
  circle: null, language: null, nsfw: false, age_category: 1, release_date: null, dl_count: null,
  rating: null, ...patch,
} as WorkSummary)

const COLS = [collection('favorites', 'Favorites', 3), collection('c2', 'Sleep', 0)]

function setup(works: WorkSummary[] = [work(3), work(2), work(1)]) {
  const getCollection = vi.fn(async (id: string, window: { limit?: number; page?: number; direction?: string } = {}) => {
    const col = COLS.find((c) => c.id === id)!
    const detail: CollectionDetail = {
      ...col, total: col.work_count,
      works: window.limit === 1 ? [works[0]!] : id === 'favorites' ? works : [],
    }
    return detail
  })
  const api = apiStub({
    listCollections: vi.fn().mockResolvedValue({ collections: COLS, total: COLS.length }),
    getCollection,
  })
  return { api, getCollection }
}

async function mountPage(api: ReturnType<typeof apiStub>) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<i />' } }, { path: '/works/:rj', component: { template: '<i />' } }] })
  await router.push('/')
  const wrapper = mount(CollectionsPage, {
    attachTo: document.body.appendChild(document.createElement('div')),
    global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return wrapper
}

const press = (key: string) => document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))

beforeEach(() => {
  vi.useRealTimers()
  localStorage.clear()
})

describe('CollectionsPage', () => {
  it('lists the collections and selects the first', async () => {
    const wrapper = await mountPage(setup().api)
    const rows = wrapper.findAll('[data-testid="collection-row"]')
    expect(rows.map((r) => r.text())).toEqual(['Favorites', 'Sleep'])
    expect(rows[0]!.attributes('aria-selected')).toBe('true')
    expect(wrapper.get('.deck-strip').text()).toBe('Favorites')
  })

  it('says what the selection holds, and what plays once the player runs', async () => {
    const wrapper = await mountPage(setup().api)
    expect(wrapper.get('[data-testid="collections-message"]').text()).toContain('Favorites 收录 3 部作品')
    usePlayerStore().$patch({ rj: 'RJ00000007', title: '作品7', elapsed: 65, duration: null, paused: false })
    await nextTick()
    const now = wrapper.get('[data-testid="collections-now-playing"]')
    expect(now.text()).toContain('RJ00000007')
    expect(now.text()).toContain('01:05 / --:--')
    expect(now.attributes('href')).toBe('/works/RJ00000007')
    expect(wrapper.find('[data-testid="collections-message"]').exists()).toBe(false)
  })

  it('asks for the newest titles and one random member for the cover', async () => {
    const { api, getCollection } = setup()
    await mountPage(api)
    expect(getCollection).toHaveBeenCalledWith('favorites', { limit: 20, direction: 'desc' }, expect.anything())
    const cover = getCollection.mock.calls.find(([, w]) => w?.limit === 1)!
    expect(cover[1]!.page).toBeGreaterThanOrEqual(1)
    expect(cover[1]!.page).toBeLessThanOrEqual(3)
    expect(cover[1]).not.toHaveProperty('direction')
  })

  it('types the newest titles out and cascades the rest, the unprinted part laid out but hidden', async () => {
    const long = '长'.repeat(90)
    const wrapper = await mountPage(setup([work(1, { title: long }), work(2, { title: long })]).api)
    const typedOf = (i: number) => wrapper.findAll('.ti')[i]!.get('span').element.firstChild?.textContent?.length ?? 0
    expect(typedOf(0)).toBeLessThan(long.length)
    expect(typedOf(1)).toBe(0) // typed lines print in order
    expect(wrapper.findAll('.ti')[0]!.text()).toBe(long) // laid out in full, so measuring is stable
    await vi.waitFor(() => expect(typedOf(1)).toBe(long.length), { timeout: 2000 }) // a whole line at once
  })

  it('prints instantly once the user switches more than twice in quick succession', async () => {
    const long = '长'.repeat(90)
    const wrapper = await mountPage(setup([work(1, { title: long })]).api)
    const typedOf = () => wrapper.get('.ti span').element.firstChild?.textContent?.length ?? 0
    expect(typedOf()).toBeLessThan(long.length)
    press('ArrowDown'); press('ArrowUp')
    await nextTick()
    expect(typedOf()).toBeLessThan(long.length) // two switches still animate
    press('ArrowDown')
    await nextTick()
    expect(typedOf()).toBe(long.length)
  })

  it('shows the cover as the picture box background, from the remote link when no local file', async () => {
    const wrapper = await mountPage(setup().api)
    expect(wrapper.get('[data-testid="collection-screen"]').attributes('style')).toContain('https://img/3.jpg')
  })

  it('hatches the cover of an R18 work while the veil is on', async () => {
    localStorage.setItem('preen.r18-veil', '1')
    const { api } = setup([work(3, { nsfw: true })])
    const wrapper = await mountPage(api)
    expect(useLibraryUiStore().veil).toBe(true)
    expect(wrapper.get('[data-testid="collection-screen"]').classes()).toContain('empty')
  })

  it('moves the selection with the arrow keys and switches the detail once it settles', async () => {
    const { api } = setup()
    const wrapper = await mountPage(api)
    press('ArrowDown')
    await nextTick()
    expect(wrapper.findAll('[data-testid="collection-row"]')[1]!.attributes('aria-selected')).toBe('true')
    await new Promise((r) => setTimeout(r, 200))
    await flushPromises()
    expect(wrapper.get('.deck-strip').text()).toBe('Sleep')
    // An empty collection has no cover and no titles.
    expect(wrapper.get('[data-testid="collection-screen"]').classes()).toContain('empty')
    expect(wrapper.findAll('.ti')).toHaveLength(0)
  })

  it('ignores page keys typed into a field', async () => {
    const wrapper = await mountPage(setup().api)
    const input = document.body.appendChild(document.createElement('input'))
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    expect(wrapper.findAll('[data-testid="collection-row"]')[0]!.attributes('aria-selected')).toBe('true')
    input.remove()
  })

  it('draws another member when R is pressed', async () => {
    const { api, getCollection } = setup()
    await mountPage(api)
    const before = getCollection.mock.calls.filter(([, w]) => w?.limit === 1).length
    press('r')
    await flushPromises()
    expect(getCollection.mock.calls.filter(([, w]) => w?.limit === 1).length).toBe(before + 1)
  })

  describe('opening a collection', () => {
    async function mountWeb(api: ReturnType<typeof apiStub>, state: object | null = null) {
      window.history.replaceState(state, '', '/collections')
      const stub = { template: '<i />' }
      const router = createRouter({ history: createWebHistory(), routes: [
        { path: '/collections', name: 'collections', component: stub },
        { path: '/collections/:id', name: 'collection-detail', component: stub },
      ] })
      await router.push('/collections')
      const wrapper = mount(CollectionsPage, {
        attachTo: document.body.appendChild(document.createElement('div')),
        global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
      })
      await flushPromises()
      return { wrapper, router }
    }

    it('opens the selected collection on Enter and on a second click, but not on the first', async () => {
      const { wrapper, router } = await mountWeb(setup().api)
      await wrapper.findAll('[data-testid="collection-row"]')[1]!.trigger('click')
      expect(router.currentRoute.value.path).toBe('/collections') // the first click only selects
      press('Enter')
      await flushPromises()
      expect(router.currentRoute.value.fullPath).toBe('/collections/c2')
    })

    it('records the id on its own entry, and restores it only on return there', async () => {
      const { wrapper } = await mountWeb(setup().api)
      await wrapper.findAll('[data-testid="collection-row"]')[1]!.trigger('click')
      press('Enter')
      await flushPromises()
      wrapper.unmount()
      window.history.back()
      await new Promise((r) => setTimeout(r, 50))
      expect(window.history.state.collectionReturn).toBe('c2')
      const saved = { ...window.history.state }

      const back = await mountWeb(setup().api, saved)
      expect(back.wrapper.findAll('[data-testid="collection-row"]')[1]!.attributes('aria-selected')).toBe('true')
      expect(back.router.currentRoute.value.fullPath).toBe('/collections') // the address stays query-free
      const fresh = await mountWeb(setup().api)
      expect(fresh.wrapper.findAll('[data-testid="collection-row"]')[0]!.attributes('aria-selected')).toBe('true')
    })
  })

  describe('creating', () => {
    it('+ NEW creates 新合集 #N, counting user collections only, and opens it', async () => {
      const stub = { template: '<i />' }
      const router = createRouter({ history: createMemoryHistory(), routes: [
        { path: '/', component: stub },
        { path: '/collections/:id', name: 'collection-detail', component: stub },
      ] })
      await router.push('/')
      const createCollection = vi.fn().mockResolvedValue(collection('c9', '新合集 #2', 0))
      const api = apiStub({ ...setup().api, createCollection })
      const wrapper = mount(CollectionsPage, {
        attachTo: document.body.appendChild(document.createElement('div')),
        global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
      })
      await flushPromises()
      await wrapper.get('[data-testid="collections-new"]').trigger('click')
      await flushPromises()
      expect(createCollection).toHaveBeenCalledWith('新合集 #2') // Favorites + Sleep: one user collection
      expect(router.currentRoute.value.fullPath).toBe('/collections/c9')
    })
  })
})