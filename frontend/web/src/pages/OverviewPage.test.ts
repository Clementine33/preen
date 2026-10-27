import type { VueWrapper } from '@vue/test-utils'
import type { Router } from 'vue-router'
import type { Library, LibraryDir, MediaFactsStatus, Workset } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import { useLibraryUiStore } from '@/stores/library-ui'
import { useMediaFactsStore } from '@/stores/media-facts'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import OverviewPage from './OverviewPage.vue'

/** The page's selection store, as a row click would drive it. */
function ui() {
  return useLibraryUiStore()
}

/**
 * The overview is where a library's conversion scope is chosen (ADR 0001).
 * These tests drive the real page against a stubbed API: what they pin is the
 * behavior the record contract names — the create carries the selected library-relative
 * paths and the record it saw, a creation reports the folders it skipped, and
 * replacing an existing record is confirmed first.
 *
 * The directory rows themselves are virtualized and covered by the DirList
 * test; here the selection is driven through the same store action a row click
 * calls, so these tests are about the page contract, not about scrolling.
 */

const library: Library = {
  id: 'lib-1',
  name: 'Archive',
  root_path: '/music',
  created_at: '',
  updated_at: '',
  last_scan_at: null,
  last_scan_status: 'completed',
  last_scan_error: '',
  storage_class: 'hdd',
}

const dirs: LibraryDir[] = [
  {
    name: 'albumA',
    path: '/music/albumA',
    rel_path: 'albumA',
    dir_id: 'dir-albumA',
    audio_file_count: 3,
    file_count: 4,
    added_at: '2026-05-01T00:00:00Z',
    work_code: null,
  },
  {
    name: 'docs',
    path: '/music/docs',
    rel_path: 'docs',
    dir_id: 'dir-docs',
    audio_file_count: 0,
    file_count: 1,
    added_at: '2026-05-02T00:00:00Z',
    work_code: null,
  },
]

const record: Workset = {
  workset_id: 'ws-1',
  title: 'Archive',
  version: 1,
  library: { library_id: 'lib-1', name: 'Archive', root_path: '/music' },
  members: [
    {
      member_id: 'm-1',
      folder_path: '/music/albumA',
      folder_name: 'albumA',
      rel_path: 'albumA',
      dir_id: 'dir-albumA',
    },
  ],
  operations: [],
  updated_at: '',
  created_at: '',
}

function routerFor(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets/:libraryId', name: 'workbench-overview', component: OverviewPage },
      {
        path: '/worksets/:libraryId/f/:dirId',
        name: 'overview-files',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/conversion',
        name: 'conversion',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/conversion/settings',
        name: 'conversion-settings',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/rename',
        name: 'rename',
        component: { template: '<div />' },
      },
      {
        path: '/worksets/:libraryId/metadata',
        name: 'metadata-refresh',
        component: { template: '<div />' },
      },
      {
        path: '/worksets',
        name: 'worksets',
        component: { template: '<div />' },
      },
    ],
  })
}

