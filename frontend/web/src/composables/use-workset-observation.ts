import { getCurrentInstance } from 'vue'
import { useRouter } from 'vue-router'
import { isCancelledError, type QueryClient } from '@tanstack/vue-query'
import type { OperationType, Workset, Library, CurrentRecordResponse } from '@/lib/api/types'
import { ApiError, useApiClient } from '@/lib/api/client'
import { queryKeys } from '@/queries/query-keys'
import { libraryListQueryOptions } from '@/queries/libraries'
import { useTaskToast } from './use-task-toast'

export type SessionIdentity = { worksetId: string; operation: OperationType; libraryId: string; libraryName: string }
// Registry identities survive page mounts and remain consumed for this SPA.
const registries = new WeakMap<QueryClient, Map<string, Map<string, SessionIdentity>>>()
const labels = { conversion: '转换', rename: '重命名', metadata_refresh: '获取元数据' }
const routes = { conversion: 'conversion', rename: 'rename', metadata_refresh: 'metadata-refresh' }

export function useWorksetObservation<T extends SessionIdentity>(queryClient: QueryClient, phase: 'planning' | 'execution' | 'covers', stop: () => void) {
  const api = useApiClient()
  const router = useRouter()
  const toast = useTaskToast()
  let phases = registries.get(queryClient)
  if (!phases) { phases = new Map(); registries.set(queryClient, phases) }
  let registry = phases.get(phase)
  if (!registry) {
    registry = new Map()
    phases.set(phase, registry)
    getCurrentInstance()?.appContext.app.onUnmount(stop)
  }
  const observations = registry as Map<string, T>
  function identity(worksetId: string) {
    const workset = queryClient.getQueryData<Workset>(queryKeys.worksets.detail(worksetId))
      ?? queryClient.getQueriesData<CurrentRecordResponse>({ queryKey: queryKeys.worksets.currentsPrefix() })
        .map(([, record]) => record?.workset).find(record => record?.workset_id === worksetId)
    const known = [...observations.values()].find(entry => entry.worksetId === worksetId)
    const libraryId = workset?.library?.library_id ?? known?.libraryId ?? String(router.currentRoute.value.params.libraryId ?? '')
    const library = queryClient.getQueryData<Library[]>(queryKeys.libraries.list())?.find(item => item.id === libraryId)
    return { libraryId, libraryName: workset?.library?.name ?? library?.name ?? known?.libraryName ?? libraryId }
  }
  function notify(entry: SessionIdentity, text: string, suppress = false) {
    const route = router.currentRoute.value
    if (suppress && route.params.libraryId === entry.libraryId && route.matched.some(record => record.name === routes[entry.operation])) return
    toast.show({ text: `${entry.libraryName} · ${labels[entry.operation]} · ${text}`,
      action: { label: '查看结果', run: () => { void viewResults() } } })
    async function viewResults() {
      try {
        const libraries = await queryClient.fetchQuery({ ...libraryListQueryOptions(api), staleTime: 0 })
        if (libraries.some(library => library.id === entry.libraryId)) {
          await router.push({ name: phase === 'execution' && entry.operation === 'conversion' ? 'conversion-execution' : routes[entry.operation], params: { libraryId: entry.libraryId } })
        } else {
          await router.push({ name: 'worksets' })
          toast.show({ text: `${entry.libraryName} · 媒体库已移除，结果不可用` })
        }
      } catch {
        notify(entry, '无法确认媒体库是否可用，请重试')
      }
    }
  }
  return { observations, identity, notify }
}

type SessionPollingState = { consumed: boolean; reading: boolean; retryDelay: number; timer?: ReturnType<typeof setTimeout> }

// Both workset phases recover by reading their original session, with one read
// or timer per identity and a shared bounded retry policy.
export function sessionPoller<T extends SessionPollingState, R>(
  read: (entry: T) => Promise<R>,
  observe: (entry: T, result: R) => void,
  unavailable: (entry: T) => void,
  disconnected?: (entry: T) => void,
  recovered?: (entry: T) => void,
) {
  return async function poll(entry: T) {
    if (entry.consumed || entry.reading) return
    clearTimeout(entry.timer)
    entry.timer = undefined
    entry.reading = true
    let delay = 2000
    try {
      const result = await read(entry)
      if (entry.consumed) return
      recovered?.(entry)
      entry.retryDelay = 2000
      observe(entry, result)
    } catch (error) {
      if (entry.consumed || isCancelledError(error)) return
      if (error instanceof ApiError && [401, 403, 404].includes(error.status)) {
        unavailable(entry)
        return
      }
      disconnected?.(entry)
      delay = entry.retryDelay
      entry.retryDelay = Math.min(delay * 2, 30000)
    } finally {
      entry.reading = false
      if (!entry.consumed) entry.timer = setTimeout(() => { void poll(entry) }, delay)
    }
  }
}
