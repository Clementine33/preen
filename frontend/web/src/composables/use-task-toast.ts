import { computed, ref } from 'vue'
import type { ToastMessage } from './use-toast'

// App-lifetime FIFO for asynchronous results, independent of editing/undo.
const queue = ref<ToastMessage[]>([])
const current = computed(() => queue.value[0] ?? null)
let sequence = 0

export function useTaskToast() {
  return {
    current,
    show(message: Omit<ToastMessage, 'id'>) { queue.value.push({ id: ++sequence, ...message }) },
    dismiss(id: number) { if (current.value?.id === id) queue.value.shift() },
  }
}
