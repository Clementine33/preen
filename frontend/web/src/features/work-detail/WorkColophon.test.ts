import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sampleWorkContent } from '@/test/work-content-fixture'
import WorkColophon from './WorkColophon.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { useWorkDetailStore } from '@/stores/work-detail'
import { useToast } from '@/composables/use-toast'
import type { WorkCollection } from '@/lib/api/types'

/**
 * The personal rows write straight through: a rating click
 * PATCHes user-data, the favorite toggles, the note saves on change, and the
 * tag chips replace the set. jsdom has no layout; only the calls' contracts.
 */

const RJ = 'RJ01000001'

async function mountColophon(overrides: Record<string, ReturnType<typeof vi.fn>> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ))
  const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()), ...overrides } as never)
  const wrapper = mount(WorkColophon, {
    global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, api }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('WorkColophon', () => {
  it('shows the original and intermediate Chinese page rather than choosing one', async () => {
    const { wrapper } = await mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({
      rj: 'RJ01449056',
      relation: { role: 'child', language: 'CHI_HANS', original_rj: 'RJ01407907', parent_rj: 'RJ01449055' },
    })) })
    expect(wrapper.text()).toContain('原作 RJ01407907 → 简中页面 RJ01449055 → 翻译作品 RJ01449056')
  })

  it('renders the catalog rows the detail query answers', async () => {
    const { wrapper } = await mountColophon()
    expect(wrapper.get('.colo-title').text()).toContain('雨宿り')
    expect(wrapper.text()).toContain('mete0r')
    expect(wrapper.text()).toContain('伊ヶ崎綾香')
    expect(wrapper.text()).toContain('2024.03.08')
  })

  it('shows each copy’s library name and retains its offline state', async () => {
    const { wrapper } = await mountColophon({
      getWork: vi.fn().mockResolvedValue(makeWorkDetail({ copies: [
        { copy_id: 'copy-a', library_id: 'lib-a', rel_path: 'works/RJ01000001' },
        { copy_id: 'copy-b', library_id: 'lib-b', rel_path: 'backup/RJ01000001' },
      ] })),
      listLibraries: vi.fn().mockResolvedValue([
        { id: 'lib-b', name: '备份库' },
        { id: 'lib-a', name: '音声库' },
      ]),
    })
    expect(wrapper.get('.copy').text()).toBe('音声库备份库·OFFLINE')
    expect(wrapper.findAll('.lib-state.off')).toHaveLength(1)
  })

  it('PATCHes a rating click and clears it by clicking the set value', async () => {
    const update = vi.fn().mockResolvedValue({ rating: 3, favorite: true, note: '' })
    const { wrapper } = await mountColophon({ updateWorkUserData: update })
    const squares = wrapper.findAll('.rating i')
    expect(squares).toHaveLength(10)
    await squares[2].trigger('click')
    expect(update).toHaveBeenCalledWith(RJ, { rating: 3 })
    // The fixture is rated 8: clicking the eighth square clears it.
    await squares[7].trigger('click')
    expect(update).toHaveBeenLastCalledWith(RJ, { rating: null })
  })

  it('toggles the favorite and saves a changed note', async () => {
    const update = vi.fn().mockResolvedValue({ rating: 8, favorite: false, note: 'x' })
    const { wrapper } = await mountColophon({ updateWorkUserData: update })
    await wrapper.get('.fav').trigger('click')
    expect(update).toHaveBeenCalledWith(RJ, { favorite: false })
    const note = wrapper.get('textarea.note')
    await note.setValue('第 2 轨的雨声最好')
    await note.trigger('change')
    expect(update).toHaveBeenLastCalledWith(RJ, { note: '第 2 轨的雨声最好' })
  })

  it('replaces the tag set: a my-tag click removes, the menu adds, a name creates', async () => {
    // The replace mock answers with the tags it was given, so the merged cache
    // keeps up with the set the test is building.
    const replace = vi.fn((_rj: string, ids: string[]) => Promise.resolve(ids.map((id) => ({ id, name: id }))))
    const create = vi.fn().mockResolvedValue({ id: 't-9', name: '反复听' })
    const { wrapper } = await mountColophon({
      replaceWorkUserTags: replace,
      createUserTag: create,
      listUserTags: vi.fn().mockResolvedValue([{ id: 't-2', name: '深夜' }]),
    })
    await flushPromises()
    // The fixture carries t-1 睡前: clicking it removes it from the set.
    await wrapper.get('.tag.mine').trigger('click')
    expect(replace).toHaveBeenCalledWith(RJ, [])

    await wrapper.get('.tag.add').trigger('click')
    const items = wrapper.findAll('.menu-item')
    expect(items).toHaveLength(2) // one existing tag + the new-name form
    await items[0].trigger('click')
    expect(replace).toHaveBeenLastCalledWith(RJ, ['t-2'])

    // The menu closed with the pick; reopening it leaves only the name form.
    await wrapper.get('.tag.add').trigger('click')
    expect(wrapper.findAll('.menu-item')).toHaveLength(1)
    const input = wrapper.get('.menu-item input')
    await input.setValue('反复听')
    await input.trigger('submit')
    await flushPromises()
    expect(create).toHaveBeenCalledWith('反复听')
    expect(replace).toHaveBeenLastCalledWith(RJ, ['t-2', 't-9'])
  })

  it('locks the personal rows of a placeholder work', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useWorkDetailStore()
    store.bind(RJ, sampleWorkContent(RJ))
    const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({ placeholder: true })) } as never)
    const wrapper = mount(WorkColophon, {
      global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
    })
    await flushPromises()
    expect(wrapper.get('.fav').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.rating i[role="button"]').exists()).toBe(false)
    expect(wrapper.get('textarea.note').attributes('disabled')).toBeDefined()
  })
})


