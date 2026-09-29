import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { WorkSummary } from '@/lib/api/types'
import WorkWall from './WorkWall.vue'

/**
 * The wall's own contracts (ADR 0012 §4): the JS column count is the single
 * authority (it drives the grid style, the cursor and the keyboard steps),
 * exactly one cell is tabbable, and the cursor composes the virtual row offset
 * with the in-row offset — offsetTop of a transformed cell would miss the
 * virtualizer's translateY.
 *
 * jsdom has no layout and no ResizeObserver: the grid width arrives through a
 * stub ResizeObserver — the same hook the production code measures with — and
 * the scroller's metrics are stubbed on the element right after mount, before
 * the virtualizer's own measurement flush runs (the DirList test's pattern).
 */
class StubResizeObserver {
  static instances: StubResizeObserver[] = []
  readonly observe = vi.fn()
  readonly unobserve = vi.fn()
  readonly disconnect = vi.fn()
  private readonly callback: (entries: Array<{ contentRect: { width: number } }>) => void

  constructor(callback: (entries: Array<{ contentRect: { width: number } }>) => void) {
    this.callback = callback
    StubResizeObserver.instances.push(this)
  }

  /** Deliver a measurement, as a real observer would after a layout settle. */
  trigger(width: number) {
    this.callback([{ contentRect: { width } }])
  }
}

function makeWorks(count: number): WorkSummary[] {
  return Array.from({ length: count }, (_, i) => ({
    rj: `RJ${String(10_000_000 + i)}`,
    title: `Title ${i}`,
    thumbnail_url: null,
    circle: 'CIRCLE',
    language: null,
    nsfw: false,
    release_date: '2024-03-08',
    dl_count: null,
    rating: null,
    favorite: false,
    user_tags: [],
    has_note: false,
    collected_at: '2026-01-01T00:00:00Z',
    copy_count: 1,
  }))
}

const WORKS = makeWorks(8)

async function scaffold(options: { works?: WorkSummary[]; selectedRj?: string | null } = {}) {
  vi.stubGlobal('ResizeObserver', StubResizeObserver)
  const wrapper = mount(WorkWall, {
    props: {
      works: options.works ?? WORKS,
      selectedRj: options.selectedRj ?? null,
      playingRj: null,
    },
  })
  // The scroller's metrics must be in place before the virtualizer's first
  // measurement flush, which runs on the microtask after mount.
  const scroller = wrapper.find('[data-testid="work-wall"]').element as HTMLElement
  let top = 0
  Object.defineProperties(scroller, {
    offsetHeight: { configurable: true, get: () => 560 },
    offsetWidth: { configurable: true, get: () => 800 },
    scrollTop: { configurable: true, get: () => top, set: (v: number) => (top = v) },
  })
  // The virtualizer registers its own observer before the wall's; deliver the
  // measurement to every one, as a real layout settle would.
  for (const observer of StubResizeObserver.instances) observer.trigger(800)
  await flushPromises()
  await nextTick()
  return {
    wrapper,
    exposed: wrapper.vm as unknown as { columns: number; move(delta: number): void },
    setScrollTop: (v: number) => {
      top = v
      scroller.dispatchEvent(new Event('scroll'))
    },
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  StubResizeObserver.instances = []
})

describe('WorkWall', () => {
  it('derives the column count from the grid width and drives the grid style with it', async () => {
    const { wrapper, exposed } = await scaffold()

    // floor((800+20)/(184+20)) = 4 columns, exactly what the prototype's
    // repeat(auto-fill, minmax(184px, 1fr)) would pick at this width.
    expect(exposed.columns).toBe(4)
    expect(wrapper.find('.grid').attributes('style')).toContain('grid-template-columns: repeat(4, minmax(0, 1fr))')
  })

  it('renders the windowed rows with exactly one tabbable cell', async () => {
    const { wrapper } = await scaffold({ selectedRj: WORKS[2].rj })
    await flushPromises()

    const cells = wrapper.findAll('.cell')
    expect(cells.length).toBeGreaterThan(0)
    const tabbable = cells.filter((cell) => cell.attributes('tabindex') === '0')
    expect(tabbable).toHaveLength(1)
    expect(tabbable[0].attributes('data-rj')).toBe(WORKS[2].rj)
    expect(tabbable[0].attributes('aria-selected')).toBe('true')
  })

  it('moves ±1 and ±columns, clamping at the edges without emitting', async () => {
    const { wrapper, exposed } = await scaffold({ selectedRj: WORKS[0].rj })

    exposed.move(4) // one full row down
    expect(wrapper.emitted('select')).toEqual([[WORKS[4].rj]])

    // The parent feeds the selection back, as WorksPage does.
    await wrapper.setProps({ selectedRj: WORKS[4].rj })
    exposed.move(-1)
    expect(wrapper.emitted('select')).toEqual([[WORKS[4].rj], [WORKS[3].rj]])

    await wrapper.setProps({ selectedRj: WORKS[0].rj })
    exposed.move(-99) // already at the start: clamped, no emission
    expect(wrapper.emitted('select')).toEqual([[WORKS[4].rj], [WORKS[3].rj]])

    exposed.move(99) // clamped to the last work
    expect(wrapper.emitted('select')).toEqual([[WORKS[4].rj], [WORKS[3].rj], [WORKS[7].rj]])
  })

  it('composes the cursor from the virtual row offset and the in-row offset', async () => {
    // Index 5 over 4 columns: row 1, column 1. colWidth = (800-60)/4 = 185,
    // coverHeight = 138.75, rowHeight = 228.75 — the cursor must sit at the
    // caption of that cell: x = 1*(185+20)-14, y = 1*228.75+138.75+14.
    const { wrapper } = await scaffold({ selectedRj: WORKS[5].rj })

    expect(wrapper.find('.cursor').attributes('style')).toContain('translate(191px, 381.5px)')
  })

  it('selects on click and opens on double click; a mobile tap routes instead', async () => {
    const { wrapper } = await scaffold({ selectedRj: WORKS[0].rj })

    await wrapper.find(`[data-rj="${WORKS[3].rj}"]`).trigger('click')
    expect(wrapper.emitted('select')).toEqual([[WORKS[3].rj]])
    expect(wrapper.emitted('open')).toBeUndefined()

    await wrapper.find(`[data-rj="${WORKS[3].rj}"]`).trigger('dblclick')
    expect(wrapper.emitted('open')).toEqual([[WORKS[3].rj]])
  })

  it('routes a mobile tap straight to the detail page instead of selecting', async () => {
    // Mobile has no "inspect the selection": the wall never keeps a selected
    // state there, so the media query is what a tap consults.
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: true,
      media: '(max-width: 768px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: () => false,
    } as unknown as MediaQueryList)
    const { wrapper } = await scaffold({ selectedRj: WORKS[0].rj })

    await wrapper.find(`[data-rj="${WORKS[3].rj}"]`).trigger('click')
    expect(wrapper.emitted('select')).toBeUndefined()
    expect(wrapper.emitted('open')).toEqual([[WORKS[3].rj]])
  })

  it('asks for the next keyset page before the scroll reaches the bottom', async () => {
    const { wrapper, setScrollTop } = await scaffold({ selectedRj: WORKS[0].rj })

    setScrollTop(0)
    expect(wrapper.emitted('loadMore')).toHaveLength(1)
  })
})
