import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { ApiClientContract, Operation, OperationDraftDocument, Workset } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import ConversionSettingsPage from './ConversionSettingsPage.vue'

/**
 * The common settings are deployment-wide: applying them rewrites every other
 * task's draft in the same transaction, so the page has to say so before the
 * button, not after the fact (ADR 0010 §2).
 */
const workset: Workset = {
  workset_id: 'ws-1',
  title: 'T',
  version: 5,
  library: { library_id: 'lib-a', name: 'Lib', root_path: '/music' },
  members: [{ member_id: 'm-1', folder_path: '/music/a', folder_name: 'a', rel_path: 'a', dir_id: 'd-a' }],
  operations: [],
  updated_at: '',
  created_at: '',
}

const operation: Operation = {
  workset_id: 'ws-1',
  operation_type: 'conversion',
  version: 5,
  planning_state: 'planned',
  current_revision: null,
  active_generation: null,
  latest_generation: null,
  active_execution: null,
  latest_execution: null,
}

const document: OperationDraftDocument = {
  schema_version: 1,
  mode: 'strict',
  classifier_tags: ['A'],
  matched: {},
  unmatched: {},
  members: [],
}

function pageApi(): ApiClientContract {
  return apiStub({
    getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
    getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn().mockResolvedValue(operation),
    getOperationDraft: vi.fn().mockResolvedValue({
      workset_id: 'ws-1',
      operation_type: 'conversion',
      version: 5,
      schema_version: 1,
      updated_at: '',
      document,
    }),
    listClassifierTags: vi.fn().mockResolvedValue({ default_tags: ['SEなし'], custom_tags: [] }),
  } as never)
}

async function mountSettings(): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets/:libraryId/conversion', name: 'conversion', component: { template: '<div />' } },
      {
        path: '/worksets/:libraryId/conversion/settings',
        name: 'conversion-settings',
        component: ConversionSettingsPage,
      },
    ],
  })
  await router.push('/worksets/lib-a/conversion/settings')
  await router.isReady()
  const wrapper = mount(ConversionSettingsPage, {
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: pageApi() },
    },
  })
  await flushPromises()
  return { wrapper, router }
}

afterEach(() => vi.restoreAllMocks())

describe('ConversionSettingsPage', () => {
  it('states that applying reaches every task and retires their plans', async () => {
    const { wrapper } = await mountSettings()

    const notice = wrapper.get('[data-testid="global-settings-notice"]').text()
    expect(notice).toContain('影响所有任务')
    expect(notice).toContain('已有计划需重新生成')
    expect(notice).toContain('孤立的只读任务不参与同步')
  })
})
