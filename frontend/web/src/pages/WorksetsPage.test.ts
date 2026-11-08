import type { Library } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import { desktopAdapterKey } from '@/lib/desktop/desktop-adapter'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import WorksetsPage from './WorksetsPage.vue'

afterEach(() => document.body.replaceChildren())
enableAutoUnmount(afterEach)

function library(id: string, name: string): Library {
  return {
    id,
    name,
    root_path: `/${id}`,
    storage_class: 'hdd',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    last_scan_at: null,
    last_scan_status: 'completed',
    last_scan_error: '',
  } as Library
}

async function mountPage(api: ReturnType<typeof apiStub>) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets', name: 'worksets', component: WorksetsPage },
      {
        path: '/worksets/:libraryId',
        name: 'workbench-overview',
        component: { template: '<div />' },
      },
      {
        path: '/works',
        name: 'works',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/worksets')
  const wrapper = mount(WorksetsPage, {
    attachTo: document.body,
    global: {
      plugins: [
        createPinia(),
        router,
        installTestQueryPlugin(),
      ],
      provide: {
        [apiClientKey as symbol]: api,
        [desktopAdapterKey as symbol]: {
          platform: 'browser',
          lyricsWindow: false,
          openLyricsWindow: async () => null,
          resolveApi: async () => ({ baseUrl: '', token: null }),
          pickFolder: async () => null,
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('worksets page: Default Library', () => {
  const libraries = [
    library('lib-a', 'Archive'),
    library('lib-b', 'Brazil'),
  ]
  const defaultInput = () => document.querySelector<HTMLInputElement>('[data-testid="library-default"]')!

  async function edit(wrapper: Awaited<ReturnType<typeof mountPage>>, id: string, toggle: boolean) {
    await wrapper.get(`[data-testid="edit-${id}"]`).trigger('click')
    await flushPromises()
    if (toggle)
      defaultInput().click()
    document.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await flushPromises()
  }

  it('tags the Default Library in the list', async () => {
    const wrapper = await mountPage(apiStub({
      listLibraries: vi.fn().mockResolvedValue(libraries),
      getPreferences: vi.fn().mockResolvedValue({ track_formats: [], default_library_id: 'lib-a' }),
    }))

    expect(wrapper.find('[data-testid="default-lib-a"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="default-lib-b"]').exists()).toBe(false)
  })

  it('moves the Default Library from the edit dialog', async () => {
    const putPreferences = vi.fn(async input => ({ track_formats: [], ...input }))
    const wrapper = await mountPage(apiStub({
      listLibraries: vi.fn().mockResolvedValue(libraries),
      getPreferences: vi.fn().mockResolvedValue({ track_formats: [], default_library_id: 'lib-a' }),
      putPreferences,
      updateLibrary: vi.fn(async (id: string) => libraries.find(item => item.id === id)),
    }))
    await wrapper.get('[data-testid="edit-lib-b"]').trigger('click')
    await flushPromises()
    expect(defaultInput().checked).toBe(false)
    defaultInput().click()
    document.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await flushPromises()

    expect(putPreferences).toHaveBeenCalledExactlyOnceWith({ default_library_id: 'lib-b' })
    expect(wrapper.find('[data-testid="default-lib-b"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="default-lib-a"]').exists()).toBe(false)
  })

  it('clears the Default Library by unticking it, and writes nothing when the choice is unchanged', async () => {
    const putPreferences = vi.fn(async input => ({ track_formats: [], ...input }))
    const wrapper = await mountPage(apiStub({
      listLibraries: vi.fn().mockResolvedValue(libraries),
      getPreferences: vi.fn().mockResolvedValue({ track_formats: [], default_library_id: 'lib-a' }),
      putPreferences,
      updateLibrary: vi.fn(async (id: string) => libraries.find(item => item.id === id)),
    }))
    await edit(wrapper, 'lib-a', false)
    expect(putPreferences).not.toHaveBeenCalled()

    await edit(wrapper, 'lib-a', true)
    expect(putPreferences).toHaveBeenCalledExactlyOnceWith({ default_library_id: null })
    expect(wrapper.find('[data-testid="default-lib-a"]').exists()).toBe(false)
  })
})
