import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sampleWorkContent } from '@/test/work-content-fixture'
import WorkLeaf from './WorkLeaf.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { findFile, makeWorkDetail } from '@/test/work-fixture'
import { workDetailTestRouter } from '@/test/work-router'
import { useWorkDetailStore } from '@/stores/work-detail'

/**
 * The leaf switcher lists only what exists right now: no
 * LISTEN without a pairing, no READ before a script has been opened, the
 * colophon always. Pairing status labels are hidden; the playing dot remains.
 */

const RJ = 'RJ01000001'

async function mountLeaf(overrides: Record<string, unknown> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ))
  const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()), ...overrides } as never)
  const wrapper = mount(WorkLeaf, {
    global: { plugins: [pinia, await workDetailTestRouter(), installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
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
    const buttons = wrapper.findAll('.seg-b')
    expect(buttons).toHaveLength(1)
    expect(buttons[0].text()).toContain('COLOPHON')
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
    expect(wrapper.find('.orient').exists()).toBe(false)
    expect(wrapper.find('.script-wrap').exists()).toBe(true)
    // The percentage gauge is the strip's only position instrument.
    expect(wrapper.find('.gauge').exists()).toBe(true)
    // READ stays after switching away: it is sticky.
    store.setLeaf('colophon')
    await flushPromises()
    expect(wrapper.findAll('.seg-b').some((button) => button.text().includes('READ'))).toBe(true)
  })

  it.each(['lrc', 'vtt'])('opens a %s file in READ without changing playback or lyric pairing', async (ext) => {
    const text = ext === 'lrc'
      ? '[00:01.00]欢迎来到精灵乐园\r\n[00:02.00]请慢慢休息'
      : 'WEBVTT\n\n00:01.000 --> 00:02.000\n欢迎来到精灵乐园\n\n00:02.000 --> 00:03.000\n请慢慢休息'
    const getWorkText = vi.fn().mockResolvedValue(text)
    const { wrapper, store } = await mountLeaf({ getWorkText })
    store.startTrack(1)
    const paired = store.lrcFor(1)
    const lyric = findFile(store.copy!, paired!)
    const rel = `lyrics/read.${ext}`
    store.selectFile({ ...lyric, name: `read.${ext}`, rel_path: rel })
    await flushPromises()
    expect(store.leaf).toBe('script')
    expect(store.activeTxt).toBe(rel)
    expect(store.track).toBe(1)
    expect(store.lrcFor(1)).toBe(paired)
    expect(getWorkText).toHaveBeenLastCalledWith({ rj: RJ, copyId: 'copy-a', relPath: rel })
    expect(wrapper.findAll('.script p').map(line => line.text())).toEqual(['1　欢迎来到精灵乐园', '2　请慢慢休息'])
    expect(wrapper.find('.orient').exists()).toBe(false)
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

  it('shows manually paired lyrics without status labels, keeping the playing dot', async () => {
    const { wrapper, store } = await mountLeaf()
    store.startTrack(0)
    await flushPromises()
    expect(store.leaf).toBe('colophon')
    store.pairLrc(0, store.autoLrc(1)!)
    await flushPromises()
    expect(store.leaf).toBe('lrc')
    expect(wrapper.get('.strip').text()).not.toContain('PAIRED')
    expect(wrapper.get('.strip').text()).not.toContain('SYNC')
    expect(wrapper.find('.live').exists()).toBe(true)
    expect(wrapper.find('.strip-file').exists()).toBe(false)
  })
})
