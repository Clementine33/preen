import type { MediaFactsStatus } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'
import { apiClientKey } from '@/lib/api/client'
import { queryKeys } from '@/queries/query-keys'
import { useMediaFactsStore } from '@/stores/media-facts'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { useMediaFactsNotice } from './use-media-facts-notice'
import { useTaskToast } from './use-task-toast'

enableAutoUnmount(afterEach)
afterEach(() => {
  const toast = useTaskToast()
  while (toast.current.value) toast.dismiss(toast.current.value.id)
})

function status(state: MediaFactsStatus['state'], unmeasurable: number): MediaFactsStatus {
  return {
    state,
    libraries: [
      { library_id: 'lib-a', pending: 0, unmeasurable },
    ],
  }
}

async function harness() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/',
        name: 'worksets',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/')
  const plugin = installTestQueryPlugin()
  plugin[1].queryClient.setQueryData(queryKeys.libraries.list(), [
    { id: 'lib-a', name: 'Archive' },
  ])
  mount(defineComponent({
    setup() {
      useMediaFactsNotice()
      return () => h(TaskToastHost)
    },
  }), {
    attachTo: document.body,
    global: {
      plugins: [
        createPinia(),
        router,
        plugin,
      ],
      provide: {
        [apiClientKey as symbol]: apiStub({}),
      },
    },
  })
  return useMediaFactsStore()
}

function notice() { return document.body.querySelector('[data-testid="task-toast"]')?.textContent ?? '' }

describe('media facts notice', () => {
  it('warns once when a pass ends with more unmeasurable files, never on connect', async () => {
    const store = await harness()
    store.status = status('idle', 3)
    await flushPromises()
    expect(notice()).toBe('')

    store.status = status('running', 3)
    await flushPromises()
    store.status = status('idle', 3)
    await flushPromises()
    expect(notice()).toContain('注意')
    expect(notice()).toContain('3 个文件无法测量')
    expect(notice()).toContain('查看日志')

    const toast = useTaskToast()
    toast.dismiss(toast.current.value!.id)
    store.status = status('running', 3)
    await flushPromises()
    store.status = status('idle', 3)
    await flushPromises()
    expect(notice()).toBe('')
  })
})
