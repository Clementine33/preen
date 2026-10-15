import { createPinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClientKey, ApiError } from '@/lib/api/client'
import type { ScanEvent } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { queryKeys } from '@/queries/query-keys'
import { useLibraryScan } from './use-library-scan'
import { useTaskToast } from './use-task-toast'
import ScanProgressBar from '@/features/libraries/ScanProgressBar.vue'
import { useScanStore } from '@/stores/scan'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'

enableAutoUnmount(afterEach)
afterEach(() => {
  const toast = useTaskToast()
  while (toast.current.value) toast.dismiss(toast.current.value.id)
})

async function harness(path = '/elsewhere') {
  let finish!: (events: ScanEvent[]) => void
  const result = new Promise<ScanEvent[]>((resolve) => { finish = resolve })
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue([{ id: 'lib-a', name: 'Archive' }]),
    scanLibrary: vi.fn((_id, signal) => ({
      async *[Symbol.asyncIterator]() {
        yield { type: 'started', data: { stage: 'scan' } } satisfies ScanEvent
        const events = await Promise.race([
          result,
          new Promise<never>((_, reject) => {
            signal.addEventListener('abort', () => reject(new DOMException('Abort', 'AbortError')), { once: true })
          }),
        ])
        yield* events
      },
    })),
  })
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/worksets/:libraryId', name: 'workbench-overview', component: { template: '<div />' }, children: [
      { path: 'f/:dirId', name: 'overview-files', component: { template: '<div />' } },
    ] },
    { path: '/elsewhere', name: 'worksets', component: { template: '<div />' } },
  ] })
  await router.push(path)
  const plugin = installTestQueryPlugin()
  plugin[1].queryClient.setQueryData(queryKeys.libraries.list(), [{ id: 'lib-a', name: 'Archive' }])
  let scan!: ReturnType<typeof useLibraryScan>
  const wrapper = mount(defineComponent({ setup() {
    scan = useLibraryScan()
    return () => [h(TaskToastHost), h(ScanProgressBar, { status: useScanStore().status, terminal: useScanStore().terminal, filesScanned: 0, dirsScanned: 0 })]
  } }), { attachTo: document.body, global: {
    plugins: [createPinia(), router, plugin], provide: { [apiClientKey as symbol]: api },
  } })
  return { api, router, scan, wrapper, finish }
}

const completed: ScanEvent = { type: 'completed', data: { stage: 'scan', files_scanned: 2 } }
function notice() { return document.body.querySelector('[data-testid="task-toast"]')?.textContent ?? '' }

describe('scan result notifications', () => {
  it('keeps observing across navigation and links to the captured Library', async () => {
    const app = await harness('/worksets/lib-a')
    const pending = app.scan.start('lib-a')
    await flushPromises()
    await app.router.push('/worksets/lib-b')
    app.finish([completed, completed])
    await pending
    await flushPromises()
    expect(notice()).toContain('Archive · 扫描')
    expect(notice()).toContain('扫描完成')
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe('workbench-overview')
    expect(app.router.currentRoute.value.params.libraryId).toBe('lib-a')
    expect(notice()).toBe('')
  })
  it.each(['/worksets/lib-a', '/worksets/lib-a/f/dir-a'])('consumes success on %s without replay on leaving', async (path) => {
    const app = await harness(path)
    const pending = app.scan.start('lib-a')
    app.finish([completed])
    await pending
    await app.router.push('/elsewhere')
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('shows confirmed failure on its own overview', async () => {
    const app = await harness('/worksets/lib-a')
    const pending = app.scan.start('lib-a')
    app.finish([{ type: 'error', data: { stage: 'scan', message: 'Permission denied' } }])
    await pending
    await flushPromises()
    expect(notice()).toContain('Archive · 扫描')
    expect(notice()).toContain('扫描失败')
  })
  it.each([
    ['/worksets/lib-a', false], ['/worksets/lib-a/f/dir-a', false], ['/elsewhere', true],
  ])('local cancel at %s preserves unknown outcome', async (path, visible) => {
    const app = await harness(path)
    const pending = app.scan.start('lib-a')
    await flushPromises()
    app.scan.cancel()
    await pending
    await flushPromises()
    expect(notice().includes('扫描停止已请求，结果尚未确认')).toBe(visible)
    expect(app.wrapper.get('[role="status"]').text()).toContain('扫描停止已请求，结果尚未确认')
    await app.router.push('/elsewhere')
    await flushPromises()
    expect(notice().includes('连接中断')).toBe(false)
  })
  it('shows a single unknown-result notice on unexpected stream loss', async () => {
    const app = await harness('/worksets/lib-a')
    const pending = app.scan.start('lib-a')
    await flushPromises()
    app.finish([])
    await pending
    await flushPromises()
    expect(notice()).toContain('连接中断，扫描结果尚未确认')
    const toast = useTaskToast()
    toast.dismiss(toast.current.value!.id)
    expect(toast.current.value).toBeNull()
  })
  it('refuses a second scan without generating another result', async () => {
    const app = await harness()
    const pending = app.scan.start('lib-a')
    await flushPromises()
    await app.scan.start('lib-b')
    app.finish([completed])
    await pending
    await flushPromises()
    const toast = useTaskToast()
    expect(notice()).toContain('Archive')
    toast.dismiss(toast.current.value!.id)
    expect(toast.current.value).toBeNull()
  })
  it.each(['/worksets/lib-a', '/elsewhere'])('confirmed cancel obeys page ownership at %s', async (path) => {
    const app = await harness(path)
    const pending = app.scan.start('lib-a')
    app.finish([{ type: 'cancelled', data: { stage: 'scan' } }])
    await pending
    await flushPromises()
    expect(notice().includes('扫描已停止')).toBe(path === '/elsewhere')
  })
  it('retains a confirmed result while its stream closes and another scan starts', async () => {
    const app = await harness()
    let close!: () => void
    let terminalSeen!: () => void
    const seen = new Promise<void>(resolve => { terminalSeen = resolve })
    const closing = new Promise<void>(resolve => { close = resolve })
    vi.mocked(app.api.scanLibrary).mockImplementationOnce(() => ({ async *[Symbol.asyncIterator]() {
      try { yield completed } finally { terminalSeen(); await closing }
    } }))
    const first = app.scan.start('lib-a')
    await seen
    const second = app.scan.start('lib-b')
    await flushPromises()
    close()
    await first
    app.finish([completed])
    await second
    await flushPromises()
    expect(notice()).toContain('Archive · 扫描')
    expect(notice()).toContain('扫描完成')
    const tasks = useTaskToast()
    tasks.dismiss(tasks.current.value!.id)
    await flushPromises()
    expect(notice()).toContain('lib-b · 扫描')
    expect(notice()).toContain('扫描完成')
    tasks.dismiss(tasks.current.value!.id)
    expect(tasks.current.value).toBeNull()
  })
  it('keeps admission errors inline without completion notices', async () => {
    const app = await harness()
    vi.mocked(app.api.scanLibrary).mockImplementation(() => ({ async *[Symbol.asyncIterator]() {
      throw new ApiError(409, 'BUSY', 'Busy')
    } }))
    await app.scan.start('lib-a')
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('falls back with an explanation when the captured Library was removed', async () => {
    const app = await harness()
    const pending = app.scan.start('lib-a')
    app.finish([completed])
    await pending
    await flushPromises()
    vi.mocked(app.api.listLibraries).mockResolvedValue([])
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe('worksets')
    expect(notice()).toContain('媒体库已移除，结果不可用')
  })
})
