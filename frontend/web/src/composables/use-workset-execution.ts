import { useQueryClient, useMutation } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import type { ExecutionStatus, ExecutionView, OperationType } from '@/lib/api/types'
import {
  executionQueryOptions,
  changeExecutionCoversMutationOptions,
  syncExecutionSnapshot,
  cancelExecutionMutationOptions,
  startExecutionMutationOptions,
  sweepAfterExecution,
  syncAfterExecutionRefusal,
  syncAfterExecutionTerminal,
} from '@/queries/worksets'
import { useWorksetExecutionStore } from '@/stores/workset-execution'
import { queryKeys } from '@/queries/query-keys'
import { sessionPoller, useWorksetObservation, type SessionIdentity } from './use-workset-observation'

import { useWorksetCovers } from './use-workset-covers'

type Observation = SessionIdentity & { executionId: string; eligible: boolean; consumed: boolean; reading: boolean; retryDelay: number; confirmed?: ExecutionStatus; terminal?: ExecutionStatus; stopped?: boolean; timer?: ReturnType<typeof setTimeout> }

// Single entry point for execution-session orchestration: pages call
// start/cancel/attach and never assemble cache invalidation themselves. The
// SSE lifecycle lives in the workset-execution Pinia store (the same
// exception as the scan store); this composable owns Vue Query coordination.

