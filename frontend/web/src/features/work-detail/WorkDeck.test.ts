import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { sampleWorkContent } from '@/test/work-content-fixture'
import WorkDeck from './WorkDeck.vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { useWorkDetailStore } from '@/stores/work-detail'

/**
 * The deck's data-driven contracts: set chips carry
 * aria-pressed, filmstrip thumbs aria-current, the R18 veil hatches every
 * set but the cover's, and the corner tick flips to the colophon reverse.
 * The shrink-wrap geometry is jsdom-blind and covered by e2e.
 */

const RJ = 'RJ01000001'

async function mountDeck() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ))
  const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({ nsfw: true })) } as never)
  const wrapper = mount(WorkDeck, {
    global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, store }
}

beforeEach(() => {
  vi.clearAllMocks()
})
// The filmstrip scheduler keeps timers; unmounting drains them.
enableAutoUnmount(afterEach)

describe('WorkDeck', () => {
  it('marks the active set chip and the current thumb', async () => {
    const { wrapper, store } = await mountDeck()
    const chips = wrapper.findAll('.chip')
    expect(chips.length).toBe(store.sets.length)
    expect(chips[0].attributes('aria-pressed')).toBe('true')
    // The cover set (表紙) carries three images.
    const thumbs = wrapper.findAll('.thumb')
    expect(thumbs).toHaveLength(3)
    expect(thumbs[0].attributes('aria-current')).toBe('true')
    await thumbs[1].trigger('click')
    expect(store.idx).toBe(1)
    expect(wrapper.findAll('.thumb')[1].attributes('aria-current')).toBe('true')
    await chips[1].trigger('click')
    expect(store.set).toBe(1)
    expect(store.idx).toBe(0)
  })

  it('flips to the reverse and back by the corner tick', async () => {
    const { wrapper, store } = await mountDeck()
    expect(wrapper.get('[aria-label="Plate deck"] .strip .strip-name').text()).toBe('PLATE')
    await wrapper.get('.tick.br').trigger('click')
    expect(store.flipped).toBe(true)
    expect(wrapper.get('[aria-label="Plate deck"] .strip .strip-name').text()).toBe('REVERSE')
    // The back face carries the compact colophon.
    expect(wrapper.find('.face.back .colo').exists()).toBe(true)
    await wrapper.get('.tick.br').trigger('click')
    expect(store.flipped).toBe(false)
  })

  it('hatches non-cover sets behind the R18 veil and reveals for the visit', async () => {
    const { wrapper, store } = await mountDeck()
    store.toggleVeil()
    await flushPromises()
    // The cover set stays visible (v3 exempts it).
    expect(wrapper.get('.face.front').classes()).not.toContain('veil')
    store.showSet(1)
    await flushPromises()
    const front = wrapper.get('.face.front')
    expect(front.classes()).toContain('veil')
    // Clicking the hatch reveals every R18 image for the rest of the visit.
    await front.trigger('click')
    expect(store.unveiled).toBe(true)
    await flushPromises()
    expect(wrapper.get('.face.front').classes()).not.toContain('veil')
  })

  it('never hatches a work without the R18 flag', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useWorkDetailStore()
    store.bind(RJ, sampleWorkContent(RJ))
    store.toggleVeil()
    store.showSet(1)
    const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({ nsfw: false })) } as never)
    const wrapper = mount(WorkDeck, {
      global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
    })
    await flushPromises()
    expect(wrapper.get('.face.front').classes()).not.toContain('veil')
  })

  it('steps with the arrows clamped at the set ends', async () => {
    const { wrapper, store } = await mountDeck()
    store.step(999)
    await flushPromises()
    expect(store.idx).toBe(store.sets[0].files.length - 1)
    // The next arrow hides at the end; prev remains.
    expect(wrapper.find('.step.next').isVisible()).toBe(false)
  })

  it('still frames an image the scan never measured', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useWorkDetailStore()
    const content = sampleWorkContent(RJ)
    const cover = content.copies[0].tree!.children.find((node) => node.name === '表紙')
    if (!cover || !('children' in cover)) throw new Error('fixture has a cover set')
    for (const item of cover.children) Object.assign(item, { width: null, height: null })
    store.bind(RJ, content)
    const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()) } as never)
    const wrapper = mount(WorkDeck, {
      global: { plugins: [pinia, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
    })
    await flushPromises()
    store.step(1)
    await flushPromises()
    expect(wrapper.get('.plate').attributes('style')).not.toContain('width: 0px')
  })
})
