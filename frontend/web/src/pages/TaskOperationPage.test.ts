import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { ApiClientContract, ExecutionView, Operation, OperationType, Workset } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import TaskOperationPage from './TaskOperationPage.vue'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'
import { useTaskToast } from '@/composables/use-task-toast'

enableAutoUnmount(afterEach)

/**
 * The rename and metadata refresh pages (ADR 0012): one component on two
 * routes, reading the record, settings and frozen plan of its operation.
 *
 * jsdom has no layout, so the plan list's virtualizer would render an empty
 * window; every element reports a fixed box instead.
 */
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(560)
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(800)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
  const toast = useTaskToast()
  while (toast.current.value) toast.dismiss(toast.current.value.id)
})

const workset: Workset = {
  workset_id: 'ws-1',
  title: 'T',
  version: 1,
  library: { library_id: 'lib-1', name: 'Lib', root_path: '/music' },
  members: [{ member_id: 'm-1', folder_path: '/music/a', folder_name: 'a', rel_path: 'a', dir_id: 'd-a' }],
  operations: [],
  updated_at: '',
  created_at: '',
}

function operationOf(type: OperationType): Operation {
  return {
    workset_id: 'ws-1',
    operation_type: type,
    version: 3,
    planning_state: 'planned',
    current_revision: {
      plan_id: 'p-1',
      revision_index: 1,
      created_at: '',
      status: 'ready',
      summary_reason: '',
      counts: { members: 1, changed: 1, unmet_targets: 0, blocked: 0, unchanged: 0 },
      validation_state: 'valid',
      stale: false,
    },
    active_generation: null,
    latest_generation: null,
    active_execution: null,
    latest_execution: null,
  }
}

const COMMON = {
  rename: { template: '[{rj}] {title}' },
  metadata_refresh: { mode: 'fill_missing', locale: 'ja-jp', download_cover: true },
}
const COMPONENTS = {
  rename: [
    { member_id: 'm-1', source_name: 'a', work_code: 'RJ01', raw_name: '', target_name: '[RJ01] Title', action: 'rename' },
  ],
  metadata_refresh: [
    { member_id: 'm-1', target: { copy_id: 'c', rj: 'RJ01', library_root: '/music', rel_path: 'a', identity: '' }, action: 'refresh' },
  ],
}

function pageApi(overrides: Partial<ApiClientContract> = {}): ApiClientContract {
  return apiStub({
    getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
    getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn(async (_id: string, type: OperationType) => operationOf(type)),
    getOperationDraft: vi.fn(async (_id: string, type: OperationType) => ({
      workset_id: 'ws-1',
      operation_type: type,
      version: 3,
      schema_version: 1,
      document: { schema_version: 1, common: COMMON[type as 'rename'], members: [] },
      updated_at: '',
    })) as never,
    getRevision: vi.fn(async (_id: string, type: OperationType) => ({
      plan_id: 'p-1',
      task: { kind: type, schema_version: 1, payload: { components: COMPONENTS[type as 'rename'] } },
      execution: null,
    })) as never,
    getTaskSettings: vi.fn(async (type: OperationType) => ({ settings: COMMON[type as 'rename'], etag: '"e1"' })) as never,
    saveTaskSettings: vi.fn(async (_type: OperationType, settings: unknown) => ({ settings, etag: '"e2"' })) as never,
    saveOperationDraft: vi.fn(async () => operationOf('metadata_refresh')),
    startExecution: vi.fn().mockReturnValue(new Promise(() => {})),
    ...overrides,
  })
}

