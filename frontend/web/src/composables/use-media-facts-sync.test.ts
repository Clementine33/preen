import { QueryObserver } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiClientKey } from '@/lib/api/client'
import type { ApiClientContract } from '@/lib/api/types'
import { queryKeys } from '@/queries/query-keys'
import { apiStub } from '@/test/api-stub'
import { createTestQueryClient, installTestQueryPlugin } from '@/test/query-client'
import { syncMediaFacts, useMediaFactsSync } from './use-media-facts-sync'

function untilAbort(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) resolve()
    else signal.addEventListener('abort', () => resolve(), { once: true })
  })
}

afterEach(() => vi.useRealTimers())

describe('media facts synchronization', () => {
  it('reloads active content and trees, removes inactive caches, and preserves unrelated work data', async () => {
    const client = createTestQueryClient()
    const active = queryKeys.works.content('RJ01000001')
    const inactive = queryKeys.works.content('RJ01000002')
    const tree = queryKeys.libraries.memberTree('lib', 'root', 'dir')
    const oldTree = queryKeys.libraries.memberTree('lib', 'root', 'inactive')
    const detail = queryKeys.works.detail('RJ01000001')
    for (const key of [active, inactive, tree, oldTree, detail]) client.setQueryData(key, 'old')
    const contentObserver = new QueryObserver(client, { queryKey: active, queryFn: async () => 'measured' })
    const treeObserver = new QueryObserver(client, { queryKey: tree, queryFn: async () => 'measured tree' })
    const stopContent = contentObserver.subscribe(() => {})
    const stopTree = treeObserver.subscribe(() => {})
    const controller = new AbortController()
    const api = apiStub({
      streamMediaFactsEvents: async function* (signal) {
        yield { type: 'changed', data: {} }
        await untilAbort(signal)
      },
    })
    const run = syncMediaFacts(api, client, controller.signal)
    await flushPromises()
    expect(client.getQueryData(active)).toBe('measured')
    expect(client.getQueryData(tree)).toBe('measured tree')
    expect(client.getQueryData(inactive)).toBeUndefined()
    expect(client.getQueryData(oldTree)).toBeUndefined()
    expect(client.getQueryData(detail)).toBe('old')
    controller.abort()
    await run
    stopContent()
    stopTree()
    client.clear()
  })

  it('cancels an initial request that read before the notification', async () => {
    const client = createTestQueryClient()
    const key = queryKeys.works.content('RJ01000001')
    let staleReply!: (value: string) => void
    const queryFn = vi.fn()
      .mockImplementationOnce(() => new Promise<string>((resolve) => { staleReply = resolve }))
      .mockResolvedValue('new facts')
    const observer = new QueryObserver(client, { queryKey: key, queryFn })
    const stop = observer.subscribe(() => {})
    const controller = new AbortController()
    const api = apiStub({ streamMediaFactsEvents: async function* (signal) {
      yield { type: 'changed', data: {} }
      await untilAbort(signal)
    } })
    const run = syncMediaFacts(api, client, controller.signal)
    await flushPromises()
    staleReply('stale facts')
    await flushPromises()
    expect(client.getQueryData(key)).toBe('new facts')
    controller.abort()
    await run
    stop()
    client.clear()
  })

  it('backs off through 1, 2, 4 seconds up to 30 and cancels the pending wait', async () => {
    vi.useFakeTimers()
    const api = apiStub({ streamMediaFactsEvents: vi.fn(async function* () { throw new Error('offline') }) })
    const client = createTestQueryClient()
    const controller = new AbortController()
    const run = syncMediaFacts(api, client, controller.signal)
    await vi.advanceTimersByTimeAsync(0)
    let attempts = 1
    for (const delay of [1, 2, 4, 8, 16, 30, 30]) {
      expect(api.streamMediaFactsEvents).toHaveBeenCalledTimes(attempts)
      await vi.advanceTimersByTimeAsync(delay * 1000 - 1)
      expect(api.streamMediaFactsEvents).toHaveBeenCalledTimes(attempts)
      await vi.advanceTimersByTimeAsync(1)
      attempts++
    }
    expect(api.streamMediaFactsEvents).toHaveBeenCalledTimes(attempts)
    controller.abort()
    await run
    expect(vi.getTimerCount()).toBe(0)
    client.clear()
  })

  it('returns to the one-second delay after reconnecting successfully', async () => {
    vi.useFakeTimers()
    const stream = vi.fn<ApiClientContract['streamMediaFactsEvents']>(async function* () { throw new Error('offline') })
    stream.mockImplementationOnce(async function* () { throw new Error('offline') })
    stream.mockImplementationOnce(async function* () { throw new Error('offline') })
    stream.mockImplementationOnce(async function* () { yield { type: 'changed' as const, data: {} } })
    const client = createTestQueryClient()
    const controller = new AbortController()
    const run = syncMediaFacts(apiStub({ streamMediaFactsEvents: stream }), client, controller.signal)
    await vi.advanceTimersByTimeAsync(3000)
    expect(stream).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(1000)
    expect(stream).toHaveBeenCalledTimes(4)
    controller.abort()
    await run
    client.clear()
  })

  it('stops on 401 instead of reconnecting', async () => {
    vi.useFakeTimers()
    const api = apiStub({ streamMediaFactsEvents: vi.fn(async function* () { throw new ApiError(401, 'UNAUTHORIZED', 'unauthorized') }) })
    const client = createTestQueryClient()
    await syncMediaFacts(api, client, new AbortController().signal)
    expect(api.streamMediaFactsEvents).toHaveBeenCalledTimes(1)
    expect(vi.getTimerCount()).toBe(0)
    client.clear()
  })

  it('uses one root connection and closes it on unmount', async () => {
    let signal!: AbortSignal
    let closed = false
    const api = apiStub({ streamMediaFactsEvents: vi.fn(async function* (value) {
      signal = value
      try { await untilAbort(value) } finally { closed = true }
    }) })
    const wrapper = mount({ setup() { useMediaFactsSync(); return () => null } }, {
      global: { plugins: [installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
    })
    await flushPromises()
    expect(api.streamMediaFactsEvents).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(closed).toBe(true)
  })
})
