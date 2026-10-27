import type { ApiClientContract, GenerationEvent, OperationType } from '@/lib/api/types'
import { defineStore } from 'pinia'
import { markRaw } from 'vue'
import { ApiError } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'

export type GenerationRunStatus
  = | 'idle'
    | 'streaming'
    | 'completed'
    | 'failed'
    | 'canceled'
    | 'interrupted'
    | 'error'

// How the terminal was reached. `event` means the backend confirmed the
// outcome over SSE (a revision may have been created); `transport` means the
// stream ended or aborted without a terminal event — the backend state is
// unknown and orchestration must recover the original session.
export type GenerationTerminal = 'event' | 'transport'

// Transient streaming state of the planning session the user is watching.
// This mirrors the scan store exception in AGENTS.md: a live process lifecycle
// (SSE), not a cacheable server resource. Cache coordination lives in
// use-workset-generation.
export const useWorksetGenerationStore = defineStore('workset-generation', {
  state: () => ({
    status: 'idle' as GenerationRunStatus,
    terminal: null as GenerationTerminal | null,
    worksetId: null as string | null,
    operation: null as OperationType | null,
    generationId: null as string | null,
    statusText: '' as string, // backend session status (queued/running/…)
    totalRoots: 0,
    completedRoots: 0,
    currentRoot: '' as string,
    errorCount: 0,
    revisionId: null as string | null,
    errorCode: null as string | null,
    errorMessage: null as string | null,
    // True once any SSE event was applied; absence of events does not prove
    // that an accepted backend session never ran.
    receivedEvent: false,
    controller: null as AbortController | null,
  }),
  actions: {
    /**
     * Attaches the SSE stream for one session. Resolves to false when the
     * attach was refused because this singleton store is already streaming
     * another session — callers MUST NOT run terminal synchronization for a
     * refused attach (there is no terminal; sweeping here would feed the
     * detail-refetch → re-attach → sweep loop). Resolves true only for an
     * actually-attached stream that has now ended (event or transport).
     */
    async attach(
      worksetId: string,
      operation: OperationType,
      generationId: string,
      client: ApiClientContract,
      onEvent?: (event: GenerationEvent) => void,
    ): Promise<boolean> {
      if (this.status === 'streaming')
        return false
      const controller = markRaw(new AbortController())
      this.controller = controller
      this.status = 'streaming'
      this.worksetId = worksetId
      this.operation = operation
      this.generationId = generationId
      this.terminal = null
      this.revisionId = null
      this.errorCode = null
      this.errorMessage = null
      this.receivedEvent = false

      try {
        for await (const event of client.streamGenerationEvents(worksetId, operation, generationId, controller.signal)) {
          // A reset() or a newer session superseded this stream — never let
          // stale events overwrite the current state.
          if (this.controller !== controller)
            return true
          this.applyEvent(event as GenerationEvent)
          onEvent?.(event)
          if (this.controller !== controller)
            return true
          if (this.status !== 'streaming')
            break // terminal event applied
        }
        if (this.controller !== controller)
          return true
        if (this.status === 'streaming') {
          // Stream ended without a terminal event: the backend outcome is
          // unknown (it may have completed just after the last event).
          this.status = 'error'
          this.terminal = 'transport'
          this.errorCode = 'STREAM_ENDED'
          this.errorMessage = '生成连接提前结束，请刷新查看结果。'
        }
      }
      catch (error) {
        if (this.controller !== controller)
          return true
        this.terminal = 'transport'
        if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) {
          this.status = 'error'
          this.errorCode = 'STREAM_ABORTED'
          this.errorMessage = '生成进度连接已断开。'
        }
        else {
          this.status = 'error'
          this.errorCode = error instanceof ApiError
            ? error.code
            : 'STREAM_ERROR'
          this.errorMessage = errorDetails(error, '生成进度连接失败。').message
        }
      }
      finally {
        if (this.controller === controller)
          this.controller = null
      }
      return true
    },
    applyEvent(event: GenerationEvent) {
      this.receivedEvent = true
      const data = event.data as Record<string, unknown>
      switch (event.type) {
        case 'session_snapshot': {
          this.statusText = String(data.status ?? '')
          if (event.data.status !== 'queued' && event.data.status !== 'running') {
            this.status = event.data.status
            this.terminal = 'event'
          }
          this.totalRoots = Number(data.total_roots ?? this.totalRoots)
          this.completedRoots = Number(data.completed_roots ?? this.completedRoots)
          this.currentRoot = String(data.current_root ?? this.currentRoot)
          this.errorCount = Number(data.error_count ?? this.errorCount)
          break
        }
        case 'progress': {
          this.totalRoots = Number(data.total_roots ?? this.totalRoots)
          this.completedRoots = Number(data.completed_roots ?? this.completedRoots)
          this.currentRoot = String(data.current_root ?? this.currentRoot)
          this.errorCount = Number(data.error_count ?? this.errorCount)
          break
        }
        case 'completed': {
          this.status = 'completed'
          this.terminal = 'event'
          this.revisionId = String(data.revision_id ?? '')
          break
        }
        case 'failed': {
          this.status = 'failed'
          this.terminal = 'event'
          this.errorCode = String(data.error_code ?? 'GENERATION_FAILED')
          this.errorMessage = String(data.error_message ?? '生成失败')
          break
        }
        case 'canceled': {
          this.status = 'canceled'
          this.terminal = 'event'
          break
        }
        case 'interrupted': {
          this.status = 'interrupted'
          this.terminal = 'event'
          break
        }
        case 'error': {
          this.status = 'error'
          this.terminal = 'event'
          this.errorCode = String(data.code ?? 'INTERNAL')
          this.errorMessage = String(data.message ?? '生成事件流错误')
          break
        }
      }
    },
    /** Aborts the SSE stream only when it belongs to the given session. */
    cancel(worksetId: string, generationId: string) {
      if (this.worksetId === worksetId && this.generationId === generationId) {
        this.controller?.abort()
      }
    },
    reset() {
      this.controller?.abort()
      this.$reset()
    },
  },
})
