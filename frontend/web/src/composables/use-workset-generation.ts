import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import type { GenerationStatus, GenerationView, OperationType } from '@/lib/api/types'
import { startGenerationMutationOptions, cancelGenerationMutationOptions, syncAfterGenerationTerminal, generationQueryOptions, syncGenerationSnapshot, syncGenerationEvent } from '@/queries/worksets'
import { queryKeys } from '@/queries/query-keys'
import { useWorksetGenerationStore } from '@/stores/workset-generation'
import { sessionPoller, useWorksetObservation } from './use-workset-observation'

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
// Planning orchestration owns session observation, cache synchronization and
// notices across page mounts. The singleton Pinia store still owns SSE state.
export function useWorksetGeneration() {
  const api = useApiClient()
  const queryClient = useQueryClient()
  const store = useWorksetGenerationStore()
  const { observations, identity, notify } = useWorksetObservation<Observation>(queryClient, 'planning', stop)
  const startMutation = useMutation(startGenerationMutationOptions(api, queryClient))
  const cancelMutation = useMutation(cancelGenerationMutationOptions(api))

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
  const poll = sessionPoller(
    (entry: Observation) => queryClient.fetchQuery(generationQueryOptions(api, entry.worksetId, entry.operation, entry.generationId)),
    (entry, session) => { syncGenerationSnapshot(queryClient, session); finalize(entry, session.status) },
    entry => {
      entry.consumed = true
      cleanup(entry)
      notify(entry, '计划生成结果尚未确认，记录不存在或无权访问')
    },
  )
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