export function useWorksetExecution() {
  const api = useApiClient()
  const queryClient = useQueryClient()
  const store = useWorksetExecutionStore()

  const { observations, identity, notify } = useWorksetObservation<Observation>(queryClient, 'execution', stop)

  const covers = useWorksetCovers(api, queryClient)
  const coverOptions = changeExecutionCoversMutationOptions(api, queryClient)
  const coversMutation = useMutation({ ...coverOptions, onMutate: input => identity(input.worksetId), onSuccess: (report, input, captured) => {
    coverOptions.onSuccess(report, input)
    if (input.action === 'retry') {
      covers.retry(input.worksetId, input.executionId, captured ?? identity(input.worksetId))
      const entry = register(input.worksetId, 'metadata_refresh', input.executionId, captured, undefined, false)
      if (entry.terminal) covers.terminal(entry, false)
      else if (entry.consumed) covers.unavailable(entry.worksetId, entry.executionId)
      else void poll(entry)
    }
  } })

  const startOptions = startExecutionMutationOptions(api, queryClient)
  const startMutation = useMutation({ ...startOptions,
    onSuccess: (result, input) => {
      const key = JSON.stringify([input.worksetId, input.operation, 'execution', result.execution.execution_id])
      if (!observations.get(key)?.consumed) startOptions.onSuccess(result, input)
    },
  })
  const cancelMutation = useMutation({ ...cancelExecutionMutationOptions(api, queryClient), onSuccess: () => {} })

  type StartInput = {
    worksetId: string
    operation: OperationType
    planId: string
    ifMatchVersion: number
    idempotencyKey: string
    /** The folders this run covers; absent runs the whole revision. */
    folderPaths?: string[]
  }

  // Starts a session and attaches the SSE stream on a fresh 202. Resolves with
  // the raw response so the caller can distinguish a key replay (200). A
  // refused start re-reads the server state before surfacing the error.
  async function start(input: StartInput) {
    try {
      const captured = identity(input.worksetId)
      const result = await startMutation.mutateAsync(input)
      register(input.worksetId, input.operation, result.execution.execution_id, captured, result.execution)
      return result
    } catch (error) {
      await syncAfterExecutionRefusal(queryClient, input.worksetId, input.operation)
      throw error
    }
  }

  // Attaches the SSE stream for an already-running session — after a page
  // reload, or when the workspace mounts while the backend session is active.
  // Overflow sessions use exact-session polling while the original singleton
  // stream continues undisturbed.
  function attach(worksetId: string, operation: OperationType, executionId: string): void {
    register(worksetId, operation, executionId)
  }

  // Explicit user cancel. The stream stays attached: a running session stops
  // cooperatively at the worker's next safe boundary, and the terminal event
  // that follows is what the UI waits for. The POST response is the fallback
  // outcome when the stream is already gone.
  async function cancel(worksetId: string, operation: OperationType, executionId: string): Promise<void> {
    store.requestCancel(worksetId, executionId)
    const entry = register(worksetId, operation, executionId)
    try {
      const result = await cancelMutation.mutateAsync({ worksetId, operation, executionId })
      if (!entry.consumed) {
        syncExecutionSnapshot(queryClient, result)
        finalize(entry, result.status, result.completed_operations)
      }
    } finally {
      if (!entry.consumed) void poll(entry)
    }
  }

  function cleanup(entry: Observation) {
    clearTimeout(entry.timer)
    entry.timer = undefined
    void queryClient.cancelQueries({ queryKey: queryKeys.worksets.execution(entry.worksetId, entry.operation, entry.executionId), exact: true })
  }
  function finalize(entry: Observation, status: ExecutionStatus, completedOperations = 0) {
    if (entry.consumed || status === 'queued' || status === 'running') return
    entry.consumed = true
    entry.terminal = status
    cleanup(entry)
    if (entry.eligible) notify(entry, status === 'succeeded' ? '执行完成' : status === 'canceled' ? '执行已停止'
      : status === 'interrupted' ? '执行已中断，请查看已提交结果，剩余工作未恢复' : completedOperations > 0 ? `执行部分失败，已完成 ${completedOperations} 项操作` : '执行失败', status === 'succeeded' || status === 'canceled')
    void syncAfterExecutionTerminal(queryClient, entry.worksetId, entry.operation).then(() => {
      if (!entry.stopped && entry.operation === 'metadata_refresh') covers.terminal(entry, entry.eligible)
    })
  }
  const poll = sessionPoller(
    (entry: Observation) => queryClient.fetchQuery(executionQueryOptions(api, entry.worksetId, entry.operation, entry.executionId)),
    (entry, session) => {
      const active = session.status === 'queued' || session.status === 'running'
      // Retain confirmed failure until a terminal detail supplies final counters.
      syncExecutionSnapshot(queryClient, entry.confirmed === 'failed' && active ? { ...session, status: 'failed' } : session)
      if (!active) finalize(entry, entry.confirmed ?? session.status, session.completed_operations)
    },
    entry => {
      entry.consumed = true
      cleanup(entry)
      if (entry.operation === 'metadata_refresh') covers.unavailable(entry.worksetId, entry.executionId)
      if (entry.eligible) notify(entry, entry.confirmed === 'failed' ? '执行失败，完整结果不可用，记录不存在或无权访问' : '执行结果尚未确认，记录不存在或无权访问')
    },
  )
  function register(worksetId: string, operation: OperationType, executionId: string, captured = identity(worksetId), initial?: ExecutionView, eligible = true) {
    const key = JSON.stringify([worksetId, operation, 'execution', executionId])
    const existing = observations.get(key)
    if (existing) {
      if (eligible && !existing.consumed) existing.eligible = true
      return existing
    }
    const entry: Observation = { worksetId, operation, executionId, ...captured, eligible, consumed: false, reading: false, retryDelay: 2000 }
    observations.set(key, entry)
    if (initial) { syncExecutionSnapshot(queryClient, initial); finalize(entry, initial.status, initial.completed_operations) }
    if (!entry.consumed) void store.attach(worksetId, operation, executionId, api, event => {
      if (entry.consumed) return
      if (store.view) syncExecutionSnapshot(queryClient, store.view)
      if (event.type === 'execution_snapshot') { finalize(entry, event.data.status, event.data.completed_operations) }
      else if (['succeeded', 'failed', 'canceled', 'interrupted'].includes(event.type)) {
        entry.confirmed = event.type as ExecutionStatus
        if (event.type === 'failed') void poll(entry)
        else finalize(entry, event.type as ExecutionStatus)
      }
    }).then(attached => {
      if (entry.consumed || entry.confirmed) return
      if (attached) {
        notify(entry, '连接中断，执行结果尚未确认')
        void sweepAfterExecution(queryClient)
      }
      void poll(entry)
    })
    return entry
  }
  function stop() {
    for (const entry of observations.values()) { entry.stopped = true; entry.consumed = true; cleanup(entry) }
    covers.stop()
    store.reset()
  }
  return { start, attach, cancel, stop, store, startMutation, cancelMutation, coversMutation, observeCovers: covers.observe }
}
