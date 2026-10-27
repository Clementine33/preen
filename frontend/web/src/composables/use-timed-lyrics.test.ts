import type { LyricsSource } from './use-timed-lyrics'
import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import { useTimedLyrics } from './use-timed-lyrics'

function setup(text: string, relPath: string) {
  const api = {
    getWorkText: vi.fn().mockResolvedValue(text),
  }
  const source = ref<LyricsSource | null>({ rj: 'RJ1', copyId: 'c', relPath })
  const clock = ref(0)
  let result!: ReturnType<typeof useTimedLyrics>
  mount(defineComponent({
    setup: () => ((result = useTimedLyrics(source, () => clock.value)), () => null),
  }), {
    global: {
      provide: { [apiClientKey as symbol]: api },
    },
  })
  return { api, source, clock, result }
}

it('fetches and parses the source, then follows the clock', async () => {
  const { api, clock, result } = setup('[00:01]first\n[00:05]second', 'a.lrc')
  await flushPromises()
  expect(api.getWorkText).toHaveBeenCalledWith({ rj: 'RJ1', copyId: 'c', relPath: 'a.lrc' }, expect.any(AbortSignal))
  expect(result.nowIndex.value).toBe(-1)
  clock.value = 2
  expect(result.nowIndex.value).toBe(0)
  clock.value = 6
  expect(result.nowIndex.value).toBe(1)
})

it('ends a WebVTT cue at its endTime and clears when the source goes', async () => {
  const { source, clock, result } = setup('WEBVTT\n\n00:01.000 --> 00:02.000\nhi', 'a.vtt')
  await flushPromises()
  clock.value = 1.5
  expect(result.nowIndex.value).toBe(0)
  clock.value = 3
  expect(result.nowIndex.value).toBe(-1)
  source.value = null
  await flushPromises()
  expect(result.lines.value).toEqual([])
})
