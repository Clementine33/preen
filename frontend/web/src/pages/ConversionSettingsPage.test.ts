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
import ConversionSettingsPage from './ConversionSettingsPage.vue'

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
  no_effect_tags: ['A'], other_tags: [],
  matched: {},
  unmatched: {},
  members: [],
}

function pageApi(alreadySaved = false): ApiClientContract {
  let globalSaved = alreadySaved
  return apiStub({
    getCurrentRecord: vi.fn(async (libraryId: string) => ({
      workset: libraryId === 'lib-b' ? { ...workset, workset_id: 'ws-2' } : workset,
    })),
    getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn().mockResolvedValue(operation),
    getOperationDraft: vi.fn(async (worksetId: string) => ({
      workset_id: worksetId,
      operation_type: 'conversion',
      version: 5,
      schema_version: 1,
      common_settings_current: globalSaved,
      updated_at: '',
      document,
    })),
    saveOperationDraft: vi.fn(async () => {
      globalSaved = true
      return { ...operation, version: 6 }
    }),
    listClassifierTags: vi.fn().mockResolvedValue({ default_no_effect_tags: ['SEなし'], default_other_tags: [], custom_tags: [] }),
  } as never)
}

async function mountSettings(alreadySaved = false): Promise<{ wrapper: VueWrapper; router: Router; api: ApiClientContract }> {
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
  const api = pageApi(alreadySaved)
  const wrapper = mount({ render: () => h(RouterView) }, {
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, router, api }
}

enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())

describe('ConversionSettingsPage', () => {
  it('can publish unchanged settings from an older task as the global value', async () => {
    const { wrapper, api } = await mountSettings()
    const apply = wrapper.get('[data-testid="apply-common"]')
    expect(apply.attributes('disabled')).toBeUndefined()
    await apply.trigger('click')
    await flushPromises()
    expect(api.saveOperationDraft).toHaveBeenCalledWith('ws-1', 'conversion', document, 5, true)
    expect(apply.attributes('disabled')).toBeDefined()
    await apply.trigger('click')
    expect(api.saveOperationDraft).toHaveBeenCalledTimes(1)

    useWorksetEditorStore().setUnit('mode', { intent: 'set', value: 'available_sources' })
    await flushPromises()
    expect(apply.attributes('disabled')).toBeUndefined()
  })

  it('keeps apply disabled after switching to a task with the same global settings', async () => {
    const { wrapper, router } = await mountSettings()
    const apply = wrapper.get('[data-testid="apply-common"]')
    await apply.trigger('click')
    await flushPromises()
    expect(apply.attributes('disabled')).toBeDefined()

    await router.push('/worksets/lib-b/conversion/settings')
    await flushPromises()
    expect(apply.attributes('disabled')).toBeDefined()
  })

  it('reads the applied state again when the page opens', async () => {
    const { wrapper } = await mountSettings(true)
    expect(wrapper.get('[data-testid="apply-common"]').attributes('disabled')).toBeDefined()
  })
})


describe('ConversionSettingsPage route leave', () => {
  it.each([false, true])('confirms unsaved edits and respects the answer %s', async (accept) => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(accept)
    const { router } = await mountSettings(false)
    const editor = useWorksetEditorStore()
    editor.setUnit('mode', { intent: 'set', value: 'available_sources' })
    await flushPromises()
    expect(editor.isDirty).toBe(true)

    await router.push('/worksets/lib-a/conversion')
    await flushPromises()

    expect(confirm).toHaveBeenCalledOnce()
    expect(router.currentRoute.value.path).toBe(accept ? '/worksets/lib-a/conversion' : '/worksets/lib-a/conversion/settings')
    expect(editor.isDirty).toBe(true)
  })
})
