import { QueryObserver } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClientKey, ApiError } from '@/lib/api/client'
import type { GenerationEvent, GenerationView, OperationType } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { queryKeys } from '@/queries/query-keys'
import { useWorksetGeneration } from './use-workset-generation'
import { useTaskToast } from './use-task-toast'
import { useOperationContext } from './use-operation-context'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'

enableAutoUnmount(afterEach)
const stops: (() => void)[] = []
afterEach(() => {
  stops.splice(0).forEach(stop => stop())
  const toast = useTaskToast()
  while (toast.current.value) toast.dismiss(toast.current.value.id)
  vi.useRealTimers()
})
function session(status: GenerationView['status'] = 'running', id = 'g1', operation: OperationType = 'conversion'): GenerationView {
  return { generation_id: id, workset_id: 'ws1', operation_type: operation, status, total_roots: 2, completed_roots: 1,
    current_root: '', error_count: 4, revision_id: status === 'completed' ? 'plan' : '', error_code: '', error_message: '', started_at: '', finished_at: '', created_at: '' }
}
async function harness(path = '/elsewhere', discovered?: GenerationView) {
  let finish!: (events: GenerationEvent[]) => void
  const stream = new Promise<GenerationEvent[]>(resolve => { finish = resolve })
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue([{ id: 'lib-a', name: 'Archive' }]),
    getWorkset: vi.fn().mockResolvedValue({ workset_id: 'ws1', library: { library_id: 'lib-a', name: 'Archive' }, members: [] }),
    getOperationDraft: vi.fn().mockResolvedValue(null),
    getOperation: vi.fn().mockResolvedValue({ workset_id: 'ws1', operation_type: 'conversion', version: 1, planning_state: 'planning', current_revision: null, active_generation: discovered?.status === 'running' ? discovered : null, latest_generation: discovered ?? null, active_execution: null, latest_execution: null }),
    startGeneration: vi.fn().mockResolvedValue({ created: true, generation: session() }),
    getGeneration: vi.fn().mockResolvedValue(session()),
    cancelGeneration: vi.fn().mockResolvedValue(session('canceled')),
    streamGenerationEvents: vi.fn((_ws, _op, _id, signal) => ({ async *[Symbol.asyncIterator]() {
      yield { type: 'session_snapshot', data: session() } satisfies GenerationEvent
      yield* await Promise.race([stream, new Promise<never>((_, reject) => {
        signal?.addEventListener('abort', () => reject(new DOMException('Abort', 'AbortError')), { once: true })
      })])
    } })),
  })
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/worksets/:libraryId/conversion', name: 'conversion', component: { template: '<div />' }, children: [
      { path: 'settings', name: 'conversion-settings', component: { template: '<div />' } },
      { path: 'execution', name: 'conversion-execution', component: { template: '<div />' } },
      { path: ':memberId/files', name: 'conversion-member-files', component: { template: '<div />' } },
    ] },
    { path: '/worksets/:libraryId/metadata', name: 'metadata-refresh', component: { template: '<div />' } },
    { path: '/worksets/:libraryId/rename', name: 'rename', component: { template: '<div />' } },
    { path: '/elsewhere', name: 'worksets', component: { template: '<div />' } },
  ] })
  await router.push(path)
  const plugin = installTestQueryPlugin()
  const client = plugin[1].queryClient
  client.setQueryData(queryKeys.libraries.list(), [{ id: 'lib-a', name: 'Archive' }])
  client.setQueryData(queryKeys.worksets.detail('ws1'), { library: { library_id: 'lib-a', name: 'Archive' } })
  let generation!: ReturnType<typeof useWorksetGeneration>
  const pinia = createPinia()
  const options = { global: { plugins: [pinia, router, plugin], provide: { [apiClientKey as symbol]: api } } }
  const visible = ref(true)
  const Page = defineComponent({ setup() { generation = discovered ? useOperationContext(ref('ws1')).generation : useWorksetGeneration(); return () => h('div') } })
  const page = mount(defineComponent({ setup() { return () => visible.value ? h(Page) : null } }), options)
  const host = mount(TaskToastHost, { ...options, attachTo: document.body })
  stops.push(() => { generation.stop(); client.clear() })
  const start = async (operation: OperationType = 'conversion') => {
    await generation.start({ worksetId: 'ws1', operation, ifMatchVersion: 1, idempotencyKey: 'key' })
    await flushPromises()
  }
  return { api, router, generation, page, host, client, finish, start, leave: () => { visible.value = false } }
}
function notice() { return document.body.querySelector('[data-testid="task-toast"]')?.textContent ?? '' }
function dismiss() { const t = useTaskToast(); if (t.current.value) t.dismiss(t.current.value.id) }