async function mountOverview(overrides: Partial<Record<string, unknown>> = {}): Promise<{
  wrapper: VueWrapper
  api: ReturnType<typeof apiStub>
  router: Router
}> {
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue([library]),
    listDirs: vi.fn().mockResolvedValue(dirs),
    getCurrentRecord: vi.fn().mockResolvedValue({ workset: null }),
    ...overrides,
  } as never)
  const router = routerFor()
  await router.push('/worksets/lib-1')
  await router.isReady()
  const wrapper = mount(OverviewPage, {
    attachTo: document.body,
    global: {
      plugins: [
        createPinia(),
        router,
        installTestQueryPlugin(),
      ],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, api, router }
}

enableAutoUnmount(afterEach)

beforeEach(() => {
  localStorage.clear()
})

it.each([
  ['scan-button', undefined],
  ['full-rescan-button', 'full'],
] as const)('starts a scan through %s', async (entry, mode) => {
  const scanLibrary = vi.fn(async function* () {
    yield {
      type: 'started',
      data: { stage: 'scan' },
    } as const
    await new Promise<void>((resolve) => { finish = resolve })
    yield {
      type: 'completed',
      data: { stage: 'scan', scan_id: 'scan-1' },
    } as const
  })
  let finish = () => {}
  const { wrapper } = await mountOverview({ scanLibrary })
  await wrapper.get(`[data-testid="${entry}"]`).trigger('click')
  expect(scanLibrary).toHaveBeenCalledWith('lib-1', expect.any(AbortSignal), undefined, mode)
  expect(wrapper.get('[data-testid="scan-button"]').attributes('disabled')).toBeDefined()
  expect(wrapper.get('[data-testid="full-rescan-button"]').attributes('disabled')).toBeDefined()
  finish()
  await flushPromises()
})

describe('workbench overview', () => {
  it('lists what the scan saw, and offers no action until something is selected', async () => {
    const { wrapper, api } = await mountOverview()

    expect(api.listDirs).toHaveBeenCalled()
    // The overview shares the workbench navigation with the conversion page
    // instead of hand-rolling its own entry list. jsdom's zero-width
    // container is the narrow tier, so the same list lives in the drawer.
    await wrapper.get('[data-testid="workbench-nav-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="nav-overview"]').attributes('aria-current')).toBe('page')
    expect(wrapper.find('[data-testid="nav-conversion"]').exists()).toBe(true)
    await wrapper.get('[data-testid="workbench-nav-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="dir-list"]').text()).toContain('共 2 个文件夹')
    // The overview has no detail to show, so it does not reserve the wide
    // tier's detail column: the list is what the page is.
    expect(wrapper.find('[data-testid="workbench-detail-inline"]').exists()).toBe(false)
    // Browsing alone creates nothing, and there is no empty action strip.
    expect(wrapper.find('[data-testid="enter-conversion"]').exists()).toBe(false)

    ui().toggleDir('albumA', dirs)
    await flushPromises()
    expect(wrapper.get('[data-testid="enter-rename"]').text()).toContain('重命名')
    expect(wrapper.find('[data-testid="enter-metadata-refresh"]').exists()).toBe(true)
  })

  it('creates the record from the selected library-relative paths', async () => {
    const createCurrentRecord = vi.fn().mockResolvedValue({
      workset: record,
      created: true,
      recorded: 1,
      skipped: [],
    })
    const { wrapper, router } = await mountOverview({ createCurrentRecord })
    ui().toggleDir('albumA', dirs)
    await flushPromises()
    await wrapper.get('[data-testid="enter-conversion"]').trigger('click')
    await flushPromises()

    const request = createCurrentRecord.mock.calls[0] as unknown as [string, string, { folder_paths: string[] }]
    expect(request[0]).toBe('lib-1')
    expect(request[1]).toBe('conversion')
    expect(request[2].folder_paths).toEqual(['albumA'])
    expect(router.currentRoute.value.name).toBe('conversion')
  })

  describe('search and order', () => {
    const coded: LibraryDir[] = [
      {
        name: 'RJ01000001 a',
        path: '/music/a',
        rel_path: 'a',
        dir_id: 'd-a',
        audio_file_count: 1,
        file_count: 1,
        added_at: '2026-05-03T00:00:00Z',
        work_code: 'RJ01000001',
      },
      {
        name: 'RJ01000003 c',
        path: '/music/c',
        rel_path: 'c',
        dir_id: 'd-c',
        audio_file_count: 1,
        file_count: 1,
        added_at: '2026-05-01T00:00:00Z',
        work_code: 'RJ01000003',
      },
      {
        name: 'plain',
        path: '/music/plain',
        rel_path: 'plain',
        dir_id: 'd-p',
        audio_file_count: 1,
        file_count: 1,
        added_at: '2026-05-02T00:00:00Z',
        work_code: null,
      },
    ]

    async function createWith(sortTestId: string) {
      const createCurrentRecord = vi.fn().mockResolvedValue({
        workset: record,
        created: true,
        recorded: 3,
        skipped: [],
      })
      const { wrapper } = await mountOverview({
        listDirs: vi.fn().mockResolvedValue(coded),
        createCurrentRecord,
      })
      await wrapper.get(`[data-testid="${sortTestId}"]`).trigger('click')
      // Click order is the reverse of every sort: it must not leak into the batch.
      for (const rel of ['plain', 'a', 'c']) ui().toggleDir(rel, coded)
      await flushPromises()
      await wrapper.get('[data-testid="enter-conversion"]').trigger('click')
      await flushPromises()
      return (createCurrentRecord.mock.calls[0] as unknown as [string, string, { folder_paths: string[] }])[2].folder_paths
    }

    it('freezes the RJ-descending order, codeless folders last, into the batch', async () => {
      expect(await createWith('sort-rjid')).toEqual(['c', 'a', 'plain'])
    })

    it('freezes the most-recently-added order into the batch', async () => {
      expect(await createWith('sort-added')).toEqual(['a', 'plain', 'c'])
    })

    it('narrows the list by search and counts what is shown', async () => {
      const { wrapper } = await mountOverview({
        listDirs: vi.fn().mockResolvedValue(coded),
      })
      await wrapper.get('[data-testid="dir-search"]').setValue('rj01000003')
      expect(wrapper.get('[data-testid="dir-list"]').text()).toContain('共 1 个文件夹')
      expect(wrapper.text()).toContain('文件夹（1 / 3）')
    })
  })

  it('keeps audio-less folders in the shared batch', async () => {
    const createCurrentRecord = vi.fn().mockResolvedValue({
      workset: record,
      created: true,
      recorded: 1,
      skipped: [],
    })
    const { wrapper, router } = await mountOverview({ createCurrentRecord })
    ui().toggleDir('docs', dirs)
    await flushPromises()
    await wrapper.get('[data-testid="enter-conversion"]').trigger('click')
    await flushPromises()

    const request = createCurrentRecord.mock.calls[0] as unknown as [string, string, { folder_paths: string[] }]
    expect(request[1]).toBe('conversion')
    expect(request[2].folder_paths).toEqual(['docs'])
    expect(router.currentRoute.value.name).toBe('conversion')
  })

  it('reports the folders a creation skipped instead of hiding them', async () => {
    const createCurrentRecord = vi.fn().mockResolvedValue({
      workset: record,
      created: true,
      recorded: 1,
      skipped: [
        { path: 'docs', reason: 'missing' },
      ],
    })
    const { wrapper } = await mountOverview({ createCurrentRecord })
    ui().toggleDir('albumA', dirs)
    await flushPromises()
    await wrapper.get('[data-testid="enter-rename"]').trigger('click')
    await flushPromises()

    // Staying on the overview is what lets the skipped folders be read.
    expect(wrapper.get('[data-testid="skipped-report"]').text()).toContain('docs')
    expect(wrapper.get('[data-testid="skipped-report"]').text()).toContain('目录不存在')
  })

  it('asks before replacing an existing record', async () => {
    const createCurrentRecord = vi.fn().mockResolvedValue({
      workset: record,
      created: true,
      recorded: 1,
      skipped: [],
    })
    const { wrapper } = await mountOverview({
      getCurrentRecord: vi.fn().mockResolvedValue({ workset: record }),
      createCurrentRecord,
    })

    expect(wrapper.find('[data-testid="current-record"]').exists()).toBe(false)
    ui().toggleDir('albumA', dirs)
    await flushPromises()

    await wrapper.get('[data-testid="enter-conversion"]').trigger('click')
    await flushPromises()

    // Nothing was sent until the confirmation is accepted, and the dialog
    // says what replacing costs (ADR 0001). The modal is portalled, so it is
    // the document that holds it.
    expect(createCurrentRecord).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('删除所有 Task 的成员设置、当前计划与执行结果')
    expect(document.body.textContent).toContain('全局设置会保留')
    const confirm = document.body.querySelector('[data-testid="confirm-replace"]') as HTMLElement
    confirm.click()
    await flushPromises()

    const request = createCurrentRecord.mock.calls[0] as unknown as [string, string, { expected_current_id?: string }]
    expect(request[2].expected_current_id).toBe('ws-1')
  })

  it('surfaces a refused creation with the folders it could not use', async () => {
    const { ApiError } = await import('@/lib/api/client')
    const createCurrentRecord = vi
      .fn()
      .mockRejectedValue(
        new ApiError(400, 'NO_MEMBERS', 'none of the selected folders can join', ['docs: missing']),
      )
    const { wrapper } = await mountOverview({ createCurrentRecord })
    ui().toggleDir('albumA', dirs)
    await flushPromises()
    await wrapper.get('[data-testid="enter-conversion"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="create-error"]').text()).toContain('none of the selected folders')
    expect(wrapper.get('[data-testid="skipped-report"]').text()).toContain('docs')
  })
})

