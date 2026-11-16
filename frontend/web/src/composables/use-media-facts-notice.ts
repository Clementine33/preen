import type { Library } from '@/lib/api/types'
import { useQueryClient } from '@tanstack/vue-query'
import { watch } from 'vue'
import { queryKeys } from '@/queries/query-keys'
import { useMediaFactsStore } from '@/stores/media-facts'
import { useTaskResultNotice } from './use-task-toast'

/**
 * Announces files a finished Media Facts Pass could not measure. Every pass
 * revisits them, so a library is announced again only when its count grows;
 * the paths themselves are in the diagnostic log.
 */
export function useMediaFactsNotice(): void {
  const store = useMediaFactsStore()
  const queryClient = useQueryClient()
  const notice = useTaskResultNotice(queryClient)
  const announced = new Map<string, number>()
  watch(() => store.status, (next, prev) => {
    if (!next || !prev || prev.state === 'idle' || next.state !== 'idle')
      return
    for (const { library_id: libraryId, unmeasurable } of next.libraries) {
      const before = announced.get(libraryId) ?? 0
      announced.set(libraryId, unmeasurable)
      if (unmeasurable <= before)
        continue
      const libraryName = queryClient.getQueryData<Library[]>(queryKeys.libraries.list())
        ?.find(library => library.id === libraryId)
        ?.name ?? libraryId
      notice({
        libraryId,
        libraryName,
        title: `${libraryName} · 媒体事实`,
        text: `${unmeasurable.toLocaleString()} 个文件无法测量`,
        tone: 'warning',
        target: 'settings',
        params: { section: 'maintenance' },
        actionLabel: '查看日志',
        quietOn: [],
      })
    }
  })
}