it.each([[1, '全年龄'], [2, 'R15'], [3, 'R18'], [null, null]] as const)('shows age category %s without UNKNOWN', async (age_category, label) => {
  const { wrapper } = await mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({ age_category })) })
  expect(wrapper.text()).not.toContain('UNKNOWN')
  if (label) expect(wrapper.text()).toContain(label)
  else for (const age of ['全年龄', 'R15', 'R18']) expect(wrapper.text()).not.toContain(age)
})

it.each([['NM', '无语言限制'], ['CHI_HANS', '简中'], ['CHI_HANT', '繁中'], ['JPN', '日语'], ['ENG', 'ENG'], [null, null]])('shows declared work language %s', async (language, label) => {
  const { wrapper } = await mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({ language })) })
  const row = wrapper.findAll('.colo-row').find((row) => row.find('dt').text() === '语言')
  if (label) expect(row?.text()).toContain(label)
  else expect(row).toBeUndefined()
})

describe('add to collection', () => {
  const toast = useToast()
  beforeEach(() => { toast.current.value = null })
  const col = (id: string, name: string): WorkCollection => ({ id, name, work_count: 0, created_at: '', updated_at: '' })
  const COLS = [col('favorites', 'Favorites'), col('c1', 'Sleep'), col('c2', 'Rain')]

  async function openMenu(overrides: Record<string, ReturnType<typeof vi.fn>> = {}, held: string[] = ['c1']) {
    const mounted = await mountColophon({
      getWork: vi.fn().mockResolvedValue(makeWorkDetail({ collection_ids: held })),
      listCollections: vi.fn().mockResolvedValue({ collections: COLS, total: COLS.length }),
      ...overrides,
    })
    mounted.wrapper.get('[data-testid="work-collections"]').element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    await flushPromises()
    const items = () => Array.from(document.body.querySelectorAll<HTMLElement>('[data-testid="work-collection-item"]'))
    const click = async (el: Element | null) => { (el as HTMLElement).click(); await flushPromises() }
    return { ...mounted, items, click }
  }

  it('lists the user collections without Favorites, ticking those in collection_ids', async () => {
    const { wrapper, items } = await openMenu()
    expect(items().map((el) => [el.dataset.id, el.textContent?.trim(), el.getAttribute('aria-checked')])).toEqual([['c1', 'Sleep', 'true'], ['c2', 'Rain', 'false']])
    wrapper.unmount()
  })

  it('appends on an unticked click and removes on a ticked one, each with a toast', async () => {
    const append = vi.fn().mockResolvedValue({})
    const remove = vi.fn().mockResolvedValue(undefined)
    const { wrapper, items, click } = await openMenu({ appendToCollection: append, removeFromCollection: remove })
    await click(items()[1]!)
    expect(append).toHaveBeenCalledWith('c2', [RJ])
    expect(toast.current.value?.text).toBe('已添加到 Rain')
    await click(items()[0]!)
    expect(remove).toHaveBeenCalledWith('c1', RJ)
    expect(toast.current.value?.text).toBe('已从 Sleep 移除')
    wrapper.unmount()
  })

  it('creates 新合集 #N with this work in it and says so', async () => {
    const create = vi.fn().mockResolvedValue(col('c3', '新合集 #3'))
    const append = vi.fn().mockResolvedValue({})
    const { wrapper, click } = await openMenu({ createCollection: create, appendToCollection: append })
    await click(document.body.querySelector('[data-testid="work-collection-new"]'))
    expect(create).toHaveBeenCalledWith('新合集 #3')
    expect(append).toHaveBeenCalledWith('c3', [RJ])
    expect(toast.current.value?.text).toBe('已新建 新合集 #3 并添加')
    expect(document.body.querySelector('[data-testid="work-collection-new"]')).toBeNull() // the menu closed
    wrapper.unmount()
  })

  it('toasts the error when a write fails', async () => {
    const { wrapper, items, click } = await openMenu({ appendToCollection: vi.fn().mockRejectedValue(new Error('boom')) })
    await click(items()[1]!)
    expect(toast.current.value?.text).toBe('boom')
    wrapper.unmount()
  })
})
