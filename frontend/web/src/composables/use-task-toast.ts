import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import type { QueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import { libraryListQueryOptions } from '@/queries/libraries'
import { useTaskToastStore, type TaskTone } from '@/stores/task-toast'

export function useTaskToast() {
  const store = useTaskToastStore()
  const { current } = storeToRefs(store)
  return { current, show: store.show, dismiss: store.dismiss }
}

export interface TaskResult {
  libraryId: string
  libraryName: string
  title: string
  text: string
  tone: TaskTone
  /** Route the 查看结果 action opens, addressed by the captured Library. */
  target: string
  /** Route names on which this result stays quiet: the page already shows it. */
  quietOn: readonly string[]
}

// The one result notice every background task (scan, planning, execution,
// covers) ends with. 查看结果 rechecks the Library first, so a result whose
// Library was removed meanwhile explains that instead of opening a dead page.
export function useTaskResultNotice(queryClient: QueryClient) {
  const api = useApiClient()
  const router = useRouter()
  const toast = useTaskToast()
  function notify(result: TaskResult): void {
    const route = router.currentRoute.value
    if (route.params.libraryId === result.libraryId
      && route.matched.some(record => result.quietOn.includes(String(record.name)))) return
    toast.show({ title: result.title, text: result.text, tone: result.tone,
      action: { label: '查看结果', run: () => { void viewResults() } } })
    async function viewResults() {
      try {
        const libraries = await queryClient.fetchQuery({ ...libraryListQueryOptions(api), staleTime: 0 })
        if (libraries.some(library => library.id === result.libraryId)) {
          await router.push({ name: result.target, params: { libraryId: result.libraryId } })
        } else {
          await router.push({ name: 'worksets' })
          toast.show({ title: result.libraryName, text: '媒体库已移除，结果不可用', tone: 'unknown' })
        }
      } catch {
        notify({ ...result, text: '无法确认媒体库是否可用，请重试', tone: 'unknown', quietOn: [] })
      }
    }
  }
  return notify
}
