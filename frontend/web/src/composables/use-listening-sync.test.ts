import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import type { WorkDetail, WorkListening } from '@/lib/api/types'
import { queryKeys } from '@/queries/query-keys'
import { usePlayerStore } from '@/stores/player'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { useListeningSync } from './use-listening-sync'

const RJ = 'RJ01000001'
const row = (rendition: string, track_key: string): WorkListening => ({
  rendition, track_key, position_sec: 1, track_index: 0, track_count: 2, finished_at: null, updated_at: '2026-10-21T00:00:00Z',
})

describe('useListeningSync', () => {
  it('patches the detail with the acknowledged row and reads nothing back', async () => {
    const query = installTestQueryPlugin()
    const client = query[1].queryClient
    const getWork = vi.fn()
    const pinia = createPinia()
    mount(defineComponent({ setup: () => useListeningSync(), template: '<div />' }), {
      global: { plugins: [pinia, query], provide: { [apiClientKey as symbol]: apiStub({ getWork } as never) } },
    })
    client.setQueryData(queryKeys.works.detail(RJ), { rj: RJ, listening: [row('SE有', 'a'), row('', 'b')] } as unknown as WorkDetail)
    const player = usePlayerStore(pinia)

    player.saved = { rj: RJ, row: row('', 'c'), lists: false }
    await flushPromises()
    expect(client.getQueryData<WorkDetail>(queryKeys.works.detail(RJ))!.listening.map((l) => l.track_key)).toEqual(['c', 'a'])

    player.saved = { rj: RJ, row: null, lists: false }
    await flushPromises()
    expect(client.getQueryData<WorkDetail>(queryKeys.works.detail(RJ))!.listening).toEqual([])
    expect(getWork).not.toHaveBeenCalled()
  })
})
