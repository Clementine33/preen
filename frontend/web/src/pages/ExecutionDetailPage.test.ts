import type { VueWrapper } from '@vue/test-utils'
import type {
  ApiClientContract,
  ExecutionComponent,
  ExecutionView,
  MemberSummary,
  Operation,
  RevisionDetailResponse,
  Workset,
} from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import { useWorksetExecutionStore } from '@/stores/workset-execution'
import { apiStub as sharedApiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import ConversionPage from './ConversionPage.vue'
import ExecutionDetailPage from './ExecutionDetailPage.vue'
import ExecutionMemberPage from './ExecutionMemberPage.vue'

enableAutoUnmount(afterEach)

const workset: Workset = {
  workset_id: 'ws-1',
  title: '夏季整理',
  version: 1,
  library: { library_id: 'lib-a', name: 'Lib', root_path: '/music' },
  members: [],
  updated_at: '2026-09-16T00:00:00Z',
  created_at: '2026-09-16T00:00:00Z',
}

const executionView: ExecutionView = {
  execution_id: 'exec-1',
  workset_id: 'ws-1',
  operation_type: 'conversion',
  plan_id: 'plan-1',
  status: 'running',
  options: { delete_mode: 'soft' },
  total_components: 2,
  completed_components: 1,
  total_operations: 3,
  completed_operations: 1,
  current_root: '/music/albumB',
  current_component_id: 'comp-b',
  current_phase: 'component',
  members: {
    counts: {
      succeeded: 0,
      failed: 0,
      canceled: 0,
      running: 1,
      pending: 1,
    },
    attention: [],
    attention_total: 0,
  },
  error_code: '',
  error_message: '',
  started_at: '',
  finished_at: '',
  created_at: '',
}

function summary(id: string, status: MemberSummary['status'], overrides: Partial<MemberSummary> = {}): MemberSummary {
  return {
    member_id: id,
    root_path: `/music/${id}`,
    status,
    total_components: 2,
    completed_components: 1,
    total_operations: 4,
    completed_operations: 2,
    error_message: '',
    inventory_sync_error: '',
    ...overrides,
  }
}

function operation(active: boolean): Operation {
  return {
    workset_id: 'ws-1',
    operation_type: 'conversion',
    version: 5,
    planning_state: 'planned',
    current_revision: {
      plan_id: 'plan-1',
      revision_index: 1,
      created_at: '2026-09-16T00:00:00Z',
      status: 'ready',
      summary_reason: '',
      counts: {
        members: 1,
        changed: 1,
        unmet_targets: 0,
        blocked: 0,
        unchanged: 0,
      },
      validation_state: 'valid',
      stale: false,
    },
    active_generation: null,
    latest_generation: null,
    active_execution: active
      ? {
          execution_id: 'exec-1',
          plan_id: 'plan-1',
          status: 'running',
          options: { delete_mode: 'soft' },
          total_components: 2,
          completed_components: 1,
          total_operations: 3,
          completed_operations: 1,
          current_root: '/music/albumB',
          current_component_id: 'comp-b',
          current_phase: 'component',
        }
      : null,
    latest_execution: null,
  }
}

function revision(execution: RevisionDetailResponse['execution']): RevisionDetailResponse {
  return {
    plan_id: 'plan-1',
    category_counts: {},
    revision_index: 1,
    created_at: '2026-09-16T00:00:00Z',
    root_path: '/music',
    snapshot_token: 'tok',
    status: 'ready',
    summary: {
      operation_count: 3,
      error_count: 0,
      total_count: 2,
      actionable_count: 2,
      summary_reason: '',
    },
    task: {
      kind: 'conversion',
      schema_version: 1,
      payload: {
        policy: {},
        policy_hash: '',
        classifier: {},
        summary: {
          component_count: 2,
          blocked_count: 0,
          operation_count: 3,
          error_count: 0,
          summary_reason: '',
        },
      },
    },
    counts: {
      members: 1,
      changed: 1,
      unmet_targets: 0,
      blocked: 0,
      unchanged: 0,
    },
    members: [],
    execution,
  }
}

function pageApi(overrides: Partial<ApiClientContract> = {}): ApiClientContract {
  return sharedApiStub({
    getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
    getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn().mockResolvedValue(operation(true)),
    getOperationDraft: vi.fn().mockResolvedValue({
      workset_id: 'ws-1',
      operation_type: 'conversion',
      version: 5,
      schema_version: 1,
      updated_at: '',
      document: {
        schema_version: 1,
        no_effect_tags: [],
        matched: {},
        unmatched: {},
        members: [],
      },
    }),
    getRevision: vi.fn().mockResolvedValue(revision(null)),
    getExecution: vi.fn().mockResolvedValue(executionView),
    // An active detail page now observes its session for the SPA lifetime.
    streamExecutionEvents: vi.fn((_ws, _op, _id, signal) => ({
      async* [Symbol.asyncIterator]() {
        await new Promise<void>(resolve => signal?.addEventListener('abort', () => resolve(), { once: true }))
      },
    })),
    ...overrides,
  })
}

async function mountPage(api: ApiClientContract, path = '/worksets/lib-a/conversion/execution'): Promise<VueWrapper> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/worksets/:libraryId/conversion',
        name: 'conversion',
        component: ConversionPage,
        children: [
          {
            path: 'execution',
            name: 'conversion-execution',
            component: ExecutionDetailPage,
            meta: { carrier: true },
          },
          {
            path: ':memberId/execution',
            name: 'conversion-member-execution',
            component: ExecutionMemberPage,
            meta: { carrier: true },
          },
        ],
      },
    ],
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount({ template: '<RouterView />' }, {
    global: {
      plugins: [
        createPinia(),
        router,
        installTestQueryPlugin(),
      ],
      provide: { [apiClientKey as symbol]: api },
      stubs: {
        WorkbenchShell: { template: '<div><slot name="main" tier="wide" /><slot name="detail" /></div>' },
        WorkbenchNav: true,
      },
    },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

describe('executionDetailPage', () => {
  it('renders the live report and offers the cancel action while the run is active', async () => {
    const cancelExecution = vi.fn().mockResolvedValue({ ...executionView, status: 'canceled' })
    const api = pageApi({ cancelExecution })
    const wrapper = await mountPage(api)

    // The member list beside the carrier keeps its conclusions: the page reads
    // the light revision even though the panel itself does not need it.
    expect(api.getRevision).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="execution-panel"]').text()).toContain('执行中')
    expect(wrapper.get('[data-testid="execution-progress"]').text()).toContain('组件 1/2')

    await wrapper.get('[data-testid="execution-cancel"]').trigger('click')
    await flushPromises()

    expect(cancelExecution).toHaveBeenCalledWith('ws-1', 'conversion', 'exec-1')
  })

  it('shows the finished report of the revision that ran, without a cancel action', async () => {
    const api = pageApi({
      getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
      getOperation: vi.fn().mockResolvedValue({
        ...operation(false),
        latest_execution: { execution_id: 'exec-1', plan_id: 'plan-1', status: 'succeeded' },
      }),
      getExecution: vi.fn().mockResolvedValue({
        ...executionView,
        status: 'succeeded',
        completed_components: 2,
        completed_operations: 3,
      }),
    })
    const wrapper = await mountPage(api)

    expect(wrapper.get('[data-testid="execution-status"]').text()).toBe('已完成')
    expect(wrapper.find('[data-testid="execution-cancel"]').exists()).toBe(false)
  })

  it('reads the summary shape and follows member events: counts, add, update, remove', async () => {
    const getExecution = vi.fn().mockResolvedValue(executionView)
    const wrapper = await mountPage(pageApi({ getExecution }))
    expect(getExecution).toHaveBeenCalledWith('ws-1', 'conversion', 'exec-1', expect.anything(), { view: 'members' })

    const store = useWorksetExecutionStore()
    store.status = 'streaming'
    store.view = { ...executionView }
    store.executionId = 'exec-1'
    store.worksetId = 'ws-1'
    store.operation = 'conversion'
    await flushPromises()
    expect(wrapper.find('[data-testid="execution-attention-item"]').exists()).toBe(false)

    const counts = (failed: number) => ({
      succeeded: 0,
      failed,
      canceled: 0,
      running: 1,
      pending: 0,
    })
    store.applyEvent({
      type: 'member',
      data: {
        member: summary('m-1', 'failed', { error_message: 'boom' }),
        counts: counts(1),
        attention_total: 1,
      },
    })
    store.applyEvent({
      type: 'member',
      data: {
        member: summary('m-2', 'running'),
        counts: counts(1),
        attention_total: 2,
      },
    })
    await flushPromises()
    expect(wrapper.findAll('[data-testid="execution-attention-item"]').map(i => i.attributes('data-member-status'))).toEqual(['failed', 'running'])
    expect(wrapper.get('[data-testid="execution-count-failed"]').text()).toBe('1')
    expect(wrapper.text()).toContain('boom')
    // The session page renders no per-component rows.
    expect(wrapper.find('[data-testid="execution-component"]').exists()).toBe(false)

    // m-2 finishes cleanly: it leaves the list; m-1 keeps its place.
    store.applyEvent({
      type: 'member',
      data: {
        member: summary('m-2', 'succeeded'),
        counts: counts(1),
        attention_total: 1,
      },
    })
    await flushPromises()
    expect(wrapper.findAll('[data-testid="execution-attention-item"]')).toHaveLength(1)

    // A replace keeps the entry's place and updates its facts.
    store.applyEvent({
      type: 'member',
      data: {
        member: summary('m-1', 'failed', { error_message: 'again' }),
        counts: counts(1),
        attention_total: 1,
      },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('again')
    expect(wrapper.findAll('[data-testid="execution-attention-item"]')).toHaveLength(1)
  })

  it('says how many exceptions beyond the server cap exist', async () => {
    const getExecution = vi.fn().mockResolvedValue({
      ...executionView,
      members: {
        counts: executionView.members!.counts,
        attention: [
          summary('m-1', 'failed'),
        ],
        attention_total: 4,
      },
    })
    const wrapper = await mountPage(pageApi({ getExecution }))
    expect(wrapper.get('[data-testid="execution-attention-more"]').text()).toBe('另有 3 个')
  })

  it('states the absence of a session instead of pretending one exists', async () => {
    const api = pageApi({
      getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
      getOperation: vi.fn().mockResolvedValue({
        ...operation(false),
        latest_execution: { execution_id: 'old-exec', plan_id: 'old-plan', status: 'succeeded' },
      }),
      getRevision: vi.fn().mockResolvedValue(revision(null)),
    })
    const wrapper = await mountPage(api)

    expect(wrapper.get('[data-testid="execution-empty"]').text()).toContain('没有执行记录')
    expect(wrapper.find('[data-testid="execution-panel"]').exists()).toBe(false)
  })
})

describe('executionMemberPage', () => {
  function memberComponent(keep: string): ExecutionComponent {
    return {
      component_index: 0,
      component_id: 'comp-0',
      root_path: '/music/albumA',
      partition: 'matched',
      status: 'succeeded',
      operations: 1,
      completed_operations: 1,
      committed: [],
      kept_decisions: [
        { path: `/music/albumA/${keep}`, resolution: 'keep', reason_code: 'UNMET_TARGET' },
      ],
      removed: [],
      remaining: [],
      recovery: [],
      inventory_synced: true,
    }
  }
  function memberView(keep: string): ExecutionView {
    return {
      ...executionView,
      members: undefined,
      member: summary('m-1', 'running'),
      components: [
        memberComponent(keep),
      ],
    }
  }
  const named: Workset = {
    ...workset,
    members: [
      {
        member_id: 'm-1',
        folder_path: '/music/albumA',
        folder_name: '专辑 A',
        rel_path: '专辑 A',
        dir_id: 'd-1',
      },
    ],
  }
  function api(getMember: (read: unknown) => Promise<ExecutionView>) {
    return pageApi({
      getWorkset: vi.fn().mockResolvedValue(named),
      getExecution: vi.fn((_ws, _op, _id, _signal, read) => read?.memberId
        ? getMember(read)
        : Promise.resolve(executionView)),
    })
  }
  const path = '/worksets/lib-a/conversion/m-1/execution'

  it('shows the member header and its kept files on the first read', async () => {
    const getMember = vi.fn().mockResolvedValue(memberView('keep-1.mp3'))
    const wrapper = await mountPage(api(getMember), path)

    expect(getMember).toHaveBeenCalledWith({ memberId: 'm-1' })
    expect(wrapper.get('[data-testid="execution-member-title"]').text()).toBe('专辑 A')
    expect(wrapper.get('[data-testid="execution-member-status"]').text()).toBe('执行中')
    expect(wrapper.get('[data-testid="execution-member-progress"]').text()).toContain('组件 1/2')
    expect(wrapper.get('[data-testid="execution-member-back"]').attributes('href')).toBe('/worksets/lib-a/conversion/execution')
    expect(wrapper.get('summary').text()).toContain('保留 1')
    expect(wrapper.get('[data-testid="execution-kept"]').text()).toContain('keep-1.mp3')
  })

  it('re-reads on a member event for this member and on a reconnect snapshot, not for another member', async () => {
    const getMember = vi.fn().mockResolvedValue(memberView('keep-1.mp3'))
    const wrapper = await mountPage(api(getMember), path)
    expect(getMember).toHaveBeenCalledTimes(1)
    const store = useWorksetExecutionStore()
    store.status = 'streaming'
    store.view = { ...executionView }
    const counts = executionView.members!.counts

    store.applyEvent({
      type: 'member',
      data: {
        member: summary('m-9', 'failed'),
        counts,
        attention_total: 1,
      },
    })
    await flushPromises()
    expect(getMember).toHaveBeenCalledTimes(1)

    getMember.mockResolvedValue(memberView('keep-2.mp3'))
    store.applyEvent({
      type: 'member',
      data: {
        member: summary('m-1', 'succeeded'),
        counts,
        attention_total: 0,
      },
    })
    await flushPromises()
    expect(getMember).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="execution-kept"]').text()).toContain('keep-2.mp3')

    getMember.mockResolvedValue(memberView('keep-3.mp3'))
    store.applyEvent({
      type: 'execution_snapshot',
      data: { ...executionView },
    })
    await flushPromises()
    expect(getMember).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-testid="execution-kept"]').text()).toContain('keep-3.mp3')
  })

  it('says so when the member is not in the execution', async () => {
    const getMember = vi.fn().mockRejectedValue(Object.assign(new Error('nope'), { status: 404, code: 'MEMBER_NOT_IN_EXECUTION' }))
    const wrapper = await mountPage(api(getMember), path)

    expect(wrapper.get('[data-testid="execution-member-empty"]').text()).toContain('没有这个文件夹的结果')
  })
})
