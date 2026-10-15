import { defineComponent, h } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { useToast } from '@/composables/use-toast'
import { useTaskToast } from '@/composables/use-task-toast'
import ToastHost from './ToastHost.vue'
import TaskToastHost from './TaskToastHost.vue'

enableAutoUnmount(afterEach)
afterEach(() => {
  const tasks = useTaskToast()
  while (tasks.current.value) tasks.dismiss(tasks.current.value.id)
  useToast().current.value = null
})

it('keeps FIFO results and editing undo independent, including stale dismissal', async () => {
  mount(defineComponent({ setup: () => () => [h(ToastHost), h(TaskToastHost)] }), { attachTo: document.body })
  const undo = vi.fn()
  useToast().show({ text: '已移除', action: { label: '撤销', run: undo } })
  const tasks = useTaskToast()
  tasks.show({ text: '扫描完成' })
  const firstId = tasks.current.value!.id
  tasks.show({ text: '规划失败' })
  tasks.show({ text: '执行完成' })
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