describe('app-lifetime planning observation', () => {
  it('notifies after the initiating page unmounts with captured identity and a current-task link', async () => {
    const app = await harness('/worksets/lib-a/conversion')
    await app.start()
    app.leave()
    await app.router.push('/worksets/lib-b/rename')
    app.finish([{ type: 'completed', data: { generation_id: 'g1', revision_id: 'plan' } }])
    await flushPromises()
    expect(notice()).toContain('Archive · 转换 · 规划')
    expect(notice()).toContain('计划生成完成')
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe('conversion')
    expect(app.router.currentRoute.value.params.libraryId).toBe('lib-a')
  })
  it('keeps observing an accepted session whose start answer carries no known status', async () => {
    const app = await harness()
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session('' as GenerationView['status']) })
    await app.start()
    expect(notice()).toBe('')
    expect(app.api.streamGenerationEvents).toHaveBeenCalledWith('ws1', 'conversion', 'g1', expect.any(AbortSignal))
    app.finish([{ type: 'completed', data: { generation_id: 'g1', revision_id: 'plan' } }])
    await flushPromises()
    expect(notice()).toContain('计划生成完成')
  })
  it('warns once on loss and follows the exact session through restart interruption', async () => {
    vi.useFakeTimers()
    const app = await harness()
    vi.mocked(app.api.getGeneration).mockResolvedValueOnce(session()).mockResolvedValueOnce(session('interrupted'))
    await app.start()
    app.finish([])
    await flushPromises()
    expect(notice()).toContain('连接中断，计划生成结果尚未确认')
    expect(app.api.getGeneration).toHaveBeenCalledWith('ws1', 'conversion', 'g1', expect.any(AbortSignal))
    dismiss()
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('未发布新计划，原有计划仍保留')
    expect(app.client.getQueryData(queryKeys.worksets.generation('ws1', 'conversion', 'g1'))).toMatchObject({ status: 'interrupted' })
    dismiss()
    await vi.advanceTimersByTimeAsync(30000)
    expect(useTaskToast().current.value).toBeNull()
  })

  it('reports a replaced plan when another tab publishes before the next observation', async () => {
    const app = await harness()
    const unsubscribe = new QueryObserver(app.client, {
      queryKey: queryKeys.worksets.operation('ws1', 'conversion'),
      queryFn: () => app.api.getOperation('ws1', 'conversion'),
    }).subscribe(() => {})
    stops.push(unsubscribe)
    await app.start()
    vi.mocked(app.api.getGeneration).mockResolvedValueOnce({ ...session('completed'), revision_id: '' })
    vi.mocked(app.api.getOperation).mockResolvedValue({ workset_id: 'ws1', operation_type: 'conversion', version: 3,
      planning_state: 'planned', current_revision: { plan_id: 'other-plan', revision_index: 2, created_at: '',
        status: 'ready', summary_reason: '', counts: { members: 1, changed: 1, unmet_targets: 0, blocked: 0, unchanged: 0 },
        validation_state: 'valid', stale: false }, active_generation: null,
      latest_generation: session('completed'), active_execution: null, latest_execution: null } satisfies Awaited<ReturnType<typeof app.api.getOperation>>)
    app.finish([])
    await vi.waitFor(() => expect(app.api.getGeneration).toHaveBeenCalledTimes(1))
    dismiss()
    await flushPromises()
    expect(notice()).toContain('原计划已被替换')
    expect(app.api.getOperation).toHaveBeenCalled()
    expect(app.api.getRevision).not.toHaveBeenCalled()
    vi.useFakeTimers()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(app.api.getGeneration).toHaveBeenCalledTimes(1)
  })

  it('observes a second accepted session independently without disturbing the stream', async () => {
    vi.useFakeTimers()
    const app = await harness()
    await app.start()
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session('running', 'g2', 'rename') })
    vi.mocked(app.api.getGeneration).mockResolvedValueOnce({ ...session('running', 'g2', 'rename'), completed_roots: 2 }).mockResolvedValueOnce(session('completed', 'g2', 'rename'))
    vi.mocked(app.api.getOperation).mockResolvedValue({ workset_id: 'ws1', operation_type: 'rename', version: 1, planning_state: 'planning', current_revision: null, active_generation: session('running', 'g2', 'rename'), latest_generation: null, active_execution: null, latest_execution: null })
    const unsubscribe = new QueryObserver(app.client, { queryKey: queryKeys.worksets.operation('ws1', 'rename'), queryFn: () => app.api.getOperation('ws1', 'rename') }).subscribe(() => {})
    stops.push(unsubscribe)
    await app.start('rename')
    expect(app.client.getQueryData(queryKeys.worksets.operation('ws1', 'rename'))).toMatchObject({ active_generation: { generation_id: 'g2', completed_roots: 2 } })
    expect(app.generation.store.generationId).toBe('g1')
    expect(app.api.getGeneration).toHaveBeenCalledWith('ws1', 'rename', 'g2', expect.any(AbortSignal))
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('重命名 · 规划')
    expect(notice()).toContain('计划生成完成')
    app.finish([{ type: 'completed', data: { generation_id: 'g1', revision_id: 'plan' } }])
    await flushPromises()
    dismiss()
    await flushPromises()
    expect(notice()).toContain('转换 · 规划')
    expect(notice()).toContain('计划生成完成')
    expect(app.client.getQueryData(queryKeys.worksets.generation('ws1', 'conversion', 'g1'))).toMatchObject({ status: 'completed', revision_id: 'plan' })
    expect(app.client.getQueryData(queryKeys.worksets.generation('ws1', 'rename', 'g2'))).toMatchObject({ status: 'completed' })
  })

  it('re-observes an active conversion found on a visited operation after reload', async () => {
    const app = await harness('/worksets/lib-a/conversion', session())
    await flushPromises()
    expect(app.generation.store.generationId).toBe('g1')
    app.leave()
    await app.router.push('/elsewhere')
    app.finish([{ type: 'interrupted', data: { generation_id: 'g1' } }])
    await flushPromises()
    expect(notice()).toContain('计划生成已中断')
  })

  it.each(['conversion', 'rename', 'metadata_refresh'] as const)('maps every terminal outcome for %s without a partial category', async operation => {
    const label = { conversion: '转换', rename: '重命名', metadata_refresh: '获取元数据' }[operation]
    const app = await harness()
    for (const [status, text, tone] of [['completed', '计划生成完成', 'success'], ['failed', '计划生成失败', 'failed'], ['canceled', '计划生成已停止', 'stopped'], ['interrupted', '计划生成已中断', 'interrupted']] as const) {
      vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session(status, status, operation) })
      await app.start(operation)
      expect(notice()).toContain(`Archive · ${label} · 规划`)
      expect(notice()).toContain(text)
      expect(document.body.querySelector('[data-testid="task-toast"]')?.getAttribute('data-tone')).toBe(tone)
      expect(notice()).not.toContain('部分')
      dismiss()
      await flushPromises()
    }
  })
  it.each([
    ['conversion', '/worksets/lib-a/conversion'],
    ['conversion', '/worksets/lib-a/conversion/settings'],
    ['conversion', '/worksets/lib-a/conversion/execution'],
    ['conversion', '/worksets/lib-a/conversion/member/files'],
    ['rename', '/worksets/lib-a/rename'],
    ['metadata_refresh', '/worksets/lib-a/metadata'],
  ] as const)('consumes success/stopped on corresponding %s route %s', async (operation, path) => {
    const app = await harness(path)
    for (const status of ['completed', 'canceled'] as const) {
      vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session(status, status, operation) })
      await app.start(operation)
      expect(notice()).toBe('')
    }
    await app.router.push('/elsewhere')
    app.generation.attach('ws1', operation, 'completed')
    app.generation.attach('ws1', operation, 'canceled')
    await flushPromises()
    expect(notice()).toBe('')
  })
  it.each(['failed', 'interrupted'] as const)('shows %s on the corresponding task', async status => {
    const app = await harness('/worksets/lib-a/conversion/settings')
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session(status) })
    await app.start()
    expect(notice()).toContain(status === 'failed' ? '失败' : '中断')
  })
  it.each(['/worksets/lib-b/conversion', '/worksets/lib-a/rename'])('treats %s as away', async path => {
    const app = await harness(path)
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session('completed') })
    await app.start()
    expect(notice()).toContain('计划生成完成')
  })
  it('retains eligibility for a created session already terminal before its first snapshot', async () => {
    const app = await harness()
    vi.mocked(app.api.streamGenerationEvents).mockImplementation(() => ({ async *[Symbol.asyncIterator]() {
      yield { type: 'session_snapshot', data: session('completed') } satisfies GenerationEvent
    } }))
    await app.start()
    expect(notice()).toContain('计划生成完成')
    app.generation.attach('ws1', 'conversion', 'g1')
    await app.start()
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('does not replay a historical terminal operation after reload', async () => {
    const app = await harness('/worksets/lib-a/conversion', session('completed'))
    await flushPromises()
    expect(app.generation.store.status).toBe('idle')
    expect(notice()).toBe('')
  })
  it('continues after a nonterminal cancel response without a false disconnect warning', async () => {
    vi.useFakeTimers()
    const app = await harness()
    await app.start()
    vi.mocked(app.api.cancelGeneration).mockResolvedValue(session())
    vi.mocked(app.api.getGeneration).mockResolvedValueOnce(session()).mockResolvedValueOnce(session('canceled'))
    await app.generation.cancel('ws1', 'conversion', 'g1')
    await flushPromises()
    expect(notice()).toBe('')
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('计划生成已停止')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('deduplicates SSE/cancel races and synchronizes affected caches once', async () => {
    const app = await harness()
    await app.start()
    app.client.setQueryData(queryKeys.worksets.revision('ws1', 'conversion', 'old'), 'old plan')
    let canceled!: (view: GenerationView) => void
    vi.mocked(app.api.cancelGeneration).mockImplementation(() => new Promise(resolve => { canceled = resolve }))
    const pending = app.generation.cancel('ws1', 'conversion', 'g1')
    app.finish([{ type: 'completed', data: { generation_id: 'g1', revision_id: 'new' } }])
    await flushPromises()
    canceled(session('canceled'))
    await pending
    await flushPromises()
    expect(notice()).toMatch(/计划生成(完成|已停止)/)
    expect(app.client.getQueryData(queryKeys.worksets.revision('ws1', 'conversion', 'old'))).toBeUndefined()
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it.each([401, 403, 404])('stops missing/inaccessible session recovery on %s with an actionable unknown result', async status => {
    vi.useFakeTimers()
    const app = await harness()
    vi.mocked(app.api.getGeneration).mockRejectedValue(new ApiError(status, 'UNAVAILABLE', 'Unavailable'))
    await app.start()
    app.finish([])
    await flushPromises()
    expect(notice()).toContain('结果尚未确认')
    dismiss()
    await flushPromises()
    expect(notice()).toContain('记录不存在或无权访问')
    expect(document.body.querySelector('[data-testid="task-toast-action"]')).not.toBeNull()
    const reads = vi.mocked(app.api.getGeneration).mock.calls.length
    await vi.advanceTimersByTimeAsync(60000)
    expect(vi.mocked(app.api.getGeneration).mock.calls.length).toBe(reads)
  })
  it('backs off transient reads to 30 seconds and resets to active cadence on recovery', async () => {
    vi.useFakeTimers()
    const app = await harness()
    vi.mocked(app.api.getGeneration).mockRejectedValue(new ApiError(503, 'OFFLINE', 'Offline'))
    await app.start()
    app.finish([])
    await flushPromises()
    dismiss()
    for (const delay of [2000, 4000, 8000, 16000, 30000, 30000]) {
      const reads = vi.mocked(app.api.getGeneration).mock.calls.length
      await vi.advanceTimersByTimeAsync(delay - 1)
      expect(vi.mocked(app.api.getGeneration).mock.calls.length).toBe(reads)
      await vi.advanceTimersByTimeAsync(1)
      await flushPromises()
      expect(vi.mocked(app.api.getGeneration).mock.calls.length).toBe(reads + 1)
      expect(notice()).toBe('')
    }
    vi.mocked(app.api.getGeneration).mockResolvedValueOnce(session()).mockResolvedValueOnce(session('completed'))
    await vi.advanceTimersByTimeAsync(30000)
    await flushPromises()
    expect(notice()).toBe('')
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('计划生成完成')
  })
  it('stops pending recovery reads and timers on app teardown', async () => {
    vi.useFakeTimers()
    const app = await harness()
    let signal: AbortSignal | undefined
    vi.mocked(app.api.getGeneration).mockImplementation((_ws, _op, _id, requestSignal) => {
      signal = requestSignal
      return new Promise(() => {})
    })
    await app.start()
    app.finish([])
    await flushPromises()
    app.page.unmount()
    expect(signal?.aborted).toBe(true)
    dismiss()
    await vi.advanceTimersByTimeAsync(60000)
    expect(notice()).toBe('')
  })
  it('falls back gracefully when the captured library was removed', async () => {
    const app = await harness()
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session('completed') })
    await app.start()
    vi.mocked(app.api.listLibraries).mockResolvedValue([])
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe('worksets')
    expect(notice()).toContain('媒体库已移除，结果不可用')
  })

  it('keeps the confirmed cancel when a pending GET races with cancellation', async () => {
    const app = await harness()
    let resolveRead!: (view: GenerationView) => void
    vi.mocked(app.api.getGeneration).mockImplementation(() => new Promise(resolve => { resolveRead = resolve }))
    await app.start()
    app.finish([])
    await flushPromises()
    dismiss()
    await app.generation.cancel('ws1', 'conversion', 'g1')
    resolveRead(session('completed'))
    await flushPromises()
    expect(notice()).toContain('计划生成已停止')
    expect(app.client.getQueryData(queryKeys.worksets.generation('ws1', 'conversion', 'g1'))).toMatchObject({ status: 'canceled' })
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })

  it('retains per-session completion while an old stream closes and a new stream starts', async () => {
    const app = await harness()
    let close!: () => void
    let seen!: () => void
    const closing = new Promise<void>(resolve => { close = resolve })
    const terminal = new Promise<void>(resolve => { seen = resolve })
    vi.mocked(app.api.streamGenerationEvents).mockImplementationOnce(() => ({ async *[Symbol.asyncIterator]() {
      try { yield { type: 'completed', data: { generation_id: 'g1', revision_id: 'plan' } } satisfies GenerationEvent }
      finally { seen(); await closing }
    } }))
    await app.start()
    await terminal
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session('running', 'g2', 'rename') })
    await app.start('rename')
    close()
    app.finish([{ type: 'failed', data: { generation_id: 'g2', error_code: 'FAIL', error_message: 'Failed' } }])
    await flushPromises()
    expect(notice()).toContain('转换 · 规划')
    expect(notice()).toContain('计划生成完成')
    dismiss()
    await flushPromises()
    expect(notice()).toContain('重命名 · 规划')
    expect(notice()).toContain('计划生成失败')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('keeps idempotency session replays eligible and consumes repeated registration', async () => {
    const app = await harness()
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: false, generation: session('completed') })
    await app.start()
    await app.start()
    app.generation.attach('ws1', 'conversion', 'g1')
    expect(notice()).toContain('计划生成完成')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('keeps admission errors inline without inventing a session', async () => {
    const app = await harness()
    vi.mocked(app.api.startGeneration).mockRejectedValue(new ApiError(409, 'BUSY', 'Busy'))
    await expect(app.start()).rejects.toMatchObject({ code: 'BUSY' })
    expect(notice()).toBe('')
    expect(app.api.getGeneration).not.toHaveBeenCalled()
  })
  it.each(['conversion', 'rename', 'metadata_refresh'] as const)('links %s results to its current task page', async operation => {
    const app = await harness()
    vi.mocked(app.api.startGeneration).mockResolvedValue({ created: true, generation: session('completed', 'g1', operation) })
    await app.start(operation)
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe(operation === 'metadata_refresh' ? 'metadata-refresh' : operation)
    expect(app.router.currentRoute.value.params.libraryId).toBe('lib-a')
  })

  it('captures the current Workset Library before admission resolves across navigation', async () => {
    const app = await harness('/worksets/lib-a/metadata')
    app.client.removeQueries({ queryKey: queryKeys.worksets.detail('ws1') })
    app.client.setQueryData(queryKeys.worksets.current('lib-a'), { workset: { workset_id: 'ws1', library: { library_id: 'lib-a', name: 'Archive' } } })
    let accepted!: (result: { created: true; generation: GenerationView }) => void
    vi.mocked(app.api.startGeneration).mockImplementation(() => new Promise(resolve => { accepted = resolve }))
    const pending = app.start('metadata_refresh')
    await flushPromises()
    await app.router.push('/worksets/lib-b/rename')
    accepted({ created: true, generation: session('completed', 'g1', 'metadata_refresh') })
    await pending
    expect(notice()).toContain('Archive · 获取元数据 · 规划')
    expect(notice()).toContain('计划生成完成')
  })
  it('recovers an accepted session when the stream fails before any event', async () => {
    const app = await harness()
    vi.mocked(app.api.streamGenerationEvents).mockImplementation(() => ({ async *[Symbol.asyncIterator]() { throw new TypeError('Offline') } }))
    vi.mocked(app.api.getGeneration).mockResolvedValue(session('completed'))
    await app.start()
    expect(notice()).toContain('连接中断，计划生成结果尚未确认')
    dismiss()
    await flushPromises()
    expect(notice()).toContain('计划生成完成')
  })

})
