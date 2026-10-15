import { QueryObserver } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClientKey, ApiError } from '@/lib/api/client'
import type { ExecutionEvent, ExecutionView, OperationType } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { queryKeys } from '@/queries/query-keys'
import { useWorksetExecution } from './use-workset-execution'
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
function session(status: ExecutionView['status'] = 'running', id = 'e1', operation: OperationType = 'conversion', completed = 0): ExecutionView {
  return { execution_id: id, workset_id: 'ws1', operation_type: operation, plan_id: 'plan', status,
    total_components: 300, completed_components: 1, total_operations: 5, completed_operations: completed,
    current_root: '', current_component_id: '', current_phase: '', components: [],
    error_code: '', error_message: '', started_at: '', finished_at: '', created_at: '' }
}
async function harness(path = '/elsewhere', discovered?: ExecutionView) {
  let finish!: (events: ExecutionEvent[]) => void
  const stream = new Promise<ExecutionEvent[]>(resolve => { finish = resolve })
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue([{ id: 'lib-a', name: 'Archive' }]),
    getWorkset: vi.fn().mockResolvedValue({ workset_id: 'ws1', library: { library_id: 'lib-a', name: 'Archive' }, members: [] }),
    getOperationDraft: vi.fn().mockResolvedValue(null),
    getOperation: vi.fn().mockResolvedValue({ workset_id: 'ws1', operation_type: 'conversion', version: 1, planning_state: 'planning', current_revision: null, active_generation: null, latest_generation: null, active_execution: discovered?.status === 'running' ? discovered : null, latest_execution: discovered ?? null }),
    startExecution: vi.fn().mockResolvedValue({ created: true, execution: session() }),
    getExecution: vi.fn().mockResolvedValue(session()),
    cancelExecution: vi.fn().mockResolvedValue(session('canceled')),
    streamExecutionEvents: vi.fn((_ws, _op, _id, signal) => ({ async *[Symbol.asyncIterator]() {
      yield { type: 'execution_snapshot', data: session() } satisfies ExecutionEvent
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
  let execution!: ReturnType<typeof useWorksetExecution>
  const pinia = createPinia()
  const options = { global: { plugins: [pinia, router, plugin], provide: { [apiClientKey as symbol]: api } } }
  const visible = ref(true)
  const Page = defineComponent({ setup() { execution = discovered ? useOperationContext(ref('ws1')).execution : useWorksetExecution(); return () => h('div') } })
  const page = mount(defineComponent({ setup() { return () => visible.value ? h(Page) : null } }), options)
  const host = mount(TaskToastHost, { ...options, attachTo: document.body })
  stops.push(() => { execution.stop(); client.clear() })
  const start = async (operation: OperationType = 'conversion') => {
    await execution.start({ worksetId: 'ws1', operation, planId: 'plan', ifMatchVersion: 1, idempotencyKey: 'key' })
    await flushPromises()
  }
  function mountContext() {
    let context!: ReturnType<typeof useOperationContext>
    mount(defineComponent({ setup() { context = useOperationContext(ref('ws1')); return () => h('div') } }), options)
    return context
  }
  return { api, router, execution, page, host, client, finish, start, mountContext, leave: () => { visible.value = false } }
}
function notice() { return document.body.querySelector('[data-testid="task-toast"]')?.textContent ?? '' }
function dismiss() { const t = useTaskToast(); if (t.current.value) t.dismiss(t.current.value.id) }

describe('app-lifetime conversion execution observation', () => {
  it.each([0, 2])('classifies failed execution from authoritative completed operations (%s), including a first failed component', async completed => {
    const app = await harness('/worksets/lib-a/conversion/settings')
    vi.mocked(app.api.getExecution).mockResolvedValue(session('failed', 'e1', 'conversion', completed))
    await app.start()
    app.finish([{ type: 'failed', data: { execution_id: 'e1', error_code: 'FAIL', error_message: 'Failed' } }])
    await flushPromises()
    expect(notice()).toContain(completed ? '执行部分失败，已完成 2 项操作' : '执行失败')
    expect(app.api.getExecution).toHaveBeenCalledWith('ws1', 'conversion', 'e1', expect.any(AbortSignal))
  })
  it('warns once on loss and reads the original session through restart interruption', async () => {
    vi.useFakeTimers()
    const app = await harness()
    vi.mocked(app.api.getExecution).mockResolvedValueOnce(session()).mockResolvedValueOnce(session('interrupted', 'e1', 'conversion', 3))
    await app.start()
    app.finish([])
    await flushPromises()
    expect(notice()).toContain('连接中断，执行结果尚未确认')
    expect(app.api.getExecution).toHaveBeenCalledWith('ws1', 'conversion', 'e1', expect.any(AbortSignal))
    dismiss()
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('执行已中断，请查看已提交结果，剩余工作未恢复')
    dismiss()
    await vi.advanceTimersByTimeAsync(60000)
    expect(notice()).toBe('')
  })
  it('waits after a cooperative cancel request and consumes an authoritative cancel once', async () => {
    vi.useFakeTimers()
    const app = await harness()
    await app.start()
    vi.mocked(app.api.cancelExecution).mockResolvedValue(session())
    vi.mocked(app.api.getExecution).mockResolvedValueOnce(session()).mockResolvedValueOnce(session('canceled', 'e1', 'conversion', 3))
    await app.execution.cancel('ws1', 'conversion', 'e1')
    expect(notice()).toBe('')
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('执行已停止')
    app.finish([{ type: 'canceled', data: { execution_id: 'e1' } }])
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('re-observes a visited active conversion after reload without replaying historical terminals', async () => {
    const app = await harness('/worksets/lib-a/conversion', session())
    await flushPromises()
    expect(app.execution.store.executionId).toBe('e1')
    app.leave()
    await app.router.push('/elsewhere')
    app.finish([{ type: 'interrupted', data: { execution_id: 'e1' } }])
    await flushPromises()
    expect(notice()).toContain('执行已中断')
  })
  it.each(['succeeded', 'failed', 'canceled', 'interrupted'] as const)('retains a newly accepted fast %s terminal snapshot', async status => {
    const app = await harness()
    vi.mocked(app.api.streamExecutionEvents).mockImplementation(() => ({ async *[Symbol.asyncIterator]() {
      yield { type: 'execution_snapshot', data: session(status, 'e1', 'conversion', 2) } satisfies ExecutionEvent
    } }))
    await app.start()
    expect(notice()).toContain({ succeeded: '执行完成', failed: '执行部分失败，已完成 2 项操作', canceled: '执行已停止', interrupted: '执行已中断' }[status])
    app.execution.attach('ws1', 'conversion', 'e1')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('notifies after leaving the page using captured identity and current execution results', async () => {
    const app = await harness('/worksets/lib-a/conversion')
    await app.start()
    app.leave()
    await app.router.push('/elsewhere')
    app.finish([{ type: 'succeeded', data: { execution_id: 'e1', plan_id: 'plan' } }])
    await flushPromises()
    expect(notice()).toContain('Archive · 转换 · 执行完成')
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe('conversion-execution')
  })
  it('keeps confirmed failure while detail is unavailable and classifies only final counters', async () => {
    vi.useFakeTimers()
    const app = await harness('/worksets/lib-a/conversion')
    vi.mocked(app.api.getExecution).mockRejectedValueOnce(new ApiError(503, 'OFFLINE', 'Offline'))
      .mockResolvedValueOnce(session()).mockResolvedValueOnce(session('failed', 'e1', 'conversion', 2))
    await app.start()
    app.finish([{ type: 'failed', data: { execution_id: 'e1', error_code: 'FAIL', error_message: 'Failed' } }])
    await flushPromises()
    expect(notice()).toBe('')
    expect(app.execution.store.view?.status).toBe('failed')
    await vi.advanceTimersByTimeAsync(2000)
    expect(notice()).toBe('')
    await app.router.push('/elsewhere')
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('执行部分失败，已完成 2 项操作')
    expect(notice()).not.toContain('组件')
  })
  it('observes concurrent accepted executions independently and publishes progress to their own query', async () => {
    vi.useFakeTimers()
    const app = await harness()
    await app.start()
    vi.mocked(app.api.startExecution).mockResolvedValue({ created: true, execution: session('running', 'e2') })
    vi.mocked(app.api.getExecution).mockResolvedValueOnce(session('running', 'e2', 'conversion', 1)).mockResolvedValueOnce(session('succeeded', 'e2', 'conversion', 5))
    app.client.setQueryData(queryKeys.worksets.operation('ws1', 'conversion'), { active_execution: session('running', 'e2') })
    const progress: unknown[] = []
    const unsubscribe = new QueryObserver(app.client, { queryKey: queryKeys.worksets.operation('ws1', 'conversion'), enabled: false }).subscribe(result => progress.push(result.data))
    stops.push(unsubscribe)
    await app.start()
    expect(progress).toContainEqual(expect.objectContaining({ active_execution: expect.objectContaining({ execution_id: 'e2', completed_operations: 1 }) }))
    expect(app.execution.store.executionId).toBe('e1')
    expect(app.api.getExecution).toHaveBeenCalledWith('ws1', 'conversion', 'e2', expect.any(AbortSignal))
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('执行完成')
    app.finish([{ type: 'interrupted', data: { execution_id: 'e1' } }])
    dismiss()
    await flushPromises()
    expect(notice()).toContain('执行已中断')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it.each(['', '/settings', '/execution', '/member/files'])('consumes Success and Stopped across conversion%s before later navigation', async suffix => {
    const app = await harness('/worksets/lib-a/conversion' + suffix)
    for (const status of ['succeeded', 'canceled'] as const) {
      vi.mocked(app.api.startExecution).mockResolvedValue({ created: true, execution: session(status, status) })
      await app.start()
      expect(notice()).toBe('')
    }
    await app.router.push('/elsewhere')
    app.execution.attach('ws1', 'conversion', 'succeeded')
    app.execution.attach('ws1', 'conversion', 'canceled')
    await flushPromises()
    expect(notice()).toBe('')
  })
  it.each(['/worksets/lib-b/conversion', '/worksets/lib-a/rename'])('notifies success away at %s', async path => {
    const app = await harness(path)
    vi.mocked(app.api.startExecution).mockResolvedValue({ created: true, execution: session('succeeded') })
    await app.start()
    expect(notice()).toContain('执行完成')
  })
  it('does not replay terminal history after reload', async () => {
    const app = await harness('/worksets/lib-a/conversion', session('succeeded'))
    await flushPromises()
    expect(notice()).toBe('')
    expect(app.execution.store.status).toBe('idle')
  })
  it('deduplicates accepted idempotency replay and repeated page registration', async () => {
    const app = await harness()
    vi.mocked(app.api.startExecution).mockResolvedValue({ created: false, execution: session('succeeded') })
    await app.start()
    await app.start()
    app.execution.attach('ws1', 'conversion', 'e1')
    expect(notice()).toContain('执行完成')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('keeps a confirmed cancellation when a pending GET and stream race', async () => {
    const app = await harness()
    let read!: (view: ExecutionView) => void
    vi.mocked(app.api.getExecution).mockImplementation(() => new Promise(resolve => { read = resolve }))
    await app.start()
    app.finish([])
    await flushPromises()
    dismiss()
    await app.execution.cancel('ws1', 'conversion', 'e1')
    read(session('succeeded'))
    await flushPromises()
    expect(notice()).toContain('执行已停止')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('does not overwrite the winning SSE result with a late cancel response', async () => {
    const app = await harness()
    await app.start()
    let canceled!: (view: ExecutionView) => void
    vi.mocked(app.api.cancelExecution).mockImplementation(() => new Promise(resolve => { canceled = resolve }))
    const pending = app.execution.cancel('ws1', 'conversion', 'e1')
    app.finish([{ type: 'succeeded', data: { execution_id: 'e1', plan_id: 'plan' } }])
    await flushPromises()
    canceled(session('canceled'))
    await pending
    await flushPromises()
    expect(notice()).toContain('执行完成')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it('treats intentional cancel transport termination as unconfirmed until the exact-session GET', async () => {
    const app = await harness()
    await app.start()
    vi.mocked(app.api.cancelExecution).mockResolvedValue(session())
    vi.mocked(app.api.getExecution).mockResolvedValue(session('canceled'))
    await app.execution.cancel('ws1', 'conversion', 'e1')
    app.finish([])
    await flushPromises()
    expect(notice()).toContain('执行已停止')
    dismiss()
    await flushPromises()
    expect(notice()).toBe('')
  })
  it.each([401, 403, 404])('stops unavailable session lookup on %s without inventing a terminal result', async status => {
    vi.useFakeTimers()
    const app = await harness()
    vi.mocked(app.api.getExecution).mockRejectedValue(new ApiError(status, 'UNAVAILABLE', 'Unavailable'))
    await app.start()
    app.finish([])
    await flushPromises()
    expect(notice()).toContain('执行结果尚未确认')
    dismiss()
    await flushPromises()
    expect(notice()).toContain('记录不存在或无权访问')
    const reads = vi.mocked(app.api.getExecution).mock.calls.length
    await vi.advanceTimersByTimeAsync(60000)
    expect(vi.mocked(app.api.getExecution).mock.calls.length).toBe(reads)
  })
  it('retries transient lookups with bounded backoff and resets to the active two-second cadence', async () => {
    vi.useFakeTimers()
    const app = await harness()
    vi.mocked(app.api.getExecution).mockRejectedValue(new ApiError(503, 'OFFLINE', 'Offline'))
    await app.start()
    app.finish([])
    await flushPromises()
    dismiss()
    for (const delay of [2000, 4000, 8000, 16000, 30000, 30000]) {
      const reads = vi.mocked(app.api.getExecution).mock.calls.length
      await vi.advanceTimersByTimeAsync(delay - 1)
      expect(vi.mocked(app.api.getExecution).mock.calls.length).toBe(reads)
      await vi.advanceTimersByTimeAsync(1)
      await flushPromises()
      expect(vi.mocked(app.api.getExecution).mock.calls.length).toBe(reads + 1)
      expect(notice()).toBe('')
    }
    vi.mocked(app.api.getExecution).mockResolvedValueOnce(session()).mockResolvedValueOnce(session('succeeded'))
    await vi.advanceTimersByTimeAsync(30000)
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(notice()).toContain('执行完成')
  })
  it('recovers an accepted session even when attach failed before the first event', async () => {
    const app = await harness()
    vi.mocked(app.api.streamExecutionEvents).mockImplementation(() => ({ async *[Symbol.asyncIterator]() { throw new TypeError('Offline') } }))
    vi.mocked(app.api.getExecution).mockResolvedValue(session('succeeded'))
    await app.start()
    expect(notice()).toContain('连接中断，执行结果尚未确认')
    dismiss()
    await flushPromises()
    expect(notice()).toContain('执行完成')
  })
  it('keeps admission refusal inline and opens unavailable current results for a removed library', async () => {
    const app = await harness()
    vi.mocked(app.api.startExecution).mockRejectedValueOnce(new ApiError(409, 'BUSY', 'Busy'))
    await expect(app.start()).rejects.toMatchObject({ code: 'BUSY' })
    expect(notice()).toBe('')
    vi.mocked(app.api.startExecution).mockResolvedValue({ created: true, execution: session('succeeded') })
    await app.start()
    vi.mocked(app.api.listLibraries).mockResolvedValue([])
    document.body.querySelector<HTMLButtonElement>('[data-testid="task-toast-action"]')!.click()
    await flushPromises()
    expect(app.router.currentRoute.value.name).toBe('worksets')
    expect(notice()).toContain('媒体库已移除，结果不可用')
  })
  it('cleans up pending exact-session reads on app teardown', async () => {
    vi.useFakeTimers()
    const app = await harness()
    let signal: AbortSignal | undefined
    vi.mocked(app.api.getExecution).mockImplementation((_ws, _op, _id, requestSignal) => { signal = requestSignal; return new Promise(() => {}) })
    await app.start()
    app.finish([])
    await flushPromises()
    app.page.unmount()
    expect(signal?.aborted).toBe(true)
    dismiss()
    await vi.advanceTimersByTimeAsync(60000)
    expect(notice()).toBe('')
  })

  it('shows a concurrent conversion page its own polled session instead of the singleton stream', async () => {
    const app = await harness()
    await app.start()
    vi.mocked(app.api.startExecution).mockResolvedValue({ created: true, execution: session('running', 'e2') })
    vi.mocked(app.api.getExecution).mockResolvedValue(session('running', 'e2', 'conversion', 2))
    vi.mocked(app.api.getOperation).mockResolvedValue({ workset_id: 'ws1', operation_type: 'conversion', version: 1, planning_state: 'planned', current_revision: null, active_generation: null, latest_generation: null, active_execution: session('running', 'e2'), latest_execution: null })
    await app.start()
    const context = app.mountContext()
    await flushPromises()
    expect(app.execution.store.executionId).toBe('e1')
    expect(context.executionView.value).toMatchObject({ execution_id: 'e2', completed_operations: 2 })
  })

})
