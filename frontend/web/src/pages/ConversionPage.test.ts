import type { EditTarget } from '@/features/worksets/draft-intents'
import type { Operation, Workset } from '@/lib/api/types'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import { queryKeys } from '@/queries/query-keys'
import { useWorksetEditorStore } from '@/stores/workset-editor'
import { useWorksetUiStore } from '@/stores/workset-ui'
import { apiStub } from '@/test/api-stub'
import { createTestQueryClient } from '@/test/query-client'
import ConversionPage from './ConversionPage.vue'

const counts = {
  members: 2,
  changed: 2,
  unmet_targets: 0,
  blocked: 0,
  unchanged: 0,
}
const operation: Operation = {
  workset_id: 'ws',
  operation_type: 'conversion',
  version: 1,
  planning_state: 'planned',
  current_revision: {
    plan_id: 'plan',
    revision_index: 1,
    created_at: '',
    status: 'ok',
    summary_reason: '',
    counts,
    validation_state: 'stale',
    stale: true,
  },
  active_generation: null,
  latest_generation: null,
  active_execution: null,
  latest_execution: null,
}
const workset: Workset = {
  workset_id: 'ws',
  title: 'test',
  version: 1,
  library: { library_id: 'lib', name: 'lib', root_path: '/music' },
  members: ['a', 'b'].map(name => ({
    member_id: name,
    folder_path: `/music/${name}`,
    folder_name: name,
    rel_path: name,
    dir_id: name,
  })),
  operations: [],
  created_at: '',
  updated_at: '',
}
function revision(selectedStale = false) {
  return {
    plan_id: 'plan',
    counts,
    execution: null,
    task: {
      kind: 'conversion',
      schema_version: 1,
      payload: { summary: {} },
    },
    members: ['a', 'b'].map(name => ({
      member_id: name,
      excluded: false,
      input: name === 'b' || selectedStale
        ? 'stale'
        : 'valid',
      has_components: true,
    })),
  }
}
async function page(getRevision = vi.fn().mockResolvedValue(revision()), blocked = 0, queryClient = createTestQueryClient()) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets/:libraryId/conversion', name: 'conversion', component: ConversionPage },
      {
        path: '/overview',
        name: 'workbench-overview',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/worksets/lib/conversion')
  const api = apiStub({
    saveOperationDraft: vi.fn().mockResolvedValue({ ...operation, version: 2 }),
    getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
    getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn().mockResolvedValue({
      ...operation,
      current_revision: {
        ...operation.current_revision,
        counts: { ...counts, blocked },
      },
    }),
    getOperationDraft: vi.fn().mockResolvedValue({
      version: 1,
      document: {
        schema_version: 1,
        no_effect_tags: [],
        matched: {},
        unmatched: {},
        members: [],
      },
    }),
    getRevision,
  })
  const wrapper = mount(ConversionPage, {
    global: {
      plugins: [
        createPinia(),
        router,
        [
          VueQueryPlugin,
          { queryClient },
        ],
      ],
      provide: { [apiClientKey as symbol]: api },
      stubs: {
        WorkbenchShell: { template: '<div><slot name="main" tier="wide" /></div>' },
        WorkbenchNav: true,
        MemberList: true,
        OperationHeader: {
          props: ['executeBlocked'],
          template: '<div data-testid="full-reason">{{ executeBlocked }}</div>',
        },
      },
    },
  })
  await flushPromises()
  useWorksetUiStore().toggleMember('a')
  await flushPromises()
  return wrapper
}
describe('selected conversion execution', () => {
  it('allows a fresh selected root when an unselected root is stale', async () => {
    const wrapper = await page()
    expect(wrapper.get('[data-testid="full-reason"]').text()).toContain('输入已变化')
    expect(wrapper.get('[data-testid="execute-selected"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
  it('blocks stale selected roots and globally blocked components', async () => {
    for (const [stale, blocked] of [
      [true, 0],
      [false, 1],
    ] as const) {
      const detail = revision(stale)
      detail.counts = { ...counts, blocked }
      const wrapper = await page(vi.fn().mockResolvedValue(detail), blocked)
      expect(wrapper.get('[data-testid="execute-selected"]').attributes('disabled')).toBeDefined()
      wrapper.unmount()
    }
  })
  it('refreshes same-plan member input and selected execution after returning past the TTL', async () => {
    vi.useFakeTimers({
      toFake: ['Date'],
    })
    try {
      const client = createTestQueryClient()
      const getRevision = vi.fn().mockResolvedValue(revision())
      const first = await page(getRevision, 0, client)
      expect(first.get('[data-testid="execute-selected"]').attributes('disabled')).toBeUndefined()
      first.unmount()
      vi.advanceTimersByTime(60_001)
      getRevision.mockResolvedValue(revision(true))
      const returned = await page(getRevision, 0, client)
      expect(getRevision).toHaveBeenCalledTimes(2)
      expect(client.getQueryData<{ members: { input: string }[] }>(queryKeys.worksets.revision('ws', 'conversion', 'plan'))?.members[0]?.input).toBe('stale')
      expect(returned.get('[data-testid="execute-selected"]').attributes('disabled')).toBeDefined()
      expect(returned.get('[data-testid="execute-selected"]').attributes('title')).toContain('输入已变化')
      returned.unmount()
    }
    finally {
      vi.useRealTimers()
    }
  })
  it('waits for revision detail instead of guessing from the summary', async () => {
    const wrapper = await page(vi.fn(() => new Promise(() => {})))
    expect(wrapper.get('[data-testid="execute-selected"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="execute-selected"]').attributes('title')).toContain('加载中')
    wrapper.unmount()
  })
})

describe('conversion editing status', () => {
  it.each<EditTarget>([
    { kind: 'member', memberId: 'a' },
    {
      kind: 'batch',
      memberIds: ['a', 'b'],
    },
  ])(
    'hides the $kind editing status after applying changes',
    async (target) => {
      const wrapper = await page()
      const editor = useWorksetEditorStore()
      editor.open({
        worksetId: 'ws',
        operation: 'conversion',
        target,
        baseVersion: 1,
        baseDocument: {
          schema_version: 1,
          no_effect_tags: [],
          matched: {},
          unmatched: {},
          members: [],
        },
      })
      editor.setUnit('no_effect_tags', {
        intent: 'set',
        value: ['tag'],
      })
      await flushPromises()
      expect(wrapper.get('[data-testid="edit-session-bar"]').text()).toContain('有未应用修改')
      await wrapper.get('[data-testid="apply-session"]').trigger('click')
      await flushPromises()
      expect(editor.isDirty).toBe(false)
      expect(editor.session?.baseVersion).toBe(2)
      expect(wrapper.find('[data-testid="edit-session-bar"]').exists()).toBe(false)
      wrapper.unmount()
    },
  )
})
