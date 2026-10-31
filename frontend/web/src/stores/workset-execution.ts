import type {
  ApiClientContract,
  ExecutionComponent,
  ExecutionEvent,
  ExecutionView,
  MemberSummary,
  OperationType,
} from '@/lib/api/types'
import { defineStore } from 'pinia'
import { markRaw } from 'vue'
import { ApiError, executionViewFor } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'

/**
 * Replaces one component's entry, or inserts it in component order when the
 * held list does not have it yet: the stream pushes components the client's
 * snapshot may not have carried (a page it never fetched), and the display
 * order is by component index everywhere else.
 */
function mergeComponent(
  components: ExecutionComponent[],
  entry: ExecutionComponent,
): ExecutionComponent[] {
  const at = components.findIndex(component => component.component_index === entry.component_index)
  if (at >= 0) {
    const merged = components.slice()
    merged[at] = entry
    return merged
  }
  const next = components.slice()
  const after = next.findIndex(component => component.component_index > entry.component_index)
  if (after < 0)
    next.push(entry)
  else next.splice(after, 0, entry)
  return next
}

/** The server caps `members.attention`; the store never grows it past that. */
const ATTENTION_CAP = 50

/** Whether a member belongs in the exception list (same rule as the server's). */
function needsAttention(member: MemberSummary): boolean {
  return member.status === 'failed' || member.status === 'canceled' || member.status === 'running' || member.inventory_sync_error !== ''
}

export type ExecutionRunStatus
  = | 'idle'
    | 'streaming'
    | 'succeeded'
    | 'failed'
    | 'canceled'
    | 'interrupted'
    | 'error'

// How the terminal was reached. `event` means the backend confirmed the
// outcome over SSE (files were or were not written); `transport` means the
// stream ended or aborted without a terminal event — the backend state is
// unknown and caches must be conservatively swept.
export type ExecutionTerminal = 'event' | 'transport'

/**
 * Transient streaming state of the execution session the user is watching.
 * Same exception as the scan and generation stores (AGENTS.md): a live process
 * lifecycle, not a cacheable resource. Cache coordination lives in
 * use-workset-execution; the components this store has not been told about come
 * from the executions query cache, which the snapshot seeds.
 *
 * `view` is the latest authoritative session state seen on the wire (snapshot,
 * merged with the component results and progress the stream pushes after it).
 * It is deliberately not the panel's only source: once the stream has ended,
 * the detail GET owns the truth.
 */