async function mountPage(
  path: string,
  overrides: Partial<ApiClientContract> = {},
): Promise<{ wrapper: VueWrapper; api: ApiClientContract; router: Router }> {
  const api = pageApi(overrides)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets/:libraryId', name: 'workbench-overview', component: { template: '<div />' } },
      { path: '/worksets/:libraryId/rename', name: 'rename', component: TaskOperationPage, props: { operation: 'rename' } },
      { path: '/worksets/:libraryId/metadata', name: 'metadata-refresh', component: TaskOperationPage, props: { operation: 'metadata_refresh' } },
      { path: '/worksets', name: 'worksets', component: { template: '<div />' } },
      { path: '/worksets/:libraryId/conversion', name: 'conversion', component: { template: '<div />' } },
      { path: '/worksets/:libraryId/conversion/settings', name: 'conversion-settings', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount({ components: { TaskToastHost }, template: '<RouterView /><TaskToastHost />' }, {
    global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, api, router }
}

describe('task operation page', () => {
  it.each(['rename', 'metadata_refresh'] as const)('shows authoritative %s details after its stream ends', async operation => {
    const view: ExecutionView = { execution_id: 'e-1', workset_id: 'ws-1', operation_type: operation, plan_id: 'p-1', status: 'running',
      total_components: 2, completed_components: 0, total_operations: 2, completed_operations: 0,
      current_root: '', current_component_id: '', current_phase: '', components: [], error_code: '', error_message: '', started_at: '', finished_at: '', created_at: '' }
    const { wrapper } = await mountPage('/worksets/lib-1/' + (operation === 'rename' ? 'rename' : 'metadata'), {
      getOperation: vi.fn().mockResolvedValue({ ...operationOf(operation), active_execution: view }),
      getExecution: vi.fn().mockResolvedValue({ ...view, status: 'failed', completed_components: 2, completed_operations: 1 }),
      streamExecutionEvents: vi.fn(() => ({ async *[Symbol.asyncIterator]() {
        yield { type: 'execution_snapshot' as const, data: view }
        yield { type: 'failed' as const, data: { execution_id: 'e-1', error_code: 'FAILED', error_message: 'Failed' } }
      } })),
      getExecutionCovers: vi.fn().mockResolvedValue({ items: [], counts: { pending: 0, running: 0, succeeded: 0, skipped: 0, failed: 0, canceled: 0 } }),
    })
    expect(wrapper.get('[data-testid="results-pane"]').text()).toContain('2/2')
    wrapper.unmount()
  })

  it('keeps Task identity and polled progress when the same page instance switches from rename to metadata', async () => {
    vi.useFakeTimers()
    const view = (operation: OperationType, id: string, done = 0): ExecutionView => ({ execution_id: id, workset_id: 'ws-1', operation_type: operation, plan_id: 'p-1', status: 'running',
      total_components: 3, completed_components: done, total_operations: 3, completed_operations: done,
      current_root: '', current_component_id: '', current_phase: '', components: [], error_code: '', error_message: '', started_at: '', finished_at: '', created_at: '' })
    let finishRename!: () => void
    const renameStream = new Promise<void>(resolve => { finishRename = resolve })
    const { wrapper, api, router } = await mountPage('/worksets/lib-1/rename', {
      getOperation: vi.fn(async (_ws, operation) => ({ ...operationOf(operation), active_execution: view(operation, operation) })),
      getExecution: vi.fn(async (_ws, operation, id) => view(operation, id, 2)),
      streamExecutionEvents: vi.fn((_ws, operation, id) => ({ async *[Symbol.asyncIterator]() {
        yield { type: 'execution_snapshot' as const, data: view(operation, id) }
        await renameStream
        yield { type: 'succeeded' as const, data: { execution_id: id, plan_id: 'p-1' } }
      } })),
      getExecutionCovers: vi.fn().mockResolvedValue({ items: [], counts: { pending: 1, running: 0, succeeded: 0, skipped: 0, failed: 0, canceled: 0 } }),
    })
    const original = wrapper.get('[data-testid="task-page-rename"]').element
    await router.push('/worksets/lib-1/metadata')
    await flushPromises()
    expect(wrapper.get('[data-testid="task-page-metadata_refresh"]').element).toBe(original)
    expect(wrapper.get('[data-testid="results-pane"]').text()).toContain('2/3')
    expect(api.getExecution).toHaveBeenCalledWith('ws-1', 'metadata_refresh', 'metadata_refresh', expect.any(AbortSignal))
    finishRename()
    await flushPromises()
    expect(wrapper.get('[data-testid="task-toast"]').text()).toContain('Lib · 重命名 · 执行')
    expect(wrapper.get('[data-testid="task-toast"]').text()).toContain('执行完成')
    const toast = useTaskToast()
    toast.dismiss(toast.current.value!.id)
    vi.mocked(api.getExecution).mockResolvedValue({ ...view('metadata_refresh', 'metadata_refresh', 3), status: 'succeeded' })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.find('[data-testid="task-toast"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="results-pane"]').text()).toContain('3/3')
    wrapper.unmount()
  })

  it('reviews frozen rename proposals and offers execution', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')

    expect(api.getCurrentRecord).toHaveBeenCalledWith('lib-1', 'rename', expect.anything())
    // A plan opens on the skipped folders; this one has none, so it opens on the renames.
    expect(wrapper.get('[data-testid="tab-rename"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="task-plan-table"]').text()).toContain('[RJ01] Title')
    expect((wrapper.get('[data-testid="rename-template"]').element as HTMLInputElement).value).toBe('[{rj}] {title}')
    expect(wrapper.find('[data-testid="execute"]').exists()).toBe(true)
  })

  it('saves all title filters and executes rename', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')
    expect((wrapper.get('[data-testid="rename-strip-bracketed"]').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-testid="rename-strip-tilde"]').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-testid="rename-strip-book"]').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('[data-testid="rename-strip-bracketed"]').setValue(true)
    await wrapper.get('[data-testid="rename-strip-tilde"]').setValue(true)
    await wrapper.get('[data-testid="rename-strip-book"]').setValue(true)
    expect(wrapper.get('[data-testid="task-settings"]').text()).not.toContain('〜')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(api.saveTaskSettings).mock.calls[0]).toMatchObject([
      'rename',
      { strip_bracketed_title: true, strip_tilde_title: true, strip_book_title: true },
      '"e1"',
    ])
    await wrapper.get('[data-testid="execute"]').trigger('click')
    await flushPromises()
    expect(api.startExecution).toHaveBeenCalledWith('ws-1', 'rename', 'p-1', expect.objectContaining({ ifMatchVersion: 3 }))
  })

  it('offers an actor separator only when the template uses {voice_actors}', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')
    expect(wrapper.find('[data-testid="rename-actor-separator"]').exists()).toBe(false)
    await wrapper.get('[data-testid="rename-template"]').setValue('[{rj}] {voice_actors}')
    await wrapper.get('[data-testid="rename-actor-separator"]').setValue(' & ')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(api.saveTaskSettings).mock.calls[0][1]).toMatchObject({
      template: '[{rj}] {voice_actors}', voice_actor_separator: ' & ',
    })
  })

  it('executes a rename plan that holds a blocked folder and lists only changes', async () => {
    const { wrapper } = await mountPage('/worksets/lib-1/rename', {
      getRevision: vi.fn(async () => ({
        plan_id: 'p-1', counts: { blocked: 1 },
        task: { kind: 'rename', schema_version: 1, payload: { components: [
          ...COMPONENTS.rename,
          { member_id: 'm-2', source_name: 'b', work_code: 'RJ02', raw_name: '', target_name: '', action: 'blocked', reason_code: 'TARGET_EXISTS' },
          { member_id: 'm-3', source_name: 'c', work_code: 'RJ03', raw_name: '', target_name: 'c', action: 'noop' },
        ] } },
      })) as never,
    })
    expect(wrapper.text()).not.toContain('排除阻塞项后重新生成')
    expect(wrapper.find('[data-testid="execute"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="tab-blocked"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="task-plan-table"]').findAll('li')).toHaveLength(1)
    expect(wrapper.get('[data-testid="task-plan-table"]').text()).toContain('目标名称已存在')
    expect(wrapper.get('[data-testid="result-tabs"]').text()).toBe('重命名 1跳过 1已手动改名 0')
    expect(wrapper.get('[data-testid="plan-unchanged"]').text()).toBe('1 个名称不变（未显示）')
  })

  it('saves a folder name into the draft next to the other records', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')
    await wrapper.get('[data-testid="manual-rename-search"]').setValue('a')
    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    const input = wrapper.get('[data-testid="manual-rename-input"]')
    await input.setValue('[RJ01] mine')
    await wrapper.get('[data-testid="manual-rename-form"]').trigger('submit')
    await flushPromises()
    expect(api.saveOperationDraft).toHaveBeenCalledWith(
      'ws-1', 'rename',
      expect.objectContaining({ members: [{ member_id: 'm-1', excluded: false, name: '[RJ01] mine' }] }),
      3,
    )
  })

  it('drops names a run already applied when the draft is saved again', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename', {
      getOperationDraft: vi.fn(async () => ({
        workset_id: 'ws-1', operation_type: 'rename', version: 3, schema_version: 1,
        document: { schema_version: 1, common: COMMON.rename, members: [{ member_id: 'm-1', excluded: false, name: 'a' }] },
        updated_at: '',
      })) as never,
    })
    await wrapper.get('[data-testid="manual-rename-search"]').setValue('a')
    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    const input = wrapper.get('[data-testid="manual-rename-input"]')
    await input.setValue('')
    await wrapper.get('[data-testid="manual-rename-form"]').trigger('submit')
    await flushPromises()
    expect(api.saveOperationDraft).toHaveBeenCalledWith(
      'ws-1', 'rename', expect.objectContaining({ members: [] }), 3,
    )
  })

  it('shows recovered rename results while the session stays interrupted', async () => {
    const { wrapper } = await mountPage('/worksets/lib-1/rename', {
      getRevision: vi.fn(async () => ({
        plan_id: 'p-1', task: { kind: 'rename', schema_version: 1, payload: { components: COMPONENTS.rename } },
        execution: { execution_id: 'e-1' },
      })) as never,
      getExecution: vi.fn(async () => ({
        execution_id: 'e-1', status: 'interrupted', completed_components: 1, total_components: 2,
        components: [{ component_id: 'm-1', root_path: '/music/a', status: 'succeeded',
          task_result: { source_path: '/music/a', target_path: '/music/title', renamed: true, recovered: true } }],
      })) as never,
    })
    expect(wrapper.get('[data-testid="results-pane"]').text()).toContain('已中断')
    // Results open on the failures; a recovered rename counts as a success.
    expect(wrapper.find('[data-testid="rename-results"]').exists()).toBe(false)
    await wrapper.get('[data-testid="tab-succeeded"]').trigger('click')
    expect(wrapper.get('[data-testid="rename-results"]').text()).toContain('已改名（启动恢复）')
    expect(wrapper.get('[data-testid="rename-results"]').text()).toContain('/music/title')
    expect(wrapper.find('[data-testid="execute"]').exists()).toBe(false)
  })

  it('summarizes the metadata plan as a count', async () => {
    const { wrapper } = await mountPage('/worksets/lib-1/metadata')

    expect(wrapper.get('[data-testid="metadata-plan-count"]').text()).toBe('标签语言 日本語 · 将刷新 1 个 · 跳过 0 个')
    expect(wrapper.find('[data-testid="task-plan-table"]').exists()).toBe(false)
  })

  it('lists only the works whose metadata fetch failed', async () => {
    const component = { root_path: '/music/a', partition: 'matched', operations: 1, completed_operations: 1, committed: [], removed: [], remaining: [], recovery: [], inventory_synced: true }
    const { wrapper } = await mountPage('/worksets/lib-1/metadata', {
      getRevision: vi.fn(async () => ({
        plan_id: 'p-1',
        task: { kind: 'metadata_refresh', schema_version: 1, payload: { components: COMPONENTS.metadata_refresh } },
        execution: { execution_id: 'e-1' },
      })) as never,
      getExecution: vi.fn(async () => ({
        execution_id: 'e-1',
        status: 'succeeded',
        total_components: 2,
        completed_components: 2,
        error_message: '',
        components: [
          { ...component, component_index: 0, component_id: 'c-1', status: 'succeeded', task_result: { rj: 'RJ01', issues: [{ source: 'asmr.one', code: 'BACKUP_FAILED', message: 'x' }] } },
          { ...component, component_index: 1, component_id: 'c-2', status: 'failed', error_code: 'METADATA_UNAVAILABLE', error_message: 'not found', task_result: { rj: 'RJ02' } },
        ],
      })) as never,
      getExecutionCovers: vi.fn(async () => ({
        items: [],
        counts: { pending: 0, running: 0, succeeded: 0, skipped: 0, failed: 0, canceled: 0 },
      })) as never,
    })

    const rows = wrapper.get('[data-testid="execution-failures"]').findAll('li')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('RJ02')
    expect(rows[0].text()).toContain('元数据获取失败')
    expect(wrapper.get('[data-testid="result-tabs"]').text()).toBe('失败 1成功 1')
    await wrapper.get('[data-testid="tab-succeeded"]').trigger('click')
    expect(wrapper.get('[data-testid="execution-failures"]').text()).toContain('RJ01')
  })

  it('saves metadata settings and executes the current plan', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/metadata')

    await wrapper.get('[data-testid="metadata-mode-refresh_dynamic"]').setValue(true)
    await flushPromises()
    // A dynamic refresh never downloads covers, and the form says why beside the box.
    expect((wrapper.get('[data-testid="metadata-cover"]').element as HTMLInputElement).disabled).toBe(true)
    expect(wrapper.get('[data-testid="metadata-cover-reason"]').text()).toBe('刷新动态数据只更新销量与评分，不下载封面')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()
    const [type, settings] = vi.mocked(api.saveTaskSettings).mock.calls[0]
    expect(type).toBe('metadata_refresh')
    expect(settings).toMatchObject({ mode: 'refresh_dynamic' })

    await wrapper.get('[data-testid="execute"]').trigger('click')
    await flushPromises()
    expect(api.startExecution).toHaveBeenCalledWith('ws-1', 'metadata_refresh', 'p-1', expect.objectContaining({ ifMatchVersion: 3 }))
  })

  it('follows the operation when the reused page switches routes', async () => {
    const { wrapper, api, router } = await mountPage('/worksets/lib-1/rename')

    await router.push('/worksets/lib-1/metadata')
    await flushPromises()
    expect(api.getOperationDraft).toHaveBeenCalledWith('ws-1', 'metadata_refresh', expect.anything())
    expect(wrapper.find('[data-testid="task-page-metadata_refresh"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="metadata-mode"]').exists()).toBe(true)
  })
})

