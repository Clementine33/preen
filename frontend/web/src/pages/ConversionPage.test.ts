import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { Operation, Workset } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { useWorksetUiStore } from '@/stores/workset-ui'
import ConversionPage from './ConversionPage.vue'

const counts = { members: 2, changed: 2, unmet_targets: 0, blocked: 0, unchanged: 0 }
const operation: Operation = {
  workset_id: 'ws', operation_type: 'conversion', version: 1, planning_state: 'planned',
  current_revision: { plan_id: 'plan', revision_index: 1, created_at: '', status: 'ok', summary_reason: '', counts, validation_state: 'stale', stale: true },
  active_generation: null, latest_generation: null, active_execution: null, latest_execution: null,
}
const workset: Workset = {
  workset_id: 'ws', title: 'test', version: 1, library: { library_id: 'lib', name: 'lib', root_path: '/music' },
  members: ['a', 'b'].map((name) => ({ member_id: name, folder_path: `/music/${name}`, folder_name: name, rel_path: name, dir_id: name })),
  operations: [], created_at: '', updated_at: '',
}
function revision(selectedStale = false) {
  return {
    plan_id: 'plan', counts, execution: null,
    task: { kind: 'conversion', schema_version: 1, payload: { components: [], summary: {} } },
    roots: ['a', 'b'].map((name, index) => ({ root_index: index, root_path: `/music/${name}`, root_status: 'ok', stale: name === 'b' || selectedStale })),
    component_roots: [0, 1].map((index) => ({ component_index: index, component_id: `${index}`, root_index: index })),
  }
}
async function page(getRevision = vi.fn().mockResolvedValue(revision()), blocked = 0) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/worksets/:libraryId/conversion', name: 'conversion', component: ConversionPage }, { path: '/overview', name: 'workbench-overview', component: { template: '<div />' } }] })
  await router.push('/worksets/lib/conversion')
  const api = apiStub({
    getCurrentRecord: vi.fn().mockResolvedValue({ workset }), getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn().mockResolvedValue({ ...operation, current_revision: { ...operation.current_revision, counts: { ...counts, blocked } } }),
    getOperationDraft: vi.fn().mockResolvedValue({ version: 1, document: { schema_version: 1, classifier_tags: [], matched: {}, unmatched: {}, members: [] } }), getRevision,
  })
  const wrapper = mount(ConversionPage, { global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api }, stubs: {
    WorkbenchShell: { template: '<div><slot name="main" tier="wide" /></div>' }, WorkbenchNav: true, MemberList: true,
    OperationHeader: { props: ['executeBlocked'], template: '<div data-testid="full-reason">{{ executeBlocked }}</div>' },
  } } })
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
    for (const [stale, blocked] of [[true, 0], [false, 1]] as const) {
      const detail = revision(stale)
      detail.counts = { ...counts, blocked }
      const wrapper = await page(vi.fn().mockResolvedValue(detail), blocked)
      expect(wrapper.get('[data-testid="execute-selected"]').attributes('disabled')).toBeDefined()
      wrapper.unmount()
    }
  })
  it('waits for revision detail instead of guessing from the summary', async () => {
    const wrapper = await page(vi.fn(() => new Promise(() => {})))
    expect(wrapper.get('[data-testid="execute-selected"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="execute-selected"]').attributes('title')).toContain('加载中')
    wrapper.unmount()
  })
})
