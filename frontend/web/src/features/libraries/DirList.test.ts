import type { VueWrapper } from '@vue/test-utils'
import type { LibraryDir } from '@/lib/api/types'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import DirList from './DirList.vue'

/**
 * jsdom has no layout, so the virtualizer's scroller reports offsetHeight 0
 * and renders an empty window (the library's real behavior for a 0-height
 * container). Tests that exercise rows stub the scroller's metrics on the
 * instance right after mount — before the pre-flush watcher measures it.
 */
function scaffold(dirs: LibraryDir[], props: Record<string, unknown> = {}) {
  const wrapper = mount(DirList, {
    props: {
      dirs,
      selectedPaths: [],
      allSelected: false,
      ...props,
    },
  })
  const scroller = wrapper.find('[data-testid="dir-list-scroller"]').element as HTMLElement
  let top = 0
  Object.defineProperties(scroller, {
    offsetHeight: {
      configurable: true,
      get: () => 560,
    },
    offsetWidth: {
      configurable: true,
      get: () => 800,
    },
    scrollTop: {
      configurable: true,
      get: () => top,
      set: (v: number) => {
        top = v
      },
    },
  })
  return {
    wrapper,
    setScrollTop: (v: number): void => {
      top = v
      scroller.dispatchEvent(new Event('scroll'))
    },
  }
}

async function settle(): Promise<void> {
  await flushPromises()
  await nextTick()
}

function makeDirs(count: number): LibraryDir[] {
  return Array.from({ length: count }, (_, i) => ({
    name: `Folder ${i}`,
    path: `/music/f-${i}`,
    rel_path: `f-${i}`,
    dir_id: `dir-${i}`,
    audio_file_count: 3,
    file_count: 5,
    added_at: '2026-05-01T00:00:00Z',
    work_code: null,
  }))
}

function rowOf(wrapper: VueWrapper, testId: string): HTMLElement | null {
  const cell = wrapper.find(`[data-testid="${testId}"]`).element
  return cell?.closest('li') ?? null
}

describe('dirList', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders only the windowed slice with the correct spacer', async () => {
    const { wrapper } = scaffold(makeDirs(100))
    await settle()

    // 20 windowed rows (ceil(560/56)=10 visible + 10 overscan).
    expect(wrapper.findAll('li')).toHaveLength(20)
    expect(wrapper.find('[data-testid="dir-checkbox-f-0"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="dir-checkbox-f-99"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="dir-list-spacer"]').attributes('style')).toContain('height: 5600px')
  })

  it('moves the window and the translateY offset on scroll', async () => {
    const { wrapper, setScrollTop } = scaffold(makeDirs(100))
    await settle()

    setScrollTop(2800) // row 50
    await settle()

    // scrollTop 2800 -> first rendered index 40 (rounded index 50 - overscan).
    expect(wrapper.find('[data-testid="dir-checkbox-f-40"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="dir-name-f-69"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="dir-checkbox-f-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="dir-checkbox-f-70"]').exists()).toBe(false)
    const row = rowOf(wrapper, 'dir-name-f-40')
    expect(row?.getAttribute('style')).toContain('translateY(2240px)')
    expect(wrapper.find('[data-testid="dir-list-spacer"]').attributes('style')).toContain('height: 5600px')
  })

  it('shows the name and only a nested folder\'s parent path, without the audio count', async () => {
    const dirs = makeDirs(2)
    dirs[1] = { ...dirs[1], rel_path: 'cat/f-1' }
    const { wrapper } = scaffold(dirs)
    await settle()

    const flat = wrapper.get('[data-testid="dir-name-f-0"]')
    expect(flat.text()).toBe('Folder 0')
    expect(wrapper.get('[data-testid="dir-name-cat/f-1"]').text()).toBe('Folder 1cat')
    expect(wrapper.text()).not.toContain('音频')
    expect(wrapper.get('[data-testid="dir-list"]').classes()).toContain('min-w-0')
  })

  it('keeps the view action on screen instead of revealing it on hover', async () => {
    const { wrapper } = scaffold(makeDirs(1))
    await settle()

    const view = wrapper.get('[aria-label="打开 Folder 0"]')
    expect(view.classes()).not.toContain('opacity-60')
    expect(view.classes()).not.toContain('group-hover:opacity-100')
  })

  it('shows the folder count next to select-all in the compact layout', async () => {
    const { wrapper } = scaffold(makeDirs(4))
    await settle()

    expect(wrapper.get('[data-testid="dir-count"]').text()).toBe('共 4 个文件夹')
  })

  it('preserves selection across windowing (selection is store-driven)', async () => {
    const { wrapper, setScrollTop } = scaffold(makeDirs(100), {
      selectedPaths: ['f-5', 'f-40'],
    })
    await settle()

    expect((wrapper.find('[data-testid="dir-checkbox-f-5"]').element as HTMLInputElement).checked).toBe(true)

    setScrollTop(2800)
    await settle()
    expect(wrapper.find('[data-testid="dir-checkbox-f-5"]').exists()).toBe(false)
    expect((wrapper.find('[data-testid="dir-checkbox-f-40"]').element as HTMLInputElement).checked).toBe(true)

    setScrollTop(0)
    await settle()
    expect((wrapper.find('[data-testid="dir-checkbox-f-5"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('emits selectAll, select and open, and opens only from the chevron', async () => {
    const { wrapper } = scaffold(makeDirs(3))
    await settle()

    await wrapper.find('[data-testid="select-all-dirs"]').setValue(true)
    expect(wrapper.emitted('selectAll')).toEqual([
      [true],
    ])

    await wrapper.find('[data-testid="dir-checkbox-f-0"]').setValue(true)
    expect(wrapper.emitted('select')).toEqual([
      ['f-0', true],
    ])

    // The name is a label for the checkbox: it never navigates.
    await wrapper.find('[data-testid="dir-name-f-1"]').trigger('click')
    expect(wrapper.emitted('open')).toBeUndefined()

    // Only the button opens the directory's page: the identity it is addressed
    // by, not the name it is displayed under.
    await wrapper.find('[aria-label="打开 Folder 1"]').trigger('click')
    expect(wrapper.emitted('open')).toEqual([
      ['dir-1'],
    ])

    await wrapper.find('[aria-label="打开 Folder 2"]').trigger('click')
    expect(wrapper.emitted('open')).toEqual([
      ['dir-1'],
      ['dir-2'],
    ])
  })

  it('renders no rows for an empty folder set', async () => {
    const { wrapper } = scaffold([])
    await settle()

    expect(wrapper.findAll('li')).toHaveLength(0)
    expect(wrapper.find('[data-testid="dir-list-spacer"]').attributes('style')).toContain('height: 0px')
  })

  it('renders an empty window for a 0-height scroller (documented library behavior)', async () => {
    // No layout stub: jsdom offsetHeight is 0, exactly like a real
    // height-collapsed container. The virtualizer renders nothing in that
    // case — no crash, no ghost rows.
    const wrapper = mount(DirList, {
      props: {
        dirs: makeDirs(2),
        selectedPaths: [],
        allSelected: false,
      },
    })
    await settle()

    expect(wrapper.findAll('li')).toHaveLength(0)
    expect(wrapper.find('[data-testid="dir-checkbox-f-0"]').exists()).toBe(false)
  })
})