export const useWorksetExecutionStore = defineStore('workset-execution', {
  state: () => ({
    status: 'idle' as ExecutionRunStatus,
    terminal: null as ExecutionTerminal | null,
    worksetId: null as string | null,
    operation: null as OperationType | null,
    executionId: null as string | null,
    view: null as ExecutionView | null,
    // A cancel request is in flight or waiting for the cooperative stop. The
    // session is only canceled once the server reaches its terminal status,
    // so the UI keeps saying 取消中 until then.
    cancelRequested: false,
    errorCode: null as string | null,
    errorMessage: null as string | null,
    // True once any SSE event was applied. A transport terminal without any
    // event means the stream never really started; caches must not be swept as
    // if the backend had done anything.
    receivedEvent: false,
    // Bumped by every snapshot and member event so a page reading one member
    // can re-read when it is that member (or any, for a snapshot: memberId null).
    memberTick: { seq: 0, memberId: null as string | null },
    controller: null as AbortController | null,
  }),
  getters: {
    /** Still waiting for the server to reach a terminal status. */
    canceling: state => state.cancelRequested && state.status === 'streaming',
  },
  actions: {
    /**
     * Attaches the SSE stream for one session. Resolves to false when the
     * attach was refused because this singleton store is already streaming
     * another session — callers MUST NOT run terminal synchronization for a
     * refused attach. Resolves true only for an actually-attached stream that
     * has now ended (event or transport).
     */
    async attach(
      worksetId: string,
      operation: OperationType,
      executionId: string,
      client: ApiClientContract,
      observe?: (event: ExecutionEvent) => void,
    ): Promise<boolean> {
      if (this.status === 'streaming')
        return false
      const controller = markRaw(new AbortController())
      this.controller = controller
      this.status = 'streaming'
      this.worksetId = worksetId
      this.operation = operation
      this.executionId = executionId
      this.terminal = null
      this.view = null
      this.errorCode = null
      this.errorMessage = null
      this.receivedEvent = false

      try {
        for await (const event of client.streamExecutionEvents(worksetId, operation, executionId, controller.signal, executionViewFor(operation))) {
          // A reset() or a newer session superseded this stream — never let
          // stale events overwrite the current state.
          if (this.controller !== controller)
            return true
          this.applyEvent(event as ExecutionEvent)
          observe?.(event as ExecutionEvent)
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
          this.errorMessage = '执行连接提前结束，请刷新查看结果。'
        }
      }
      catch (error) {
        if (this.controller !== controller)
          return true
        this.terminal = 'transport'
        if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) {
          this.status = 'error'
          this.errorCode = 'STREAM_ABORTED'
          this.errorMessage = '执行进度连接已断开。'
        }
        else {
          this.status = 'error'
          this.errorCode = error instanceof ApiError
            ? error.code
            : 'STREAM_ERROR'
          this.errorMessage = errorDetails(error, '执行进度连接失败。').message
        }
      }
      finally {
        if (this.controller === controller)
          this.controller = null
      }
      return true
    },
    applyEvent(event: ExecutionEvent) {
      this.receivedEvent = true
      switch (event.type) {
        case 'execution_snapshot': {
          // The held components are replaced wholesale, never mutated: keep the
          // large list free of per-entry reactive proxies.
          if (event.data.components)
            markRaw(event.data.components)
          this.view = event.data
          this.memberTick = { seq: this.memberTick.seq + 1, memberId: null }
          break
        }
        case 'member': {
          // Counts and the exception list follow the member's latest summary.
          // The wire carries no frozen position, so a member that newly
          // qualifies is appended in arrival order (and not at all once the
          // list holds the server's cap; attention_total still says how many
          // there are). The terminal calibration read restores frozen order.
          const { member, counts, attention_total } = event.data
          if (this.view?.members) {
            const held = this.view.members.attention
            const at = held.findIndex(entry => entry.member_id === member.member_id)
            let attention = held
            if (!needsAttention(member)) {
              attention = at >= 0
                ? held.filter((_, i) => i !== at)
                : held
            }
            else if (at >= 0) {
              attention = held.map((entry, i) => (i === at
                ? member
                : entry))
            }
            else if (held.length < ATTENTION_CAP) {
              attention = [...held, member]
            }
            this.view = {
              ...this.view,
              members: { counts, attention, attention_total },
            }
          }
          this.memberTick = { seq: this.memberTick.seq + 1, memberId: member.member_id }
          break
        }
        case 'component': {
          // The wire carries one component's own entry as its result lands, so
          // the panel fills in component by component instead of re-reading the
          // session. The entry is replaced wholesale, and a component the held
          // list does not know yet (a page the client never fetched) is inserted
          // in component order.
          if (this.view) {
            this.view = {
              ...this.view,
              components: markRaw(mergeComponent(this.view.components ?? [], event.data)),
            }
          }
          break
        }
        case 'progress': {
          if (this.view) {
            const data = event.data
            this.view = {
              ...this.view,
              status: data.status,
              total_components: data.total_components,
              completed_components: data.completed_components,
              total_operations: data.total_operations,
              completed_operations: data.completed_operations,
              current_root: data.current_root,
              current_component_id: data.current_component_id,
              current_phase: data.current_phase,
            }
          }
          break
        }
        case 'succeeded': {
          this.status = 'succeeded'
          this.terminal = 'event'
          this.cancelRequested = false
          if (this.view)
            this.view = { ...this.view, status: 'succeeded' }
          break
        }
        case 'failed': {
          this.status = 'failed'
          this.terminal = 'event'
          this.cancelRequested = false
          this.errorCode = event.data.error_code
          this.errorMessage = event.data.error_message
          if (this.view) {
            this.view = {
              ...this.view,
              status: 'failed',
              error_code: event.data.error_code,
              error_message: event.data.error_message,
            }
          }
          break
        }
        case 'canceled': {
          this.status = 'canceled'
          this.terminal = 'event'
          this.cancelRequested = false
          if (this.view)
            this.view = { ...this.view, status: 'canceled' }
          break
        }
        case 'interrupted': {
          this.status = 'interrupted'
          this.terminal = 'event'
          this.cancelRequested = false
          if (this.view)
            this.view = { ...this.view, status: 'interrupted' }
          break
        }
        case 'error': {
          this.status = 'error'
          this.terminal = 'event'
          this.errorCode = event.data.code
          this.errorMessage = event.data.message
          break
        }
      }
    },
    /**
     * Marks the intent to stop the current session. The stream is deliberately
     * left attached: the cooperative cancel only ends at the worker's next
     * safe boundary, and the terminal event that follows is the authoritative
     * outcome the UI must wait for.
     */
    requestCancel(worksetId: string, executionId: string) {
      if (this.worksetId === worksetId && this.executionId === executionId) {
        this.cancelRequested = true
      }
    },
    reset() {
      this.controller?.abort()
      this.$reset()
    },
  },
})
