import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { loadWorkContent } from '@/features/work-detail/mock-content'
import WorkColophon from './WorkColophon.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { useWorkDetailStore } from '@/stores/work-detail'

/**
 * The personal rows write straight through (draft spec §6.5): a rating click
 * PATCHes user-data, the favorite toggles, the note saves on change, and the
 * tag chips replace the set. jsdom has no layout; only the calls' contracts.
 */

const RJ = 'RJ01000001'

async function mountColophon(overrides: Record<string, ReturnType<typeof vi.fn>> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, await loadWorkContent(RJ))
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
  it('renders the catalog rows the detail query answers', async () => {
    const { wrapper } = await mountColophon()
    expect(wrapper.get('.colo-title').text()).toContain('雨宿り')
    expect(wrapper.text()).toContain('mete0r')
    expect(wrapper.text()).toContain('伊ヶ崎綾香')
    expect(wrapper.text()).toContain('2024.03.08')
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
    store.bind(RJ, await loadWorkContent(RJ))
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
