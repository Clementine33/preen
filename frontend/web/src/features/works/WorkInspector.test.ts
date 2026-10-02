import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { WorkDetail } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import WorkInspector from './WorkInspector.vue'

/**
 * The inspector's contracts (ADR 0012 §4): browsing faster than the detail
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
    source_url: null,
    cover_url: `https://covers.test/${rj}.jpg`,
    sample_cover_url: null,
    series_id: null,
    series_name: null,
    voice_actors: [{ id: `va-${rj}`, source: 'dlsite', source_id: '1', name: voice }],
    source_tags: [],
    note: '',
    language_editions: null,
    relation: { role: null, language: null, original_rj: null, parent_rj: null },
    copies: [],
    placeholder: false,
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

async function mountInspector(getWork: ReturnType<typeof vi.fn>, props: { rj: string; standalone?: boolean }) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
  const wrapper = mount(WorkInspector, {
    props,
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: apiStub({ getWork } as never) },
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
    expect(onWall.find('.keys').exists()).toBe(true)

    const standalone = await mountInspector(getWork, { rj: FIRST, standalone: true })
    expect(standalone.text()).not.toContain('OPEN DETAIL')
    expect(standalone.find('.keys').exists()).toBe(false)
  })
})
