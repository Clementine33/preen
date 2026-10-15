import { defineStore } from 'pinia'
import type { ToastMessage } from '@/composables/use-toast'

// App-lifetime FIFO for asynchronous results, independent of editing/undo.
export const useTaskToastStore = defineStore('task-toast', {
  state: () => ({ queue: [] as ToastMessage[], sequence: 0 }),
  getters: {
    current: state => state.queue[0] ?? null,
  },
  actions: {
    show(message: Omit<ToastMessage, 'id'>) { this.queue.push({ id: ++this.sequence, ...message }) },
    dismiss(id: number) { if (this.current?.id === id) this.queue.shift() },
  },
})
