import type {
  ApiClientContract,
  ExecutionComponent,
  ExecutionEvent,
  ExecutionView,
  MemberSummary,
} from '@/lib/api/types'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { isReactive } from 'vue'
import { apiStub } from '@/test/api-stub'
import { useWorksetExecutionStore } from './workset-execution'

function view(overrides: Partial<ExecutionView> = {}): ExecutionView {
  return {
    execution_id: 'exec-1',
    workset_id: 'ws-1',
    operation_type: 'conversion',
    plan_id: 'plan-1',
    status: 'running',
    options: { delete_mode: 'soft' },
    total_components: 2,
    completed_components: 0,
    total_operations: 3,
    completed_operations: 0,
    current_root: '',
    current_component_id: '',
    current_phase: 'component',
    components: [],
    error_code: '',
    error_message: '',
    started_at: '',
    finished_at: '',
    created_at: '',
    ...overrides,
  }
}

async function* events(items: ExecutionEvent[]): AsyncGenerator<ExecutionEvent> {
  for (const item of items) yield item
}

function makeEventStream(
  terminal: ExecutionEvent['type'],
  extra: Record<string, unknown> = {},
): ApiClientContract['streamExecutionEvents'] {
  return vi.fn(() =>
    events([
      {
        type: 'execution_snapshot',
        data: view(),
      } as ExecutionEvent,
      {
        type: 'progress',
        data: {
          execution_id: 'exec-1',
          status: 'running',
          total_components: 2,
          completed_components: 1,
          total_operations: 3,
          completed_operations: 1,
          current_root: 'albumB',
          current_component_id: 'c2',
          current_phase: 'component',
        },
      } as ExecutionEvent,
      terminal === 'succeeded'
        ? ({
            type: 'succeeded',
            data: { execution_id: 'exec-1', plan_id: 'plan-1', ...extra },
          } as ExecutionEvent)
        : terminal === 'failed'
          ? ({
              type: 'failed',
              data: {
                execution_id: 'exec-1',
                error_code: 'COMPONENT_FAILED',
                error_message: 'encode failed',
                ...extra,
              },
            } as ExecutionEvent)
          : ({
              type: terminal,
              data: { execution_id: 'exec-1', ...extra },
            } as ExecutionEvent),
    ]),
  ) as ApiClientContract['streamExecutionEvents']
}

