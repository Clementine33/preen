import { useQuery, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { createTestQueryClient } from '@/test/query-client'
import { refreshOrRemoveQueries } from './cache-sync'

describe('refreshOrRemoveQueries', () => {
  it('refetches active members and removes inactive members under the prefix', async () => {
    const queryClient = createTestQueryClient()

    const activeCall = vi.fn().mockResolvedValueOnce('first').mockResolvedValueOnce('second')
    const activeKey = ['family', 'active'] as const
    const inactiveCall = vi.fn().mockResolvedValue('inactive-data')
    const inactiveKey = ['family', 'inactive'] as const

    // Seed an inactive (never observed) query so it is a cache member.
    await queryClient.fetchQuery({ queryKey: inactiveKey, queryFn: inactiveCall })

    // Mount one active observer under the family prefix.
    const Active = defineComponent({
      setup() {
        const query = useQuery({ queryKey: activeKey, queryFn: activeCall })
        return { query }
      },
      template: `<div>{{ query.data ?? 'none' }}</div>`,
    })
    const wrapper = mount(Active, {
      global: {
        plugins: [
          [
            VueQueryPlugin,
            { queryClient },
          ],
        ],
      },
    })
    await flushPromises()
    expect(wrapper.text()).toBe('first')
    expect(inactiveCall).toHaveBeenCalledTimes(1)

    await refreshOrRemoveQueries(queryClient, ['family'] as const)
    await flushPromises()

    // Active member refetched and now holds the fresh data.
    expect(activeCall).toHaveBeenCalledTimes(2)
    expect(queryClient.getQueryData(activeKey)).toBe('second')
    // Inactive members were removed — the next mount cold-fetches.
    expect(queryClient.getQueryData(inactiveKey)).toBeUndefined()
  })

  it('keeps an active member whose key an inactive one partially matches', async () => {
    const queryClient = createTestQueryClient()
    const inactiveKey = [
      'family',
      'list',
      { page: 1 },
    ] as const
    const activeKey = [
      'family',
      'list',
      { library: 'lib-a', page: 1 },
    ] as const
    await queryClient.fetchQuery({
      queryKey: inactiveKey,
      queryFn: () => 'unscoped',
    })
    const activeCall = vi.fn().mockResolvedValue('scoped')
    const Active = defineComponent({
      setup: () => ({
        query: useQuery({ queryKey: activeKey, queryFn: activeCall }),
      }),
      template: `<div>{{ query.data ?? 'none' }}</div>`,
    })
    mount(Active, {
      global: {
        plugins: [
          [
            VueQueryPlugin,
            { queryClient },
          ],
        ],
      },
    })
    await flushPromises()

    await refreshOrRemoveQueries(queryClient, ['family'] as const)
    await flushPromises()

    expect(queryClient.getQueryData(inactiveKey)).toBeUndefined()
    expect(queryClient.getQueryData(activeKey)).toBe('scoped')
  })

  it('leaves keys outside the prefix untouched', async () => {
    const queryClient = createTestQueryClient()
    const outsideKey = ['plans', 'list', 'lib-a', 100] as const
    const outsideCall = vi.fn().mockResolvedValue([])
    await queryClient.fetchQuery({ queryKey: outsideKey, queryFn: outsideCall })

    await refreshOrRemoveQueries(queryClient, ['family'] as const)

    expect(queryClient.getQueryData(outsideKey)).toEqual([])
    expect(outsideCall).toHaveBeenCalledTimes(1)
  })
})