it('keeps failed cover retries visible inline without a fictional task result', async () => {
  const view: ExecutionView = { execution_id: 'e-1', workset_id: 'ws-1', operation_type: 'metadata_refresh', plan_id: 'p-1', status: 'succeeded',
    total_components: 1, completed_components: 1, total_operations: 1, completed_operations: 1,
    current_root: '', current_component_id: '', current_phase: '', components: [], error_code: '', error_message: '', started_at: '', finished_at: '', created_at: '' }
  const { wrapper } = await mountPage('/worksets/lib-1/metadata', {
    getRevision: vi.fn().mockResolvedValue({ plan_id: 'p-1', task: { kind: 'metadata_refresh', schema_version: 1, payload: { components: [] } }, execution: view }),
    getExecution: vi.fn().mockResolvedValue(view),
    getExecutionCovers: vi.fn().mockResolvedValue({ items: [], counts: { pending: 0, running: 0, succeeded: 0, skipped: 0, failed: 1, canceled: 0 } }),
    changeExecutionCovers: vi.fn().mockRejectedValue(new Error('封面重试不可用')),
  })
  const retry = wrapper.get('[data-testid="covers"]').findAll('button').find(button => button.text() === '重试')!
  await retry.trigger('click')
  await flushPromises()
  expect(wrapper.get('[data-testid="covers"] [role="alert"]').text()).toBe('封面重试不可用')
  expect(wrapper.find('[data-testid="task-toast"]').exists()).toBe(false)
})

