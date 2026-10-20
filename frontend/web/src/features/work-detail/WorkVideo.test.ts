import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { sampleWorkContent } from '@/test/work-content-fixture'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'
import BayFiles from './BayFiles.vue'
import WorkVideo from './WorkVideo.vue'

const RJ = 'RJ01000001'

describe('WorkVideo', () => {
  it('opens from a video row, pauses the player and stays paused after closing, keeping the bottom bay open', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useWorkDetailStore()
    const content = sampleWorkContent(RJ)
    content.copies[0].tree!.children.push({
      name: 'clip.mp4', rel_path: 'clip.mp4', kind: 'vid', size: 1024,
      format: null, bitrate: null, width: null, height: null, duration_sec: null, thumb_v: null,
    })
    store.bind(RJ, content)
    store.startTrack(0)
    store.openBayTab('files')
    const player = usePlayerStore()
    expect(player.paused).toBe(false)

    const wrapper = mount(
      defineComponent({ render: () => [h(BayFiles), h(WorkVideo)] }),
      { global: { plugins: [pinia], provide: { [apiClientKey as symbol]: apiStub() } } },
    )
    const row = wrapper.findAll('.row').find((r) => r.text().includes('clip.mp4'))!
    await row.trigger('click')
    await flushPromises()

    expect(wrapper.find('dialog').attributes('open')).toBeDefined()
    expect(wrapper.find('video').exists()).toBe(true)
    expect(player.paused).toBe(true)

    await wrapper.find('dialog .btn').trigger('click')
    expect(store.video).toBeNull()
    expect(player.paused).toBe(true)
    expect(store.bayOpen).toBe(true)
  })
})
