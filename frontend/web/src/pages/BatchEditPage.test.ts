import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { h } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, RouterView, type Router } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import type { ApiClientContract, Operation, OperationDraftDocument, Workset } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { useWorksetEditorStore } from '@/stores/workset-editor'
import { useWorksetUiStore } from '@/stores/workset-ui'
import BatchEditPage from './BatchEditPage.vue'

/** Batch editing answers an unapplied-edits conflict the same way the member editor does. */
const workset: Workset = {
  workset_id: 'ws-1',
  title: 'T',
  version: 5,
  library: { library_id: 'lib-a', name: 'Lib', root_path: '/music' },
  members: [
    { member_id: 'm-1', folder_path: '/music/a', folder_name: 'a', rel_path: 'a', dir_id: 'd-a' },
    { member_id: 'm-2', folder_path: '/music/b', folder_name: 'b', rel_path: 'b', dir_id: 'd-b' },
  ],
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
  no_effect_tags: ['A'],
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
    getRevision: vi.fn().mockResolvedValue(null),
    listClassifierTags: vi.fn().mockResolvedValue({ default_no_effect_tags: [], default_other_tags: [], custom_tags: [] }),
  } as never)
}

function seedPendingCommonEdit(pinia: ReturnType<typeof createPinia>): void {
  const editor = useWorksetEditorStore(pinia)
  editor.open({
    worksetId: 'ws-1',
    operation: 'conversion',
    target: { kind: 'common' },
    baseVersion: 5,
    baseDocument: document,
  })
  editor.setUnit('mode', { intent: 'set', value: 'available_sources' })
}

async function mountBatch(seedPending: boolean): Promise<{ wrapper: VueWrapper; router: Router }> {
  const pinia = createPinia()
  if (seedPending) seedPendingCommonEdit(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets/:libraryId/conversion', name: 'conversion', component: { template: '<div />' } },
      { path: '/worksets/:libraryId/conversion/batch-edit', name: 'conversion-batch-edit', component: BatchEditPage },
      { path: '/worksets/:libraryId/conversion/settings', name: 'conversion-settings', component: { template: '<div />' } },
      { path: '/worksets/:libraryId/conversion/:memberId/edit', name: 'conversion-member-edit', component: { template: '<div />' } },
    ],
  })
  await router.push('/worksets/lib-a/conversion/batch-edit')
  await router.isReady()
  const ui = useWorksetUiStore(pinia)
  ui.batchMemberIds = ['m-1', 'm-2']
  const wrapper = mount({ render: () => h(RouterView) }, {
    global: { plugins: [pinia, router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: pageApi() } },
  })
  await flushPromises()
  return { wrapper, router }
}

enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())

describe('BatchEditPage pending-edit conflict', () => {
  it('drops the other edit and opens the batch when the user accepts', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const { router } = await mountBatch(true)

    expect(confirm).toHaveBeenCalledOnce()
    expect(useWorksetEditorStore().session?.target).toEqual({ kind: 'batch', memberIds: ['m-1', 'm-2'] })
    expect(router.currentRoute.value.name).toBe('conversion-batch-edit')
  })

  it('keeps the other edit when the user declines', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { router } = await mountBatch(true)

    expect(confirm).toHaveBeenCalledOnce()
    expect(useWorksetEditorStore().session?.target).toEqual({ kind: 'common' })
    expect(router.currentRoute.value.name).toBe('conversion')
  })
})

describe('BatchEditPage route leave', () => {
  it.each([false, true])('confirms unsaved edits and respects the answer %s', async (accept) => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(accept)
    const { router } = await mountBatch(false)
    expect(router.currentRoute.value.name).toBe('conversion-batch-edit')
    const editor = useWorksetEditorStore()
    editor.setUnit('mode', { intent: 'set', value: 'available_sources' })
    await flushPromises()
    expect(editor.isDirty).toBe(true)

    await router.push('/worksets/lib-a/conversion')
    await flushPromises()

    expect(confirm).toHaveBeenCalledOnce()
    expect(router.currentRoute.value.path).toBe(accept ? '/worksets/lib-a/conversion' : '/worksets/lib-a/conversion/batch-edit')
    expect(editor.isDirty).toBe(true)
  })
})
