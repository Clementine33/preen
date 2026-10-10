import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { ApiClientContract, Operation, OperationType, Workset } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import TaskOperationPage from './TaskOperationPage.vue'

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
afterEach(() => vi.restoreAllMocks())

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
      common_settings_current: true,
      updated_at: '',
    })) as never,
    getRevision: vi.fn(async (_id: string, type: OperationType) => ({
      plan_id: 'p-1',
      task: { kind: type, schema_version: 1, payload: { components: COMPONENTS[type as 'rename'] } },
      execution: null,
    })) as never,
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
  const wrapper = mount({ template: '<RouterView />' }, {
    global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return { wrapper, api, router }
}

describe('task operation page', () => {
  it('reviews frozen rename proposals and offers execution', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')

    expect(api.getCurrentRecord).toHaveBeenCalledWith('lib-1', 'rename', expect.anything())
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
    await wrapper.get('[data-testid="task-settings"]').trigger('submit')
    await flushPromises()
    expect(vi.mocked(api.saveOperationDraft).mock.calls[0][2]).toMatchObject({
      common: { strip_bracketed_title: true, strip_tilde_title: true, strip_book_title: true },
    })
    await wrapper.get('[data-testid="execute"]').trigger('click')
    await flushPromises()
    expect(api.startExecution).toHaveBeenCalledWith('ws-1', 'rename', 'p-1', expect.objectContaining({ ifMatchVersion: 3 }))
  })

  it('offers an actor separator only when the template uses {voice_actors}', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename')
    expect(wrapper.find('[data-testid="rename-actor-separator"]').exists()).toBe(false)
    await wrapper.get('[data-testid="rename-template"]').setValue('[{rj}] {voice_actors}')
    await wrapper.get('[data-testid="rename-actor-separator"]').setValue(' & ')
    await wrapper.get('[data-testid="task-settings"]').trigger('submit')
    await flushPromises()
    expect(vi.mocked(api.saveOperationDraft).mock.calls[0][2]).toMatchObject({
      common: { template: '[{rj}] {voice_actors}', voice_actor_separator: ' & ' },
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
    expect(wrapper.get('[data-testid="task-plan-table"]').findAll('li')).toHaveLength(2)
    expect(wrapper.get('[data-testid="plan-summary"]').text()).toBe('重命名 1 · 跳过 1 · 1 个名称不变（未显示）')
    expect(wrapper.get('[data-testid="task-plan-table"]').text()).toContain('目标名称已存在')
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
      3, undefined,
    )
  })

  it('drops names a run already applied when the draft is saved again', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/rename', {
      getOperationDraft: vi.fn(async () => ({
        workset_id: 'ws-1', operation_type: 'rename', version: 3, schema_version: 1,
        document: { schema_version: 1, common: COMMON.rename, members: [{ member_id: 'm-1', excluded: false, name: 'a' }] },
        common_settings_current: true, updated_at: '',
      })) as never,
    })
    await wrapper.get('[data-testid="manual-rename-search"]').setValue('a')
    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    const input = wrapper.get('[data-testid="manual-rename-input"]')
    await input.setValue('')
    await wrapper.get('[data-testid="manual-rename-form"]').trigger('submit')
    await flushPromises()
    expect(api.saveOperationDraft).toHaveBeenCalledWith(
      'ws-1', 'rename', expect.objectContaining({ members: [] }), 3, undefined,
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
    expect(wrapper.get('[data-testid="execution"]').text()).toContain('已中断')
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
        downloads: {},
      })) as never,
    })

    const rows = wrapper.get('[data-testid="execution-failures"]').findAll('li')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('RJ02')
    expect(rows[0].text()).toContain('元数据获取失败')
    expect(wrapper.get('[data-testid="execution"]').text()).toContain('失败 1 个作品')
  })

  it('saves metadata settings and executes the current plan', async () => {
    const { wrapper, api } = await mountPage('/worksets/lib-1/metadata')

    await wrapper.get('[data-testid="metadata-mode"]').setValue('refresh_dynamic')
    await wrapper.get('[data-testid="task-settings"]').trigger('submit')
    await flushPromises()
    const [, type, document, version] = vi.mocked(api.saveOperationDraft).mock.calls[0]
    expect(type).toBe('metadata_refresh')
    expect(version).toBe(3)
    // A dynamic refresh never downloads covers.
    expect(document).toMatchObject({ common: { mode: 'refresh_dynamic', download_cover: false }, members: [] })

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
