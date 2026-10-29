import type { CollectionDetail, WorkSummary } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, createWebHistory } from 'vue-router'
import { useToast } from '@/composables/use-toast'
import RailSelect from '@/features/works/RailSelect.vue'
import { apiClientKey, ApiError } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { sampleWorkContent } from '@/test/work-content-fixture'
import CollectionDetailPage from './CollectionDetailPage.vue'

const playWork = vi.fn()
const playFrom = vi.fn()
vi.mock('@/features/works/use-play-work', () => ({
  usePlayWork: () => ({ playWork, playFrom }),
}))

enableAutoUnmount(afterEach)

function work(n: number): WorkSummary {
  return {
    rj: `RJ${String(n).padStart(8, '0')}`,
    title: `作品${n}`,
    thumbnail_url: null,
    circle: `社${n}`,
    language: null,
    nsfw: false,
    age_category: 1,
    release_date: null,
    dl_count: null,
    rating: null,
  } as WorkSummary
}

const ALL = Array.from({ length: 205 }, (_, i) => work(i + 1))

function setup(total = ALL.length) {
  const getCollection = vi.fn(async (_id: string, w: { limit?: number, page?: number } = {}) => {
    const start = ((w.page ?? 1) - 1) * (w.limit ?? 200)
    return {
      id: 'c1',
      name: 'Sleep',
      work_count: total,
      created_at: '',
      updated_at: '',
      total,
      works: ALL.slice(start, start + (w.limit ?? 200)).slice(0, total),
    } as CollectionDetail
  })
  return {
    getCollection,
    api: apiStub({ getCollection }),
  }
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
    global: {
      plugins: [
        createPinia(),
        router,
        installTestQueryPlugin(),
      ],
      provide: { [apiClientKey as symbol]: api },
    },
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

describe('collectionDetailPage', () => {
  it('lists the saved order, numbered by position, with the name and count in the header', async () => {
    const { getCollection, api } = setup(3)
    const { wrapper } = await mountPage(api)
    expect(getCollection).toHaveBeenCalledWith('c1', { limit: 200, page: 1, direction: 'asc' }, expect.anything())
    expect(wrapper.get('h1').text()).toBe('Sleep')
    expect(rows(wrapper).map(r => r.get('.pos').text())).toEqual(['1', '2', '3'])
  })

  it('numbers a descending custom order from the end, and other sorts by row', async () => {
    const { api } = setup(3)
    const desc = await mountPage(api, '/collections/c1?direction=desc')
    expect(rows(desc.wrapper).map(r => r.get('.pos').text())).toEqual(['3', '2', '1'])
    const sorted = await mountPage(api, '/collections/c1?sort=rj')
    expect(rows(sorted.wrapper).map(r => r.get('.pos').text())).toEqual(['1', '2', '3'])
  })

  it('writes a sort change to the address and requests it, newest first by default', async () => {
    const { getCollection, api } = setup(3)
    const { wrapper, router } = await mountPage(api)
    // The menu is virtualized (no layout in jsdom): choose through its model.
    wrapper.getComponent(RailSelect).vm.$emit('update:modelValue', 'collected_at')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ sort: 'collected_at' })
    expect(getCollection).toHaveBeenLastCalledWith('c1', {
      limit: 200,
      page: 1,
      direction: 'desc',
      sort: 'collected_at',
    }, expect.anything())
    await wrapper.get('[data-testid="collection-direction"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ sort: 'collected_at', direction: 'asc' })
    expect(getCollection).toHaveBeenLastCalledWith('c1', {
      limit: 200,
      page: 1,
      direction: 'asc',
      sort: 'collected_at',
    }, expect.anything())
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
    fire([
      { isIntersecting: true },
    ])
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

  it('bACK steps back to the entry it was opened from, else opens the entry fresh', async () => {
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
      global: {
        plugins: [
          createPinia(),
          router,
          installTestQueryPlugin(),
        ],
        provide: { [apiClientKey as symbol]: setup(3).api },
      },
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
    }
    finally {
      vi.useRealTimers()
    }
  })

  describe('editing', () => {
    const toast = useToast()
    beforeEach(() => { toast.current.value = null })

    // jsdom has no DataTransfer: a bare event with the few members the handlers read.
    function drag(el: Element, type: string, clientY = 0) {
      const e = Object.assign(new Event(type, { bubbles: true, cancelable: true }), {
        clientY,
        dataTransfer: {
          setData() {},
          setDragImage() {},
          effectAllowed: '',
        },
      })
      el.dispatchEvent(e)
    }
    const dropOn = (wrapper: ReturnType<typeof mount>, from: number, to: number, below = false) => {
      drag(rows(wrapper)[from]!.get('.handle').element, 'dragstart')
      drag(rows(wrapper)[to]!.element, 'dragover', below ? 10 : 0)
      drag(rows(wrapper)[to]!.element, 'drop', below ? 10 : 0)
    }
    // A server that keeps the saved order, so the re-read after a write agrees with it.
    const edited = (n = 4) => {
      let saved = ALL.slice(0, n)
      const getCollection = vi.fn(async (_id: string, w: { direction?: string, sort?: string } = {}) => {
        const view = w.sort
          ? [...saved].sort((a, b) => a.rj.localeCompare(b.rj))
          : [...saved]
        if (w.direction === 'desc')
          view.reverse()
        return {
          id: 'c1',
          name: 'Sleep',
          work_count: saved.length,
          created_at: '',
          updated_at: '',
          total: saved.length,
          works: view,
        } as CollectionDetail
      })
      const moveCollectionMember = vi.fn(async (_id: string, rj: string, before: string | null) => {
        const w = saved.find(x => x.rj === rj)!
        saved = saved.filter(x => x !== w)
        saved.splice(before === null
          ? saved.length
          : saved.findIndex(x => x.rj === before), 0, w)
      })
      const removeFromCollection = vi.fn(async (_id: string, rj: string) => { saved = saved.filter(x => x.rj !== rj) })
      const appendToCollection = vi.fn(async (_id: string, rjs: string[]) => {
        saved = [
          ...saved,
          ...rjs.map(rj => ALL.find(x => x.rj === rj)!),
        ]
        return { added: rjs, skipped: [], work_count: saved.length }
      })
      return {
        getCollection,
        moveCollectionMember,
        removeFromCollection,
        appendToCollection,
        api: apiStub({
          getCollection,
          moveCollectionMember,
          removeFromCollection,
          appendToCollection,
        }),
      }
    }
    const mountEditing = async (api: ReturnType<typeof apiStub>, path?: string) => {
      const mounted = await mountPage(api, path)
      await mounted.wrapper.get('[data-testid="collection-edit"]').trigger('click')
      return mounted
    }
    const openMenu = async (wrapper: ReturnType<typeof mount>, row: number) => {
      rows(wrapper)[row]!.get('[data-testid="collection-more"]').element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
      await flushPromises()
    }
    const menu = async (wrapper: ReturnType<typeof mount>, row: number, item: string) => {
      await openMenu(wrapper, row)
      document.body.querySelector<HTMLElement>(`[data-testid="${item}"]`)!.click()
      await flushPromises()
    }

    it('edits only in edit mode: EDIT swaps play for the row menu and handles, Esc leaves it', async () => {
      const { api, removeFromCollection } = edited()
      const { wrapper } = await mountPage(api)
      expect(wrapper.findAll('[data-testid="collection-handle"]')).toHaveLength(0)
      expect(wrapper.findAll('[data-testid="collection-more"]')).toHaveLength(0)
      press('Delete')
      await flushPromises()
      expect(removeFromCollection).not.toHaveBeenCalled()
      await wrapper.get('[data-testid="collection-edit"]').trigger('click')
      expect(wrapper.get('[data-testid="collection-edit"]').text()).toBe('DONE')
      expect(wrapper.findAll('[data-testid="collection-handle"]')).toHaveLength(4)
      expect(wrapper.findAll('[data-testid="collection-more"]')).toHaveLength(4)
      expect(wrapper.findAll('[data-testid="collection-play"]')).toHaveLength(0)
      expect(wrapper.find('[data-testid="collection-fold"]').exists()).toBe(false)
      await rows(wrapper)[1]!.trigger('dblclick')
      await flushPromises()
      expect(wrapper.vm.$route.name).toBe('collection-detail')
      press('Escape')
      await flushPromises()
      expect(wrapper.get('[data-testid="collection-edit"]').text()).toBe('EDIT')
      expect(wrapper.findAll('[data-testid="collection-play"]')).toHaveLength(4)
    })

    it('mounts one row menu at a time, on its first press, already open', async () => {
      const { api } = edited()
      const { wrapper } = await mountEditing(api)
      const triggers = () => wrapper.findAll('[data-testid="collection-more"][aria-expanded]')
      expect(triggers()).toHaveLength(0)
      // jsdom has no PointerEvent; the handler reads only button and ctrlKey.
      const pressMore = (row: number, button: number) => rows(wrapper)[row]!.get('[data-testid="collection-more"]').element.dispatchEvent(new MouseEvent('pointerdown', { button, bubbles: true, cancelable: true }))
      // Escape goes to the menu, as a real key does from its focus; at the page it would end the edit.
      const closeMenu = () => document.body.querySelector('[role="menu"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
      pressMore(2, 0)
      await flushPromises()
      expect(document.body.querySelector('[data-testid="collection-move-top"]')).not.toBeNull()
      expect(triggers().map(t => t.element.closest('.row')?.getAttribute('data-rj'))).toEqual(['RJ00000003'])
      // A secondary press mounts nothing; as a press outside, it closes the open menu.
      pressMore(1, 2)
      await flushPromises()
      expect(triggers()).toHaveLength(1)
      expect(document.body.querySelector('[role="menu"]')).toBeNull()
      pressMore(0, 0)
      await flushPromises()
      expect(triggers().map(t => t.element.closest('.row')?.getAttribute('data-rj'))).toEqual(['RJ00000001'])
      closeMenu()
      await flushPromises()
      await wrapper.get('[data-testid="collection-edit"]').trigger('click')
      await wrapper.get('[data-testid="collection-edit"]').trigger('click')
      expect(triggers()).toHaveLength(0)
    })

    it('drops a row before the row under the gap in ascending order, the bottom meaning the end', async () => {
      const { api, moveCollectionMember } = edited()
      const { wrapper } = await mountEditing(api)
      dropOn(wrapper, 3, 1)
      await flushPromises()
      expect(moveCollectionMember).toHaveBeenLastCalledWith('c1', 'RJ00000004', 'RJ00000002')
      expect(rows(wrapper).map(r => r.attributes('data-rj'))).toEqual([
        'RJ00000001',
        'RJ00000004',
        'RJ00000002',
        'RJ00000003',
      ])
      expect(rows(wrapper)[1]!.attributes('aria-selected')).toBe('true')
      dropOn(wrapper, 0, 3, true)
      await flushPromises()
      expect(moveCollectionMember).toHaveBeenLastCalledWith('c1', 'RJ00000001', null)
    })

    it('in descending order a gap below row A is before A, and the very top is the end', async () => {
      const { api, moveCollectionMember } = edited()
      const { wrapper } = await mountEditing(api, '/collections/c1?direction=desc')
      dropOn(wrapper, 3, 1) // screen is 4,3,2,1: the gap above row 1 is below row 0 (RJ4)
      await flushPromises()
      expect(moveCollectionMember).toHaveBeenLastCalledWith('c1', 'RJ00000001', 'RJ00000004')
      dropOn(wrapper, 3, 0)
      await flushPromises()
      expect(moveCollectionMember).toHaveBeenLastCalledWith('c1', expect.any(String), null)
    })

    it('has no handles or move items under a view sort, but still removes', async () => {
      const { api, removeFromCollection } = edited()
      const { wrapper } = await mountEditing(api, '/collections/c1?sort=rj')
      expect(wrapper.findAll('[data-testid="collection-handle"]')).toHaveLength(0)
      await openMenu(wrapper, 0)
      expect(document.body.querySelector('[data-testid="collection-move-top"]')).toBeNull()
      document.body.querySelector<HTMLElement>('[data-testid="collection-remove"]')!.click()
      await flushPromises()
      expect(removeFromCollection).toHaveBeenCalledWith('c1', 'RJ00000004') // a view sort lists newest keys first
      expect(toast.current.value?.text).toBe('已从 Sleep 移除（撤销将放回末尾）')
    })

    it('moves from the row menu', async () => {
      const { api, moveCollectionMember } = edited()
      const { wrapper } = await mountEditing(api)
      await menu(wrapper, 2, 'collection-move-top')
      expect(moveCollectionMember).toHaveBeenLastCalledWith('c1', 'RJ00000003', 'RJ00000001')
      await menu(wrapper, 0, 'collection-move-down') // the row that is now first
      expect(moveCollectionMember).toHaveBeenLastCalledWith('c1', 'RJ00000003', 'RJ00000002')
    })

    it('removes the selected row with Delete, selects the next, and undo appends then moves it back', async () => {
      const { api, removeFromCollection, appendToCollection, moveCollectionMember } = edited()
      const { wrapper } = await mountEditing(api)
      press('ArrowDown')
      await flushPromises()
      press('Delete')
      await flushPromises()
      expect(removeFromCollection).toHaveBeenCalledWith('c1', 'RJ00000002')
      expect(rows(wrapper).map(r => r.attributes('data-rj'))).toEqual(['RJ00000001', 'RJ00000003', 'RJ00000004'])
      expect(rows(wrapper)[1]!.attributes('aria-selected')).toBe('true')
      expect(toast.current.value).toMatchObject({
        text: '已从 Sleep 移除',
        action: { label: '撤销' },
      })
      toast.current.value!.action!.run()
      await flushPromises()
      expect(appendToCollection).toHaveBeenCalledWith('c1', ['RJ00000002'])
      expect(moveCollectionMember).toHaveBeenCalledWith('c1', 'RJ00000002', 'RJ00000003')
    })

    it('undo under a view sort only appends', async () => {
      const { api, appendToCollection, moveCollectionMember } = edited()
      const { wrapper } = await mountEditing(api, '/collections/c1?sort=rj')
      await menu(wrapper, 1, 'collection-remove')
      toast.current.value!.action!.run()
      await flushPromises()
      expect(appendToCollection).toHaveBeenCalledWith('c1', ['RJ00000003'])
      expect(moveCollectionMember).not.toHaveBeenCalled()
    })

    it('reloads and says so when a move meets a conflict, and toasts other failures plainly', async () => {
      const { api, getCollection, moveCollectionMember } = edited()
      const { wrapper } = await mountEditing(api)
      moveCollectionMember.mockRejectedValueOnce(new ApiError(409, 'COLLECTION_MEMBER_CONFLICT', 'gone'))
      getCollection.mockClear()
      dropOn(wrapper, 3, 0)
      await flushPromises()
      expect(toast.current.value?.text).toBe('合集已在别处变动，已刷新')
      expect(getCollection).toHaveBeenCalled()
      expect(rows(wrapper).map(r => r.attributes('data-rj'))[0]).toBe('RJ00000001')
      moveCollectionMember.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', 'boom'))
      dropOn(wrapper, 3, 0)
      await flushPromises()
      expect(toast.current.value?.text).toBe('boom')
    })
  })

  describe('rename and delete', () => {
    const toast = useToast()
    beforeEach(() => { toast.current.value = null })
    const openItem = async (item: string) => {
      document.body.querySelector('[data-testid="collection-menu"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
      await flushPromises()
      document.body.querySelector<HTMLElement>(`[data-testid="${item}"]`)!.click()
      await flushPromises()
    }
    const input = () => document.body.querySelector<HTMLInputElement>('[data-testid="collection-rename-input"]')!
    const type = async (v: string) => { input().value = v; input().dispatchEvent(new Event('input', { bubbles: true })); await flushPromises() }
    const submit = async () => { input().form!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flushPromises() }

    it('offers the header menu on a user collection, empty or not, never on Favorites', async () => {
      const full = await mountPage(setup(3).api)
      expect(full.wrapper.find('[data-testid="collection-menu"]').exists()).toBe(true)
      const empty = await mountPage(setup(0).api)
      expect(empty.wrapper.find('[data-testid="collection-menu"]').exists()).toBe(true)
      const fav = await mountPage(setup(3).api, '/collections/favorites')
      expect(fav.wrapper.find('[data-testid="collection-menu"]').exists()).toBe(false)
    })

    it('refuses a blank name, then renames with the trimmed one and retitles in place', async () => {
      let current = 'Sleep'
      const getCollection = vi.fn(async () => ({
        id: 'c1',
        name: current,
        work_count: 0,
        created_at: '',
        updated_at: '',
        total: 0,
        works: [],
      }) as CollectionDetail)
      const renameCollection = vi.fn(async (_id: string, n: string) => { current = n; return {} as never })
      const { wrapper } = await mountPage(apiStub({ getCollection, renameCollection }))
      await openItem('collection-rename')
      expect(input().value).toBe('Sleep')
      await type('   ')
      await submit()
      expect(renameCollection).not.toHaveBeenCalled()
      await type('  Calm  ')
      await submit()
      expect(renameCollection).toHaveBeenCalledWith('c1', 'Calm')
      expect(wrapper.get('h1').text()).toBe('Calm')
      expect(toast.current.value).toBeNull()
      expect(document.body.querySelector('[data-testid="collection-rename-input"]')).toBeNull()
    })

    it('toasts the error when a rename fails', async () => {
      const renameCollection = vi.fn().mockRejectedValue(new Error('boom'))
      await mountPage(apiStub({
        ...setup(3).api,
        renameCollection,
      }))
      await openItem('collection-rename')
      await type('Calm')
      await submit()
      expect(toast.current.value?.text).toBe('boom')
    })

    it('deletes, goes back to the entry and toasts', async () => {
      const deleteCollection = vi.fn().mockResolvedValue(undefined)
      const { wrapper, router } = await mountPage(apiStub({
        ...setup(3).api,
        deleteCollection,
      }))
      await openItem('collection-delete')
      expect(document.body.textContent).toContain('删除合集「Sleep」？其中 3 部作品与文件不受影响。')
      document.body.querySelector<HTMLElement>('[data-testid="collection-delete-confirm"]')!.click()
      await flushPromises()
      expect(deleteCollection).toHaveBeenCalledWith('c1')
      expect(router.currentRoute.value.name).toBe('collections')
      expect(toast.current.value?.text).toBe('已删除 Sleep')
      expect(wrapper.exists()).toBe(true)
    })
  })
})
