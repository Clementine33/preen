import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sampleWorkContent } from '@/test/work-content-fixture'
import WorkColophon from './WorkColophon.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { workDetailTestRouter } from '@/test/work-router'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'
import { useToast } from '@/composables/use-toast'
import type { WorkCollection, WorkEditionSlot } from '@/lib/api/types'

/**
 * The personal rows write straight through: a rating click
 * PATCHes user-data, the favorite toggles, the note saves on change, and the
 * tag chips replace the set. jsdom has no layout; only the calls' contracts.
 */

const RJ = 'RJ01000001'

async function mountColophon(overrides: Record<string, ReturnType<typeof vi.fn>> = {}, compact = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ))
  const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()), ...overrides } as never)
  const wrapper = mount(WorkColophon, {
    props: { compact },
    global: { plugins: [pinia, await workDetailTestRouter(), installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, api }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('WorkColophon', () => {
  const node = (rj: string, role: WorkEditionSlot['root']['role'], is_work = true, held = false) => ({ rj, role, is_work, held })
  const TREE: WorkEditionSlot[] = [
    { language: 'JPN', label: null, held: true, root: node('RJ01287230', 'original', true, true), translations: [] },
    {
      language: 'CHI_HANS', label: '简中', held: true, root: node('RJ01289610', 'edition_page'),
      translations: [node('RJ01289611', 'translation', true, true), node('RJ01289700', 'translation')],
    },
    { language: 'CHI_HANT', label: '繁中', held: false, root: node('RJ01291590', 'edition_page', false), translations: [] },
  ]
  const onTranslation = (compact = false) => mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({
    rj: 'RJ01289611',
    edition_tree: TREE,
    relation: { role: 'child', language: 'CHI_HANS', original_rj: 'RJ01287230', parent_rj: 'RJ01289610' },
  })) }, compact)

  it('renders the edition tree, linking only the other held works', async () => {
    const { wrapper } = await onTranslation()
    const lines = wrapper.findAll('.ed-line').map((line) => line.text())
    expect(lines).toEqual([
      '日语原作 RJ01287230HELD',
      '简中页面 RJ01289610→翻译 RJ01289611HELD·翻译 RJ01289700',
      '繁中页面 RJ01291590',
    ])
    const hrefs = wrapper.findAll('.ed a').map((a) => a.attributes('href'))
    // Only other held works link: not the page, the placeholder or the work itself.
    expect(hrefs).toEqual(['/works/RJ01287230'])
    expect(wrapper.text()).not.toContain('关系')
  })

  it('marks the page slot and the translation itself current on a translation page', async () => {
    const { wrapper } = await onTranslation()
    const current = wrapper.findAll('.ed-line').map((line) => line.attributes('aria-current'))
    expect(current).toEqual([undefined, 'true', undefined])
    expect(wrapper.findAll('.ed .tag[aria-current="true"]').map((el) => el.text())).toEqual(['翻译 RJ01289611HELD'])
  })

  it('marks the original’s slot on the original and an edition page’s own slot', async () => {
    const original = await mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({
      rj: 'RJ01287230', edition_tree: TREE, relation: { role: 'original', language: null, original_rj: null, parent_rj: null },
    })) })
    expect(original.wrapper.findAll('.ed-line').map((l) => l.attributes('aria-current'))).toEqual(['true', undefined, undefined])
    const page = await mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({
      rj: 'RJ01289610', edition_tree: TREE, relation: { role: 'parent', language: 'CHI_HANS', original_rj: 'RJ01287230', parent_rj: null },
    })) })
    expect(page.wrapper.findAll('.ed-line').map((l) => l.attributes('aria-current'))).toEqual([undefined, 'true', undefined])
  })

  it('keeps only the roots and this work’s own translation in compact mode', async () => {
    const { wrapper } = await onTranslation(true)
    expect(wrapper.findAll('.ed-line').map((line) => line.text())).toEqual([
      '日语原作 RJ01287230HELD',
      '简中页面 RJ01289610→翻译 RJ01289611HELD',
      '繁中页面 RJ01291590',
    ])
  })

  it('marks a slot held only through a translation in compact mode', async () => {
    const { wrapper } = await mountColophon({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({
      rj: 'RJ01287230', edition_tree: TREE, relation: { role: 'original', language: null, original_rj: null, parent_rj: null },
    })) }, true)
    expect(wrapper.findAll('.ed-line').map((line) => line.text())[1]).toBe('简中页面 RJ01289610HELD')
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
        { copy_id: 'copy-a', library_id: 'lib-a', rel_path: 'works/RJ01000001', dir_id: 'd1' },
        { copy_id: 'copy-b', library_id: 'lib-b', rel_path: 'backup/RJ01000001', dir_id: 'd2' },
      ] })),
      listLibraries: vi.fn().mockResolvedValue([
        { id: 'lib-b', name: '备份库' },
        { id: 'lib-a', name: '音声库' },
      ]),
    })
    expect(wrapper.get('.copy').text()).toBe('音声库备份库·OFFLINE')
  })

  it('links a copy to its folder, or to the library overview when it is offline', async () => {
    const { wrapper } = await mountColophon({
      getWork: vi.fn().mockResolvedValue(makeWorkDetail({ copies: [
        { copy_id: 'copy-a', library_id: 'lib-a', rel_path: 'works/RJ01000001', dir_id: 'd1' },
        { copy_id: 'copy-b', library_id: 'lib-b', rel_path: 'backup/RJ01000001', dir_id: 'd2' },
      ] })),
    })
    expect(wrapper.findAll('.copy a').map((a) => a.attributes('href'))).toEqual(['/worksets/lib-a/f/d1', '/worksets/lib-b'])
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
      global: { plugins: [pinia, await workDetailTestRouter(), installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
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

describe('WorkColophon, listening', () => {
  const heard = (track_key: string | null = '本編/03_本編 ―添い寝―') => makeWorkDetail({
    listening: [{
      rendition: '本編', track_key, position_sec: track_key ? 271 : null, track_index: track_key ? 2 : null,
      track_count: track_key ? 8 : null, finished_at: track_key ? null : '2026-10-01T00:00:00.000000000Z',
      updated_at: '2026-10-01T00:00:00.000000000Z',
    }],
  })
  const mountHeard = (work = heard()) => mountColophon({ getWork: vi.fn().mockResolvedValue(work) })
  const button = (wrapper: Awaited<ReturnType<typeof mountColophon>>['wrapper'], label: string) =>
    wrapper.findAll('.colo-row button').find((b) => b.text().includes(label))

  it('shows nothing for a work never listened to', async () => {
    const { wrapper } = await mountColophon()
    expect(button(wrapper, '继续')).toBeUndefined()
  })

  it('shows 继续 with the track and second, and resumes there', async () => {
    const { wrapper } = await mountHeard()
    const resume = button(wrapper, '继续')!
    expect(resume.text()).toContain('3/8 · 04:31')
    const start = vi.spyOn(useWorkDetailStore(), 'startTrack')
    await resume.trigger('click')
    expect(start).toHaveBeenCalledWith(2, undefined, 271)
  })

  it('offers 从头播放 when only an orphan row is left', async () => {
    const { wrapper } = await mountHeard(heard('本編/gone'))
    expect(button(wrapper, '▶')!.text()).toBe('▶ 从头播放')
  })

  it('offers 重听 from track 1 when only a Finished row is left', async () => {
    const { wrapper } = await mountHeard(heard(null))
    const resume = button(wrapper, '重听')!
    expect(resume.text()).toBe('▶ 重听')
    const start = vi.spyOn(useWorkDetailStore(), 'startTrack')
    await resume.trigger('click')
    expect(start).toHaveBeenCalledWith(0, undefined, undefined)
  })

  it('clears the whole Work through the player', async () => {
    const { wrapper, api } = await mountHeard()
    usePlayerStore().bindClient(api)
    await button(wrapper, '标记为未听')!.trigger('click')
    await flushPromises()
    expect(api.deleteWorkListening).toHaveBeenCalledWith(RJ)
  })
})