describe('workset execution store', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('merges snapshot and progress into the view, then confirms the succeeded terminal', async () => {
    const store = useWorksetExecutionStore()
    const api = apiStub({
      streamExecutionEvents: makeEventStream('succeeded'),
    })

    await store.attach('ws-1', 'conversion', 'exec-1', api)

    expect(store.status).toBe('succeeded')
    expect(store.terminal).toBe('event')
    expect(store.view?.status).toBe('succeeded')
    expect(store.view?.completed_components).toBe(1)
    expect(store.view?.total_operations).toBe(3)
    expect(store.view?.current_component_id).toBe('c2')
    expect(store.receivedEvent).toBe(true)
  })

  it('tracks failed and canceled terminals distinctly', async () => {
    const failed = useWorksetExecutionStore()
    await failed.attach('ws-1', 'conversion', 'exec-1', apiStub({
      streamExecutionEvents: makeEventStream('failed'),
    }))
    expect(failed.status).toBe('failed')
    expect(failed.errorCode).toBe('COMPONENT_FAILED')
    expect(failed.view?.status).toBe('failed')

    const canceled = useWorksetExecutionStore()
    await canceled.attach('ws-1', 'conversion', 'exec-2', apiStub({
      streamExecutionEvents: makeEventStream('canceled'),
    }))
    expect(canceled.status).toBe('canceled')
    expect(canceled.terminal).toBe('event')
  })

  it('marks a transport terminal when the stream ends without a terminal event', async () => {
    const streamExecutionEvents = vi.fn(() =>
      events([
        {
          type: 'execution_snapshot',
          data: view(),
        } as ExecutionEvent,
      ]),
    )
    const store = useWorksetExecutionStore()
    await store.attach('ws-1', 'conversion', 'exec-1', apiStub({ streamExecutionEvents }))

    expect(store.status).toBe('error')
    expect(store.terminal).toBe('transport')
    expect(store.errorCode).toBe('STREAM_ENDED')
  })

  it('keeps reporting 取消中 until the cooperative cancel reaches its terminal event', async () => {
    let pushCanceled: () => void = () => {}
    const streamExecutionEvents = vi.fn(() =>
      (async function* (): AsyncGenerator<ExecutionEvent> {
        yield {
          type: 'execution_snapshot',
          data: view(),
        } as ExecutionEvent
        // The running worker stops at its next safe boundary; only then does
        // the terminal event arrive.
        await new Promise<void>((resolve) => {
          pushCanceled = resolve
        })
        yield {
          type: 'canceled',
          data: { execution_id: 'exec-1' },
        } as ExecutionEvent
      })(),
    )
    const store = useWorksetExecutionStore()
    const attached = store.attach('ws-1', 'conversion', 'exec-1', apiStub({ streamExecutionEvents }))
    await Promise.resolve()
    await Promise.resolve()

    store.requestCancel('ws-1', 'exec-1')
    expect(store.canceling).toBe(true)

    pushCanceled()
    await attached

    expect(store.status).toBe('canceled')
    expect(store.canceling).toBe(false)
  })

  it('refuses a second attach while streaming', async () => {
    const streamExecutionEvents = vi.fn((_w: string, _o: string, _e: string, signal: AbortSignal) =>
      (async function* (): AsyncGenerator<ExecutionEvent> {
        yield {
          type: 'execution_snapshot',
          data: view(),
        } as ExecutionEvent
        await new Promise<void>((resolve) => {
          signal.addEventListener('abort', () => resolve(), { once: true })
        })
      })(),
    )
    const store = useWorksetExecutionStore()
    const api = apiStub({ streamExecutionEvents })

    const first = store.attach('ws-1', 'conversion', 'exec-1', api)
    const second = await store.attach('ws-1', 'conversion', 'exec-2', api)
    expect(second).toBe(false)

    await Promise.resolve()
    await Promise.resolve()
    store.reset()
    await first
    expect(streamExecutionEvents).toHaveBeenCalledTimes(1)
  })
})

