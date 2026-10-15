import { defineComponent, h } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, getActivePinia, setActivePinia } from 'pinia'
import { useToast } from '@/composables/use-toast'
import { useTaskToast } from '@/composables/use-task-toast'
import ToastHost from './ToastHost.vue'
import TaskToastHost from './TaskToastHost.vue'

enableAutoUnmount(afterEach)
beforeEach(() => { setActivePinia(createPinia()) })
afterEach(() => {
  const tasks = useTaskToast()
  while (tasks.current.value) tasks.dismiss(tasks.current.value.id)
  useToast().current.value = null
})

it('keeps FIFO results and editing undo independent, including stale dismissal', async () => {
  mount(defineComponent({ setup: () => () => [h(ToastHost), h(TaskToastHost)] }), { attachTo: document.body, global: { plugins: [getActivePinia()!] } })
  const undo = vi.fn()
  useToast().show({ text: '已移除', action: { label: '撤销', run: undo } })
  const tasks = useTaskToast()
  tasks.show({ title: 'Archive', text: '扫描完成', tone: 'success' })
  const firstId = tasks.current.value!.id
  tasks.show({ title: 'Archive', text: '规划失败', tone: 'failed' })
  tasks.show({ title: 'Archive', text: '执行完成', tone: 'success' })
  await flushPromises()
  expect(document.body.querySelector('[data-testid="task-toast"]')?.textContent).toContain('扫描完成')
  document.body.querySelector<HTMLButtonElement>('[data-testid="toast-action"]')!.click()
  await flushPromises()
  expect(undo).toHaveBeenCalledOnce()
  expect(document.body.querySelector('[data-testid="toast"]')).toBeNull()
  document.body.querySelector<HTMLButtonElement>('[aria-label="关闭通知"]')!.click()
  await flushPromises()
  expect(document.body.querySelector('[data-testid="task-toast"]')?.textContent).toContain('规划失败')
  tasks.dismiss(firstId)
  expect(tasks.current.value?.text).toBe('规划失败')
  document.body.querySelector<HTMLButtonElement>('[aria-label="关闭通知"]')!.click()
  await flushPromises()
  expect(document.body.querySelector('[data-testid="task-toast"]')?.textContent).toContain('执行完成')
})

it('keeps task queues independent between app lifetimes', () => {
  const first = useTaskToast()
  first.show({ title: 'Archive', text: 'Archive scan complete', tone: 'success' })
  setActivePinia(createPinia())
  const second = useTaskToast()
  expect(second.current.value).toBeNull()
  second.show({ title: 'Archive', text: 'Other library planning failed', tone: 'failed' })
  expect(first.current.value?.text).toBe('Archive scan complete')
  expect(second.current.value?.text).toBe('Other library planning failed')
})

it('expires success after 8s but keeps failures until dismissed', async () => {
  vi.useFakeTimers()
  try {
    mount(TaskToastHost, { attachTo: document.body, global: { plugins: [getActivePinia()!] } })
    const tasks = useTaskToast()
    tasks.show({ title: 'Archive · 扫描', text: '扫描完成', tone: 'success' })
    tasks.show({ title: 'Archive · 转换 · 执行', text: '执行失败', tone: 'failed' })
    await flushPromises()
    const toast = () => document.body.querySelector('[data-testid="task-toast"]')?.textContent ?? ''
    expect(toast()).toContain('完成')
    await vi.advanceTimersByTimeAsync(7000)
    expect(toast()).toContain('扫描完成')
    await vi.advanceTimersByTimeAsync(1500)
    expect(toast()).toContain('执行失败')
    expect(toast()).toContain('Archive · 转换 · 执行')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(toast()).toContain('执行失败')
  } finally {
    vi.useRealTimers()
  }
})
