import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ApiClient, apiClientKey } from '@/lib/api/client'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'
import { sampleWorkContent } from '@/test/work-content-fixture'
import LeafLyrics from './LeafLyrics.vue'

const RJ = 'RJ01000001'
let audio: HTMLAudioElement
const scrollTo = vi.fn()
const originalScrollTo = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollTo')

beforeEach(() => {
  setActivePinia(createPinia())
  audio = document.createElement('audio')
  // eslint-disable-next-line prefer-arrow-callback -- called with `new`
  vi.stubGlobal('Audio', function () { return audio })
  vi.stubGlobal('fetch', vi.fn())
  Object.defineProperty(HTMLElement.prototype, 'scrollTo', { configurable: true, value: scrollTo })
})
afterEach(() => {
  usePlayerStore().release()
  usePlayerStore().stopClock()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  scrollTo.mockReset()
  if (originalScrollTo)
    Object.defineProperty(HTMLElement.prototype, 'scrollTo', originalScrollTo)
  else Reflect.deleteProperty(HTMLElement.prototype, 'scrollTo')
})
enableAutoUnmount(afterEach)

async function listen(rel: string, text: string) {
  vi.mocked(fetch).mockResolvedValue(new Response(text, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  }))
  const api = new ApiClient({ baseUrl: '/api/v1', token: 'test-token' })
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ), ref => api.workFileUrl(ref))
  store.pairLrc(1, rel)
  store.startTrack(1)
  const wrapper = mount(LeafLyrics, {
    global: {
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return wrapper
}

it('loads LRC through the file API, follows audio time, scrolls the updated line and seeks audio', async () => {
  const wrapper = await listen('lyrics/a.lrc', '[00:01]first\n[00:05]second')
  expect(vi.mocked(fetch).mock.calls[0][0]).toBe(`/api/v1/works/${RJ}/content/copy-a/lyrics/a.lrc`)
  expect(new Headers(vi.mocked(fetch).mock.calls[0][1]?.headers).get('Authorization')).toBe('Bearer test-token')
  expect(wrapper.find('.now').exists()).toBe(false)
  const scrolled: string[] = []
  scrollTo.mockImplementation(() => { scrolled.push(wrapper.find('.now').text()) })
  audio.currentTime = 2
  audio.dispatchEvent(new Event('timeupdate'))
  await flushPromises()
  expect(wrapper.get('.now').text()).toContain('first')
  audio.currentTime = 6
  audio.dispatchEvent(new Event('timeupdate'))
  await flushPromises()
  expect(scrolled.at(-1)).toContain('second')
  await wrapper.findAll('button')[0].trigger('click')
  expect(audio.currentTime).toBe(1)
})

it.each(['vtt', 'VTT'])('loads %s cues and respects their end times and gaps', async (ext) => {
  const wrapper = await listen(`lyrics/a.${ext}`, 'WEBVTT\n\nintro\n00:01.000 --> 00:03.000\nfirst\n\n00:05.000 --> 00:07.000\nsecond')
  expect(wrapper.findAll('button')).toHaveLength(2)
  audio.currentTime = 2
  audio.dispatchEvent(new Event('timeupdate'))
  await flushPromises()
  expect(wrapper.get('.now').text()).toContain('first')
  audio.currentTime = 4
  audio.dispatchEvent(new Event('timeupdate'))
  await flushPromises()
  expect(wrapper.find('.now').exists()).toBe(false)
})

it('does not highlight or seek another playing work', async () => {
  const wrapper = await listen('lyrics/a.lrc', '[00:01]first')
  usePlayerStore().play('RJ02000002', [
    {
      title: 'other',
      source: { src: '/other.mp3', durationSec: null, format: 'MP3' },
    },
  ])
  audio.currentTime = 2
  audio.dispatchEvent(new Event('timeupdate'))
  await flushPromises()
  expect(wrapper.find('.now').exists()).toBe(false)
  await wrapper.get('button').trigger('click')
  expect(audio.currentTime).toBe(2)
})

it('reloads the same path for another copy and ignores the previous response', async () => {
  const wrapper = await listen('lyrics/a.lrc', '[00:01]initial')
  let finishOld: (response: Response) => void = () => {}
  vi.mocked(fetch).mockImplementationOnce(() => new Promise<Response>((resolve) => { finishOld = resolve }))
  const store = useWorkDetailStore()
  store.bind(RJ, {
    ...sampleWorkContent(RJ),
    copies: [
      { ...store.activeCopy!, copy_id: 'copy-b' },
    ],
  })
  await flushPromises()
  vi.mocked(fetch).mockResolvedValue(new Response('[00:01]new copy'))
  store.bind(RJ, {
    ...sampleWorkContent(RJ),
    copies: [
      { ...store.activeCopy!, copy_id: 'copy-c' },
    ],
  })
  await flushPromises()
  finishOld(new Response('[00:01]stale copy'))
  await flushPromises()
  expect(wrapper.text()).toContain('new copy')
  expect(wrapper.text()).not.toContain('stale copy')
})

it('shows file loading failures instead of an empty pane', async () => {
  const wrapper = await listen('lyrics/a.lrc', '[00:01]initial')
  vi.mocked(fetch).mockResolvedValue(new Response('{}', { status: 404 }))
  useWorkDetailStore().pairLrc(1, 'gone.lrc')
  await flushPromises()
  expect(wrapper.get('[role="status"]').text()).toBe('无法读取歌词文件')
})
