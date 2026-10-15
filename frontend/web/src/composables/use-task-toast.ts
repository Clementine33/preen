import { storeToRefs } from 'pinia'
import { useTaskToastStore } from '@/stores/task-toast'

export function useTaskToast() {
  const store = useTaskToastStore()
  const { current } = storeToRefs(store)
  return { current, show: store.show, dismiss: store.dismiss }
}