describe('task settings column', () => {
  it('keeps the apply bar for unsaved edits only and discards them', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
    await wrapper.get('[data-testid="rename-template"]').setValue('{rj}')
    expect(wrapper.get('[data-testid="settings-apply"]').text()).toBe('应用')
    expect(wrapper.find('[data-testid="rename-template-modified"]').exists()).toBe(true)
    await wrapper.get('[data-testid="settings-discard"]').trigger('click')
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="rename-template"]').element as HTMLInputElement).value).toBe('[{rj}] {title}')
    expect(api.saveTaskSettings).not.toHaveBeenCalled()
  })

  it('folds into a one-line summary in a narrow container and opens in place', async () => {
    const { wrapper } = await mountPage('/worksets/lib-1/rename')
    const summary = wrapper.get('[data-testid="settings-summary"]')
    expect(summary.text()).toContain('[{rj}] {title}')
    expect(wrapper.get('[data-testid="settings-body"]').attributes('style')).toContain('display: none')
    await wrapper.get('[data-testid="rename-strip-bracketed"]').setValue(true)
    expect(summary.text()).toBe('[{rj}] {title} · 移除【】▾')
    await summary.trigger('click')
    expect(summary.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-testid="settings-body"]').attributes('style') ?? '').not.toContain('display: none')
  })

  it('summarizes metadata settings', async () => {
    const { wrapper } = await mountPage('/worksets/lib-1/metadata')
    expect(wrapper.get('[data-testid="settings-summary"]').text()).toBe('只补缺失 · 日本語 · 封面▾')
  })

  it('works without a record: settings stay, the right pane is the empty state', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename', {
      getCurrentRecord: vi.fn().mockResolvedValue({ workset: null }),
    })
    expect(wrapper.find('[data-testid="no-record"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="manual-rename-search"]').exists()).toBe(false)
    await wrapper.get('[data-testid="rename-template"]').setValue('{rj}')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()
    expect(api.saveTaskSettings).toHaveBeenCalledWith('rename', expect.objectContaining({ template: '{rj}' }), '"e1"')
  })

  it('asks before leaving with unapplied edits', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { wrapper, router } = await mountPage('/worksets/lib-1/rename')
    await wrapper.get('[data-testid="rename-template"]').setValue('{rj}')
    await router.push('/worksets/lib-1')
    await flushPromises()
    expect(confirm).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/worksets/lib-1/rename')
  })
})
