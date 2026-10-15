import type { QueryClient } from '@tanstack/vue-query'
import type { ApiClientContract } from '@/lib/api/types'
import { executionCoversQueryOptions } from '@/queries/worksets'
import { queryKeys } from '@/queries/query-keys'
import { sessionPoller, useWorksetObservation, type SessionIdentity } from './use-workset-observation'

type Round = SessionIdentity & { executionId: string; eligible: boolean; terminal: boolean; initial: boolean; consumed: boolean; reading: boolean; retryDelay: number; outage: boolean; timer?: ReturnType<typeof setTimeout> }

// One app-lifetime report observer per execution; rounds are client memory,
// independent of the API's stable delivery IDs and cumulative counters.
// Each fresh entry is the round token; cleanup makes replaced reads inert.
export function useWorksetCovers(api: ApiClientContract, client: QueryClient) {
  const { observations, identity, notify } = useWorksetObservation<Round>(client, 'covers', stop)
  const key = (worksetId: string, executionId: string) => JSON.stringify([worksetId, executionId])
  function cleanup(entry: Round) {
    entry.consumed = true
    clearTimeout(entry.timer)
    void client.cancelQueries({ queryKey: queryKeys.worksets.covers(entry.worksetId, entry.executionId), exact: true })
  }
  const poll = sessionPoller(
    (entry: Round) => client.fetchQuery(executionCoversQueryOptions(api, entry.worksetId, entry.executionId)),
    (entry, report) => {
      const c = report.counts
      if (c.pending + c.running || (entry.eligible && !entry.terminal)) return
      entry.consumed = true
      if (!entry.eligible || !report.items.length) return
      const label = c.failed ? (c.succeeded + c.skipped ? '部分失败' : '失败') : c.canceled ? '已停止' : '完成'
      notify(entry, `封面${label}：成功 ${c.succeeded}，跳过 ${c.skipped}，失败 ${c.failed}，停止 ${c.canceled}`, !c.failed)
    },
    entry => { entry.consumed = true; if (entry.eligible) notify(entry, '封面结果尚未确认，记录不存在或无权访问') },
    entry => { if (entry.eligible && !entry.outage) notify(entry, '连接中断，封面结果尚未确认'); entry.outage = true },
    entry => { entry.outage = false },
  )
  function open(worksetId: string, executionId: string, eligible: boolean, captured = identity(worksetId)) {
    const previous = observations.get(key(worksetId, executionId))
    if (previous) cleanup(previous)
    const entry: Round = { ...captured, worksetId, executionId, operation: 'metadata_refresh',
      eligible, terminal: previous?.terminal ?? false, initial: previous?.initial ?? false, consumed: false, reading: false, retryDelay: 2000, outage: false }
    observations.set(key(worksetId, executionId), entry)
    return entry
  }
  function terminal(session: SessionIdentity & { executionId: string }, eligible: boolean) {
    let entry = observations.get(key(session.worksetId, session.executionId))
    if (eligible && !entry?.initial) {
      if (!entry?.eligible || entry.consumed) entry = open(session.worksetId, session.executionId, true, session)
      entry.initial = true
    }
    if (!entry) return
    if (entry.terminal) return
    entry.terminal = true
    // Cancel a pre-terminal read: only a newly fetched report can settle.
    if (!entry.consumed) {
      cleanup(entry)
      const fresh = { ...entry, consumed: false, reading: false, timer: undefined }
      observations.set(key(entry.worksetId, entry.executionId), fresh)
      void poll(fresh)
    }
  }
  function retry(worksetId: string, executionId: string, captured: Pick<SessionIdentity, 'libraryId' | 'libraryName'>) {
    const entry = open(worksetId, executionId, true, captured)
    void poll(entry)
  }
  function observe(worksetId: string, executionId: string) {
    if (!observations.has(key(worksetId, executionId))) void poll(open(worksetId, executionId, false))
  }
  function unavailable(worksetId: string, executionId: string) {
    const entry = observations.get(key(worksetId, executionId))
    if (!entry || entry.consumed) return
    cleanup(entry)
    if (entry.eligible) notify(entry, '封面结果尚未确认，记录不存在或无权访问')
  }
  function stop() { for (const entry of observations.values()) cleanup(entry) }
  return { terminal, retry, observe, unavailable, stop }
}
