import type { ToastAction } from '@/composables/use-toast'
import { defineStore } from 'pinia'

// The notification category a result maps to. Failure-like tones stay until
// dismissed; success and stopped expire.
export type TaskTone = 'success' | 'stopped' | 'partial' | 'failed' | 'interrupted' | 'unknown'
export interface TaskNotice { id: number, title: string, text: string, tone: TaskTone, action?: ToastAction }

// App-lifetime FIFO for asynchronous results, independent of editing/undo.
export const useTaskToastStore = defineStore('task-toast', {
  state: () => ({ queue: [] as TaskNotice[], sequence: 0 }),
  getters: {
    current: state => state.queue[0] ?? null,
  },
  actions: {
    show(message: Omit<TaskNotice, 'id'>) { this.queue.push({ id: ++this.sequence, ...message }) },
    dismiss(id: number) {
      if (this.current?.id === id)
        this.queue.shift()
    },
  },
})
