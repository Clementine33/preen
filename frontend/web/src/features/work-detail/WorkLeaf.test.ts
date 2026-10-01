import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sampleWorkContent } from '@/test/work-content-fixture'
import WorkLeaf from './WorkLeaf.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { findFile, makeWorkDetail } from '@/test/work-fixture'
import { useWorkDetailStore } from '@/stores/work-detail'

/**
 * The leaf switcher lists only what exists right now: no
 * LISTEN without a pairing (the `LRC ▾` entry replaces it), no READ before a
 * script has been opened, the colophon always. A manual pairing switches the
 * strip to PAIRED.
 */

const RJ = 'RJ01000001'

async function mountLeaf(overrides: Record<string, unknown> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ))
  const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()), ...overrides } as never)
  const wrapper = mount(WorkLeaf, {
    global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, store }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('WorkLeaf', () => {
  it('starts on the colophon with only the entries that exist', async () => {
    const { wrapper } = await mountLeaf()
    const buttons = wrapper.findAll('.seg-b').filter((button) => !button.classes().some((name) => name === 'pick'))
    expect(buttons).toHaveLength(1)
    expect(buttons[0].text()).toContain('奥付')
    // No pairing for the track the page is on: the pair entry replaces LISTEN.
    expect(wrapper.get('.seg-b.pick').text()).toBe('LRC ▾')
  })

  it('shows LISTEN when the track has a pairing, and follows a new track back to the colophon', async () => {
    const { wrapper, store } = await mountLeaf()
    store.startTrack(1) // 02 has the automatic pairing
    await flushPromises()
    const labels = wrapper.findAll('.seg-b').map((button) => button.text())
    expect(labels.some((text) => text.includes('LISTEN'))).toBe(true)
    expect(store.leaf).toBe('lrc')

    store.startTrack(0) // 01 has none
    await flushPromises()
    expect(store.leaf).toBe('colophon')
    expect(wrapper.findAll('.seg-b').some((button) => button.text().includes('LISTEN'))).toBe(false)
  })

  it('keeps READ once a script has been opened', async () => {
    const { wrapper, store } = await mountLeaf()
    store.selectFile(findFile(store.copy!, '台本/台本_全編.txt'))
    await flushPromises()
    expect(store.leaf).toBe('script')
    // The percentage gauge is the strip's only position instrument.
    expect(wrapper.find('.gauge').exists()).toBe(true)
    // READ stays after switching away: it is sticky.
    store.setLeaf('colophon')
    await flushPromises()
    expect(wrapper.findAll('.seg-b').some((button) => button.text().includes('READ'))).toBe(true)
  })

  it('leaves a leaf empty when its text cannot be loaded', async () => {
    const { wrapper, store } = await mountLeaf({ getWorkText: vi.fn().mockRejectedValue(new Error('gone')) })
    store.startTrack(1)
    await flushPromises()
    store.selectFile(findFile(store.copy!, '台本/台本_全編.txt'))
    await flushPromises()
    // The rejection is absorbed: the leaf opens, it just has no lines.
    expect(store.leaf).toBe('script')
    expect(wrapper.find('.gauge').exists()).toBe(true)
  })

  it('pairs a lyric by hand and switches the strip to PAIRED', async () => {
    const { wrapper, store } = await mountLeaf()
    store.startTrack(0)
    await flushPromises()
    expect(store.leaf).toBe('colophon')
    await wrapper.get('.seg-b.pick').trigger('click')
    const items = wrapper.findAll('.menu-item')
    expect(items).toHaveLength(2)
    await items[0].trigger('click')
    expect(store.leaf).toBe('lrc')
    expect(store.pairs[0]).toBeDefined()
    expect(wrapper.text()).toContain('PAIRED')
  })
})
