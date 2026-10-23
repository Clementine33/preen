import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { WorkDetail } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { usePlayerStore } from '@/stores/player'
import type { QueryClient } from '@tanstack/vue-query'
import { queryKeys } from '@/queries/query-keys'
import { installTestQueryPlugin } from '@/test/query-client'
import WorkInspector from './WorkInspector.vue'

/**
 * The inspector's contracts: browsing faster than the detail
 * arrives dims the last body instead of blanking it, a reduced-motion
 * preference skips the cover wipe but still paints the cover, and on the
 * detail page there is no wall to open or to steer.
 */

function detail(rj: string, voice: string): WorkDetail {
  return {
    rj,
    title: `Title ${rj}`,
    thumbnail_url: `https://covers.test/${rj}_sam.jpg`,
    circle: 'mete0r',
    language: null,
    nsfw: false,
    age_category: 1,
    release_date: '2024-03-08',
    dl_count: null,
    rating: null,
    favorite: false,
    user_tags: [],
    has_note: false,
    collected_at: '2026-01-01T00:00:00Z',
    copy_count: 1,
    listening: [],
    source_url: null,
    cover_url: `https://covers.test/${rj}.jpg`,
    sample_cover_url: null,
    series_id: null,
    series_name: null,
    voice_actors: [{ id: `va-${rj}`, source: 'dlsite', source_id: '1', name: voice }],
    source_tags: [],
    note: '',
    language_editions: null, edition_tree: null,
    relation: { role: null, language: null, original_rj: null, parent_rj: null },
    copies: [],
    collection_ids: [],
    relation_warnings: [],
    relation_nodes: [],
  }
}

const FIRST = 'RJ01000001'
const SECOND = 'RJ01000002'

function reducedMotion(matches: boolean): void {
  vi.spyOn(window, 'matchMedia').mockImplementation((query: string) => ({
    matches: matches && query === '(prefers-reduced-motion: reduce)',
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: () => false,
  }))
}

/** The last mounted inspector's query client, to refetch through. */
let queryClient: QueryClient

async function mountInspector(
  getWork: ReturnType<typeof vi.fn>,
  props: { rj: string; standalone?: boolean },
  getWorkContent?: ReturnType<typeof vi.fn>,
  formatPreference: string[] = [],
) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
  const query = installTestQueryPlugin()
  queryClient = query[1].queryClient
  const wrapper = mount(WorkInspector, {
    props,
    global: {
      plugins: [createPinia(), router, query],
      provide: { [apiClientKey as symbol]: apiStub({
        getWork,
        getPreferences: vi.fn().mockResolvedValue({ track_formats: formatPreference }),
        ...(getWorkContent ? { getWorkContent } : {}),
      } as never) },
    },
  })
  await flushPromises()
  return wrapper
}

