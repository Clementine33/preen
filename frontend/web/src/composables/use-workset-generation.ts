import { getCurrentInstance } from 'vue'
import { useRouter } from 'vue-router'
import { useMutation, useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { ApiError, useApiClient } from '@/lib/api/client'
import type { GenerationStatus, GenerationView, OperationType, Workset, Library, CurrentRecordResponse } from '@/lib/api/types'
import { startGenerationMutationOptions, cancelGenerationMutationOptions, syncAfterGenerationTerminal, generationQueryOptions, syncGenerationSnapshot, syncGenerationEvent } from '@/queries/worksets'
import { queryKeys } from '@/queries/query-keys'
import { libraryListQueryOptions } from '@/queries/libraries'
import { useWorksetGenerationStore } from '@/stores/workset-generation'
import { useTaskToast } from './use-task-toast'

type Observation = {
  worksetId: string
  operation: OperationType
  generationId: string
  libraryId: string
  libraryName: string
  consumed: boolean
  intentional: boolean
  reading: boolean
  retryDelay: number
  timer?: ReturnType<typeof setTimeout>
}
// Shared by every page in this SPA; consumed identities outlive their streams.
const registries = new WeakMap<QueryClient, Map<string, Observation>>()
const labels = { conversion: '转换', rename: '重命名', metadata_refresh: '获取元数据' }
const routes = { conversion: 'conversion', rename: 'rename', metadata_refresh: 'metadata-refresh' }

// Planning orchestration owns session observation, cache synchronization and
// notices across page mounts. The singleton Pinia store still owns SSE state.
export function useWorksetGeneration() {
  const api = useApiClient()
  const queryClient = useQueryClient()
  const store = useWorksetGenerationStore()
  const router = useRouter()
  const toast = useTaskToast()
  let registry = registries.get(queryClient)
  if (!registry) {
    registry = new Map()
    registries.set(queryClient, registry)
    getCurrentInstance()?.appContext.app.onUnmount(() => stop())
  }
  const observations = registry
  const startMutation = useMutation(startGenerationMutationOptions(api, queryClient))
  const cancelMutation = useMutation(cancelGenerationMutationOptions(api))

  function identity(worksetId: string) {
    const workset = queryClient.getQueryData<Workset>(queryKeys.worksets.detail(worksetId))
      ?? queryClient.getQueriesData<CurrentRecordResponse>({ queryKey: queryKeys.worksets.currentsPrefix() })
        .map(([, record]) => record?.workset).find(record => record?.workset_id === worksetId)
    const known = [...observations.values()].find(entry => entry.worksetId === worksetId)
    const libraryId = workset?.library?.library_id ?? known?.libraryId ?? String(router.currentRoute.value.params.libraryId ?? '')
    const library = queryClient.getQueryData<Library[]>(queryKeys.libraries.list())?.find(item => item.id === libraryId)
    return { libraryId, libraryName: workset?.library?.name ?? library?.name ?? known?.libraryName ?? libraryId }
  }
  function notify(entry: Observation, text: string, suppress = false) {
    const route = router.currentRoute.value
    if (suppress && route.params.libraryId === entry.libraryId && route.matched.some(record => record.name === routes[entry.operation])) return
    toast.show({ text: `${entry.libraryName} · ${labels[entry.operation]} · ${text}`,
      action: { label: '查看结果', run: () => { void viewResults() } } })
    async function viewResults() {
      try {
        const libraries = await queryClient.fetchQuery({ ...libraryListQueryOptions(api), staleTime: 0 })
        if (libraries.some(library => library.id === entry.libraryId)) {
          await router.push({ name: routes[entry.operation], params: { libraryId: entry.libraryId } })
        } else {
          await router.push({ name: 'worksets' })
          toast.show({ text: `${entry.libraryName} · 媒体库已移除，结果不可用` })
        }
      } catch {
        notify(entry, '无法确认媒体库是否可用，请重试')
      }
    }
  }
  function finalize(entry: Observation, status: GenerationStatus) {
    if (entry.consumed || status === 'queued' || status === 'running') return
    entry.consumed = true
    cleanup(entry)
    const text = status === 'completed' ? '计划生成完成' : status === 'failed' ? '计划生成失败'
      : status === 'canceled' ? '计划生成已停止' : '计划生成已中断，未发布新计划，原有计划仍保留'
    notify(entry, text, status === 'completed' || status === 'canceled')
    void syncAfterGenerationTerminal(queryClient, entry.worksetId, entry.operation)
  }
  function cleanup(entry: Observation) {
    clearTimeout(entry.timer)
    entry.timer = undefined
    void queryClient.cancelQueries({ queryKey: queryKeys.worksets.generation(entry.worksetId, entry.operation, entry.generationId), exact: true })
  }
  async function poll(entry: Observation) {
    if (entry.consumed || entry.reading) return
    clearTimeout(entry.timer)
    entry.timer = undefined
    entry.reading = true
    let delay = 2000
    try {
      const session = await queryClient.fetchQuery(generationQueryOptions(api, entry.worksetId, entry.operation, entry.generationId))
      if (entry.consumed) return
      syncGenerationSnapshot(queryClient, session)
      entry.retryDelay = 2000
      finalize(entry, session.status)
    } catch (error) {
      if (entry.consumed) return
      if (error instanceof ApiError && [401, 403, 404].includes(error.status)) {
        entry.consumed = true
        cleanup(entry)
        notify(entry, '计划生成结果尚未确认，记录不存在或无权访问')
        return
      }
      delay = entry.retryDelay
      entry.retryDelay = Math.min(delay * 2, 30000)
    } finally {
      entry.reading = false
      if (!entry.consumed) entry.timer = setTimeout(() => { void poll(entry) }, delay)
    }
  }
  function register(worksetId: string, operation: OperationType, generationId: string, captured = identity(worksetId), initial?: GenerationView) {
    const key = JSON.stringify([worksetId, operation, 'planning', generationId])
    const existing = observations.get(key)
    if (existing) return existing
    const entry: Observation = { worksetId, operation, generationId, ...captured, consumed: false, intentional: false, reading: false, retryDelay: 2000 }
    observations.set(key, entry)
    if (initial) {
      syncGenerationSnapshot(queryClient, initial)
      finalize(entry, initial.status)
      if (entry.consumed) return entry
    }
    void store.attach(worksetId, operation, generationId, api, event => {
      if (entry.consumed) return
      syncGenerationEvent(queryClient, worksetId, operation, generationId, event)
      if (event.type === 'session_snapshot') {
        finalize(entry, event.data.status)
      } else if (event.type === 'completed' || event.type === 'failed' || event.type === 'canceled' || event.type === 'interrupted') {
        finalize(entry, event.type)
      }
    }).then(attached => {
      if (entry.consumed || entry.intentional) return
      if (attached) notify(entry, '连接中断，计划生成结果尚未确认')
      void poll(entry)
    })
    return entry
  }
  async function start(input: { worksetId: string; operation: OperationType; ifMatchVersion: number; idempotencyKey: string }) {
    const captured = identity(input.worksetId)
    const result = await startMutation.mutateAsync(input)
    if ('generation' in result) {
      register(input.worksetId, input.operation, result.generation.generation_id, captured, result.generation)
    }
    return result
  }
  function attach(worksetId: string, operation: OperationType, generationId: string) { register(worksetId, operation, generationId) }
  async function cancel(worksetId: string, operation: OperationType, generationId: string): Promise<void> {
    const entry = register(worksetId, operation, generationId)
    entry.intentional = true
    store.cancel(worksetId, generationId)
    try {
      const result = await cancelMutation.mutateAsync({ worksetId, operation, generationId })
      if (!entry.consumed) {
        syncGenerationSnapshot(queryClient, result)
        finalize(entry, result.status)
      }
    } finally {
      if (!entry.consumed) void poll(entry)
    }
  }

  function stop() {
    for (const entry of observations.values()) { entry.consumed = true; cleanup(entry) }
    store.reset()
  }
  return { start, attach, cancel, stop, store, startMutation, cancelMutation }
}
