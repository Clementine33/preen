import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useToast } from '@/composables/use-toast'
import ToastHost from './ToastHost.vue'

enableAutoUnmount(afterEach)

describe('ToastHost', () => {
  it('shows one toast at a time, a new one replacing the old', async () => {
    const wrapper = mount(ToastHost, { attachTo: document.body })
    const toast = useToast()
    toast.show({ text: '一' })
    await flushPromises()
    toast.show({ text: '二' })
    await flushPromises()
    const texts = [...document.body.querySelectorAll('[data-testid="toast"]')].map((e) => e.textContent)
    expect(texts).toHaveLength(1)
    expect(texts[0]).toBe('二')
    wrapper.unmount()
  })

  it('runs the action and closes', async () => {
    const wrapper = mount(ToastHost, { attachTo: document.body })
    const run = vi.fn()
    useToast().show({ text: '已移除', action: { label: '撤销', run } })
    await flushPromises()
    document.body.querySelector<HTMLElement>('[data-testid="toast-action"]')!.click()
    await flushPromises()
    expect(run).toHaveBeenCalledOnce()
    expect(document.body.querySelector('[data-testid="toast"]')).toBeNull()
    wrapper.unmount()
  })
})
