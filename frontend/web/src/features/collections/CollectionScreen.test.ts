import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import CollectionScreen from './CollectionScreen.vue'

enableAutoUnmount(afterEach)

let frames: Map<number, FrameRequestCallback>
let nextId: number

beforeEach(() => {
  vi.useFakeTimers()
  frames = new Map()
  nextId = 1
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    frames.set(nextId, cb)
    return nextId++
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => void frames.delete(id))
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function runFrames() {
  const pending = [
    ...frames.values(),
  ]
  frames.clear()
  pending.forEach(cb => cb(0))
}

describe('collectionScreen', () => {
  it('shows one picture when idle and mounts the scan passes while scanning', async () => {
    const wrapper = mount(CollectionScreen, {
      props: { url: '/a.jpg' },
    })
    expect(wrapper.findAll('img')).toHaveLength(1)
    await wrapper.setProps({ url: '/b.jpg' })
    runFrames()
    await nextTick()
    expect(wrapper.findAll('img')).toHaveLength(3)
    expect(wrapper.get('img.scr-base').attributes('src')).toBe('/a.jpg')
    expect(wrapper.get('img.p1').attributes('src')).toBe('/b.jpg')
    vi.advanceTimersByTime(400)
    await nextTick()
    expect(wrapper.findAll('img')).toHaveLength(1)
    expect(wrapper.get('img.scr-base').attributes('src')).toBe('/b.jpg')
  })

  it('replaces the base picture element when the url changes, so an old picture never lingers', async () => {
    const wrapper = mount(CollectionScreen, {
      props: { url: '/a.jpg' },
    })
    const before = wrapper.get('img.scr-base').element
    await wrapper.setProps({ url: '/b.jpg', instant: true })
    expect(wrapper.get('img.scr-base').element).not.toBe(before)
  })

  it('cancels a pending frame when a later instant change lands, so the old url does not come back', async () => {
    const wrapper = mount(CollectionScreen, {
      props: { url: '/a.jpg' },
    })
    await wrapper.setProps({ url: '/b.jpg' }) // schedules a frame
    await wrapper.setProps({ url: '/c.jpg', instant: true })
    runFrames()
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(wrapper.classes()).not.toContain('scanning')
    expect(wrapper.findAll('img')).toHaveLength(1)
    expect(wrapper.get('img.scr-base').attributes('src')).toBe('/c.jpg')
  })

  it('reports a base picture that fails to load', async () => {
    const wrapper = mount(CollectionScreen, {
      props: { url: '/a.jpg' },
    })
    await wrapper.get('img.scr-base').trigger('error')
    expect(wrapper.emitted('failed')).toEqual([
      ['/a.jpg'],
    ])
  })
})
