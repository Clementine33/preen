import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { createPinia } from 'pinia'
import WorkDetailPage from './WorkDetailPage.vue'
import { apiClientKey } from '@/lib/api/client'
import { sampleWorkContent } from '@/test/work-content-fixture'
import { apiStub } from '@/test/api-stub'
import { createTestQueryClient, installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'
import { queryKeys } from '@/queries/query-keys'
import { VueQueryPlugin } from '@tanstack/vue-query'

/**
 * The page-level contracts: the route's work number
 * canonicalizes, the address keeps no query of its own, and the keyboard is
 * the prototype's — I flips, the arrows step, Esc closes the topmost layer,
 * Space only pauses and never inside a text field.
 */

const RJ = 'RJ01000001'

function routerFor(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/works', name: 'works', component: { template: '<div />' } },
      { path: '/works/:rj', name: 'work-detail', component: WorkDetailPage },
    ],
  })
}

async function mountPage(path = `/works/${RJ}`, attach = false): Promise<{ wrapper: ReturnType<typeof mount>; router: Router }> {
  const api = apiStub({
    getWork: vi.fn().mockResolvedValue(makeWorkDetail()),
    getWorkContent: vi.fn(async (rj: string) => sampleWorkContent(rj)),
  } as never)
  const router = routerFor()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(WorkDetailPage, {
    ...(attach ? { attachTo: document.body.appendChild(document.createElement('div')) } : {}),
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, router }
}

/** The page listens on the window, so the keyboard arrives as real events. */
function press(key: string): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key }))
}

beforeEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

enableAutoUnmount(afterEach)

describe('work detail page', () => {
  it('renders the work and its colophon from the detail query', async () => {
    const { wrapper } = await mountPage()
    expect(wrapper.get('.h-title').text()).toContain('雨宿り')
    expect(wrapper.text()).toContain('mete0r')
    expect(wrapper.text()).toContain('伊ヶ崎綾香')
  })

  it('reads the 6-digit spelling as the canonical work number, with no query of its own', async () => {
    // RJ00100002 is also spelled rj100002; RJ01000001 has no 6-digit form.
    const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail({ rj: 'RJ00100002' })) } as never)
    const router = routerFor()
    await router.push('/works/rj100002')
    await router.isReady()
    mount(WorkDetailPage, {
      global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
    })
    await flushPromises()
    expect(useWorkDetailStore().rj).toBe('RJ00100002')
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('flips with I and steps with the arrows', async () => {
    const { wrapper } = await mountPage()
    const store = useWorkDetailStore()
    expect(store.idx).toBe(0)
    press('ArrowRight')
    expect(store.idx).toBe(1)
    press('ArrowLeft')
    expect(store.idx).toBe(0)
    press('i')
    expect(store.flipped).toBe(true)
    await flushPromises()
    // The flip hint reads FRONT while the reverse is up.
    expect(wrapper.get('.flip-hint').text()).toBe('FRONT')
  })

  it('closes the topmost layer with Esc: pair menu first, then the lifted bay', async () => {
    await mountPage()
    const store = useWorkDetailStore()
    store.openBayTab('tracks')
    store.picking = true
    press('Escape')
    expect(store.picking).toBe(false)
    expect(store.bayOpen).toBe(true)
    press('Escape')
    expect(store.bayOpen).toBe(false)
  })

  it('pauses with Space but never inside a text field', async () => {
    const { wrapper } = await mountPage(`/works/${RJ}`, true)
    const store = useWorkDetailStore()
    store.startTrack(1)
    const player = usePlayerStore()
    expect(player.paused).toBe(false)

    press(' ')
    expect(player.paused).toBe(true)
    press(' ')
    expect(player.paused).toBe(false)

    const note = wrapper.get('textarea.note').element
    note.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }))
    expect(player.paused).toBe(false) // the note keeps its keys
    wrapper.unmount()
  })
  it('binds cached works on entry and preserves reading, pairing and playback on refetch', async () => {
    const client = createTestQueryClient()
    const cached = structuredClone(sampleWorkContent(RJ))
    client.setQueryData(queryKeys.works.content(RJ), cached)
    const other = 'RJ01000002'
    client.setQueryData(queryKeys.works.content(other), sampleWorkContent(other))
    const api = apiStub({ getWork: vi.fn().mockResolvedValue(makeWorkDetail()) })
    const router = routerFor()
    await router.push(`/works/${RJ}`)
    await router.isReady()
    const wrapper = mount(WorkDetailPage, { global: {
      plugins: [createPinia(), router, [VueQueryPlugin, { queryClient: client }]],
      provide: { [apiClientKey as symbol]: api },
    } })
    await flushPromises()
    const store = useWorkDetailStore()
    expect(store.sets.length).toBeGreaterThan(0)
    await router.push(`/works/${other}`)
    await flushPromises()
    expect(store.rj).toBe(other)
    await router.push(`/works/${RJ}`)
    await flushPromises()
    expect(store.rj).toBe(RJ)
    expect(store.tracks).toHaveLength(8)
    expect(api.getWorkContent).not.toHaveBeenCalled()
    store.pairLrc(0, '本編/lyrics/04_おやすみ.lrc')
    store.startTrack(0)
    store.activeTxt = '台本/台本_全編.txt'
    store.setLeaf('script')
    store.step(1)
    const player = usePlayerStore()
    const play = vi.spyOn(player, 'play')
    const updated = structuredClone(cached)
    const cover = updated.copies[0].tree!.children.find((entry) => entry.name === '表紙')!
    if (!('children' in cover)) throw new Error('cover directory missing')
    Object.assign(cover.children[0], { width: 2000, height: 1000 })
    client.setQueryData(queryKeys.works.content(RJ), updated)
    await flushPromises()
    expect(store.sets[0].files[0].width).toBe(2000)
    expect(store.idx).toBe(1)
    expect(store.leaf).toBe('script')
    expect(store.activeTxt).toBe('台本/台本_全編.txt')
    expect(store.pairs[0]).toBe('本編/lyrics/04_おやすみ.lrc')
    expect(player.trackIndex).toBe(0)
    expect(player.paused).toBe(false)
    expect(play).not.toHaveBeenCalled()
    wrapper.unmount()
    client.clear()
  })

})
