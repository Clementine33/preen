import { ref } from 'vue'
import { errorDetails } from '@/lib/api/error'

// The app's one toast: a single message at a time, a new one replaces the old.
// State is module-level so any page can `show` and the one ToastHost renders it.
export interface ToastAction { label: string, run: () => void }
export interface ToastMessage { id: number, text: string, action?: ToastAction }

const current = ref<ToastMessage | null>(null)
let seq = 0

function show(message: { text: string, action?: ToastAction }) { current.value = { id: ++seq, ...message } }

export function useToast() {
  return {
    current,
    show,
    // A failed action the user just took; code-specific wording stays with the caller.
    error(error: unknown, fallback?: string) { show({ text: errorDetails(error, fallback).message }) },
    dismiss(id: number) {
      if (current.value?.id === id)
        current.value = null
    },
  }
}