describe('component events', () => {
  beforeEach(() => setActivePinia(createPinia()))

  function component(index: number, status: string): ExecutionComponent {
    return {
      component_index: index,
      kept_decisions: [
        { path: `/music/album${index}/kept.mp3`, resolution: 'keep', reason_code: 'KEEP_ENCODED_SATISFIED' },
      ],
      component_id: `comp-${index}`,
      root_path: `/music/album${index}`,
      partition: 'matched',
      status: status as ExecutionComponent['status'],
      operations: 1,
      completed_operations: status === 'succeeded'
        ? 1
        : 0,
      committed: status === 'succeeded'
        ? [`/music/album${index}/00.mp3`]
        : [],
      removed: [],
      remaining: [],
      recovery: [],
      inventory_synced: status === 'succeeded',
    }
  }

  it('keeps the held components out of deep reactivity', () => {
    const store = useWorksetExecutionStore()
    store.status = 'streaming'
    store.applyEvent({
      type: 'execution_snapshot',
      data: view({
        components: [
          component(0, 'pending'),
        ],
      }),
    })
    expect(isReactive(store.view?.components?.[0])).toBe(false)
    store.applyEvent({
      type: 'component',
      data: component(0, 'succeeded'),
    })
    expect(isReactive(store.view?.components?.[0])).toBe(false)
  })

  it('replaces the entry the snapshot carried and inserts one it did not', () => {
    const store = useWorksetExecutionStore()
    store.status = 'streaming'
    store.view = view({
      components: [
        {
          ...component(0, 'pending'),
          kept_decisions: [],
        },
        component(2, 'pending'),
      ],
    })

    store.applyEvent({
      type: 'component',
      data: component(0, 'succeeded'),
    })
    expect(store.view?.components?.[0]?.status).toBe('succeeded')
    expect(store.view?.components?.[0]).toEqual(component(0, 'succeeded'))

    // A component beyond the page the snapshot carried arrives out of order:
    // it lands where component order puts it, not at the end.
    store.applyEvent({
      type: 'component',
      data: component(1, 'succeeded'),
    })
    expect(store.view?.components?.map(entry => entry.component_index)).toEqual([0, 1, 2])
    expect(store.view?.components?.[1]).toEqual(component(1, 'succeeded'))
  })

  it('ignores a component event before any view exists', () => {
    const store = useWorksetExecutionStore()
    store.applyEvent({
      type: 'component',
      data: component(0, 'succeeded'),
    })
    expect(store.view).toBeNull()
  })

  describe('member events (view=members)', () => {
    const none = {
      succeeded: 0,
      failed: 0,
      canceled: 0,
      running: 0,
      pending: 0,
    }
    function member(id: string, status: MemberSummary['status'], sync = ''): MemberSummary {
      return {
        member_id: id,
        root_path: `/music/${id}`,
        status,
        total_components: 1,
        completed_components: 0,
        total_operations: 1,
        completed_operations: 0,
        error_message: '',
        inventory_sync_error: sync,
      }
    }
    function seeded(attention: MemberSummary[] = [], total = attention.length) {
      const store = useWorksetExecutionStore()
      store.status = 'streaming'
      store.view = view({
        components: undefined,
        members: { counts: none, attention, attention_total: total },
      })
      return store
    }
    const ids = (store: ReturnType<typeof seeded>) => store.view?.members?.attention.map(m => m.member_id)

    it('adds, replaces and removes the entry and takes counts from the event', () => {
      const store = seeded()
      const counts = { ...none, failed: 1 }
      store.applyEvent({
        type: 'member',
        data: {
          member: member('a', 'failed'),
          counts,
          attention_total: 1,
        },
      })
      store.applyEvent({
        type: 'member',
        data: {
          member: member('b', 'running'),
          counts,
          attention_total: 2,
        },
      })
      expect(ids(store)).toEqual(['a', 'b'])
      expect(store.view?.members?.counts).toEqual(counts)
      expect(store.view?.members?.attention_total).toBe(2)
      expect(store.view?.components).toBeUndefined()

      store.applyEvent({
        type: 'member',
        data: {
          member: member('a', 'canceled'),
          counts,
          attention_total: 2,
        },
      })
      expect(store.view?.members?.attention[0]?.status).toBe('canceled')

      // Succeeded without a warning no longer qualifies; a warning keeps it.
      store.applyEvent({
        type: 'member',
        data: {
          member: member('b', 'succeeded'),
          counts,
          attention_total: 1,
        },
      })
      expect(ids(store)).toEqual(['a'])
      store.applyEvent({
        type: 'member',
        data: {
          member: member('a', 'succeeded', 'locked'),
          counts,
          attention_total: 1,
        },
      })
      expect(ids(store)).toEqual(['a'])
    })

    it('never grows the list past the server cap', () => {
      const held = Array.from({ length: 50 }, (_, i) => member(`m-${i}`, 'failed'))
      const store = seeded(held, 50)
      store.applyEvent({
        type: 'member',
        data: {
          member: member('late', 'failed'),
          counts: none,
          attention_total: 51,
        },
      })
      expect(store.view?.members?.attention).toHaveLength(50)
      expect(store.view?.members?.attention_total).toBe(51)
    })

    it('asks for the summary shape per operation: conversion yes, rename and metadata no', async () => {
      for (const [operation, expected] of [
        ['conversion', 'members'],
        ['rename', undefined],
        ['metadata_refresh', undefined],
      ] as const) {
        setActivePinia(createPinia())
        const stream = vi.fn(() => events([]))
        await useWorksetExecutionStore().attach('ws-1', operation, 'exec-1', apiStub({ streamExecutionEvents: stream }))
        expect(stream).toHaveBeenCalledWith('ws-1', operation, 'exec-1', expect.any(AbortSignal), expected)
      }
    })
  })
})