/** Let the 70ms deferred swap run. */
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 100))
  await flushPromises()
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('WorkInspector', () => {
  it('paints a local cover once it is proven loadable, and the remote one when it is not', async () => {
    const outcomes: Array<'load' | 'error'> = ['load', 'error']
    class StubImage {
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      set src(_: string) {
        const outcome = outcomes.shift() ?? 'error'
        queueMicrotask(() => (outcome === 'load' ? this.onload : this.onerror)?.())
      }
    }
    vi.stubGlobal('Image', StubImage)
    reducedMotion(true)
    const local = { copy_id: 'c1', rel_path: 'cover.jpg', version: '1-2-3' }
    const getWork = vi.fn(async (rj: string) => ({ ...detail(rj, 'va'), local_cover: local }))
    const wrapper = await mountInspector(getWork, { rj: FIRST })
    await settle()
    expect(wrapper.get<HTMLElement>('.cover.front').element.style.backgroundImage).toContain('/files/')

    const failing = await mountInspector(getWork, { rj: SECOND })
    await settle()
    expect(failing.get<HTMLElement>('.cover.front').element.style.backgroundImage).toContain(`${SECOND}.jpg`)
    vi.unstubAllGlobals()
  })

  it('keeps the last body, dimmed, while the next work is still loading', async () => {
    const getWork = vi.fn((rj: string) =>
      rj === FIRST ? Promise.resolve(detail(FIRST, '伊ヶ崎綾香')) : new Promise<WorkDetail>(() => {}),
    )
    const wrapper = await mountInspector(getWork, { rj: FIRST })
    await settle()
    expect(wrapper.get('.spec').text()).toContain('伊ヶ崎綾香')

    await wrapper.setProps({ rj: SECOND })
    await settle()

    // Never an empty plate between two works: the previous body stays, dimmed.
    expect(wrapper.get('.spec').text()).toContain('伊ヶ崎綾香')
    expect(wrapper.get('.spec').classes()).toContain('dim')
    expect(wrapper.get('.cover.front').classes()).not.toContain('hatch')
  })

  it('swaps a refetch of the work on show quietly: no dim, no title scramble', async () => {
    const getWork = vi.fn((rj: string) => Promise.resolve(detail(rj, '伊ヶ崎綾香')))
    const wrapper = await mountInspector(getWork, { rj: FIRST })
    await settle()
    const frame = vi.spyOn(window, 'requestAnimationFrame')
    getWork.mockResolvedValueOnce(detail(FIRST, '沢城みゆき'))
    await queryClient.refetchQueries({ queryKey: queryKeys.works.detail(FIRST) })
    await flushPromises()

    expect(wrapper.get('.spec').classes()).not.toContain('dim')
    expect(wrapper.get('.spec').text()).toContain('沢城みゆき')
    expect(frame).not.toHaveBeenCalled()
  })

  it('paints the cover without the wipe under a reduced-motion preference', async () => {
    reducedMotion(true)
    const getWork = vi.fn((rj: string) => Promise.resolve(detail(rj, '沢城みゆき')))
    const wrapper = await mountInspector(getWork, { rj: FIRST })
    await settle()

    const front = wrapper.get<HTMLElement>('.cover.front').element
    expect(front.classList.contains('hatch')).toBe(false)
    expect(front.style.backgroundImage).toContain(`${FIRST}.jpg`)
    expect(front.classList.contains('run')).toBe(false)
  })

  it('offers no way back to a wall it is not on', async () => {
    const getWork = vi.fn((rj: string) => Promise.resolve(detail(rj, '沢城みゆき')))
    const onWall = await mountInspector(getWork, { rj: FIRST })
    expect(onWall.text()).toContain('OPEN DETAIL')

    const standalone = await mountInspector(getWork, { rj: FIRST, standalone: true })
    expect(standalone.text()).not.toContain('OPEN DETAIL')
  })
  it('lists the tracks of the first online copy', async () => {
    const getWork = vi.fn((rj: string) => Promise.resolve(detail(rj, '沢城みゆき')))
    const aud = (name: string, format: string, duration: number | null) => ({
      name, rel_path: name, kind: 'aud', size: 1, format, bitrate: null,
      width: null, height: null, duration_sec: duration, thumb_v: null,
    })
    const getWorkContent = vi.fn(async (rj: string) => ({
      rj,
      copies: [
        { copy_id: 'off', library_id: 'l', dir_id: 'd0', online: false },
        {
          copy_id: 'on', library_id: 'l', dir_id: 'd1', online: true,
          tree: { name: '', rel_path: '', children: [aud('01.mp3', 'MP3', 90), aud('01.wav', 'WAV', 90), aud('02.mp3', 'MP3', null)] },
        },
      ],
    }))
    const wrapper = await mountInspector(getWork, { rj: FIRST }, getWorkContent)
    await settle()

    // One row per track, not per format; an unmeasured duration reads as a dash.
    const rows = wrapper.findAll('.trk').map((row) => row.text())
    expect(rows).toEqual(['0101' + '01:30', '0202' + '—'])
    expect(wrapper.text()).toContain('2 · 01:30')
    expect(wrapper.text()).not.toContain('NO TRACK DATA')

    // A row plays the work from that track, in its first format, and is marked.
    await wrapper.findAll('.trk')[1].trigger('click')
    const player = usePlayerStore()
    expect(player.rj).toBe(FIRST)
    expect(player.trackIndex).toBe(1)
    expect(player.queue.map((item) => item.source?.src)).toEqual(['/files/01.mp3', '/files/02.mp3'])
    expect(wrapper.findAll('.trk').map((row) => row.classes('now'))).toEqual([false, true])
  })
  it('lists and plays the preferred format, and keeps every format when the work ships none of them', async () => {
    const getWork = vi.fn((rj: string) => Promise.resolve(detail(rj, '沢城みゆき')))
    const aud = (name: string, format: string) => ({
      name, rel_path: name, kind: 'aud', size: 1, format, bitrate: null,
      width: null, height: null, duration_sec: 10, thumb_v: null,
    })
    const getWorkContent = vi.fn(async (rj: string) => ({
      rj,
      copies: [{
        copy_id: 'on', library_id: 'l', dir_id: 'd1', online: true,
        tree: { name: '', rel_path: '', children: [aud('01.mp3', 'MP3'), aud('02.mp3', 'MP3'), aud('02.wav', 'WAV'), aud('03.wav', 'WAV')] },
      }],
    }))
    const wrapper = await mountInspector(getWork, { rj: FIRST }, getWorkContent, ['FLAC', 'WAV'])
    await settle()
    expect(wrapper.get('[data-testid="tracklist-format"]').text()).toBe('WAV')
    expect(wrapper.findAll('.trk').map((row) => row.get('.nm').text())).toEqual(['02', '03'])

    await wrapper.findAll('.trk')[0].trigger('click')
    const player = usePlayerStore()
    expect(player.queue.map((item) => item.source?.src)).toEqual(['/files/02.wav', '/files/03.wav'])
    expect(wrapper.findAll('.trk').map((row) => row.classes('now'))).toEqual([true, false])

    const other = await mountInspector(getWork, { rj: FIRST }, getWorkContent, ['FLAC'])
    await settle()
    expect(other.findAll('.trk')).toHaveLength(3)
    expect(other.find('[data-testid="tracklist-format"]').exists()).toBe(false)
  })
  it('fills a remounted inspector from the cached work', async () => {
    const getWork = vi.fn((rj: string) => Promise.resolve(detail(rj, '沢城みゆき')))
    // The wall's layout switch: the same page unmounts and remounts the
    // inspector while the work stays cached.
    const shown = ref(true)
    const Host = defineComponent(() => () => (shown.value ? h(WorkInspector, { rj: FIRST }) : null))
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })
    const wrapper = mount(Host, {
      global: {
        plugins: [createPinia(), router, installTestQueryPlugin()],
        provide: { [apiClientKey as symbol]: apiStub({ getWork } as never) },
      },
    })
    await flushPromises()
    await settle()
    shown.value = false
    await flushPromises()
    shown.value = true
    await flushPromises()

    expect(wrapper.get('.spec').text()).toContain('沢城みゆき')
    const front = wrapper.get<HTMLElement>('.cover.front').element
    expect(front.classList.contains('hatch')).toBe(false)
    expect(front.style.backgroundImage).toContain(`${FIRST}.jpg`)
  })
})