describe('media facts status line', () => {
  async function lineFor(status: MediaFactsStatus | null): Promise<string | null> {
    const { wrapper } = await mountOverview()
    useMediaFactsStore().status = status
    await flushPromises()
    const line = wrapper.find('[data-testid="media-facts-status"]')
    return line.exists()
      ? line.text()
      : null
  }
  const counts = (pending: number, unmeasurable: number) => [
    { library_id: 'lib-1', pending, unmeasurable },
  ]

  it('shows what is left while measuring, and that it will resume when paused', async () => {
    expect(await lineFor({
      state: 'running',
      libraries: counts(12345, 2),
    })).toBe('媒体事实 · 测量中 · 剩 12,345')
    expect(await lineFor({
      state: 'paused',
      libraries: counts(7, 0),
    })).toBe('媒体事实 · 已暂停，将自动继续 · 剩 7')
  })

  it('reports files that cannot be measured once nothing is pending', async () => {
    expect(await lineFor({
      state: 'idle',
      libraries: counts(0, 1500),
    })).toBe('媒体事实 · 1,500 个文件无法测量')
    expect(await lineFor({
      state: 'running',
      libraries: counts(0, 3),
    })).toBe('媒体事实 · 3 个文件无法测量')
  })

  it('stays hidden without a status, or when this library has nothing unknown', async () => {
    expect(await lineFor(null)).toBeNull()
    expect(await lineFor({ state: 'running', libraries: [] })).toBeNull()
    expect(await lineFor({
      state: 'running',
      libraries: [
        { library_id: 'other', pending: 5, unmeasurable: 5 },
      ],
    })).toBeNull()
  })
})
