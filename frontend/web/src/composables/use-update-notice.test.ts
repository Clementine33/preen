import type { UpdateCheck } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { useToast } from '@/composables/use-toast'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { UPDATE_NOTIFIED_KEY, useUpdateNotice } from './use-update-notice'

enableAutoUnmount(afterEach)

const AVAILABLE: UpdateCheck = { state: 'available', latest_version: 'v0.3.0', release_url: 'https://example.test/releases/v0.3.0' }

function mountNotice(getUpdateCheck: () => Promise<UpdateCheck>) {
  return mount(defineComponent({
    setup: () => useUpdateNotice(),
    template: '<div />',
  }), {
    global: {
      plugins: [
        installTestQueryPlugin(),
      ],
      provide: {
        [apiClientKey as symbol]: apiStub({
          getUpdateCheck: vi.fn(getUpdateCheck),
        }),
      },
    },
  })
}

beforeEach(() => {
  localStorage.clear()
  useToast().current.value = null
})
afterEach(() => vi.unstubAllGlobals())

describe('useUpdateNotice', () => {
  it('announces a new version until its action is taken, then records it', async () => {
    mountNotice(async () => AVAILABLE)
    await flushPromises()

    const toast = useToast().current.value
    expect(toast?.text).toBe('发现新版本 v0.3.0')
    expect(toast?.action?.label).toBe('查看')
    expect(localStorage.getItem(UPDATE_NOTIFIED_KEY)).toBeNull()

    const open = vi.fn()
    vi.stubGlobal('open', open)
    toast!.action!.run()
    expect(localStorage.getItem(UPDATE_NOTIFIED_KEY)).toBe('v0.3.0')
    expect(open).toHaveBeenCalledWith('https://example.test/releases/v0.3.0', '_blank', 'noopener,noreferrer')

    // The same version does not announce itself again, even after the toast is gone.
    useToast().current.value = null
    mountNotice(async () => AVAILABLE)
    await flushPromises()
    expect(useToast().current.value).toBeNull()
  })

  it('stays silent for a version already recorded and speaks again for a newer one', async () => {
    localStorage.setItem(UPDATE_NOTIFIED_KEY, 'v0.3.0')
    mountNotice(async () => AVAILABLE)
    await flushPromises()
    expect(useToast().current.value).toBeNull()

    mountNotice(async () => ({ ...AVAILABLE, latest_version: 'v0.4.0' } as UpdateCheck))
    await flushPromises()
    expect(useToast().current.value?.text).toBe('发现新版本 v0.4.0')
  })

  it('announces again next launch when the toast was replaced without taking its action', async () => {
    mountNotice(async () => AVAILABLE)
    await flushPromises()
    useToast().show({ text: 'something else' })
    expect(localStorage.getItem(UPDATE_NOTIFIED_KEY)).toBeNull()

    useToast().current.value = null
    mountNotice(async () => AVAILABLE)
    await flushPromises()
    expect(useToast().current.value?.text).toBe('发现新版本 v0.3.0')
  })

  it.each<UpdateCheck>([
    { state: 'none' },
    { state: 'checking' },
  ])('shows nothing while the state is $state', async (state) => {
    mountNotice(async () => state)
    await flushPromises()
    expect(useToast().current.value).toBeNull()
    expect(localStorage.getItem(UPDATE_NOTIFIED_KEY)).toBeNull()
  })

  it('polls while checking and announces once the check settles', async () => {
    vi.useFakeTimers()
    try {
      const getUpdateCheck = vi.fn<() => Promise<UpdateCheck>>()
        .mockResolvedValueOnce({ state: 'checking' })
        .mockResolvedValue(AVAILABLE)
      mountNotice(getUpdateCheck)
      await vi.advanceTimersByTimeAsync(0)
      expect(getUpdateCheck).toHaveBeenCalledTimes(1)
      expect(useToast().current.value).toBeNull()

      await vi.advanceTimersByTimeAsync(5000)
      expect(getUpdateCheck).toHaveBeenCalledTimes(2)
      expect(useToast().current.value?.text).toBe('发现新版本 v0.3.0')

      // Settled: no more polling.
      await vi.advanceTimersByTimeAsync(30000)
      expect(getUpdateCheck).toHaveBeenCalledTimes(2)
    }
    finally {
      vi.useRealTimers()
    }
  })
})
