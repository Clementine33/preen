import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { loadWorkContent } from '@/features/work-detail/mock-content'
import WorkBay from './WorkBay.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'

/**
 * The bay's contracts (draft spec §6.6): a tab click lifts, the open tab's
 * click closes; the track list is one list with format chips — a track missing
 * the browsed format stays listed, tagged with its only one, and plays it;
 * the FILES tab's rows act on the whole page.
 */

const RJ = 'RJ01000001'

async function mountBay() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, await loadWorkContent(RJ))
  const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()) } as never)
  const wrapper = mount(WorkBay, {
    global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, store }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('WorkBay', () => {
  it('counts the tabs and lifts on the first tab click, closing on the second', async () => {
    const { wrapper, store } = await mountBay()
    const tabs = wrapper.findAll('.tab')
    expect(tabs[0].text()).toContain('TRACKS')
    expect(tabs[0].text()).toContain('8')
    expect(tabs[1].text()).toContain('FILES')
    expect(store.bayOpen).toBe(false)
    await tabs[0].trigger('click')
    expect(store.bayOpen).toBe(true)
    await tabs[0].trigger('click')
    expect(store.bayOpen).toBe(false)
  })

  it('lists every track and plays the missing-format one in its only format', async () => {
    const { wrapper } = await mountBay()
    const store = useWorkDetailStore()
    const player = usePlayerStore()
    store.openBayTab('tracks')
    await flushPromises()
    const rows = wrapper.findAll('.trk')
    expect(rows).toHaveLength(8)
    // 08 exists only as WAV: listed, dimmed, tagged — and playable.
    const last = rows[7]
    expect(last.classes()).toContain('missing')
    expect(last.text()).toContain('WAV ONLY')
    expect(last.text()).not.toContain('¶') // 08 has no paired lyric
    await last.trigger('click')
    expect(player.trackIndex).toBe(7)
    expect(store.playFmt).toBe('WAV')
  })

  it('flips the missing tags with the format chips', async () => {
    const { wrapper } = await mountBay()
    const store = useWorkDetailStore()
    store.openBayTab('tracks')
    await flushPromises()
    const chips = wrapper.findAll('.chip')
    expect(chips.map((chip) => chip.attributes('aria-pressed'))).toEqual(['true', 'false'])
    await chips[1].trigger('click')
    expect(store.fmt).toBe('WAV')
    const rows = wrapper.findAll('.trk')
    expect(rows[7].classes()).not.toContain('missing')
    expect(rows[7].text()).not.toContain('ONLY')
    expect(rows[7].find('.only').exists()).toBe(false)
  })

  it('navigates directories and acts on file rows', async () => {
    const { wrapper, store } = await mountBay()
    const player = usePlayerStore()
    store.openBayTab('files')
    await flushPromises()

    // The browser opens at the copy root; the first directory is 本編.
    const dirRow = wrapper.findAll('.row.dir').find((row) => row.text().includes('本編'))!
    await dirRow.trigger('click')
    expect(store.cwd).toEqual(['本編'])
    // The up-row returns.
    await wrapper.get('.row.dir').trigger('click')
    expect(store.cwd).toEqual([])

    // An image row locates its set on the deck: enter 表紙 first.
    const coverDir = wrapper.findAll('.row.dir').find((row) => row.text().includes('表紙'))!
    await coverDir.trigger('click')
    const imgRow = wrapper.findAll('.row').find((row) => row.text().includes('cover_main.jpg'))!
    await imgRow.trigger('click')
    expect(store.set).toBe(0)
    expect(store.sel).toBe('表紙/cover_main.jpg')

    // Back to the root: a text row opens READ.
    await wrapper.get('.row.dir').trigger('click')
    const txtRow = wrapper.findAll('.row').find((row) => row.text().includes('はじめにお読みください.txt'))!
    await txtRow.trigger('click')
    expect(store.leaf).toBe('script')
    expect(store.bayOpen).toBe(false) // the text pick closed the bottom drawer

    // An audio row, two levels deep, starts its track.
    await wrapper.findAll('.row.dir').find((row) => row.text().includes('本編'))!.trigger('click')
    await wrapper.findAll('.row.dir').find((row) => row.text().includes('mp3'))!.trigger('click')
    const mp3Row = wrapper.findAll('.row').find((row) => row.text().includes('01_導入'))!
    await mp3Row.trigger('click')
    expect(player.trackIndex).toBe(0)
  })
})
